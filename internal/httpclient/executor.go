package httpclient

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/telemetry"
	"github.com/unkn0wn-root/resterm/internal/vars"
	"nhooyr.io/websocket"
)

type Options struct {
	Timeout            time.Duration
	FollowRedirects    bool
	InsecureSkipVerify bool
	ProxyURL           string
	RootCAs            []string
	ClientCert         string
	ClientKey          string
	BaseDir            string
	Trace              bool
	TraceBudget        *nettrace.Budget
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

// NewClient constructs an HTTP client with default transport, cookie jar, and telemetry settings.
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
	Status       string
	StatusCode   int
	Proto        string
	Headers      http.Header
	Body         []byte
	Duration     time.Duration
	EffectiveURL string
	Request      *restfile.Request
	Timeline     *nettrace.Timeline
	TraceReport  *nettrace.Report
}

// Execute prepares the HTTP request from a restfile definition, executes it,
// records traces, and returns a structured response.
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
		return &Response{Request: req, Duration: duration, Timeline: timeline, TraceReport: traceReport}, errdef.Wrap(errdef.CodeHTTP, err, "perform request")
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

	resp = &Response{
		Status:       httpResp.Status,
		StatusCode:   httpResp.StatusCode,
		Proto:        httpResp.Proto,
		Headers:      httpResp.Header.Clone(),
		Body:         body,
		EffectiveURL: httpResp.Request.URL.String(),
		Request:      req,
		Timeline:     timeline,
		TraceReport:  traceReport,
	}
	resp.Duration = duration

	return resp, nil
}

// prepareHTTPRequest builds an http.Request with resolved variables, headers,
// body, and per-request settings.
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
	if resolver != nil {
		expandedURL, err = resolver.ExpandTemplates(expandedURL)
		if err != nil {
			return nil, opts, errdef.Wrap(errdef.CodeHTTP, err, "expand url")
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

// prepareBody selects the appropriate body reader based on inline text,
// external files, or GraphQL definitions.
func (c *Client) prepareBody(req *restfile.Request, resolver *vars.Resolver, opts Options) (io.Reader, error) {
	if req.Body.GraphQL != nil {
		return c.prepareGraphQLBody(req, resolver, opts)
	}

	switch {
	case req.Body.FilePath != "":
		path := req.Body.FilePath
		if !filepath.IsAbs(path) && opts.BaseDir != "" {
			path = filepath.Join(opts.BaseDir, path)
		}

		data, err := c.fs.ReadFile(path)
		if err != nil {
			return nil, errdef.Wrap(errdef.CodeFilesystem, err, "read body file %s", path)
		}

		if resolver != nil && req.Body.Options.ExpandTemplates {
			text := string(data)
			expanded, err := resolver.ExpandTemplates(text)
			if err != nil {
				return nil, errdef.Wrap(errdef.CodeHTTP, err, "expand body file templates")
			}

			processed, procErr := c.injectBodyIncludes(expanded, opts.BaseDir)
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
		processed, err := c.injectBodyIncludes(expanded, opts.BaseDir)
		if err != nil {
			return nil, err
		}
		return strings.NewReader(processed), nil
	default:
		return nil, nil
	}
}

// prepareGraphQLBody assembles the query, variables, and operation payload.
func (c *Client) prepareGraphQLBody(req *restfile.Request, resolver *vars.Resolver, opts Options) (io.Reader, error) {
	gql := req.Body.GraphQL
	query, err := c.graphQLSectionContent(gql.Query, gql.QueryFile, opts.BaseDir, "GraphQL query")
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

	variablesRaw, err := c.graphQLSectionContent(gql.Variables, gql.VariablesFile, opts.BaseDir, "GraphQL variables")
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
		variablesMap  map[string]any
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
		parsedURL, urlErr := url.Parse(req.URL)
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

	payload := map[string]any{
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

// graphQLSectionContent returns either the inline content or file contents for
// query and variable sections.
func (c *Client) graphQLSectionContent(inline, filePath, baseDir, label string) (string, error) {
	inline = strings.TrimSpace(inline)
	if inline != "" {
		return inline, nil
	}

	if filePath == "" {
		return "", nil
	}

	resolved := filePath
	if !filepath.IsAbs(resolved) && baseDir != "" {
		resolved = filepath.Join(baseDir, resolved)
	}

	data, err := c.fs.ReadFile(resolved)
	if err != nil {
		return "", errdef.Wrap(errdef.CodeFilesystem, err, "read %s %s", strings.ToLower(label), filePath)
	}
	return string(data), nil
}

// decodeGraphQLVariables parses the JSON object portion of GraphQL variables.
func decodeGraphQLVariables(raw string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "parse graphql variables")
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, errdef.New(errdef.CodeHTTP, "unexpected trailing data in graphql variables")
		}
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "parse graphql variables")
	}
	return payload, nil
}

// buildHTTPClient configures mutual TLS, timeouts, proxies, and cookie jars for each request.
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

	if opts.InsecureSkipVerify || len(opts.RootCAs) > 0 || opts.ClientCert != "" {
		tlsConfig := &tls.Config{InsecureSkipVerify: opts.InsecureSkipVerify} // nolint:gosec
		if len(opts.RootCAs) > 0 {
			pool, err := loadRootCAs(opts.RootCAs)
			if err != nil {
				return nil, errdef.Wrap(errdef.CodeHTTP, err, "load root cas")
			}
			tlsConfig.RootCAs = pool
		}
		if opts.ClientCert != "" && opts.ClientKey != "" {
			cert, err := tls.LoadX509KeyPair(opts.ClientCert, opts.ClientKey)
			if err != nil {
				return nil, errdef.Wrap(errdef.CodeHTTP, err, "load client certificate")
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		transport.TLSClientConfig = tlsConfig
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

// loadRootCAs loads additional root certificates from disk, falling back to the system pool.
func loadRootCAs(paths []string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "read root ca %s", p)
		}
		if ok := pool.AppendCertsFromPEM(data); !ok {
			return nil, errdef.New(errdef.CodeHTTP, "append cert from %s", p)
		}
	}
	return pool, nil
}

// applyRequestSettings applies per-request overrides to client options.
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

// applyAuthentication injects Authorization or header based credentials from restfile metadata.
func (c *Client) applyAuthentication(req *http.Request, resolver *vars.Resolver, auth *restfile.AuthSpec) {
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

// injectBodyIncludes processes < includes inside request bodies, replacing them with file contents.
func (c *Client) injectBodyIncludes(body string, baseDir string) (string, error) {
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
		if len(trimmed) > 1 && strings.HasPrefix(trimmed, "@") && !strings.HasPrefix(trimmed, "@{") {
			includePath := strings.TrimSpace(trimmed[1:])
			if includePath != "" {
				path := includePath
				if !filepath.IsAbs(path) && baseDir != "" {
					path = filepath.Join(baseDir, path)
				}

				data, err := c.fs.ReadFile(path)
				if err != nil {
					return "", errdef.Wrap(errdef.CodeFilesystem, err, "include body file %s", includePath)
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
