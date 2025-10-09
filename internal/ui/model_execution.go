package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/oauth"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/parser/curl"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) sendActiveRequest() tea.Cmd {
	content := m.editor.Value()
	doc := parser.Parse(m.currentFile, []byte(content))
	cursorLine := currentCursorLine(m.editor)
	req := findRequestAtLine(doc, cursorLine)
	if req != nil && (cursorLine < req.LineRange.Start || cursorLine > req.LineRange.End) {
		req = nil
	}
	if req == nil {
		if inline := buildInlineRequest(content, cursorLine); inline != nil {
			req = inline
		} else {
			return func() tea.Msg {
				return statusMsg{text: "No request at cursor", level: statusWarn}
			}
		}
	}

	m.doc = doc
	m.syncRequestList(doc)
	m.setActiveRequest(req)

	cloned := cloneRequest(req)
	m.currentRequest = cloned
	m.testResults = nil
	m.scriptError = nil

	options := m.cfg.HTTPOptions
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}

	m.sending = true
	base := fmt.Sprintf("Sending %s", cloned.URL)
	m.statusPulseBase = base
	m.statusPulseFrame = -1
	m.setStatusMessage(statusMsg{text: base, level: statusInfo})

	execCmd := m.executeRequest(doc, cloned, options)
	if tick := m.scheduleStatusPulse(); tick != nil {
		return tea.Batch(execCmd, tick)
	}
	return execCmd
}

func (m *Model) executeRequest(doc *restfile.Document, req *restfile.Request, options httpclient.Options) tea.Cmd {
	client := m.client
	runner := m.scriptRunner
	envName := m.cfg.EnvironmentName
	baseVars := m.collectVariables(doc, req)

	return func() tea.Msg {
		preVars := cloneStringMap(baseVars)
		preGlobals := m.collectGlobalValues(doc)
		preResult, err := runner.RunPreRequest(req.Metadata.Scripts, scripts.PreRequestInput{
			Request:   req,
			Variables: preVars,
			Globals:   preGlobals,
			BaseDir:   options.BaseDir,
		})
		if err != nil {
			return responseMsg{err: errdef.Wrap(errdef.CodeScript, err, "pre-request script")}
		}

		if err := applyPreRequestOutput(req, preResult); err != nil {
			return responseMsg{err: err}
		}

		m.applyGlobalMutations(preResult.Globals)

		scriptVars := cloneStringMap(preResult.Variables)
		resolver := m.buildResolver(doc, req, scriptVars)
		effectiveTimeout := defaultTimeout(resolveRequestTimeout(req, options.Timeout))
		if err := m.ensureOAuth(req, resolver, options, effectiveTimeout); err != nil {
			return responseMsg{err: err}
		}

		if req.GRPC != nil {
			if err := m.prepareGRPCRequest(req, resolver); err != nil {
				return responseMsg{err: err}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), effectiveTimeout)
		defer cancel()

		if req.GRPC != nil {
			grpcOpts := m.grpcOptions
			if grpcOpts.BaseDir == "" {
				grpcOpts.BaseDir = options.BaseDir
				if grpcOpts.BaseDir == "" && m.currentFile != "" {
					grpcOpts.BaseDir = filepath.Dir(m.currentFile)
				}
			}

			if grpcOpts.DialTimeout == 0 {
				grpcOpts.DialTimeout = effectiveTimeout
			}

			grpcResp, grpcErr := m.grpcClient.Execute(ctx, req, req.GRPC, grpcOpts)
			return responseMsg{
				grpc:        grpcResp,
				err:         grpcErr,
				executed:    req,
				requestText: renderRequestText(req),
				environment: envName,
			}
		}

		response, err := client.Execute(ctx, req, resolver, options)
		if err != nil {
			return responseMsg{err: err}
		}

		if err := m.applyCaptures(doc, req, resolver, response); err != nil {
			return responseMsg{err: err}
		}

		testVars := mergeVariableMaps(baseVars, scriptVars)
		testGlobals := m.collectGlobalValues(doc)
		tests, globalChanges, testErr := runner.RunTests(req.Metadata.Scripts, scripts.TestInput{
			Response:  response,
			Variables: testVars,
			Globals:   testGlobals,
			BaseDir:   options.BaseDir,
		})
		m.applyGlobalMutations(globalChanges)

		return responseMsg{
			response:    response,
			tests:       tests,
			scriptErr:   testErr,
			executed:    req,
			requestText: renderRequestText(req),
			environment: envName,
		}
	}
}

const statusPulseInterval = 300 * time.Millisecond

func (m *Model) scheduleStatusPulse() tea.Cmd {
	if !m.sending {
		return nil
	}
	return tea.Tick(statusPulseInterval, func(time.Time) tea.Msg {
		return statusPulseMsg{}
	})
}

func (m *Model) handleStatusPulse() tea.Cmd {
	if !m.sending {
		return nil
	}

	m.statusPulseFrame++
	if m.statusPulseFrame >= 3 {
		m.statusPulseFrame = 0
	}

	base := strings.TrimSpace(m.statusPulseBase)
	if base == "" {
		base = "Sending"
	}

	dots := strings.Repeat(".", m.statusPulseFrame+1)
	m.setStatusMessage(statusMsg{text: base + dots, level: statusInfo})
	return m.scheduleStatusPulse()
}

func defaultTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return 30 * time.Second
}

func resolveRequestTimeout(req *restfile.Request, base time.Duration) time.Duration {
	if req != nil {
		if raw, ok := req.Settings["timeout"]; ok {
			if dur, err := time.ParseDuration(raw); err == nil && dur > 0 {
				return dur
			}
		}
	}
	return base
}

func (m *Model) buildResolver(doc *restfile.Document, req *restfile.Request, extras ...map[string]string) *vars.Resolver {
	providers := make([]vars.Provider, 0, 8)

	for _, extra := range extras {
		if len(extra) > 0 {
			providers = append(providers, vars.NewMapProvider("script", extra))
		}
	}

	if req != nil {
		reqVars := make(map[string]string)
		for _, v := range req.Variables {
			reqVars[v.Name] = v.Value
		}
		if len(reqVars) > 0 {
			providers = append(providers, vars.NewMapProvider("request", reqVars))
		}
	}

	if m.globals != nil {
		if snapshot := m.globals.snapshot(m.cfg.EnvironmentName); len(snapshot) > 0 {
			values := make(map[string]string, len(snapshot))
			for key, entry := range snapshot {
				name := entry.Name
				if strings.TrimSpace(name) == "" {
					name = key
				}
				values[name] = entry.Value
			}
			providers = append(providers, vars.NewMapProvider("global", values))
		}
	}

	if doc != nil {
		globalVars := make(map[string]string)
		for _, v := range doc.Globals {
			globalVars[v.Name] = v.Value
		}
		if len(globalVars) > 0 {
			providers = append(providers, vars.NewMapProvider("document-global", globalVars))
		}
	}

	fileVars := make(map[string]string)
	if doc != nil {
		for _, v := range doc.Variables {
			fileVars[v.Name] = v.Value
		}
	}
	if len(fileVars) > 0 {
		providers = append(providers, vars.NewMapProvider("file", fileVars))
	}

	if envValues := m.environmentValues(); len(envValues) > 0 {
		providers = append(providers, vars.NewMapProvider("environment", envValues))
	}

	providers = append(providers, vars.EnvProvider{})
	return vars.NewResolver(providers...)
}

func (m *Model) environmentValues() map[string]string {
	if m.cfg.EnvironmentSet == nil || m.cfg.EnvironmentName == "" {
		return nil
	}
	if env, ok := m.cfg.EnvironmentSet[m.cfg.EnvironmentName]; ok {
		return env
	}
	return nil
}

func (m *Model) collectVariables(doc *restfile.Document, req *restfile.Request) map[string]string {
	vars := make(map[string]string)
	if env := m.environmentValues(); env != nil {
		for k, v := range env {
			vars[k] = v
		}
	}
	if doc != nil {
		for _, v := range doc.Variables {
			vars[v.Name] = v.Value
		}
		for _, v := range doc.Globals {
			vars[v.Name] = v.Value
		}
	}
	if m.globals != nil {
		if snapshot := m.globals.snapshot(m.cfg.EnvironmentName); len(snapshot) > 0 {
			for key, entry := range snapshot {
				name := entry.Name
				if strings.TrimSpace(name) == "" {
					name = key
				}
				vars[name] = entry.Value
			}
		}
	}
	if req != nil {
		for _, v := range req.Variables {
			vars[v.Name] = v.Value
		}
	}
	return vars
}

func (m *Model) collectGlobalValues(doc *restfile.Document) map[string]scripts.GlobalValue {
	globals := make(map[string]scripts.GlobalValue)
	if doc != nil {
		for _, v := range doc.Globals {
			name := strings.TrimSpace(v.Name)
			if name == "" {
				continue
			}
			globals[name] = scripts.GlobalValue{Name: name, Value: v.Value, Secret: v.Secret}
		}
	}
	if m.globals != nil {
		if snapshot := m.globals.snapshot(m.cfg.EnvironmentName); len(snapshot) > 0 {
			for key, entry := range snapshot {
				name := strings.TrimSpace(entry.Name)
				if name == "" {
					name = key
				}
				globals[name] = scripts.GlobalValue{Name: name, Value: entry.Value, Secret: entry.Secret}
			}
		}
	}
	if len(globals) == 0 {
		return nil
	}
	return globals
}

func (m *Model) applyGlobalMutations(changes map[string]scripts.GlobalValue) {
	if len(changes) == 0 || m.globals == nil {
		return
	}
	env := m.cfg.EnvironmentName
	for _, change := range changes {
		name := strings.TrimSpace(change.Name)
		if name == "" {
			continue
		}
		if change.Delete {
			m.globals.delete(env, name)
			continue
		}
		m.globals.set(env, name, change.Value, change.Secret)
	}
}

func (m *Model) showGlobalSummary() tea.Cmd {
	text := m.buildGlobalSummary()
	if strings.TrimSpace(text) == "" {
		text = "Globals: (empty)"
	}
	m.setStatusMessage(statusMsg{level: statusInfo, text: text})
	return nil
}

func (m *Model) buildGlobalSummary() string {
	var segments []string

	if snapshot := m.globalsSnapshot(); len(snapshot) > 0 {
		entries := make([]summaryEntry, 0, len(snapshot))
		for key, value := range snapshot {
			name := strings.TrimSpace(value.Name)
			if name == "" {
				name = key
			}
			entries = append(entries, summaryEntry{name: name, value: value.Value, secret: value.Secret})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
		parts := make([]string, 0, len(entries))
		for _, entry := range entries {
			parts = append(parts, fmt.Sprintf("%s=%s", entry.name, maskSecret(entry.value, entry.secret)))
		}
		segments = append(segments, "Globals: "+strings.Join(parts, ", "))
	}

	if doc := m.doc; doc != nil {
		entries := make([]summaryEntry, 0, len(doc.Globals))
		for _, global := range doc.Globals {
			name := strings.TrimSpace(global.Name)
			if name == "" {
				continue
			}
			entries = append(entries, summaryEntry{name: name, value: global.Value, secret: global.Secret})
		}
		if len(entries) > 0 {
			sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
			parts := make([]string, 0, len(entries))
			for _, entry := range entries {
				parts = append(parts, fmt.Sprintf("%s=%s", entry.name, maskSecret(entry.value, entry.secret)))
			}
			segments = append(segments, "Doc: "+strings.Join(parts, ", "))
		}
	}

	return strings.Join(segments, " | ")
}

func (m *Model) globalsSnapshot() map[string]globalValue {
	if m.globals == nil {
		return nil
	}
	return m.globals.snapshot(m.cfg.EnvironmentName)
}

func (m *Model) clearGlobalValues() tea.Cmd {
	if m.globals == nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "No global store available"})
		return nil
	}
	env := m.cfg.EnvironmentName
	m.globals.clear(env)
	label := env
	if strings.TrimSpace(label) == "" {
		label = "default"
	}
	m.setStatusMessage(statusMsg{level: statusInfo, text: fmt.Sprintf("Cleared globals for %s", label)})
	return nil
}

type summaryEntry struct {
	name   string
	value  string
	secret bool
}

func maskSecret(value string, secret bool) string {
	if secret {
		return "•••"
	}
	return value
}

func (m *Model) applyCaptures(doc *restfile.Document, req *restfile.Request, resolver *vars.Resolver, resp *httpclient.Response) error {
	if req == nil || resp == nil {
		return nil
	}
	if len(req.Metadata.Captures) == 0 {
		return nil
	}
	ctx := newCaptureContext(resp)
	for _, capture := range req.Metadata.Captures {
		value, err := ctx.evaluate(capture.Expression, resolver)
		if err != nil {
			return errdef.Wrap(errdef.CodeScript, err, "evaluate capture %s", capture.Name)
		}
		switch capture.Scope {
		case restfile.CaptureScopeRequest:
			upsertVariable(&req.Variables, restfile.ScopeRequest, capture.Name, value, capture.Secret)
		case restfile.CaptureScopeFile:
			if doc != nil {
				upsertVariable(&doc.Variables, restfile.ScopeFile, capture.Name, value, capture.Secret)
			}
		case restfile.CaptureScopeGlobal:
			if m.globals != nil {
				m.globals.set(m.cfg.EnvironmentName, capture.Name, value, capture.Secret)
			}
		}
	}
	return nil
}

type captureContext struct {
	response  *httpclient.Response
	body      string
	headers   http.Header
	jsonOnce  sync.Once
	jsonValue interface{}
	jsonErr   error
}

var captureTemplatePattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func newCaptureContext(resp *httpclient.Response) *captureContext {
	body := ""
	if resp != nil {
		body = string(resp.Body)
	}
	return &captureContext{response: resp, body: body, headers: cloneHeader(resp.Headers)}
}

func (c *captureContext) evaluate(expr string, resolver *vars.Resolver) (string, error) {
	var firstErr error
	expanded := captureTemplatePattern.ReplaceAllStringFunc(expr, func(match string) string {
		name := strings.TrimSpace(captureTemplatePattern.FindStringSubmatch(match)[1])
		if name == "" {
			return match
		}
		if strings.HasPrefix(strings.ToLower(name), "response.") {
			value, err := c.lookupResponse(strings.TrimSpace(name[len("response."):]))
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return match
			}
			return value
		}
		if resolver != nil {
			res, err := resolver.ExpandTemplates(match)
			if err == nil {
				return res
			}
			if firstErr == nil {
				firstErr = err
			}
		}
		return match
	})
	if firstErr != nil {
		return "", firstErr
	}
	return expanded, nil
}

func (c *captureContext) lookupResponse(path string) (string, error) {
	switch strings.ToLower(path) {
	case "body":
		return c.body, nil
	case "status":
		if c.response != nil {
			return c.response.Status, nil
		}
		return "", nil
	case "statuscode":
		if c.response != nil {
			return strconv.Itoa(c.response.StatusCode), nil
		}
		return "", nil
	}
	if strings.HasPrefix(strings.ToLower(path), "headers.") {
		key := path[len("headers."):]
		if c.headers == nil {
			return "", fmt.Errorf("header %s not available", key)
		}
		values := c.headers.Values(key)
		if len(values) == 0 {
			values = c.headers.Values(http.CanonicalHeaderKey(key))
		}
		if len(values) == 0 {
			return "", fmt.Errorf("header %s not found", key)
		}
		return strings.Join(values, ", "), nil
	}
	if strings.HasPrefix(strings.ToLower(path), "json") {
		return c.lookupJSON(path), nil
	}
	return "", fmt.Errorf("unsupported response reference %q", path)
}

func (c *captureContext) lookupJSON(path string) string {
	c.jsonOnce.Do(func() {
		if strings.TrimSpace(c.body) == "" {
			c.jsonErr = fmt.Errorf("response body empty")
			return
		}
		var data interface{}
		if err := json.Unmarshal([]byte(c.body), &data); err != nil {
			c.jsonErr = err
			return
		}
		c.jsonValue = data
	})
	if c.jsonErr != nil {
		return ""
	}
	trimmed := strings.TrimSpace(path[len("json"):])
	if trimmed == "" {
		return c.body
	}
	trimmed = strings.TrimPrefix(trimmed, ".")
	current := c.jsonValue
	for _, segment := range splitJSONPath(trimmed) {
		switch typed := current.(type) {
		case map[string]interface{}:
			val, ok := typed[segment.name]
			if !ok {
				return ""
			}
			current = val
		case []interface{}:
			if segment.index == nil {
				return ""
			}
			idx := *segment.index
			if idx < 0 || idx >= len(typed) {
				return ""
			}
			current = typed[idx]
		default:
			return ""
		}
	}
	return stringifyJSONValue(current)
}

type jsonPathSegment struct {
	name  string
	index *int
}

func splitJSONPath(path string) []jsonPathSegment {
	parts := strings.Split(path, ".")
	segments := make([]jsonPathSegment, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		segment := jsonPathSegment{}
		if bracket := strings.Index(part, "["); bracket != -1 {
			segment.name = part[:bracket]
			end := strings.Index(part[bracket:], "]")
			if end > 1 {
				idxStr := part[bracket+1 : bracket+end]
				if n, err := strconv.Atoi(idxStr); err == nil {
					segment.index = &n
				}
			}
		} else {
			segment.name = part
		}
		segments = append(segments, segment)
	}
	return segments
}

func stringifyJSONValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if float64(int64(v)) == v {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(data)
	}
}

func upsertVariable(list *[]restfile.Variable, scope restfile.VariableScope, name, value string, secret bool) {
	lower := strings.ToLower(name)
	vars := *list
	for i := range vars {
		if strings.ToLower(vars[i].Name) == lower {
			vars[i].Value = value
			vars[i].Scope = scope
			vars[i].Secret = secret
			return
		}
	}
	*list = append(vars, restfile.Variable{Name: name, Value: value, Scope: scope, Secret: secret})
}

func (m *Model) ensureOAuth(req *restfile.Request, resolver *vars.Resolver, opts httpclient.Options, timeout time.Duration) error {
	if req == nil || req.Metadata.Auth == nil {
		return nil
	}
	if !strings.EqualFold(req.Metadata.Auth.Type, "oauth2") {
		return nil
	}
	if m.oauth == nil {
		return errdef.New(errdef.CodeHTTP, "oauth support is not initialised")
	}
	cfg, err := m.buildOAuthConfig(req.Metadata.Auth, resolver)
	if err != nil {
		return err
	}
	if cfg.TokenURL == "" {
		return errdef.New(errdef.CodeHTTP, "@auth oauth2 requires token_url")
	}
	header := cfg.Header
	if strings.TrimSpace(header) == "" {
		header = "Authorization"
	}
	if req.Headers != nil && req.Headers.Get(header) != "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	token, err := m.oauth.Token(ctx, m.cfg.EnvironmentName, cfg, opts)
	if err != nil {
		return errdef.Wrap(errdef.CodeHTTP, err, "fetch oauth token")
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	if req.Headers.Get(header) != "" {
		return nil
	}
	value := token.AccessToken
	if strings.EqualFold(header, "authorization") {
		typeValue := strings.TrimSpace(token.TokenType)
		if typeValue == "" {
			typeValue = "Bearer"
		}
		value = strings.TrimSpace(typeValue) + " " + token.AccessToken
	}
	req.Headers.Set(header, value)
	return nil
}

func (m *Model) buildOAuthConfig(auth *restfile.AuthSpec, resolver *vars.Resolver) (oauth.Config, error) {
	cfg := oauth.Config{Extra: make(map[string]string)}
	if auth == nil {
		return cfg, errdef.New(errdef.CodeHTTP, "missing oauth spec")
	}
	expand := func(key string) (string, error) {
		value := auth.Params[key]
		if strings.TrimSpace(value) == "" {
			return "", nil
		}
		if resolver == nil {
			return strings.TrimSpace(value), nil
		}
		expanded, err := resolver.ExpandTemplates(value)
		if err != nil {
			return "", errdef.Wrap(errdef.CodeHTTP, err, "expand oauth param %s", key)
		}
		return strings.TrimSpace(expanded), nil
	}

	var err error
	if cfg.TokenURL, err = expand("token_url"); err != nil {
		return cfg, err
	}
	if cfg.ClientID, err = expand("client_id"); err != nil {
		return cfg, err
	}
	if cfg.ClientSecret, err = expand("client_secret"); err != nil {
		return cfg, err
	}
	if cfg.Scope, err = expand("scope"); err != nil {
		return cfg, err
	}
	if cfg.Audience, err = expand("audience"); err != nil {
		return cfg, err
	}
	if cfg.Resource, err = expand("resource"); err != nil {
		return cfg, err
	}
	if cfg.Username, err = expand("username"); err != nil {
		return cfg, err
	}
	if cfg.Password, err = expand("password"); err != nil {
		return cfg, err
	}
	if cfg.ClientAuth, err = expand("client_auth"); err != nil {
		return cfg, err
	}
	if cfg.GrantType, err = expand("grant"); err != nil {
		return cfg, err
	}
	if cfg.Header, err = expand("header"); err != nil {
		return cfg, err
	}
	if cfg.CacheKey, err = expand("cache_key"); err != nil {
		return cfg, err
	}

	known := map[string]struct{}{
		"token_url":     {},
		"client_id":     {},
		"client_secret": {},
		"scope":         {},
		"audience":      {},
		"resource":      {},
		"username":      {},
		"password":      {},
		"client_auth":   {},
		"grant":         {},
		"header":        {},
		"cache_key":     {},
	}
	for key, raw := range auth.Params {
		if _, ok := known[strings.ToLower(key)]; ok {
			continue
		}
		if strings.TrimSpace(raw) == "" {
			continue
		}
		value, err := expand(key)
		if err != nil {
			return cfg, err
		}
		if value != "" {
			cfg.Extra[strings.ToLower(strings.ReplaceAll(key, "-", "_"))] = value
		}
	}
	if len(cfg.Extra) == 0 {
		cfg.Extra = nil
	}
	return cfg, nil
}

func mergeVariableMaps(base map[string]string, additions map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(additions))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range additions {
		merged[k] = v
	}
	return merged
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	clone := make(map[string]string, len(input))
	for k, v := range input {
		clone[k] = v
	}
	return clone
}

func (m *Model) prepareGRPCRequest(req *restfile.Request, resolver *vars.Resolver) error {
	grpcReq := req.GRPC
	if grpcReq == nil {
		return nil
	}

	if strings.TrimSpace(grpcReq.FullMethod) == "" {
		service := strings.TrimSpace(grpcReq.Service)
		method := strings.TrimSpace(grpcReq.Method)
		if service != "" && method != "" {
			if grpcReq.Package != "" {
				grpcReq.FullMethod = "/" + grpcReq.Package + "." + service + "/" + method
			} else {
				grpcReq.FullMethod = "/" + service + "/" + method
			}
		} else {
			return errdef.New(errdef.CodeHTTP, "grpc method metadata is incomplete")
		}
	}

	if text := strings.TrimSpace(req.Body.Text); text != "" {
		grpcReq.Message = req.Body.Text
		grpcReq.MessageFile = ""
	} else if file := strings.TrimSpace(req.Body.FilePath); file != "" {
		grpcReq.MessageFile = req.Body.FilePath
		grpcReq.Message = ""
	}

	if resolver != nil {
		target, err := resolver.ExpandTemplates(grpcReq.Target)
		if err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc target")
		}

		grpcReq.Target = strings.TrimSpace(target)
		if strings.TrimSpace(grpcReq.Message) != "" {
			expanded, err := resolver.ExpandTemplates(grpcReq.Message)
			if err != nil {
				return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc message")
			}
			grpcReq.Message = expanded
		}
		if len(grpcReq.Metadata) > 0 {
			for key, value := range grpcReq.Metadata {
				expanded, err := resolver.ExpandTemplates(value)
				if err != nil {
					return errdef.Wrap(errdef.CodeHTTP, err, "expand grpc metadata %s", key)
				}
				grpcReq.Metadata[key] = expanded
			}
		}

		if req.Headers != nil {
			for key, values := range req.Headers {
				for i, value := range values {
					expanded, err := resolver.ExpandTemplates(value)
					if err != nil {
						return errdef.Wrap(errdef.CodeHTTP, err, "expand header %s", key)
					}
					req.Headers[key][i] = expanded
				}
			}
		}
	}

	grpcReq.Target = strings.TrimSpace(grpcReq.Target)
	if grpcReq.Target == "" {
		return errdef.New(errdef.CodeHTTP, "grpc target not specified")
	}
	req.URL = grpcReq.Target
	return nil
}

func applyPreRequestOutput(req *restfile.Request, out scripts.PreRequestOutput) error {
	if out.Method != nil {
		req.Method = strings.ToUpper(strings.TrimSpace(*out.Method))
	}

	if out.URL != nil {
		req.URL = strings.TrimSpace(*out.URL)
	}

	if len(out.Query) > 0 {
		parsed, err := url.Parse(req.URL)
		if err != nil {
			return errdef.Wrap(errdef.CodeScript, err, "invalid url after script")
		}

		query := parsed.Query()
		for key, value := range out.Query {
			query.Set(key, value)
		}
		parsed.RawQuery = query.Encode()
		req.URL = parsed.String()
	}
	if out.Headers != nil {
		if req.Headers == nil {
			req.Headers = make(http.Header)
		}
		for name, values := range out.Headers {
			req.Headers.Del(name)
			for _, value := range values {
				req.Headers.Add(name, value)
			}
		}
	}
	if out.Body != nil {
		req.Body.FilePath = ""
		req.Body.Text = *out.Body
		req.Body.GraphQL = nil
	}
	if len(out.Variables) > 0 {
		existing := make(map[string]int)
		for i, v := range req.Variables {
			existing[strings.ToLower(v.Name)] = i
		}

		for name, value := range out.Variables {
			key := strings.ToLower(name)
			if idx, ok := existing[key]; ok {
				req.Variables[idx].Value = value
			} else {
				req.Variables = append(req.Variables, restfile.Variable{
					Name:  name,
					Value: value,
					Scope: restfile.ScopeRequest,
				})
			}
		}
	}
	return nil
}

func cloneRequest(req *restfile.Request) *restfile.Request {
	if req == nil {
		return nil
	}

	clone := *req
	clone.Headers = cloneHeader(req.Headers)
	if req.Settings != nil {
		clone.Settings = make(map[string]string, len(req.Settings))
		for k, v := range req.Settings {
			clone.Settings[k] = v
		}
	}

	clone.Variables = append([]restfile.Variable(nil), req.Variables...)
	clone.Metadata.Tags = append([]string(nil), req.Metadata.Tags...)
	clone.Metadata.Scripts = append([]restfile.ScriptBlock(nil), req.Metadata.Scripts...)
	clone.Metadata.Captures = append([]restfile.CaptureSpec(nil), req.Metadata.Captures...)
	if req.Body.GraphQL != nil {
		gql := *req.Body.GraphQL
		clone.Body.GraphQL = &gql
	}
	if req.GRPC != nil {
		grpcCopy := *req.GRPC
		if len(grpcCopy.Metadata) > 0 {
			meta := make(map[string]string, len(grpcCopy.Metadata))
			for k, v := range grpcCopy.Metadata {
				meta[k] = v
			}
			grpcCopy.Metadata = meta
		}
		clone.GRPC = &grpcCopy
	}
	return &clone
}

func cloneHeader(h http.Header) http.Header {
	if h == nil {
		return nil
	}

	cloned := make(http.Header, len(h))
	for k, values := range h {
		cloned[k] = append([]string(nil), values...)
	}
	return cloned
}

func renderRequestText(req *restfile.Request) string {
	if req == nil {
		return ""
	}

	builder := strings.Builder{}
	builder.WriteString(fmt.Sprintf("%s %s\n", req.Method, req.URL))
	headerNames := make([]string, 0, len(req.Headers))
	for name := range req.Headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		for _, value := range req.Headers[name] {
			builder.WriteString(fmt.Sprintf("%s: %s\n", name, value))
		}
	}

	builder.WriteString("\n")
	if req.GRPC != nil {
		grpc := req.GRPC
		if grpc.FullMethod != "" {
			builder.WriteString("# @grpc ")
			builder.WriteString(strings.TrimPrefix(grpc.FullMethod, "/"))
			builder.WriteString("\n")
		}
		if grpc.DescriptorSet != "" {
			builder.WriteString("# @grpc-descriptor " + grpc.DescriptorSet + "\n")
		}
		if !grpc.UseReflection {
			builder.WriteString("# @grpc-reflection false\n")
		}
		if grpc.PlaintextSet {
			builder.WriteString(fmt.Sprintf("# @grpc-plaintext %t\n", grpc.Plaintext))
		}
		if grpc.Authority != "" {
			builder.WriteString("# @grpc-authority " + grpc.Authority + "\n")
		}
		if len(grpc.Metadata) > 0 {
			keys := make([]string, 0, len(grpc.Metadata))
			for k := range grpc.Metadata {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				builder.WriteString(fmt.Sprintf("# @grpc-metadata %s: %s\n", k, grpc.Metadata[k]))
			}
		}
		builder.WriteString("\n")
		if strings.TrimSpace(grpc.Message) != "" {
			builder.WriteString(grpc.Message)
			if !strings.HasSuffix(grpc.Message, "\n") {
				builder.WriteString("\n")
			}
		} else if strings.TrimSpace(grpc.MessageFile) != "" {
			builder.WriteString("< " + strings.TrimSpace(grpc.MessageFile) + "\n")
		}
	} else if req.Body.GraphQL != nil {
		gql := req.Body.GraphQL
		builder.WriteString("# @graphql\n")
		if strings.TrimSpace(gql.OperationName) != "" {
			builder.WriteString("# @operation " + strings.TrimSpace(gql.OperationName) + "\n")
		}

		if strings.TrimSpace(gql.Query) != "" {
			builder.WriteString(gql.Query)
			if !strings.HasSuffix(gql.Query, "\n") {
				builder.WriteString("\n")
			}
		} else if strings.TrimSpace(gql.QueryFile) != "" {
			builder.WriteString("< " + strings.TrimSpace(gql.QueryFile) + "\n")
		}

		if strings.TrimSpace(gql.Variables) != "" || strings.TrimSpace(gql.VariablesFile) != "" {
			builder.WriteString("\n# @variables\n")
			if strings.TrimSpace(gql.Variables) != "" {
				builder.WriteString(gql.Variables)
				if !strings.HasSuffix(gql.Variables, "\n") {
					builder.WriteString("\n")
				}
			} else if strings.TrimSpace(gql.VariablesFile) != "" {
				builder.WriteString("< " + strings.TrimSpace(gql.VariablesFile) + "\n")
			}
		}
	} else if req.Body.FilePath != "" {
		builder.WriteString("< " + req.Body.FilePath + "\n")
	} else if strings.TrimSpace(req.Body.Text) != "" {
		builder.WriteString(req.Body.Text)
		if !strings.HasSuffix(req.Body.Text, "\n") {
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func buildInlineRequest(content string, lineNumber int) *restfile.Request {
	if lineNumber < 1 {
		return nil
	}

	lines := strings.Split(content, "\n")
	if req := inlineCurlRequest(lines, lineNumber); req != nil {
		return req
	}

	if lineNumber > len(lines) {
		return nil
	}
	return inlineRequestFromLine(lines[lineNumber-1], lineNumber)
}

func inlineCurlRequest(lines []string, lineNumber int) *restfile.Request {
	idx := lineNumber - 1
	if idx < 0 || idx >= len(lines) {
		return nil
	}

	start, end, command := extractCurlCommand(lines, idx)
	if command == "" {
		return nil
	}

	parsed, err := curl.ParseCommand(command)
	if err != nil {
		return nil
	}
	parsed.LineRange = restfile.LineRange{Start: start + 1, End: end + 1}
	parsed.OriginalText = strings.Join(lines[start:end+1], "\n")
	return parsed
}

func extractCurlCommand(lines []string, cursorIdx int) (start int, end int, command string) {
	start = -1
	var builder strings.Builder
	for i := cursorIdx; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if i == cursorIdx {
				continue
			}
			break
		}
		if isCurlStartLine(trimmed) {
			start = i
			break
		}
		if !continuesCurl(trimmed) {
			return -1, -1, ""
		}
	}
	if start == -1 {
		return -1, -1, ""
	}

	builder.Reset()
	end = start
	for i := start; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" && i > start {
			break
		}

		end = i
		stripped := strings.TrimLeft(line, " \t")
		tail := strings.TrimRight(stripped, " \t")
		continued := false
		if strings.HasSuffix(tail, "\\") && !strings.HasSuffix(tail, "\\\\") {
			continued = true
			stripped = strings.TrimSuffix(tail, "\\")
		}

		builder.WriteString(stripped)
		if continued {
			builder.WriteByte(' ')
			continue
		}
		break
	}
	return start, end, strings.TrimSpace(builder.String())
}

func isCurlStartLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "sudo ")
	trimmed = strings.TrimSpace(trimmed)
	if strings.HasPrefix(trimmed, "curl ") || trimmed == "curl" {
		return true
	}
	for _, prefix := range []string{"$", "%", ">", "!"} {
		prefixed := prefix + " "
		if strings.HasPrefix(trimmed, prefixed) {
			candidate := strings.TrimSpace(trimmed[len(prefixed):])
			if strings.HasPrefix(candidate, "curl ") || candidate == "curl" {
				return true
			}
		}
	}
	return false
}

func continuesCurl(line string) bool {
	if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "--") {
		return true
	}
	if strings.HasSuffix(line, "\\") {
		return true
	}
	return false
}

func inlineRequestFromLine(raw string, lineNumber int) *restfile.Request {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	method := "GET"
	url := ""

	fields := strings.Fields(trimmed)
	if len(fields) == 1 {
		url = fields[0]
	} else if len(fields) >= 2 {
		candidate := strings.ToUpper(fields[0])
		if isInlineHTTPMethod(candidate) {
			method = candidate
			url = fields[1]
		}
	}

	if url == "" {
		url = trimmed
	}

	url = strings.Trim(url, "\"'")
	if !looksLikeHTTPRequestURL(url) {
		return nil
	}

	return &restfile.Request{
		Method: method,
		URL:    url,
		LineRange: restfile.LineRange{
			Start: lineNumber,
			End:   lineNumber,
		},
		OriginalText: raw,
	}
}

func isInlineHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func looksLikeHTTPRequestURL(url string) bool {
	if url == "" {
		return false
	}
	lower := strings.ToLower(url)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
