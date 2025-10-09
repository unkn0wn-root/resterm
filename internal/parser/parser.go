package parser

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/parser/graphqlbuilder"
	"github.com/unkn0wn-root/resterm/internal/parser/grpcbuilder"
	"github.com/unkn0wn-root/resterm/internal/parser/httpbuilder"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

var (
	variableLineRe = regexp.MustCompile(`^@(?:(global)\s+)?([A-Za-z0-9_.-]+)\s*(?::|=)\s*(.+)$`)
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
	inBlock    bool
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
}

func newDocumentBuilder(doc *restfile.Document) *documentBuilder {
	return &documentBuilder{doc: doc}
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
		value := strings.TrimSpace(matches[3])
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

	if b.handleScopedVariableDirective(key, rest, line) {
		return
	}

	if !b.ensureRequest(line) {
		return
	}

	if b.request.grpc.HandleDirective(key, rest) {
		return
	}
	if b.request.graphql.HandleDirective(key, rest) {
		return
	}
	switch key {
	case "name":
		if rest != "" {
			b.request.metadata.Name = rest
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
	if idx := strings.IndexAny(trimmed, ":="); idx != -1 {
		name := strings.TrimSpace(trimmed[:idx])
		value := strings.TrimSpace(trimmed[idx+1:])
		return name, value
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", ""
	}
	name := fields[0]
	value := ""
	if len(trimmed) > len(name) {
		value = strings.TrimSpace(trimmed[len(name):])
	}
	return name, value
}

func splitDirective(text string) (string, string) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}

	key := strings.ToLower(fields[0])
	var rest string
	if len(text) > len(fields[0]) {
		rest = strings.TrimSpace(text[len(fields[0]):])
	}
	return key, rest
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

	b.inRequest = true
	b.request = &requestBuilder{
		startLine:         line,
		metadata:          restfile.RequestMetadata{Tags: []string{}},
		currentScriptKind: "test",
		http:              httpbuilder.New(),
		graphql:           graphqlbuilder.New(),
		grpc:              grpcbuilder.New(),
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

func (b *documentBuilder) finish() {
	b.flushRequest(0)
	b.doc.Variables = append(b.doc.Variables, b.fileVars...)
	b.doc.Globals = append(b.doc.Globals, b.globalVars...)
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

	if grpcReq, body, mime, ok := r.grpc.Finalize(r.http.MimeType()); ok {
		req.GRPC = grpcReq
		req.Body = body
		if mime != "" {
			req.Body.MimeType = mime
		}
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
	}

	if r.settings != nil {
		req.Settings = r.settings
	}

	return req
}
