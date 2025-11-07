package parser

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/parser/graphqlbuilder"
	"github.com/unkn0wn-root/resterm/internal/parser/grpcbuilder"
	"github.com/unkn0wn-root/resterm/internal/parser/httpbuilder"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

var (
	variableLineRe = regexp.MustCompile(`^@(?:(global)\s+)?([A-Za-z0-9_.-]+)(?:\s*(?::|=)\s*(.+?)|\s+(\S.*))$`)
	nameValueRe    = regexp.MustCompile(`^([A-Za-z0-9_.-]+)(?:\s*(?::|=)\s*(.*?)|\s+(\S.*))?$`)
)

func Parse(path string, data []byte) *restfile.Document {
	scanner := bufio.NewScanner(bytes.NewReader(normalizeNewlines(data)))
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

	doc := &restfile.Document{Path: path, Raw: data}
	builder := newDocumentBuilder(doc)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		builder.processLine(lineNumber, line)
	}

	builder.finish()

	return doc
}

func normalizeNewlines(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

type documentBuilder struct {
	doc        *restfile.Document
	inRequest  bool
	request    *requestBuilder
	fileVars   []restfile.Variable
	globalVars []restfile.Variable
	consts     []restfile.Constant
	inBlock    bool
	workflow   *workflowBuilder
}

type requestBuilder struct {
	startLine         int
	endLine           int
	metadata          restfile.RequestMetadata
	variables         []restfile.Variable
	originalLines     []string
	currentScriptKind string
	scriptBufferKind  string
	scriptBuffer      []string
	settings          map[string]string
	http              *httpbuilder.Builder
	graphql           *graphqlbuilder.Builder
	grpc              *grpcbuilder.Builder
	sse               *sseBuilder
	websocket         *wsBuilder
	bodyOptions       restfile.BodyOptions
}

type workflowBuilder struct {
	startLine int
	endLine   int
	workflow  restfile.Workflow
}

func newDocumentBuilder(doc *restfile.Document) *documentBuilder {
	return &documentBuilder{doc: doc}
}

func (b *documentBuilder) addError(line int, message string) {
	if b == nil || b.doc == nil {
		return
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return
	}
	b.doc.Errors = append(b.doc.Errors, restfile.ParseError{
		Line:    line,
		Message: msg,
	})
}

func (b *documentBuilder) processLine(lineNumber int, line string) {
	trimmed := strings.TrimSpace(line)

	if b.inRequest && b.request != nil && !strings.HasPrefix(trimmed, ">") {
		b.request.flushPendingScript()
	}

	if b.inBlock {
		content, closed := parseBlockCommentLine(trimmed, false)
		if content != "" {
			b.handleComment(lineNumber, content)
		}
		b.appendLine(line)
		if closed {
			b.inBlock = false
		}
		return
	}

	if isBlockCommentStart(trimmed) {
		content, closed := parseBlockCommentLine(trimmed, true)
		if content != "" {
			b.handleComment(lineNumber, content)
		}
		b.appendLine(line)
		if !closed {
			b.inBlock = true
		}
		return
	}

	if strings.HasPrefix(trimmed, "###") {
		if b.workflow != nil {
			b.flushWorkflow(lineNumber - 1)
		}
		b.flushRequest(lineNumber - 1)
		return
	}

	if commentText, ok := stripComment(trimmed); ok {
		b.handleComment(lineNumber, commentText)
		b.appendLine(line)
		return
	}

	if strings.HasPrefix(trimmed, ">") {
		b.handleScript(lineNumber, line)
		b.appendLine(line)
		return
	}

	if matches := variableLineRe.FindStringSubmatch(trimmed); matches != nil {
		scopeToken := strings.ToLower(strings.TrimSpace(matches[1]))
		name := matches[2]
		valueCandidate := matches[3]
		if valueCandidate == "" {
			valueCandidate = matches[4]
		}
		value := strings.TrimSpace(valueCandidate)
		variable := restfile.Variable{
			Name:  name,
			Value: value,
			Line:  lineNumber,
		}
		if scopeToken == "global" {
			variable.Scope = restfile.ScopeGlobal
			b.globalVars = append(b.globalVars, variable)
			b.appendLine(line)
			return
		}
		if b.inRequest && !b.request.http.HasMethod() {
			variable.Scope = restfile.ScopeRequest
			b.request.variables = append(b.request.variables, variable)
		} else if !b.inRequest {
			variable.Scope = restfile.ScopeFile
			b.fileVars = append(b.fileVars, variable)
		} else {
			variable.Scope = restfile.ScopeRequest
			b.request.variables = append(b.request.variables, variable)
		}
		b.appendLine(line)
		return
	}

	if trimmed == "" {
		if b.inRequest {
			if !b.request.http.HasMethod() {
			} else if !b.request.http.HeaderDone() {
				b.request.http.MarkHeadersDone()
			} else if b.request.graphql.HandleBodyLine(line) {
			} else if b.request.grpc.HandleBodyLine(line) {
			} else {
				b.request.http.AppendBodyLine("")
			}
			b.appendLine(line)
		}
		return
	}

	if b.inRequest && b.request.http.HasMethod() && b.request.http.HeaderDone() {
		b.handleBodyLine(line)
		b.appendLine(line)
		return
	}

	if grpcbuilder.IsMethodLine(line) {
		if !b.ensureRequest(lineNumber) {
			return
		}
		fields := strings.Fields(line)
		target := ""
		if len(fields) > 1 {
			target = strings.Join(fields[1:], " ")
		}

		b.request.http.SetMethodAndURL(strings.ToUpper(fields[0]), target)
		b.request.grpc.SetTarget(target)
		b.appendLine(line)
		return
	}

	if method, url, ok := httpbuilder.ParseMethodLine(line); ok {
		if !b.ensureRequest(lineNumber) {
			return
		}

		b.request.http.SetMethodAndURL(method, url)
		b.appendLine(line)
		return
	}

	if url, ok := httpbuilder.ParseWebSocketURLLine(line); ok {
		if !b.ensureRequest(lineNumber) {
			return
		}

		b.request.http.SetMethodAndURL(http.MethodGet, url)
		b.appendLine(line)
		return
	}

	if b.inRequest && b.request.http.HasMethod() && !b.request.http.HeaderDone() {
		if idx := strings.Index(line, ":"); idx != -1 {
			headerName := strings.TrimSpace(line[:idx])
			headerValue := strings.TrimSpace(line[idx+1:])
			if headerName != "" {
				b.request.http.AddHeader(headerName, headerValue)
			}
		}
		b.appendLine(line)
		return
	}

	if b.ensureRequest(lineNumber) && !b.request.http.HasMethod() {
		if b.request.metadata.Description != "" {
			b.request.metadata.Description += "\n"
		}

		b.request.metadata.Description += trimmed
		b.appendLine(line)
		return
	}

	b.appendLine(line)
}

func stripComment(trimmed string) (string, bool) {
	switch {
	case strings.HasPrefix(trimmed, "//"):
		return strings.TrimSpace(trimmed[2:]), true
	case strings.HasPrefix(trimmed, "#"):
		return strings.TrimSpace(trimmed[1:]), true
	case strings.HasPrefix(trimmed, "--"):
		return strings.TrimSpace(trimmed[2:]), true
	default:
		return "", false
	}
}

func isBlockCommentStart(trimmed string) bool {
	return strings.HasPrefix(trimmed, "/*")
}

func parseBlockCommentLine(trimmed string, start bool) (string, bool) {
	working := trimmed
	if start && strings.HasPrefix(working, "/*") {
		working = working[2:]
	}

	closed := false
	if idx := strings.Index(working, "*/"); idx >= 0 {
		closed = true
		working = working[:idx]
	}

	working = strings.TrimSpace(working)
	for strings.HasPrefix(working, "*") {
		working = strings.TrimSpace(strings.TrimPrefix(working, "*"))
	}
	return working, closed
}

func (b *documentBuilder) handleComment(line int, text string) {
	if !strings.HasPrefix(text, "@") {
		return
	}

	directive := strings.TrimSpace(text[1:])
	if directive == "" {
		return
	}

	key, rest := splitDirective(directive)
	if key == "" {
		return
	}

	if key == "workflow" {
		b.startWorkflow(line, rest)
		return
	}
	if key == "step" {
		if b.workflow != nil {
			b.workflow.addStep(line, rest)
		}
		return
	}
	if b.workflow != nil && !b.inRequest {
		if b.workflow.handleDirective(key, rest, line) {
			return
		}
	}

	if b.handleScopedVariableDirective(key, rest, line) {
		return
	}

	if key == "const" {
		if name, value := parseNameValue(rest); name != "" {
			b.addConstant(name, value, line)
		}
		return
	}

	if !b.ensureRequest(line) {
		return
	}

	if b.request.grpc.HandleDirective(key, rest) {
		return
	}
	if b.request.websocket.HandleDirective(key, rest) {
		return
	}
	if b.request.sse.HandleDirective(key, rest) {
		return
	}
	if b.request.graphql.HandleDirective(key, rest) {
		return
	}
	if key == "body" {
		if b.request != nil && b.request.handleBodyDirective(rest) {
			return
		}
	}
	switch key {
	case "name":
		if rest != "" {
			value := trimQuotes(strings.TrimSpace(rest))
			b.request.metadata.Name = value
		}
	case "description", "desc":
		if b.request.metadata.Description != "" {
			b.request.metadata.Description += "\n"
		}
		b.request.metadata.Description += rest
	case "tag", "tags":
		tags := strings.Fields(rest)
		if len(tags) == 0 {
			tags = strings.Split(rest, ",")
		}
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if !contains(b.request.metadata.Tags, tag) {
				b.request.metadata.Tags = append(b.request.metadata.Tags, tag)
			}
		}
	case "no-log", "nolog":
		b.request.metadata.NoLog = true
	case "log-sensitive-headers", "log-secret-headers":
		if rest == "" {
			b.request.metadata.AllowSensitiveHeaders = true
			return
		}
		if value, ok := parseBool(rest); ok {
			b.request.metadata.AllowSensitiveHeaders = value
		}
	case "auth":
		spec := parseAuthSpec(rest)
		if spec != nil {
			b.request.metadata.Auth = spec
		}
	case "setting":
		key, value := splitDirective(rest)
		if key != "" {
			if b.request.settings == nil {
				b.request.settings = make(map[string]string)
			}
			b.request.settings[key] = value
		}
	case "timeout":
		if b.request.settings == nil {
			b.request.settings = make(map[string]string)
		}
		b.request.settings["timeout"] = rest
	case "var":
		name, value := parseNameValue(rest)
		if name == "" {
			return
		}
		variable := restfile.Variable{
			Name:   name,
			Value:  value,
			Line:   line,
			Scope:  restfile.ScopeRequest,
			Secret: false,
		}
		b.request.variables = append(b.request.variables, variable)
	case "script":
		if rest != "" {
			b.request.currentScriptKind = strings.ToLower(rest)
		}
	case "capture":
		if capture, ok := b.parseCaptureDirective(rest, line); ok {
			b.request.metadata.Captures = append(b.request.metadata.Captures, capture)
		}
	case "profile":
		if spec := parseProfileSpec(rest); spec != nil {
			b.request.metadata.Profile = spec
		}
	case "trace":
		if spec := parseTraceSpec(rest); spec != nil {
			b.request.metadata.Trace = spec
		}
	case "compare":
		if !b.ensureRequest(line) {
			return
		}
		if b.request.metadata.Compare != nil {
			b.addError(line, "@compare directive already defined for this request")
			return
		}
		spec, err := parseCompareDirective(rest)
		if err != nil {
			b.addError(line, err.Error())
			return
		}
		b.request.metadata.Compare = spec
	}
}

func (b *documentBuilder) parseCaptureDirective(rest string, line int) (restfile.CaptureSpec, bool) {
	scopeToken, remainder := splitDirective(rest)
	if scopeToken == "" {
		return restfile.CaptureSpec{}, false
	}
	scope, secret, ok := parseCaptureScope(scopeToken)
	if !ok {
		return restfile.CaptureSpec{}, false
	}
	trimmed := strings.TrimSpace(remainder)
	if trimmed == "" {
		return restfile.CaptureSpec{}, false
	}
	nameEnd := strings.IndexAny(trimmed, " \t")
	if nameEnd == -1 {
		return restfile.CaptureSpec{}, false
	}
	name := strings.TrimSpace(trimmed[:nameEnd])
	expression := strings.TrimSpace(trimmed[nameEnd:])
	if expression == "" {
		return restfile.CaptureSpec{}, false
	}
	if strings.HasPrefix(expression, "=") {
		expression = strings.TrimSpace(expression[1:])
	}
	if expression == "" {
		return restfile.CaptureSpec{}, false
	}
	return restfile.CaptureSpec{
		Scope:      scope,
		Name:       name,
		Expression: expression,
		Secret:     secret,
	}, true
}

func parseCaptureScope(token string) (restfile.CaptureScope, bool, bool) {
	lowered := strings.ToLower(strings.TrimSpace(token))
	secret := false
	if strings.HasSuffix(lowered, "-secret") {
		secret = true
		lowered = strings.TrimSuffix(lowered, "-secret")
	}
	switch lowered {
	case "request":
		return restfile.CaptureScopeRequest, secret, true
	case "file":
		return restfile.CaptureScopeFile, secret, true
	case "global":
		return restfile.CaptureScopeGlobal, secret, true
	default:
		return 0, false, false
	}
}

func (b *documentBuilder) handleScript(line int, rawLine string) {
	if !b.ensureRequest(line) {
		return
	}

	stripped := strings.TrimLeft(rawLine, " \t")
	if !strings.HasPrefix(stripped, ">") {
		return
	}
	body := strings.TrimPrefix(stripped, ">")
	if len(body) > 0 {
		if body[0] == ' ' || body[0] == '\t' {
			body = body[1:]
		}
	}
	body = strings.TrimRight(body, " \t")
	kind := b.request.currentScriptKind
	if kind == "" {
		kind = "test"
	}
	trimmedHead := strings.TrimLeft(body, " \t")
	if strings.HasPrefix(trimmedHead, "<") {
		path := strings.TrimSpace(strings.TrimPrefix(trimmedHead, "<"))
		if path != "" {
			b.request.appendScriptInclude(kind, path)
		}
		return
	}
	b.request.appendScriptLine(kind, body)
}

func parseAuthSpec(rest string) *restfile.AuthSpec {
	fields := splitAuthFields(rest)
	if len(fields) == 0 {
		return nil
	}
	authType := strings.ToLower(fields[0])
	params := make(map[string]string)
	switch authType {
	case "basic":
		if len(fields) >= 3 {
			params["username"] = fields[1]
			params["password"] = strings.Join(fields[2:], " ")
		}
	case "bearer":
		if len(fields) >= 2 {
			params["token"] = strings.Join(fields[1:], " ")
		}
	case "apikey", "api-key":
		if len(fields) >= 4 {
			params["placement"] = strings.ToLower(fields[1])
			params["name"] = fields[2]
			params["value"] = strings.Join(fields[3:], " ")
		}
	case "oauth2":
		if len(fields) < 2 {
			return nil
		}
		for key, value := range parseKeyValuePairs(fields[1:]) {
			params[key] = value
		}
		if params["token_url"] == "" {
			return nil
		}
		if params["grant"] == "" {
			params["grant"] = "client_credentials"
		}
		if params["client_auth"] == "" {
			params["client_auth"] = "basic"
		}
	default:
		if len(fields) >= 2 {
			params["header"] = fields[0]
			params["value"] = strings.Join(fields[1:], " ")
			authType = "header"
		}
	}
	if len(params) == 0 {
		return nil
	}
	return &restfile.AuthSpec{Type: authType, Params: params}
}

func parseProfileSpec(rest string) *restfile.ProfileSpec {
	trimmed := strings.TrimSpace(rest)
	spec := &restfile.ProfileSpec{}

	if trimmed == "" {
		spec.Count = 10
		return spec
	}

	fields := splitAuthFields(trimmed)
	params := parseKeyValuePairs(fields)

	if spec.Count == 0 {
		if raw, ok := params["count"]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n > 0 {
				spec.Count = n
			}
		}
	}

	if spec.Count == 0 && len(fields) == 1 && !strings.Contains(fields[0], "=") {
		if n, err := strconv.Atoi(fields[0]); err == nil && n > 0 {
			spec.Count = n
		}
	}

	if raw, ok := params["warmup"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && n >= 0 {
			spec.Warmup = n
		}
	}

	if raw, ok := params["delay"]; ok {
		if dur, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && dur >= 0 {
			spec.Delay = dur
		}
	}

	if spec.Count <= 0 {
		spec.Count = 10
	}
	if spec.Warmup < 0 {
		spec.Warmup = 0
	}
	return spec
}

func parseTraceSpec(rest string) *restfile.TraceSpec {
	spec := &restfile.TraceSpec{Enabled: true}
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return spec
	}

	fields := splitAuthFields(trimmed)
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		switch lower {
		case "off", "disable", "disabled", "false":
			spec.Enabled = false
			continue
		case "on", "enable", "enabled", "true":
			spec.Enabled = true
			continue
		}

		if parts := strings.SplitN(value, "<=", 2); len(parts) == 2 {
			name := normalizeTracePhaseName(parts[0])
			dur := parseDuration(parts[1])
			if dur <= 0 {
				continue
			}
			if name == "total" {
				spec.Budgets.Total = dur
				continue
			}
			if spec.Budgets.Phases == nil {
				spec.Budgets.Phases = make(map[string]time.Duration)
			}
			spec.Budgets.Phases[name] = dur
			continue
		}

		if idx := strings.Index(value, "="); idx != -1 {
			key := strings.ToLower(strings.TrimSpace(value[:idx]))
			val := strings.TrimSpace(value[idx+1:])
			switch key {
			case "enabled":
				if b, ok := parseBool(val); ok {
					spec.Enabled = b
				}
			case "total":
				if dur := parseDuration(val); dur > 0 {
					spec.Budgets.Total = dur
				}
			case "tolerance", "allowance", "grace":
				if dur := parseDuration(val); dur >= 0 {
					spec.Budgets.Tolerance = dur
				}
			default:
				dur := parseDuration(val)
				if dur <= 0 {
					continue
				}
				name := normalizeTracePhaseName(key)
				if name == "total" {
					spec.Budgets.Total = dur
					continue
				}
				if spec.Budgets.Phases == nil {
					spec.Budgets.Phases = make(map[string]time.Duration)
				}
				spec.Budgets.Phases[name] = dur
			}
		}
	}

	if len(spec.Budgets.Phases) == 0 {
		spec.Budgets.Phases = nil
	}
	return spec
}

func parseCompareDirective(rest string) (*restfile.CompareSpec, error) {
	fields := splitAuthFields(rest)
	envs := make([]string, 0, len(fields))
	seen := make(map[string]struct{})
	var baseline string

	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		if idx := strings.Index(value, "="); idx != -1 {
			key := strings.ToLower(strings.TrimSpace(value[:idx]))
			val := strings.TrimSpace(value[idx+1:])
			switch key {
			case "base", "baseline", "primary", "ref":
				if val == "" {
					return nil, fmt.Errorf("@compare baseline cannot be empty")
				}
				baseline = val
			default:
				return nil, fmt.Errorf("@compare unsupported option %q", key)
			}
			continue
		}
		lowered := strings.ToLower(value)
		if _, exists := seen[lowered]; exists {
			return nil, fmt.Errorf("@compare duplicate environment %q", value)
		}
		seen[lowered] = struct{}{}
		envs = append(envs, value)
	}

	if len(envs) < 2 {
		return nil, fmt.Errorf("@compare requires at least two environments")
	}

	if baseline == "" {
		baseline = envs[0]
	} else {
		match := ""
		for _, env := range envs {
			if strings.EqualFold(env, baseline) {
				match = env
				break
			}
		}
		if match == "" {
			return nil, fmt.Errorf("@compare baseline %q must match one of the environments", baseline)
		}
		baseline = match
	}

	return &restfile.CompareSpec{
		Environments: envs,
		Baseline:     baseline,
	}, nil
}

func parseDuration(value string) time.Duration {
	dur, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return dur
}

func normalizeTracePhaseName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dns", "lookup", "name":
		return "dns"
	case "connect", "dial":
		return "connect"
	case "tls", "handshake":
		return "tls"
	case "headers", "request_headers", "req_headers", "header":
		return "request_headers"
	case "body", "request_body", "req_body":
		return "request_body"
	case "ttfb", "first_byte", "wait":
		return "ttfb"
	case "transfer", "download":
		return "transfer"
	case "total", "overall":
		return "total"
	default:
		return strings.ToLower(strings.TrimSpace(name))
	}
}

// Splits on spaces but keeps quoted strings together.
// Quotes themselves get stripped - "hello resterm" becomes a single field: hello resterm
func splitAuthFields(input string) []string {
	var fields []string
	var current strings.Builder
	inQuote := false
	var quoteRune rune

	flush := func() {
		if current.Len() > 0 {
			fields = append(fields, current.String())
			current.Reset()
		}
	}

	for _, r := range input {
		switch {
		case inQuote:
			if r == quoteRune {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
		case unicode.IsSpace(r):
			flush()
		case r == '"' || r == '\'':
			inQuote = true
			quoteRune = r
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return fields
}

func parseKeyValuePairs(fields []string) map[string]string {
	params := make(map[string]string, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if idx := strings.Index(field, "="); idx != -1 {
			key := strings.ToLower(strings.TrimSpace(field[:idx]))
			value := strings.TrimSpace(field[idx+1:])
			key = strings.ReplaceAll(key, "-", "_")
			params[key] = value
		}
	}
	return params
}

func parseBool(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "t", "1", "yes", "on":
		return true, true
	case "false", "f", "0", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func (b *documentBuilder) handleScopedVariableDirective(key, rest string, line int) bool {
	switch key {
	case "global", "global-secret":
		name, value := parseNameValue(rest)
		if name == "" {
			return true
		}
		b.addGlobalVariable(name, value, line, strings.HasSuffix(key, "-secret"))
		return true
	case "var":
		scopeToken, remainder := splitFirst(rest)
		if scopeToken == "" {
			return false
		}
		scope := strings.ToLower(scopeToken)
		secret := false
		if strings.HasSuffix(scope, "-secret") {
			secret = true
			scope = strings.TrimSuffix(scope, "-secret")
		}
		name, value := parseNameValue(remainder)
		if name == "" {
			return true
		}
		switch scope {
		case "global":
			b.addGlobalVariable(name, value, line, secret)
			return true
		case "file":
			variable := restfile.Variable{Name: name, Value: value, Line: line, Scope: restfile.ScopeFile, Secret: secret}
			b.fileVars = append(b.fileVars, variable)
			return true
		case "request":
			if !b.ensureRequest(line) {
				return true
			}
			variable := restfile.Variable{Name: name, Value: value, Line: line, Scope: restfile.ScopeRequest, Secret: secret}
			b.request.variables = append(b.request.variables, variable)
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func (b *documentBuilder) addGlobalVariable(name, value string, line int, secret bool) {
	variable := restfile.Variable{
		Name:   name,
		Value:  value,
		Line:   line,
		Scope:  restfile.ScopeGlobal,
		Secret: secret,
	}
	b.globalVars = append(b.globalVars, variable)
}

func (b *documentBuilder) addConstant(name, value string, line int) {
	constant := restfile.Constant{
		Name:  name,
		Value: value,
		Line:  line,
	}
	b.consts = append(b.consts, constant)
}

func splitFirst(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", ""
	}
	token := fields[0]
	remainder := strings.TrimSpace(trimmed[len(token):])
	return token, remainder
}

func parseNameValue(input string) (string, string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", ""
	}
	matches := nameValueRe.FindStringSubmatch(trimmed)
	if matches == nil {
		return "", ""
	}
	name := matches[1]
	valueCandidate := matches[2]
	if valueCandidate == "" {
		valueCandidate = matches[3]
	}
	return name, strings.TrimSpace(valueCandidate)
}

func splitDirective(text string) (string, string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}

	key := strings.ToLower(strings.TrimRight(fields[0], ":"))
	var rest string
	if len(text) > len(fields[0]) {
		rest = strings.TrimSpace(text[len(fields[0]):])
	}
	return key, rest
}

func parseOptionTokens(input string) map[string]string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return map[string]string{}
	}
	tokens := tokenizeOptionTokens(trimmed)
	if len(tokens) == 0 {
		return map[string]string{}
	}
	options := make(map[string]string, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		key := token
		value := "true"
		if idx := strings.Index(token, "="); idx >= 0 {
			key = strings.TrimSpace(token[:idx])
			value = strings.TrimSpace(token[idx+1:])
		}
		if key == "" {
			continue
		}
		options[strings.ToLower(key)] = trimQuotes(value)
	}
	return options
}

// Like splitAuthFields but handles backslash escapes.
// A trailing backslash gets preserved if nothing follows it.
func tokenizeOptionTokens(input string) []string {
	var tokens []string
	var current strings.Builder
	var quote rune
	escaping := false

	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}

	for _, r := range input {
		switch {
		case escaping:
			current.WriteRune(r)
			escaping = false
		case r == '\\':
			escaping = true
		case quote != 0:
			if r == quote {
				quote = 0
				break
			}
			current.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			current.WriteRune(r)
		}
	}
	if escaping {
		current.WriteRune('\\')
	}
	flush()
	return tokens
}

func trimQuotes(value string) string {
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func parseWorkflowFailureMode(value string) (restfile.WorkflowFailureMode, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return "", false
	}
	switch trimmed {
	case "stop", "fail", "abort":
		return restfile.WorkflowOnFailureStop, true
	case "continue", "skip":
		return restfile.WorkflowOnFailureContinue, true
	default:
		return "", false
	}
}

func parseTagList(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || r == ','
	})
	var tags []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func (r *requestBuilder) appendScriptLine(kind, body string) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "test"
	}

	if r.scriptBufferKind != "" && !strings.EqualFold(r.scriptBufferKind, kind) {
		r.flushPendingScript()
	}
	if r.scriptBufferKind == "" {
		r.scriptBufferKind = kind
	}
	r.scriptBuffer = append(r.scriptBuffer, body)
}

func (r *requestBuilder) flushPendingScript() {
	if len(r.scriptBuffer) == 0 {
		return
	}
	script := strings.Join(r.scriptBuffer, "\n")
	r.metadata.Scripts = append(r.metadata.Scripts, restfile.ScriptBlock{Kind: r.scriptBufferKind, Body: script})
	r.scriptBuffer = nil
	r.scriptBufferKind = ""
}

func (r *requestBuilder) appendScriptInclude(kind, path string) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		kind = "test"
	}
	r.flushPendingScript()
	r.metadata.Scripts = append(r.metadata.Scripts, restfile.ScriptBlock{Kind: kind, FilePath: path})
}

func (r *requestBuilder) handleBodyDirective(rest string) bool {
	value := strings.TrimSpace(rest)
	if value == "" {
		return false
	}
	key, val := splitDirective(value)
	if key == "" {
		key = value
	}
	switch strings.ToLower(key) {
	case "expand", "expand-templates":
		enabled := true
		if strings.TrimSpace(val) != "" {
			if parsed, ok := parseBool(val); ok {
				enabled = parsed
			}
		}
		r.bodyOptions.ExpandTemplates = enabled
		return true
	default:
		return false
	}
}

func (b *documentBuilder) handleBodyLine(line string) {
	if b.request.graphql.HandleBodyLine(line) {
		return
	}
	if b.request.grpc.HandleBodyLine(line) {
		return
	}

	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "<") {
		b.request.http.SetBodyFromFile(strings.TrimSpace(strings.TrimPrefix(trimmed, "<")))
		return
	}
	if strings.HasPrefix(trimmed, "@") && strings.Contains(trimmed, "<") {
		parts := strings.SplitN(trimmed, "<", 2)
		if len(parts) == 2 {
			b.request.http.SetBodyFromFile(strings.TrimSpace(parts[1]))
			return
		}
	}
	b.request.http.AppendBodyLine(line)
}

func (b *documentBuilder) ensureRequest(line int) bool {
	if b.inRequest {
		return true
	}

	if b.workflow != nil {
		b.flushWorkflow(line - 1)
	}

	b.inRequest = true
	b.request = &requestBuilder{
		startLine:         line,
		metadata:          restfile.RequestMetadata{Tags: []string{}},
		currentScriptKind: "test",
		http:              httpbuilder.New(),
		graphql:           graphqlbuilder.New(),
		grpc:              grpcbuilder.New(),
		sse:               newSSEBuilder(),
		websocket:         newWebSocketBuilder(),
	}
	return true
}

func (b *documentBuilder) appendLine(line string) {
	if b.inRequest {
		if b.request.startLine == 0 {
			b.request.startLine = 1
		}
		b.request.originalLines = append(b.request.originalLines, line)
		b.request.endLine++
	}
}

func (b *documentBuilder) flushRequest(_ int) {
	if !b.inRequest {
		return
	}

	b.request.flushPendingScript()

	req := b.request.build()
	if req.Method != "" && req.URL != "" {
		b.doc.Requests = append(b.doc.Requests, req)
	}

	b.inRequest = false
	b.request = nil
	b.inBlock = false
}

func (b *documentBuilder) flushWorkflow(line int) {
	if b.workflow == nil {
		return
	}
	scene := b.workflow.build(line)
	if len(scene.Steps) > 0 {
		b.doc.Workflows = append(b.doc.Workflows, scene)
	}
	b.workflow = nil
}

func (b *documentBuilder) finish() {
	b.flushRequest(0)
	b.flushWorkflow(0)
	b.doc.Variables = append(b.doc.Variables, b.fileVars...)
	b.doc.Globals = append(b.doc.Globals, b.globalVars...)
	b.doc.Constants = append(b.doc.Constants, b.consts...)
}

func (r *requestBuilder) build() *restfile.Request {
	r.flushPendingScript()

	req := &restfile.Request{
		Metadata:     r.metadata,
		Method:       r.http.Method(),
		URL:          strings.TrimSpace(r.http.URL()),
		Headers:      r.http.HeaderMap(),
		Body:         restfile.BodySource{},
		Variables:    r.variables,
		Settings:     map[string]string{},
		LineRange:    restfile.LineRange{Start: r.startLine, End: r.startLine + len(r.originalLines) - 1},
		OriginalText: strings.Join(r.originalLines, "\n"),
	}

	if wsReq, ok := r.websocket.Finalize(); ok {
		req.WebSocket = wsReq
	}
	if sseReq, ok := r.sse.Finalize(); ok {
		req.SSE = sseReq
	}

	if req.WebSocket == nil && req.SSE == nil {
		if grpcReq, body, mime, ok := r.grpc.Finalize(r.http.MimeType()); ok {
			req.GRPC = grpcReq
			req.Body = body
			if mime != "" {
				req.Body.MimeType = mime
			}
			if r.settings != nil {
				req.Settings = r.settings
			}
			return req
		} else if gql, mime, ok := r.graphql.Finalize(r.http.MimeType()); ok {
			req.Body.GraphQL = gql
			if mime != "" {
				req.Body.MimeType = mime
			}
		} else {
			if file := r.http.BodyFromFile(); file != "" {
				req.Body.FilePath = file
			} else if text := r.http.BodyText(); text != "" {
				req.Body.Text = text
			}
			if mime := r.http.MimeType(); mime != "" {
				req.Body.MimeType = mime
			}
			req.Body.Options = r.bodyOptions
		}
	}

	if file := r.http.BodyFromFile(); file != "" {
		req.Body.FilePath = file
	} else if text := r.http.BodyText(); text != "" {
		req.Body.Text = text
	}
	if mime := r.http.MimeType(); mime != "" {
		req.Body.MimeType = mime
	}

	if r.settings != nil {
		req.Settings = r.settings
	}

	return req
}

func (b *documentBuilder) startWorkflow(line int, rest string) {
	if b.inRequest {
		b.flushRequest(line - 1)
	}
	nameToken, remainder := splitFirst(rest)
	if nameToken == "" || strings.Contains(nameToken, "=") {
		return
	}
	b.flushWorkflow(line - 1)
	sb := newWorkflowBuilder(line, nameToken)
	sb.applyOptions(parseOptionTokens(remainder))
	sb.touch(line)
	b.workflow = sb
}

func newWorkflowBuilder(line int, name string) *workflowBuilder {
	return &workflowBuilder{
		startLine: line,
		endLine:   line,
		workflow: restfile.Workflow{
			Name:             strings.TrimSpace(name),
			Tags:             []string{},
			DefaultOnFailure: restfile.WorkflowOnFailureStop,
		},
	}
}

func (s *workflowBuilder) touch(line int) {
	if line > s.endLine {
		s.endLine = line
	}
}

func (s *workflowBuilder) applyOptions(opts map[string]string) {
	if len(opts) == 0 {
		return
	}
	leftovers := make(map[string]string)
	for key, value := range opts {
		switch key {
		case "on-failure", "onfailure":
			if mode, ok := parseWorkflowFailureMode(value); ok {
				s.workflow.DefaultOnFailure = mode
			}
		default:
			leftovers[key] = value
		}
	}
	if len(leftovers) > 0 {
		if s.workflow.Options == nil {
			s.workflow.Options = make(map[string]string, len(leftovers))
		}
		for key, value := range leftovers {
			s.workflow.Options[key] = value
		}
	}
}

func (s *workflowBuilder) handleDirective(key, rest string, line int) bool {
	switch key {
	case "description", "desc":
		if rest == "" {
			return true
		}
		if s.workflow.Description != "" {
			s.workflow.Description += "\n"
		}
		s.workflow.Description += rest
		s.touch(line)
		return true
	case "tag", "tags":
		tags := parseTagList(rest)
		if len(tags) == 0 {
			return true
		}
		for _, tag := range tags {
			if !contains(s.workflow.Tags, tag) {
				s.workflow.Tags = append(s.workflow.Tags, tag)
			}
		}
		s.touch(line)
		return true
	default:
		return false
	}
}

func (s *workflowBuilder) addStep(line int, rest string) {
	remainder := strings.TrimSpace(rest)
	if remainder == "" {
		return
	}
	name := ""
	firstToken, remainderAfterFirst := splitFirst(remainder)
	if firstToken != "" && !strings.Contains(firstToken, "=") {
		name = firstToken
		remainder = remainderAfterFirst
	}
	options := parseOptionTokens(remainder)
	if explicitName, ok := options["name"]; ok {
		if name == "" {
			name = explicitName
		}
		delete(options, "name")
	}
	using := options["using"]
	if using == "" {
		return
	}
	delete(options, "using")
	step := restfile.WorkflowStep{
		Name:      name,
		Using:     strings.TrimSpace(using),
		OnFailure: s.workflow.DefaultOnFailure,
		Line:      line,
	}
	if mode, ok := options["on-failure"]; ok {
		if parsed, ok := parseWorkflowFailureMode(mode); ok {
			step.OnFailure = parsed
		}
		delete(options, "on-failure")
	}
	if len(options) > 0 {
		leftover := make(map[string]string)
		for key, value := range options {
			switch {
			case strings.HasPrefix(key, "expect."):
				suffix := strings.TrimPrefix(key, "expect.")
				if suffix == "" {
					continue
				}
				if step.Expect == nil {
					step.Expect = make(map[string]string)
				}
				step.Expect[suffix] = value
			case strings.HasPrefix(key, "vars."):
				sanitized := strings.TrimSpace(key)
				if sanitized == "" {
					continue
				}
				if step.Vars == nil {
					step.Vars = make(map[string]string)
				}
				step.Vars[sanitized] = value
			default:
				leftover[key] = value
			}
		}
		if len(leftover) > 0 {
			step.Options = leftover
		}
	}
	s.workflow.Steps = append(s.workflow.Steps, step)
	s.touch(line)
}

func (s *workflowBuilder) build(line int) restfile.Workflow {
	if line > 0 {
		s.touch(line)
	}
	s.workflow.LineRange = restfile.LineRange{Start: s.startLine, End: s.endLine}
	if s.workflow.LineRange.End < s.workflow.LineRange.Start {
		s.workflow.LineRange.End = s.workflow.LineRange.Start
	}
	return s.workflow
}
