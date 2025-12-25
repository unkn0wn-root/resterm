package httpclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/telemetry"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
	"golang.org/x/net/http2"
	"nhooyr.io/websocket"
)

type Options struct {
	Timeout            time.Duration
	FollowRedirects    bool
	InsecureSkipVerify bool
	ProxyURL           string
	RootCAs            []string
	RootMode           tlsconfig.RootMode
	ClientCert         string
	ClientKey          string
	BaseDir            string
	FallbackBaseDirs   []string
	NoFallback         bool
	Trace              bool
	TraceBudget        *nettrace.Budget
	SSH                *ssh.Plan
}

type FileSystem interface {
	ReadFile(name string) ([]byte, error)
}

type OSFileSystem struct{}

func (OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

type Client struct {
	fs          FileSystem
	jar         http.CookieJar
	httpFactory func(Options) (*http.Client, error)
	wsDial      func(context.Context, string, *websocket.DialOptions) (*websocket.Conn, *http.Response, error)
	telemetry   telemetry.Instrumenter
}

func (c *Client) resolveHTTPFactory() func(Options) (*http.Client, error) {
	if c == nil {
		return nil
	}
	if c.httpFactory != nil {
		return c.httpFactory
	}
	return c.buildHTTPClient
}

func NewClient(fs FileSystem) *Client {
	if fs == nil {
		fs = OSFileSystem{}
	}

	jar, _ := cookiejar.New(nil)
	c := &Client{fs: fs, jar: jar, telemetry: telemetry.Noop()}
	c.httpFactory = c.buildHTTPClient
	c.wsDial = websocket.Dial
	return c
}

// SetHTTPFactory allows callers to override how http.Client instances are created.
// Passing nil restores the default factory.
func (c *Client) SetHTTPFactory(factory func(Options) (*http.Client, error)) {
	c.httpFactory = factory
}

// SetTelemetry configures the instrumenter used to emit OpenTelemetry spans. Passing nil restores the no-op implementation.
func (c *Client) SetTelemetry(instr telemetry.Instrumenter) {
	if instr == nil {
		instr = telemetry.Noop()
	}
	c.telemetry = instr
}

type Response struct {
	Status         string
	StatusCode     int
	Proto          string
	Headers        http.Header
	ReqMethod      string
	RequestHeaders http.Header
	ReqHost        string
	ReqLen         int64
	ReqTE          []string
	Body           []byte
	Duration       time.Duration
	EffectiveURL   string
	Request        *restfile.Request
	Timeline       *nettrace.Timeline
	TraceReport    *nettrace.Report
}

// Wraps the HTTP roundtrip with telemetry spans and network tracing.
// Trace session hooks into http.Client's transport to capture timing info,
// while the defer ensures we always report metrics even on failure.
func (c *Client) Execute(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (resp *Response, err error) {
	httpReq, effectiveOpts, err := c.prepareHTTPRequest(ctx, req, resolver, opts)
	if err != nil {
		return nil, err
	}

	factory := c.resolveHTTPFactory()
	if factory == nil {
		return nil, errdef.New(errdef.CodeHTTP, "http client factory unavailable")
	}

	client, err := factory(effectiveOpts)
	if err != nil {
		return nil, err
	}

	var (
		timeline    *nettrace.Timeline
		traceSess   *traceSession
		traceReport *nettrace.Report
	)

	instrumenter := c.telemetry
	if !effectiveOpts.Trace || instrumenter == nil {
		instrumenter = telemetry.Noop()
	}

	var budgetCopy *nettrace.Budget
	if effectiveOpts.TraceBudget != nil {
		clone := effectiveOpts.TraceBudget.Clone()
		budgetCopy = &clone
	}

	spanCtx, requestSpan := instrumenter.Start(httpReq.Context(), telemetry.RequestStart{
		Request:     req,
		HTTPRequest: httpReq,
		Budget:      budgetCopy,
	})
	httpReq = httpReq.WithContext(spanCtx)

	defer func() {
		if requestSpan == nil {
			return
		}
		if timeline != nil || traceReport != nil {
			requestSpan.RecordTrace(timeline, traceReport)
		}
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		requestSpan.End(telemetry.RequestResult{
			Err:        err,
			StatusCode: statusCode,
			Report:     traceReport,
		})
	}()

	if effectiveOpts.Trace {
		traceSess = newTraceSession()
		httpReq = traceSess.bind(httpReq)
	}

	buildTraceReport := func(tl *nettrace.Timeline) *nettrace.Report {
		if tl == nil {
			return nil
		}
		var budget nettrace.Budget
		if effectiveOpts.TraceBudget != nil {
			budget = effectiveOpts.TraceBudget.Clone()
		}
		return nettrace.NewReport(tl, budget)
	}

	start := time.Now()
	httpResp, err := client.Do(httpReq)
	if err != nil {
		duration := time.Since(start)
		if traceSess != nil {
			traceSess.fail(err)
			timeline = traceSess.complete()
			traceReport = buildTraceReport(timeline)
		}
		return &Response{
				Request:     req,
				Duration:    duration,
				Timeline:    timeline,
				TraceReport: traceReport,
			}, errdef.Wrap(
				errdef.CodeHTTP,
				err,
				"perform request",
			)
	}

	defer func() {
		if closeErr := httpResp.Body.Close(); closeErr != nil && err == nil {
			err = errdef.Wrap(errdef.CodeHTTP, closeErr, "close response body")
		}
	}()

	body, err := io.ReadAll(httpResp.Body)
	if traceSess != nil {
		traceSess.finishTransfer(err)
	}
	if err != nil {
		if traceSess != nil {
			traceSess.fail(err)
			traceSess.complete()
		}
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "read response body")
	}

	if traceSess != nil {
		timeline = traceSess.complete()
		traceReport = buildTraceReport(timeline)
	}
	duration := time.Since(start)

	meta := captureReqMeta(httpReq, httpResp)

	resp = &Response{
		Status:         httpResp.Status,
		StatusCode:     httpResp.StatusCode,
		Proto:          httpResp.Proto,
		Headers:        httpResp.Header.Clone(),
		ReqMethod:      meta.method,
		RequestHeaders: meta.headers,
		ReqHost:        meta.host,
		ReqLen:         meta.length,
		ReqTE:          meta.te,
		Body:           body,
		EffectiveURL:   httpResp.Request.URL.String(),
		Request:        req,
		Timeline:       timeline,
		TraceReport:    traceReport,
	}
	resp.Duration = duration

	return resp, nil
}

func (c *Client) prepareHTTPRequest(
	ctx context.Context,
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (*http.Request, Options, error) {
	if req == nil {
		return nil, opts, errdef.New(errdef.CodeHTTP, "request is nil")
	}

	bodyReader, err := c.prepareBody(req, resolver, opts)
	if err != nil {
		return nil, opts, err
	}

	expandedURL := strings.TrimSpace(req.URL)
	if expandedURL == "" {
		return nil, opts, errdef.New(errdef.CodeHTTP, "request url is empty")
	}
	if req.Body.GraphQL == nil || !strings.EqualFold(req.Method, "GET") {
		if resolver != nil {
			expandedURL, err = resolver.ExpandTemplates(expandedURL)
			if err != nil {
				return nil, opts, errdef.Wrap(errdef.CodeHTTP, err, "expand url")
			}
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, expandedURL, bodyReader)
	if err != nil {
		return nil, opts, errdef.Wrap(errdef.CodeHTTP, err, "build request")
	}

	if req.Headers != nil {
		for name, values := range req.Headers {
			for _, value := range values {
				finalValue := value
				if resolver != nil {
					if expanded, expandErr := resolver.ExpandTemplates(value); expandErr == nil {
						finalValue = expanded
					}
				}
				httpReq.Header.Add(name, finalValue)
			}
		}
	}

	if req.Body.GraphQL != nil && !strings.EqualFold(req.Method, "GET") {
		if httpReq.Header.Get("Content-Type") == "" {
			httpReq.Header.Set("Content-Type", "application/json")
		}
	}

	c.applyAuthentication(httpReq, resolver, req.Metadata.Auth)
	effectiveOpts := applyRequestSettings(opts, req.Settings)
	return httpReq, effectiveOpts, nil
}

func (c *Client) prepareBody(
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (io.Reader, error) {
	if req.Body.GraphQL != nil {
		return c.prepareGraphQLBody(req, resolver, opts)
	}

	fallbacks, allowRaw := resolveFileLookup(opts.BaseDir, opts)

	switch {
	case req.Body.FilePath != "":
		data, _, err := c.readFileWithFallback(
			req.Body.FilePath,
			opts.BaseDir,
			fallbacks,
			allowRaw,
			"body file",
		)
		if err != nil {
			return nil, err
		}

		if resolver != nil && req.Body.Options.ExpandTemplates {
			text := string(data)
			expanded, err := resolver.ExpandTemplates(text)
			if err != nil {
				return nil, errdef.Wrap(errdef.CodeHTTP, err, "expand body file templates")
			}

			processed, procErr := c.injectBodyIncludes(expanded, opts.BaseDir, fallbacks, allowRaw)
			if procErr != nil {
				return nil, procErr
			}
			return strings.NewReader(processed), nil
		}
		return bytes.NewReader(data), nil
	case req.Body.Text != "":
		expanded := req.Body.Text
		if resolver != nil {
			var err error
			expanded, err = resolver.ExpandTemplates(req.Body.Text)
			if err != nil {
				return nil, errdef.Wrap(errdef.CodeHTTP, err, "expand body template")
			}
		}
		processed, err := c.injectBodyIncludes(expanded, opts.BaseDir, fallbacks, allowRaw)
		if err != nil {
			return nil, err
		}
		return strings.NewReader(processed), nil
	default:
		return nil, nil
	}
}

// GET requests put everything in query params, POST uses JSON body.
// Variables need special handling since they must be valid JSON in both cases.
func (c *Client) prepareGraphQLBody(
	req *restfile.Request,
	resolver *vars.Resolver,
	opts Options,
) (io.Reader, error) {
	gql := req.Body.GraphQL
	fallbacks, allowRaw := resolveFileLookup(opts.BaseDir, opts)

	query, err := c.graphQLSectionContent(
		gql.Query,
		gql.QueryFile,
		opts.BaseDir,
		fallbacks,
		allowRaw,
		"GraphQL query",
	)
	if err != nil {
		return nil, err
	}

	if resolver != nil {
		if expanded, expandErr := resolver.ExpandTemplates(query); expandErr == nil {
			query = expanded
		} else {
			return nil, errdef.Wrap(errdef.CodeHTTP, expandErr, "expand graphql query")
		}
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errdef.New(errdef.CodeHTTP, "graphql query is empty")
	}

	operationName := strings.TrimSpace(gql.OperationName)
	if operationName != "" && resolver != nil {
		if expanded, expandErr := resolver.ExpandTemplates(operationName); expandErr == nil {
			operationName = strings.TrimSpace(expanded)
		} else {
			return nil, errdef.Wrap(errdef.CodeHTTP, expandErr, "expand graphql operation name")
		}
	}

	variablesRaw, err := c.graphQLSectionContent(
		gql.Variables,
		gql.VariablesFile,
		opts.BaseDir,
		fallbacks,
		allowRaw,
		"GraphQL variables",
	)
	if err != nil {
		return nil, err
	}

	variablesRaw = strings.TrimSpace(variablesRaw)
	if variablesRaw != "" && resolver != nil {
		if expanded, expandErr := resolver.ExpandTemplates(variablesRaw); expandErr == nil {
			variablesRaw = strings.TrimSpace(expanded)
		} else {
			return nil, errdef.Wrap(errdef.CodeHTTP, expandErr, "expand graphql variables")
		}
	}

	var (
		variablesMap  map[string]interface{}
		variablesJSON string
	)

	if variablesRaw != "" {
		parsed, parseErr := decodeGraphQLVariables(variablesRaw)
		if parseErr != nil {
			return nil, parseErr
		}

		variablesMap = parsed
		normalised, marshalErr := json.Marshal(parsed)
		if marshalErr != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, marshalErr, "encode graphql variables")
		}
		variablesJSON = string(normalised)
	}

	if strings.EqualFold(req.Method, "GET") {
		expandedURL := strings.TrimSpace(req.URL)
		if resolver != nil {
			if expanded, expandErr := resolver.ExpandTemplates(expandedURL); expandErr == nil {
				expandedURL = strings.TrimSpace(expanded)
			} else {
				return nil, errdef.Wrap(errdef.CodeHTTP, expandErr, "expand graphql request url")
			}
		}
		if expandedURL == "" {
			return nil, errdef.New(errdef.CodeHTTP, "graphql request url is empty")
		}

		parsedURL, urlErr := url.Parse(expandedURL)
		if urlErr != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, urlErr, "parse graphql request url")
		}

		values := parsedURL.Query()
		values.Set("query", query)
		if operationName != "" {
			values.Set("operationName", operationName)
		} else {
			values.Del("operationName")
		}

		if variablesJSON != "" {
			values.Set("variables", variablesJSON)
		} else {
			values.Del("variables")
		}

		parsedURL.RawQuery = values.Encode()
		req.URL = parsedURL.String()
		return nil, nil
	}

	payload := map[string]interface{}{
		"query": query,
	}

	if operationName != "" {
		payload["operationName"] = operationName
	}

	if variablesMap != nil {
		payload["variables"] = variablesMap
	}

	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, marshalErr, "encode graphql payload")
	}
	return bytes.NewReader(body), nil
}

func (c *Client) graphQLSectionContent(
	inline, filePath, baseDir string,
	fallbacks []string,
	allowRaw bool,
	label string,
) (string, error) {
	inline = strings.TrimSpace(inline)
	if inline != "" {
		return inline, nil
	}

	if filePath == "" {
		return "", nil
	}

	data, _, err := c.readFileWithFallback(
		filePath,
		baseDir,
		fallbacks,
		allowRaw,
		strings.ToLower(label),
	)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Client) buildHTTPClient(opts Options) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	if opts.ProxyURL != "" {
		proxyURL, err := url.Parse(opts.ProxyURL)
		if err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "parse proxy url")
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	if opts.InsecureSkipVerify || len(opts.RootCAs) > 0 || opts.ClientCert != "" ||
		opts.ClientKey != "" {
		tlsCfg, err := tlsconfig.Build(tlsconfig.Files{
			RootCAs:    opts.RootCAs,
			RootMode:   opts.RootMode,
			ClientCert: opts.ClientCert,
			ClientKey:  opts.ClientKey,
			Insecure:   opts.InsecureSkipVerify,
		}, opts.BaseDir)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsCfg
	}

	if sshPlan := opts.SSH; sshPlan != nil && sshPlan.Active() {
		cfgCopy := *sshPlan.Config
		dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
			return sshPlan.Manager.DialContext(ctx, cfgCopy, network, address)
		}
		transport.Proxy = nil
		transport.DialContext = dialer
		if err := http2.ConfigureTransport(transport); err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "enable http2 over ssh")
		}
	}

	client := &http.Client{Transport: transport, Jar: c.jar}
	if opts.Timeout > 0 {
		client.Timeout = opts.Timeout
	}
	if !opts.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client, nil
}

func (c *Client) applyAuthentication(
	req *http.Request,
	resolver *vars.Resolver,
	auth *restfile.AuthSpec,
) {
	if auth == nil || len(auth.Params) == 0 {
		return
	}

	expand := func(value string) string {
		if value == "" {
			return ""
		}
		if resolver == nil {
			return value
		}
		if expanded, err := resolver.ExpandTemplates(value); err == nil {
			return expanded
		}
		return value
	}

	switch strings.ToLower(auth.Type) {
	case "basic":
		user := expand(auth.Params["username"])
		pass := expand(auth.Params["password"])
		if req.Header.Get("Authorization") == "" {
			req.SetBasicAuth(user, pass)
		}
	case "bearer":
		token := expand(auth.Params["token"])
		if req.Header.Get("Authorization") == "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	case "apikey", "api-key":
		placement := strings.ToLower(auth.Params["placement"])
		name := expand(auth.Params["name"])
		value := expand(auth.Params["value"])
		if placement == "query" {
			q := req.URL.Query()
			q.Set(name, value)
			req.URL.RawQuery = q.Encode()
		} else {
			if name == "" {
				name = "X-API-Key"
			}
			if req.Header.Get(name) == "" {
				req.Header.Set(name, value)
			}
		}
	case "header":
		name := expand(auth.Params["header"])
		value := expand(auth.Params["value"])
		if name != "" && req.Header.Get(name) == "" {
			req.Header.Set(name, value)
		}
	}
}

func (c *Client) readFileWithFallback(
	path string,
	baseDir string,
	fallbacks []string,
	allowRaw bool,
	label string,
) ([]byte, string, error) {
	if c == nil || c.fs == nil {
		return nil, "", errdef.New(errdef.CodeFilesystem, "file reader unavailable")
	}

	if path == "" {
		return nil, "", errdef.New(
			errdef.CodeFilesystem,
			"%s path is empty",
			strings.ToLower(label),
		)
	}

	if filepath.IsAbs(path) {
		data, err := c.fs.ReadFile(path)
		if err == nil {
			return data, path, nil
		}
		return nil, "", errdef.Wrap(
			errdef.CodeFilesystem,
			err,
			"read %s %s",
			strings.ToLower(label),
			path,
		)
	}

	candidates := buildPathCandidates(path, baseDir, fallbacks, allowRaw)

	var lastErr error
	var lastPath string
	for _, candidate := range candidates {
		data, err := c.fs.ReadFile(candidate)
		if err == nil {
			return data, candidate, nil
		}
		if stopReadFallback(err) {
			return nil, "", errdef.Wrap(
				errdef.CodeFilesystem,
				err,
				"read %s %s",
				strings.ToLower(label),
				candidate,
			)
		}
		lastErr = err
		lastPath = candidate
	}

	if lastErr == nil {
		lastErr = os.ErrNotExist
		lastPath = path
	}
	return nil, "", errdef.Wrap(
		errdef.CodeFilesystem,
		lastErr,
		"read %s %s (last tried %s)",
		strings.ToLower(label),
		path,
		lastPath,
	)
}

// Lines starting with @ get replaced with the file contents.
// @{variable} syntax is left alone so template expansion can handle it.
func (c *Client) injectBodyIncludes(
	body string,
	baseDir string,
	fallbacks []string,
	allowRaw bool,
) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

	var b strings.Builder
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if !first {
			b.WriteByte('\n')
		}

		first = false
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 1 && strings.HasPrefix(trimmed, "@") &&
			!strings.HasPrefix(trimmed, "@{") {
			includePath := strings.TrimSpace(trimmed[1:])
			if includePath != "" {
				data, _, err := c.readFileWithFallback(
					includePath,
					baseDir,
					fallbacks,
					allowRaw,
					"include body file",
				)
				if err != nil {
					return "", err
				}
				b.WriteString(string(data))
				continue
			}
		}
		b.WriteString(line)
	}

	if err := scanner.Err(); err != nil {
		return "", errdef.Wrap(errdef.CodeFilesystem, err, "scan body includes")
	}
	return b.String(), nil
}

func buildPathCandidates(path, baseDir string, fallbacks []string, allowRaw bool) []string {
	list := make([]string, 0, 2+len(fallbacks))
	if baseDir != "" {
		list = append(list, filepath.Join(baseDir, path))
	}
	for _, fb := range fallbacks {
		if fb == "" {
			continue
		}
		list = append(list, filepath.Join(fb, path))
	}
	if allowRaw {
		list = append(list, path)
	}
	return util.DedupeNonEmptyStrings(list)
}

func resolveFileLookup(baseDir string, opts Options) ([]string, bool) {
	if opts.NoFallback {
		return nil, baseDir == ""
	}
	return opts.FallbackBaseDirs, true
}

func stopReadFallback(err error) bool {
	return isPerm(err) || isDirErr(err) || errors.Is(err, os.ErrInvalid)
}

func isPerm(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, fs.ErrPermission)
}

func isDirErr(err error) bool {
	if errors.Is(err, syscall.EISDIR) {
		return true
	}
	var pe *fs.PathError
	if errors.As(err, &pe) && errors.Is(pe.Err, syscall.EISDIR) {
		return true
	}
	return false
}

type reqMeta struct {
	headers http.Header
	method  string
	host    string
	length  int64
	te      []string
}

func captureReqMeta(sent *http.Request, resp *http.Response) reqMeta {
	var h http.Header

	// Prefer the final request attached to the response, since redirects and transports can mutate it.
	reqForMeta := sent
	if resp != nil && resp.Request != nil {
		reqForMeta = resp.Request
	}

	if reqForMeta != nil && reqForMeta.Header != nil {
		h = reqForMeta.Header.Clone()
	} else if sent != nil && sent.Header != nil {
		h = sent.Header.Clone()
	}
	if h == nil {
		h = make(http.Header)
	}

	host := ""
	length := int64(0)
	var te []string
	method := ""

	if reqForMeta != nil {
		host = reqForMeta.Host
		if strings.TrimSpace(host) == "" && reqForMeta.URL != nil {
			host = reqForMeta.URL.Host
		}
		length = reqForMeta.ContentLength
		if len(reqForMeta.TransferEncoding) > 0 {
			te = append([]string(nil), reqForMeta.TransferEncoding...)
		}
		method = reqForMeta.Method
	}

	return reqMeta{headers: h, method: method, host: host, length: length, te: te}
}

// Second Decode call checks for trailing garbage after the JSON object.
// Without this, extra content would silently get ignored.
func decodeGraphQLVariables(raw string) (map[string]interface{}, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]interface{}
	if err := decoder.Decode(&payload); err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "parse graphql variables")
	}

	if err := decoder.Decode(new(interface{})); err != io.EOF {
		if err == nil {
			return nil, errdef.New(errdef.CodeHTTP, "unexpected trailing data in graphql variables")
		}
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "parse graphql variables")
	}
	return payload, nil
}

func applyRequestSettings(opts Options, settings map[string]string) Options {
	if len(settings) == 0 {
		return opts
	}

	effective := opts
	norm := make(map[string]string, len(settings))
	for k, v := range settings {
		norm[strings.ToLower(k)] = v
	}

	if value, ok := norm["timeout"]; ok {
		if dur, err := time.ParseDuration(value); err == nil {
			effective.Timeout = dur
		}
	}

	if value, ok := norm["proxy"]; ok && value != "" {
		effective.ProxyURL = value
	}

	if value, ok := norm["followredirects"]; ok {
		if b, err := strconv.ParseBool(value); err == nil {
			effective.FollowRedirects = b
		}
	}

	if value, ok := norm["insecure"]; ok {
		if b, err := strconv.ParseBool(value); err == nil {
			effective.InsecureSkipVerify = b
		}
	}

	return effective
}
