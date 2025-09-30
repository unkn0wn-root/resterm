package httpclient

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
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
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
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
}

type FileSystem interface {
	ReadFile(name string) ([]byte, error)
}

type OSFileSystem struct{}

func (OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

type Client struct {
	fs  FileSystem
	jar http.CookieJar
}

func NewClient(fs FileSystem) *Client {
	if fs == nil {
		fs = OSFileSystem{}
	}
	jar, _ := cookiejar.New(nil)
	return &Client{fs: fs, jar: jar}
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
}

func (c *Client) Execute(ctx context.Context, req *restfile.Request, resolver *vars.Resolver, opts Options) (*Response, error) {
	bodyReader, err := c.prepareBody(req, resolver, opts)
	if err != nil {
		return nil, err
	}

	expandedURL := req.URL
	if resolver != nil {
		expandedURL, err = resolver.ExpandTemplates(req.URL)
		if err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "expand url")
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, expandedURL, bodyReader)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "build request")
	}

	if req.Headers != nil {
		for name, values := range req.Headers {
			for _, value := range values {
				expanded, expandErr := resolver.ExpandTemplates(value)
				if expandErr == nil {
					httpReq.Header.Add(name, expanded)
				} else {
					httpReq.Header.Add(name, value)
				}
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
	client, err := c.buildHTTPClient(effectiveOpts)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	resp, err := client.Do(httpReq)
	duration := time.Since(start)
	if err != nil {
		return &Response{Request: req, Duration: duration}, errdef.Wrap(errdef.CodeHTTP, err, "perform request")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errdef.Wrap(errdef.CodeHTTP, err, "read response body")
	}

	response := &Response{
		Status:       resp.Status,
		StatusCode:   resp.StatusCode,
		Proto:        resp.Proto,
		Headers:      resp.Header.Clone(),
		Body:         body,
		Duration:     duration,
		EffectiveURL: resp.Request.URL.String(),
		Request:      req,
	}

	return response, nil
}

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
		return strings.NewReader(string(data)), nil
	case req.Body.Text != "":
		expanded, err := resolver.ExpandTemplates(req.Body.Text)
		if err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "expand body template")
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

func (c *Client) buildHTTPClient(opts Options) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
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

func applyRequestSettings(opts Options, settings map[string]string) Options {
	if len(settings) == 0 {
		return opts
	}
	effective := opts
	if value, ok := settings["timeout"]; ok {
		if dur, err := time.ParseDuration(value); err == nil {
			effective.Timeout = dur
		}
	}
	if value, ok := settings["proxy"]; ok && value != "" {
		effective.ProxyURL = value
	}
	if value, ok := settings["followredirects"]; ok {
		if b, err := strconv.ParseBool(value); err == nil {
			effective.FollowRedirects = b
		}
	}
	if value, ok := settings["insecure"]; ok {
		if b, err := strconv.ParseBool(value); err == nil {
			effective.InsecureSkipVerify = b
		}
	}
	return effective
}

func (c *Client) applyAuthentication(req *http.Request, resolver *vars.Resolver, auth *restfile.AuthSpec) {
	if auth == nil || len(auth.Params) == 0 {
		return
	}
	expand := func(value string) string {
		if value == "" {
			return ""
		}
		expanded, err := resolver.ExpandTemplates(value)
		if err != nil {
			return value
		}
		return expanded
	}

	switch strings.ToLower(auth.Type) {
	case "basic":
		user := expand(auth.Params["username"])
		pass := expand(auth.Params["password"])
		token := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		if req.Header.Get("Authorization") == "" {
			req.Header.Set("Authorization", "Basic "+token)
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

func (c *Client) injectBodyIncludes(body string, baseDir string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
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

				lines = append(lines, string(data))
				continue
			}
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return "", errdef.Wrap(errdef.CodeFilesystem, err, "scan body includes")
	}
	return strings.Join(lines, "\n"), nil
}
