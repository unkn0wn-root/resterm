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
	variableLineRe = regexp.MustCompile(
		`^@(?:(global(?:-secret)?|file(?:-secret)?|request(?:-secret)?)\s+)?([A-Za-z0-9_.-]+)(?:\s*(?::|=)\s*(.+?)|\s+(\S.*))$`,
	)
	nameValueRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+)(?:\s*(?::|=)\s*(.*?)|\s+(\S.*))?$`)
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
	doc          *restfile.Document
	inRequest    bool
	request      *requestBuilder
	fileVars     []restfile.Variable
	globalVars   []restfile.Variable
	fileSettings map[string]string
	consts       []restfile.Constant
	sshDefs      []restfile.SSHProfile
	fileUses     []restfile.UseSpec
	inBlock      bool
	workflow     *workflowBuilder
}

type requestBuilder struct {
	startLine         int
	endLine           int
	metadata          restfile.RequestMetadata
	variables         []restfile.Variable
	originalLines     []string
	currentScriptKind string
	currentScriptLang string
	scriptBufferKind  string
	scriptBufferLang  string
	scriptBuffer      []string
	settings          map[string]string
	http              *httpbuilder.Builder
	graphql           *graphqlbuilder.Builder
	grpc              *grpcbuilder.Builder
	sse               *sseBuilder
	websocket         *wsBuilder
	bodyOptions       restfile.BodyOptions
	ssh               *restfile.SSHSpec
}

type workflowBuilder struct {
	startLine      int
	endLine        int
	workflow       restfile.Workflow
	pendingWhen    *restfile.ConditionSpec
	pendingForEach *restfile.ForEachSpec
	openSwitch     *workflowSwitchBuilder
	openIf         *workflowIfBuilder
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
		b.flushFileSettings()
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
		scopeToken, secret := parseScopeToken(matches[1])
		name := matches[2]
		valueCandidate := matches[3]
		if valueCandidate == "" {
			valueCandidate = matches[4]
		}
		value := strings.TrimSpace(valueCandidate)
		switch scopeToken {
		case "global":
			b.addScopedVariable(name, value, lineNumber, restfile.ScopeGlobal, secret)
		case "request":
			if !b.addScopedVariable(name, value, lineNumber, restfile.ScopeRequest, secret) {
				return
			}
		case "file":
			b.addScopedVariable(name, value, lineNumber, restfile.ScopeFile, secret)
		default:
			scope := restfile.ScopeRequest
			if !b.inRequest {
				scope = restfile.ScopeFile
			}
			if !b.addScopedVariable(name, value, lineNumber, scope, secret) {
				return
			}
		}
		b.appendLine(line)
		return
	}

	if trimmed == "" {
		if b.inRequest {
			if !b.request.http.HasMethod() {
			} else if !b.request.http.HeaderDone() {
				b.request.markHeadersDone()
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
			if err := b.workflow.addStep(line, rest); err != "" {
				b.addError(line, err)
			}
		}
		return
	}

	if key == "use" {
		spec, err := parseUseSpec(rest, line)
		if err != nil {
			b.addError(line, err.Error())
			return
		}
		if b.inRequest && b.request != nil {
			b.request.metadata.Uses = append(b.request.metadata.Uses, spec)
		} else {
			b.fileUses = append(b.fileUses, spec)
		}
		return
	}
	if b.workflow != nil && !b.inRequest {
		if handled, errMsg := b.workflow.handleDirective(key, rest, line); handled {
			if errMsg != "" {
				b.addError(line, errMsg)
			}
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

	if key == "ssh" {
		b.handleSSH(line, rest)
		return
	}

	if key == "setting" && !b.inRequest {
		b.handleFileSetting(rest)
		return
	}
	if key == "settings" && !b.inRequest {
		b.fileSettings = applySettingsTokens(b.fileSettings, rest)
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
	case "settings":
		if b.inRequest {
			b.request.settings = applySettingsTokens(b.request.settings, rest)
		} else {
			b.fileSettings = applySettingsTokens(b.fileSettings, rest)
		}
	case "setting":
		key, value := splitDirective(rest)
		if key != "" {
			if b.inRequest {
				if b.request.settings == nil {
					b.request.settings = make(map[string]string)
				}
				b.request.settings[key] = value
			} else {
				if b.fileSettings == nil {
					b.fileSettings = make(map[string]string)
				}
				b.fileSettings[key] = value
			}
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
			kind, lang := parseScriptSpec(rest)
			b.request.currentScriptKind = kind
			b.request.currentScriptLang = lang
		}
	case "apply":
		if spec, ok := parseApplySpec(rest, line); ok {
			b.request.metadata.Applies = append(b.request.metadata.Applies, spec)
		} else {
			b.addError(line, "@apply expression missing")
		}
	case "capture":
		if capture, ok := b.parseCaptureDirective(rest, line); ok {
			b.request.metadata.Captures = append(b.request.metadata.Captures, capture)
		}
	case "assert":
		if spec, ok := b.parseAssertDirective(rest, line); ok {
			b.request.metadata.Asserts = append(b.request.metadata.Asserts, spec)
		} else {
			b.addError(line, "@assert expression missing")
		}
	case "when", "skip-if":
		negate := key == "skip-if"
		spec, err := parseConditionSpec(rest, line, negate)
		if err != nil {
			b.addError(line, err.Error())
			return
		}
		if b.request.metadata.When != nil {
			b.addError(line, "@when directive already defined for this request")
			return
		}
		b.request.metadata.When = spec
	case "for-each":
		spec, err := parseForEachSpec(rest, line)
		if err != nil {
			b.addError(line, err.Error())
			return
		}
		if b.request.metadata.ForEach != nil {
			b.addError(line, "@for-each directive already defined for this request")
			return
		}
		b.request.metadata.ForEach = spec
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

func (b *documentBuilder) parseCaptureDirective(
	rest string,
	line int,
) (restfile.CaptureSpec, bool) {
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

func (b *documentBuilder) parseAssertDirective(rest string, line int) (restfile.AssertSpec, bool) {
	expr, msg := splitAssert(rest)
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return restfile.AssertSpec{}, false
	}
	return restfile.AssertSpec{
		Expression: expr,
		Message:    msg,
		Line:       line,
	}, true
}

func parseApplySpec(rest string, line int) (restfile.ApplySpec, bool) {
	expr := strings.TrimSpace(rest)
	if strings.HasPrefix(expr, "=") {
		expr = strings.TrimSpace(strings.TrimPrefix(expr, "="))
	}
	if expr == "" {
		return restfile.ApplySpec{}, false
	}
	return restfile.ApplySpec{
		Expression: expr,
		Line:       line,
		Col:        1,
	}, true
}

func parseUseSpec(rest string, line int) (restfile.UseSpec, error) {
	fields := splitAuthFields(rest)
	if len(fields) < 3 {
		return restfile.UseSpec{}, fmt.Errorf("@use requires a path and alias")
	}
	if !strings.EqualFold(fields[1], "as") {
		return restfile.UseSpec{}, fmt.Errorf("@use must use 'as' to define an alias")
	}
	if len(fields) > 3 {
		return restfile.UseSpec{}, fmt.Errorf("@use has too many tokens")
	}
	path := strings.TrimSpace(fields[0])
	alias := strings.TrimSpace(fields[2])
	if path == "" || alias == "" {
		return restfile.UseSpec{}, fmt.Errorf("@use requires a non-empty path and alias")
	}
	if !isIdent(alias) {
		return restfile.UseSpec{}, fmt.Errorf("@use alias %q is invalid", alias)
	}
	return restfile.UseSpec{
		Path:  path,
		Alias: alias,
		Line:  line,
	}, nil
}

func parseConditionSpec(rest string, line int, negate bool) (*restfile.ConditionSpec, error) {
	expr := strings.TrimSpace(rest)
	if expr == "" {
		return nil, fmt.Errorf("@when expression missing")
	}
	return &restfile.ConditionSpec{
		Expression: expr,
		Line:       line,
		Col:        1,
		Negate:     negate,
	}, nil
}

func parseForEachSpec(rest string, line int) (*restfile.ForEachSpec, error) {
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return nil, fmt.Errorf("@for-each expression missing")
	}
	if idx := strings.LastIndex(trimmed, " as "); idx >= 0 {
		expr := strings.TrimSpace(trimmed[:idx])
		name := strings.TrimSpace(trimmed[idx+4:])
		if expr == "" || name == "" {
			return nil, fmt.Errorf("@for-each requires '<expr> as <name>'")
		}
		if !isIdent(name) {
			return nil, fmt.Errorf("@for-each name %q is invalid", name)
		}
		return &restfile.ForEachSpec{Expression: expr, Var: name, Line: line, Col: 1}, nil
	}
	if idx := strings.Index(trimmed, " in "); idx >= 0 {
		name := strings.TrimSpace(trimmed[:idx])
		expr := strings.TrimSpace(trimmed[idx+4:])
		if expr == "" || name == "" {
			return nil, fmt.Errorf("@for-each requires '<name> in <expr>'")
		}
		if !isIdent(name) {
			return nil, fmt.Errorf("@for-each name %q is invalid", name)
		}
		return &restfile.ForEachSpec{Expression: expr, Var: name, Line: line, Col: 1}, nil
	}
	return nil, fmt.Errorf("@for-each must use 'as' or 'in'")
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
	lang := b.request.currentScriptLang
	trimmedHead := strings.TrimLeft(body, " \t")
	if strings.HasPrefix(trimmedHead, "<") {
		path := strings.TrimSpace(strings.TrimPrefix(trimmedHead, "<"))
		if path != "" {
			b.request.appendScriptInclude(kind, lang, path)
		}
		return
	}
	b.request.appendScriptLine(kind, lang, body)
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
		if params["token_url"] == "" && params["cache_key"] == "" {
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
			return nil, fmt.Errorf(
				"@compare baseline %q must match one of the environments",
				baseline,
			)
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

func parseScopeToken(token string) (string, bool) {
	tok := strings.ToLower(strings.TrimSpace(token))
	if tok == "" {
		return "", false
	}
	secret := strings.HasSuffix(tok, "-secret")
	if secret {
		tok = strings.TrimSuffix(tok, "-secret")
	}
	return tok, secret
}

func isAlpha(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isIdentStartRune(r rune) bool {
	return r == '_' || isAlpha(r)
}

func isIdentRune(r rune) bool {
	return isIdentStartRune(r) || isDigit(r)
}

func isOptionKeyRune(r rune) bool {
	return r == '_' || r == '-' || r == '.' || isAlpha(r) || isDigit(r)
}

func isIdent(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if !isIdentStartRune(r) {
				return false
			}
			continue
		}
		if !isIdentRune(r) {
			return false
		}
	}
	return true
}

func (b *documentBuilder) addScopedVariable(
	name, value string,
	line int,
	scope restfile.VariableScope,
	secret bool,
) bool {
	if name == "" {
		return true
	}
	variable := restfile.Variable{
		Name:   name,
		Value:  value,
		Line:   line,
		Scope:  scope,
		Secret: secret,
	}
	switch scope {
	case restfile.ScopeGlobal:
		b.globalVars = append(b.globalVars, variable)
	case restfile.ScopeFile:
		b.fileVars = append(b.fileVars, variable)
	case restfile.ScopeRequest:
		if !b.ensureRequest(line) {
			return false
		}
		b.request.variables = append(b.request.variables, variable)
	default:
		return false
	}
	return true
}

func (b *documentBuilder) handleScopedVariableDirective(key, rest string, line int) bool {
	scopeToken := key
	args := rest
	if key == "var" {
		scopeToken, args = splitFirst(rest)
		if scopeToken == "" {
			return false
		}
	}

	scopeStr, secret := parseScopeToken(scopeToken)
	name, value := parseNameValue(args)

	switch scopeStr {
	case "global":
		return b.addScopedVariable(name, value, line, restfile.ScopeGlobal, secret)
	case "file":
		return b.addScopedVariable(name, value, line, restfile.ScopeFile, secret)
	case "request":
		return b.addScopedVariable(name, value, line, restfile.ScopeRequest, secret)
	default:
		return false
	}
}

type sshDirective struct {
	scope   restfile.SSHScope
	profile restfile.SSHProfile
	spec    *restfile.SSHSpec
}

func (b *documentBuilder) handleSSH(line int, rest string) {
	res, err := parseSSHDirective(rest)
	if err != nil {
		b.addError(line, err.Error())
		return
	}

	if res.scope == restfile.SSHScopeRequest {
		if !b.ensureRequest(line) {
			return
		}
		if b.request.ssh != nil {
			b.addError(line, "@ssh already defined for this request")
			return
		}
		b.request.ssh = res.spec
		return
	}

	if res.scope == restfile.SSHScopeGlobal || res.scope == restfile.SSHScopeFile {
		res.profile.Scope = res.scope
		b.sshDefs = append(b.sshDefs, res.profile)
	}
}

func parseSSHDirective(rest string) (sshDirective, error) {
	res := sshDirective{}
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return res, fmt.Errorf("@ssh requires options")
	}

	fields := tokenizeOptionTokens(trimmed)
	if len(fields) == 0 {
		return res, fmt.Errorf("@ssh requires options")
	}

	scope := restfile.SSHScopeRequest
	idx := 0
	if sc, ok := parseSSHScope(fields[idx]); ok {
		scope = sc
		idx++
	}

	name := "default"
	if idx < len(fields) && !strings.Contains(fields[idx], "=") {
		name = strings.TrimSpace(fields[idx])
		idx++
	}
	if name == "" {
		name = "default"
	}

	opts := parseOptionTokens(strings.Join(fields[idx:], " "))
	prof := restfile.SSHProfile{Scope: scope, Name: name}
	applySSHOptions(&prof, opts)
	if scope == restfile.SSHScopeRequest {
		// Request-scoped persist is ignored to avoid leaking tunnels.
		prof.Persist = restfile.SSHOpt[bool]{}
	}

	if scope != restfile.SSHScopeRequest {
		if strings.TrimSpace(prof.Host) == "" {
			return res, fmt.Errorf("@ssh %s scope requires host", sshScopeLabel(scope))
		}
		res.scope = scope
		res.profile = prof
		return res, nil
	}

	use := strings.TrimSpace(opts["use"])
	inline := buildInlineSSH(prof)
	if use == "" && inline == nil {
		return res, fmt.Errorf("@ssh requires host or use=")
	}

	res.scope = scope
	res.profile = prof
	res.spec = &restfile.SSHSpec{Use: use, Inline: inline}
	return res, nil
}

func parseSSHScope(token string) (restfile.SSHScope, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "global":
		return restfile.SSHScopeGlobal, true
	case "file":
		return restfile.SSHScopeFile, true
	case "request":
		return restfile.SSHScopeRequest, true
	default:
		return 0, false
	}
}

func applySSHOptions(prof *restfile.SSHProfile, opts map[string]string) {
	if host := strings.TrimSpace(opts["host"]); host != "" {
		prof.Host = host
	}
	if port := strings.TrimSpace(opts["port"]); port != "" {
		prof.PortStr = port
		if n, err := strconv.Atoi(port); err == nil && n > 0 {
			prof.Port = n
		}
	}
	if user := strings.TrimSpace(opts["user"]); user != "" {
		prof.User = user
	}
	if pw := strings.TrimSpace(opts["password"]); pw != "" {
		prof.Pass = pw
	} else if pw := strings.TrimSpace(opts["pass"]); pw != "" {
		prof.Pass = pw
	}
	if key := strings.TrimSpace(opts["key"]); key != "" {
		prof.Key = key
	}
	if kp := strings.TrimSpace(opts["passphrase"]); kp != "" {
		prof.KeyPass = kp
	}
	setSSHBool(&prof.Agent, opts, "agent")
	if kh := strings.TrimSpace(opts["known_hosts"]); kh != "" {
		prof.KnownHosts = kh
	} else if kh := strings.TrimSpace(opts["known-hosts"]); kh != "" {
		prof.KnownHosts = kh
	}
	setSSHBool(&prof.Strict, opts, "strict_hostkey", "strict-hostkey", "strict_host_key")
	setSSHBool(&prof.Persist, opts, "persist")

	if raw := strings.TrimSpace(opts["timeout"]); raw != "" {
		prof.TimeoutStr = raw
		prof.Timeout.Set = true
		if dur, err := time.ParseDuration(raw); err == nil && dur >= 0 {
			prof.Timeout.Val = dur
		}
	}
	if raw := strings.TrimSpace(opts["keepalive"]); raw != "" {
		prof.KeepAliveStr = raw
		prof.KeepAlive.Set = true
		if dur, err := time.ParseDuration(raw); err == nil && dur >= 0 {
			prof.KeepAlive.Val = dur
		}
	}
	if raw := strings.TrimSpace(opts["retries"]); raw != "" {
		prof.RetriesStr = raw
		prof.Retries.Set = true
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			prof.Retries.Val = n
		}
	}
}

func setSSHBool(opt *restfile.SSHOpt[bool], opts map[string]string, keys ...string) {
	for _, key := range keys {
		if raw, ok := opts[key]; ok {
			opt.Set = true
			val := true
			if raw != "" {
				if parsed, ok := parseBool(raw); ok {
					val = parsed
				}
			}
			opt.Val = val
			return
		}
	}
}

func buildInlineSSH(prof restfile.SSHProfile) *restfile.SSHProfile {
	if !sshInlineSet(prof) {
		return nil
	}
	copy := prof
	copy.Scope = restfile.SSHScopeRequest
	return &copy
}

func sshInlineSet(prof restfile.SSHProfile) bool {
	return prof.Host != "" ||
		prof.PortStr != "" ||
		prof.User != "" ||
		prof.Pass != "" ||
		prof.Key != "" ||
		prof.KeyPass != "" ||
		prof.KnownHosts != "" ||
		prof.Agent.Set ||
		prof.Strict.Set ||
		prof.Persist.Set ||
		prof.Timeout.Set ||
		prof.KeepAlive.Set ||
		prof.Retries.Set
}

func sshScopeLabel(scope restfile.SSHScope) string {
	switch scope {
	case restfile.SSHScopeGlobal:
		return "global"
	case restfile.SSHScopeFile:
		return "file"
	default:
		return "request"
	}
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

func splitAssert(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}

	inQuote := false
	var quote byte
	for i := 0; i < len(trimmed)-1; i++ {
		ch := trimmed[i]
		if inQuote {
			if ch == quote {
				inQuote = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = true
			quote = ch
			continue
		}
		if ch == '=' && trimmed[i+1] == '>' {
			left := strings.TrimSpace(trimmed[:i])
			right := strings.TrimSpace(trimmed[i+2:])
			return left, trimQuotes(right)
		}
	}
	return trimmed, ""
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

func splitExprOptions(input string) (string, map[string]string) {
	tokens := splitAuthFields(strings.TrimSpace(input))
	if len(tokens) == 0 {
		return "", map[string]string{}
	}
	optIndex := -1
	for i, token := range tokens {
		if isOptionToken(token) {
			optIndex = i
			break
		}
	}
	if optIndex == -1 {
		return strings.Join(tokens, " "), map[string]string{}
	}
	expr := strings.Join(tokens[:optIndex], " ")
	opts := parseOptionTokens(strings.Join(tokens[optIndex:], " "))
	return expr, opts
}

func isOptionToken(token string) bool {
	idx := strings.Index(token, "=")
	if idx <= 0 {
		return false
	}
	key := token[:idx]
	for _, r := range key {
		if !isOptionKeyRune(r) {
			return false
		}
	}
	return true
}

func applySettingsTokens(dst map[string]string, raw string) map[string]string {
	opts := parseOptionTokens(raw)
	if len(opts) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]string, len(opts))
	}
	for k, v := range opts {
		if k == "" {
			continue
		}
		dst[k] = v
	}
	return dst
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

func parseScriptSpec(rest string) (string, string) {
	fields := splitAuthFields(rest)
	kind := ""
	lang := ""
	for _, field := range fields {
		if strings.Contains(field, "=") {
			continue
		}
		if kind == "" {
			kind = field
			continue
		}
		if lang == "" {
			if v, ok := scriptLangToken(field); ok {
				lang = v
			}
		}
	}
	params := parseKeyValuePairs(fields)
	if v := params["lang"]; v != "" {
		lang = v
	}
	if v := params["language"]; v != "" && lang == "" {
		lang = v
	}
	return normScriptKind(kind), normScriptLang(lang)
}

func normScriptKind(kind string) string {
	out := strings.ToLower(strings.TrimSpace(kind))
	if out == "" {
		return "test"
	}
	return out
}

func normScriptLang(lang string) string {
	out := strings.ToLower(strings.TrimSpace(lang))
	switch out {
	case "":
		return "js"
	case "javascript":
		return "js"
	case "restermlang":
		return "rts"
	default:
		return out
	}
}

func scriptLangToken(tok string) (string, bool) {
	out := strings.ToLower(strings.TrimSpace(tok))
	switch out {
	case "js", "javascript":
		return "js", true
	case "rts", "restermlang":
		return "rts", true
	default:
		return "", false
	}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func (r *requestBuilder) appendScriptLine(kind, lang, body string) {
	kind = normScriptKind(kind)
	lang = normScriptLang(lang)
	if r.scriptBufferKind != "" &&
		(!strings.EqualFold(r.scriptBufferKind, kind) || !strings.EqualFold(r.scriptBufferLang, lang)) {
		r.flushPendingScript()
	}
	if r.scriptBufferKind == "" {
		r.scriptBufferKind = kind
		r.scriptBufferLang = lang
	}
	r.scriptBuffer = append(r.scriptBuffer, body)
}

func (r *requestBuilder) flushPendingScript() {
	if len(r.scriptBuffer) == 0 {
		return
	}
	script := strings.Join(r.scriptBuffer, "\n")
	r.metadata.Scripts = append(r.metadata.Scripts, restfile.ScriptBlock{
		Kind: r.scriptBufferKind,
		Lang: r.scriptBufferLang,
		Body: script,
	})
	r.scriptBuffer = nil
	r.scriptBufferKind = ""
	r.scriptBufferLang = ""
}

func (r *requestBuilder) appendScriptInclude(kind, lang, path string) {
	kind = normScriptKind(kind)
	lang = normScriptLang(lang)
	r.flushPendingScript()
	r.metadata.Scripts = append(r.metadata.Scripts, restfile.ScriptBlock{
		Kind:     kind,
		Lang:     lang,
		FilePath: path,
	})
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
		currentScriptLang: "js",
		http:              httpbuilder.New(),
		graphql:           graphqlbuilder.New(),
		grpc:              grpcbuilder.New(),
		sse:               newSSEBuilder(),
		websocket:         newWebSocketBuilder(),
	}
	return true
}

func (r *requestBuilder) markHeadersDone() {
	if r == nil || r.http == nil || r.http.HeaderDone() {
		return
	}
	r.http.MarkHeadersDone()
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
	if err := b.workflow.flushFlow(line); err != "" {
		b.addError(line, err)
	}
	if err := b.workflow.requireNoPending(); err != "" {
		b.addError(line, err)
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
	if len(b.fileSettings) > 0 {
		if b.doc.Settings == nil {
			b.doc.Settings = make(map[string]string, len(b.fileSettings))
		}
		for k, v := range b.fileSettings {
			b.doc.Settings[k] = v
		}
	}
	b.doc.Variables = append(b.doc.Variables, b.fileVars...)
	b.doc.Globals = append(b.doc.Globals, b.globalVars...)
	b.doc.Constants = append(b.doc.Constants, b.consts...)
	b.doc.Uses = append(b.doc.Uses, b.fileUses...)
	b.doc.SSH = append(b.doc.SSH, b.sshDefs...)
}

func (b *documentBuilder) handleFileSetting(rest string) {
	keyName, value := splitDirective(rest)
	if keyName == "" {
		return
	}
	if b.fileSettings == nil {
		b.fileSettings = make(map[string]string)
	}
	b.fileSettings[keyName] = value
}

func (b *documentBuilder) flushFileSettings() {
	if len(b.fileSettings) == 0 {
		return
	}
	if b.doc.Settings == nil {
		b.doc.Settings = make(map[string]string, len(b.fileSettings))
	}
	for k, v := range b.fileSettings {
		b.doc.Settings[k] = v
	}
	b.fileSettings = nil
}

func (r *requestBuilder) build() *restfile.Request {
	r.flushPendingScript()

	vars := append([]restfile.Variable(nil), r.variables...)

	req := &restfile.Request{
		Metadata:  r.metadata,
		Method:    r.http.Method(),
		URL:       strings.TrimSpace(r.http.URL()),
		Headers:   r.http.HeaderMap(),
		Body:      restfile.BodySource{},
		Variables: vars,
		Settings:  map[string]string{},
		LineRange: restfile.LineRange{
			Start: r.startLine,
			End:   r.startLine + len(r.originalLines) - 1,
		},
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
			if r.ssh != nil {
				req.SSH = r.ssh
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
	if r.ssh != nil {
		req.SSH = r.ssh
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

type workflowSwitchBuilder struct {
	expr  string
	cases []restfile.WorkflowSwitchCase
	def   *restfile.WorkflowSwitchCase
	line  int
}

type workflowIfBuilder struct {
	then  restfile.WorkflowIfBranch
	elifs []restfile.WorkflowIfBranch
	els   *restfile.WorkflowIfBranch
	line  int
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

func (s *workflowBuilder) handleDirective(key, rest string, line int) (bool, string) {
	if s.openSwitch != nil && key != "case" && key != "default" {
		if err := s.flushFlow(line); err != "" {
			return true, err
		}
	}
	if s.openIf != nil && key != "elif" && key != "else" {
		if err := s.flushFlow(line); err != "" {
			return true, err
		}
	}
	switch key {
	case "description", "desc":
		if rest == "" {
			return true, ""
		}
		if s.workflow.Description != "" {
			s.workflow.Description += "\n"
		}
		s.workflow.Description += rest
		s.touch(line)
		return true, ""
	case "tag", "tags":
		tags := parseTagList(rest)
		if len(tags) == 0 {
			return true, ""
		}
		for _, tag := range tags {
			if !contains(s.workflow.Tags, tag) {
				s.workflow.Tags = append(s.workflow.Tags, tag)
			}
		}
		s.touch(line)
		return true, ""
	case "when", "skip-if":
		if err := s.requireNoPending(); err != "" {
			return true, err
		}
		spec, err := parseConditionSpec(rest, line, key == "skip-if")
		if err != nil {
			return true, err.Error()
		}
		if s.pendingWhen != nil {
			return true, "@when directive already defined for next step"
		}
		s.pendingWhen = spec
		s.touch(line)
		return true, ""
	case "for-each":
		if err := s.requireNoPending(); err != "" {
			return true, err
		}
		spec, err := parseForEachSpec(rest, line)
		if err != nil {
			return true, err.Error()
		}
		if s.pendingForEach != nil {
			return true, "@for-each directive already defined for next step"
		}
		s.pendingForEach = spec
		s.touch(line)
		return true, ""
	case "switch":
		if err := s.requireNoPending(); err != "" {
			return true, err
		}
		if err := s.flushFlow(line); err != "" {
			return true, err
		}
		expr := strings.TrimSpace(rest)
		if expr == "" {
			return true, "@switch expression missing"
		}
		s.openSwitch = &workflowSwitchBuilder{expr: expr, line: line}
		s.touch(line)
		return true, ""
	case "case":
		if s.openSwitch == nil {
			return true, "@case without @switch"
		}
		if err := s.openSwitch.addCase(rest, line); err != "" {
			return true, err
		}
		s.touch(line)
		return true, ""
	case "default":
		if s.openSwitch == nil {
			return true, "@default without @switch"
		}
		if err := s.openSwitch.addDefault(rest, line); err != "" {
			return true, err
		}
		s.touch(line)
		return true, ""
	case "if":
		if err := s.requireNoPending(); err != "" {
			return true, err
		}
		if err := s.flushFlow(line); err != "" {
			return true, err
		}
		cond, opts := splitExprOptions(rest)
		cond = strings.TrimSpace(cond)
		if cond == "" {
			return true, "@if expression missing"
		}
		run, fail, err := parseWorkflowRunOptions(opts)
		if err != "" {
			return true, err
		}
		s.openIf = &workflowIfBuilder{
			then: restfile.WorkflowIfBranch{Cond: cond, Run: run, Fail: fail, Line: line},
			line: line,
		}
		s.touch(line)
		return true, ""
	case "elif":
		if s.openIf == nil {
			return true, "@elif without @if"
		}
		cond, opts := splitExprOptions(rest)
		cond = strings.TrimSpace(cond)
		if cond == "" {
			return true, "@elif expression missing"
		}
		run, fail, err := parseWorkflowRunOptions(opts)
		if err != "" {
			return true, err
		}
		s.openIf.elifs = append(
			s.openIf.elifs,
			restfile.WorkflowIfBranch{Cond: cond, Run: run, Fail: fail, Line: line},
		)
		s.touch(line)
		return true, ""
	case "else":
		if s.openIf == nil {
			return true, "@else without @if"
		}
		if s.openIf.els != nil {
			return true, "@else already defined"
		}
		opts := parseOptionTokens(rest)
		run, fail, err := parseWorkflowRunOptions(opts)
		if err != "" {
			return true, err
		}
		s.openIf.els = &restfile.WorkflowIfBranch{Run: run, Fail: fail, Line: line}
		s.touch(line)
		return true, ""
	default:
		return false, ""
	}
}

func (s *workflowBuilder) requireNoPending() string {
	if s.pendingWhen != nil {
		return "@when must be followed by @step"
	}
	if s.pendingForEach != nil {
		return "@for-each must be followed by @step"
	}
	return ""
}

func (s *workflowBuilder) flushFlow(line int) string {
	if s.openSwitch != nil {
		if len(s.openSwitch.cases) == 0 && s.openSwitch.def == nil {
			return "@switch requires at least one @case or @default"
		}
		step := restfile.WorkflowStep{
			Kind: restfile.WorkflowStepKindSwitch,
			Switch: &restfile.WorkflowSwitch{
				Expr:    s.openSwitch.expr,
				Cases:   s.openSwitch.cases,
				Default: s.openSwitch.def,
				Line:    s.openSwitch.line,
			},
			Line:      s.openSwitch.line,
			OnFailure: s.workflow.DefaultOnFailure,
		}
		s.workflow.Steps = append(s.workflow.Steps, step)
		s.openSwitch = nil
		s.touch(line)
	}
	if s.openIf != nil {
		step := restfile.WorkflowStep{
			Kind: restfile.WorkflowStepKindIf,
			If: &restfile.WorkflowIf{
				Cond:  s.openIf.then.Cond,
				Then:  s.openIf.then,
				Elifs: s.openIf.elifs,
				Else:  s.openIf.els,
				Line:  s.openIf.line,
			},
			Line:      s.openIf.line,
			OnFailure: s.workflow.DefaultOnFailure,
		}
		s.workflow.Steps = append(s.workflow.Steps, step)
		s.openIf = nil
		s.touch(line)
	}
	return ""
}

func (b *workflowSwitchBuilder) addCase(rest string, line int) string {
	expr, opts := splitExprOptions(rest)
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "@case expression missing"
	}
	run, fail, err := parseWorkflowRunOptions(opts)
	if err != "" {
		return err
	}
	b.cases = append(
		b.cases,
		restfile.WorkflowSwitchCase{Expr: expr, Run: run, Fail: fail, Line: line},
	)
	return ""
}

func (b *workflowSwitchBuilder) addDefault(rest string, line int) string {
	if b.def != nil {
		return "@default already defined"
	}
	opts := parseOptionTokens(rest)
	run, fail, err := parseWorkflowRunOptions(opts)
	if err != "" {
		return err
	}
	b.def = &restfile.WorkflowSwitchCase{Run: run, Fail: fail, Line: line}
	return ""
}

func parseWorkflowRunOptions(opts map[string]string) (string, string, string) {
	run := strings.TrimSpace(opts["run"])
	if run == "" {
		run = strings.TrimSpace(opts["using"])
	}
	fail := strings.TrimSpace(opts["fail"])
	if run == "" && fail == "" {
		return "", "", "expected run=... or fail=..."
	}
	if run != "" && fail != "" {
		return "", "", "cannot combine run and fail"
	}
	return run, fail, ""
}

func (s *workflowBuilder) addStep(line int, rest string) string {
	if err := s.flushFlow(line); err != "" {
		return err
	}
	remainder := strings.TrimSpace(rest)
	if remainder == "" {
		return "@step missing content"
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
		using = options["run"]
	}
	if using == "" {
		return "@step missing using request"
	}
	delete(options, "using")
	delete(options, "run")
	step := restfile.WorkflowStep{
		Kind:      restfile.WorkflowStepKindRequest,
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
	if s.pendingWhen != nil {
		step.When = s.pendingWhen
		s.pendingWhen = nil
	}
	if s.pendingForEach != nil {
		step.Kind = restfile.WorkflowStepKindForEach
		step.ForEach = &restfile.WorkflowForEach{
			Expr: s.pendingForEach.Expression,
			Var:  s.pendingForEach.Var,
			Line: s.pendingForEach.Line,
		}
		s.pendingForEach = nil
	}
	s.workflow.Steps = append(s.workflow.Steps, step)
	s.touch(line)
	return ""
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
