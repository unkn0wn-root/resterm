package parser

import (
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/capture"
	"github.com/unkn0wn-root/resterm/internal/httpver"
	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
	graphqlbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/graphql"
	grpcbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/grpc"
	httpbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/http"
	ssebuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/sse"
	wsbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/websocket"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/lex"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	dscope "github.com/unkn0wn-root/resterm/internal/parser/directive/scope"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type documentBuilder struct {
	doc                  *restfile.Document
	inRequest            bool
	request              *requestBuilder
	fileVars             []restfile.Variable
	globalVars           []restfile.Variable
	fileSettings         map[string]string
	consts               []restfile.Constant
	authDefs             []restfile.AuthProfile
	sshDefs              []restfile.SSHProfile
	k8sDefs              []restfile.K8sProfile
	patchDefs            []restfile.PatchProfile
	fileUses             []restfile.UseSpec
	inBlock              bool
	workflow             *workflowBuilder
	inScriptBlock        bool
	scriptBlockStartLine int
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

func (b *documentBuilder) addWarning(line int, message string) {
	if b == nil || b.doc == nil {
		return
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return
	}
	b.doc.Warnings = append(b.doc.Warnings, restfile.ParseError{
		Line:    line,
		Message: msg,
	})
}

func (b *documentBuilder) processLine(lineNumber int, line string) {
	trimmed := strings.TrimSpace(line)

	if b.inBlock {
		if b.handleBlockComment(lineNumber, line, trimmed) {
			return
		}
	}

	if b.inScriptBlock {
		if b.handleScriptBlockLine(lineNumber, line, trimmed) {
			return
		}
	} else if b.handleScriptBlockStart(lineNumber, line, trimmed) {
		return
	}

	b.flushScriptIfNeeded(trimmed)

	if b.handleBlockComment(lineNumber, line, trimmed) {
		return
	}
	if b.handleSeparator(lineNumber, trimmed) {
		return
	}
	if b.handleCommentLine(lineNumber, line, trimmed) {
		return
	}
	if b.handleScriptLine(lineNumber, line, trimmed) {
		return
	}
	if b.handleVariableLine(lineNumber, line, trimmed) {
		return
	}
	if b.handleBlankLine(line, trimmed) {
		return
	}
	if b.handleBodyContinuation(line) {
		return
	}
	if b.handleMethodLine(lineNumber, line) {
		return
	}
	if b.handleHeaderLine(line) {
		return
	}
	if b.handleDescriptionLine(lineNumber, line, trimmed) {
		return
	}

	b.appendLine(line)
}

func (b *documentBuilder) flushScriptIfNeeded(trimmed string) {
	if b.inRequest && b.request != nil && !strings.HasPrefix(trimmed, ">") {
		b.request.flushPendingScript()
	}
}

func (b *documentBuilder) handleBlockComment(lineNumber int, line, trimmed string) bool {
	if b.inBlock {
		content, closed := parseBlockCommentLine(trimmed, false)
		if content != "" {
			b.handleComment(lineNumber, content)
		}
		b.appendLine(line)
		if closed {
			b.inBlock = false
		}
		return true
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
		return true
	}
	return false
}

func (b *documentBuilder) handleSeparator(lineNumber int, trimmed string) bool {
	if !strings.HasPrefix(trimmed, "###") {
		return false
	}
	if b.workflow != nil {
		b.flushWorkflow(lineNumber - 1)
	}
	b.flushRequest(lineNumber - 1)
	b.flushFileSettings()
	return true
}

func (b *documentBuilder) handleCommentLine(lineNumber int, line, trimmed string) bool {
	if commentText, ok := stripComment(trimmed); ok {
		b.handleComment(lineNumber, commentText)
		b.appendLine(line)
		return true
	}
	return false
}

func (b *documentBuilder) handleScriptLine(lineNumber int, line, trimmed string) bool {
	if !strings.HasPrefix(trimmed, ">") {
		return false
	}
	b.handleScript(lineNumber, line)
	b.appendLine(line)
	return true
}

func (b *documentBuilder) handleVariableLine(lineNumber int, line, trimmed string) bool {
	matches := variableLineRe.FindStringSubmatch(trimmed)
	if matches == nil {
		return false
	}
	scopeToken, secret := dscope.ParseToken(matches[1])
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
			return true
		}
	case "file":
		b.addScopedVariable(name, value, lineNumber, restfile.ScopeFile, secret)
	default:
		scope := restfile.ScopeRequest
		if !b.inRequest {
			scope = restfile.ScopeFile
		}
		if !b.addScopedVariable(name, value, lineNumber, scope, secret) {
			return true
		}
	}
	b.appendLine(line)
	return true
}

func (b *documentBuilder) handleBlankLine(line, trimmed string) bool {
	if trimmed != "" {
		return false
	}
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
	return true
}

func (b *documentBuilder) handleBodyContinuation(line string) bool {
	if b.inRequest && b.request.http.HasMethod() && b.request.http.HeaderDone() {
		b.handleBodyLine(line)
		b.appendLine(line)
		return true
	}
	return false
}

func (b *documentBuilder) handleMethodLine(lineNumber int, line string) bool {
	if grpcbuilder.IsMethodLine(line) {
		b.ensureRequest(lineNumber)
		fields := strings.Fields(line)
		target := ""
		if len(fields) > 1 {
			target = strings.Join(fields[1:], " ")
		}

		b.request.http.SetMethodAndURL(strings.ToUpper(fields[0]), target)
		b.request.grpc.SetTarget(target)
		b.appendLine(line)
		return true
	}

	if method, url, ver, ok := httpbuilder.ParseMethodLine(line); ok {
		b.ensureRequest(lineNumber)

		b.request.http.SetMethodAndURL(method, url)
		b.request.settings = httpver.SetIfMissing(b.request.settings, ver)
		b.appendLine(line)
		return true
	}

	if url, ok := httpbuilder.ParseWebSocketURLLine(line); ok {
		b.ensureRequest(lineNumber)

		b.request.http.SetMethodAndURL(http.MethodGet, url)
		b.appendLine(line)
		return true
	}

	return false
}

func (b *documentBuilder) handleHeaderLine(line string) bool {
	if !b.inRequest || !b.request.http.HasMethod() || b.request.http.HeaderDone() {
		return false
	}
	if idx := strings.Index(line, ":"); idx != -1 {
		headerName := strings.TrimSpace(line[:idx])
		headerValue := strings.TrimSpace(line[idx+1:])
		if headerName != "" {
			b.request.http.AddHeader(headerName, headerValue)
		}
	}
	b.appendLine(line)
	return true
}

func (b *documentBuilder) handleDescriptionLine(lineNumber int, line, trimmed string) bool {
	b.ensureRequest(lineNumber)
	if b.request.http.HasMethod() {
		return false
	}
	if b.request.metadata.Description != "" {
		b.request.metadata.Description += "\n"
	}
	b.request.metadata.Description += trimmed
	b.appendLine(line)
	return true
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

func (b *documentBuilder) parseCaptureDirective(
	rest string,
	line int,
) (restfile.CaptureSpec, bool) {
	scopeToken, remainder := lex.SplitDirective(rest)
	if scopeToken == "" {
		b.addWarning(line, "@capture missing scope (use request, file, or global)")
		return restfile.CaptureSpec{}, false
	}
	scope, secret, ok := parseCaptureScope(scopeToken)
	if !ok {
		b.addWarning(
			line,
			fmt.Sprintf(
				"@capture scope %q is invalid (use request, file, global, with optional -secret)",
				scopeToken,
			),
		)
		return restfile.CaptureSpec{}, false
	}
	trimmed := strings.TrimSpace(remainder)
	if trimmed == "" {
		b.addWarning(line, "@capture missing '<name> <expression>'")
		return restfile.CaptureSpec{}, false
	}
	nameEnd := strings.IndexAny(trimmed, " \t")
	if nameEnd == -1 {
		b.addWarning(line, "@capture missing expression after capture name")
		return restfile.CaptureSpec{}, false
	}
	name := strings.TrimSpace(trimmed[:nameEnd])
	expression := strings.TrimSpace(trimmed[nameEnd:])
	if expression == "" {
		b.addWarning(line, "@capture expression missing")
		return restfile.CaptureSpec{}, false
	}
	if strings.HasPrefix(expression, "=") {
		expression = strings.TrimSpace(expression[1:])
	}
	if expression == "" {
		b.addWarning(line, "@capture expression missing after '='")
		return restfile.CaptureSpec{}, false
	}
	return restfile.CaptureSpec{
		Scope:      scope,
		Name:       name,
		Expression: expression,
		Mode:       captureExprMode(expression),
		Secret:     secret,
		Line:       line,
	}, true
}

func captureExprMode(ex string) restfile.CaptureExprMode {
	if capture.HasUnquotedTemplateMarker(ex) {
		return restfile.CaptureExprModeTemplate
	}
	return restfile.CaptureExprModeRTS
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

func splitAssert(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", ""
	}

	inQuote := false
	var quote byte
	escaped := false
	for i := 0; i < len(trimmed)-1; i++ {
		ch := trimmed[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
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
			return left, lex.TrimQuotes(right)
		}
	}
	return trimmed, ""
}

func (b *documentBuilder) handleScript(ln int, raw string) {
	body, ok := trimScriptLine(raw, false)
	if !ok {
		return
	}
	b.addScript(ln, body, true)
}

func (b *documentBuilder) addScript(ln int, body string, inc bool) {
	b.ensureRequest(ln)
	k := b.request.currentScriptKind
	l := b.request.currentScriptLang
	if inc {
		if p, ok := scriptInc(body); ok {
			b.request.appendScriptInclude(k, l, p)
			return
		}
	}
	b.request.appendScriptLine(k, l, body)
}

func scriptInc(body string) (string, bool) {
	h := str.TrimLeft(body)
	if !strings.HasPrefix(h, "<") {
		return "", false
	}
	p := strings.TrimSpace(strings.TrimPrefix(h, "<"))
	if p == "" {
		return "", false
	}
	return p, true
}

func trimScriptLine(raw string, allow bool) (string, bool) {
	s := str.TrimLeft(raw)
	if after, ok := strings.CutPrefix(s, ">"); ok {
		b := str.TrimLeadingOnce(after)
		return str.TrimRight(b), true
	}
	if allow {
		return str.TrimRight(raw), true
	}
	return "", false
}

func (b *documentBuilder) handleScriptBlockStart(ln int, line, tr string) bool {
	if !isSBStart(tr) {
		return false
	}
	b.ensureRequest(ln)
	b.inScriptBlock = true
	b.scriptBlockStartLine = ln
	b.appendLine(line)
	return true
}

func (b *documentBuilder) handleScriptBlockLine(ln int, line, tr string) bool {
	if isSBEnd(tr) {
		b.appendLine(line)
		b.endSB(false)
		return true
	}

	if b.handleSeparator(ln, tr) {
		return true
	}

	body, _ := trimScriptLine(line, true)
	b.addScript(ln, body, false)
	b.appendLine(line)
	return true
}

func (b *documentBuilder) endSB(err bool) {
	if !b.inScriptBlock {
		return
	}
	if err {
		b.addError(b.scriptBlockStartLine, "script block missing %}")
	}
	b.inScriptBlock = false
	b.scriptBlockStartLine = 0
	if b.request != nil {
		b.request.flushPendingScript()
	}
}

func isSBStart(trimmed string) bool {
	if !strings.HasPrefix(trimmed, ">") {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
	return rest == "{%"
}

func isSBEnd(trimmed string) bool {
	if after, ok := strings.CutPrefix(trimmed, ">"); ok {
		trimmed = str.TrimLeft(after)
	}
	if !strings.HasPrefix(trimmed, "%}") {
		return false
	}
	rest := strings.TrimPrefix(trimmed, "%}")
	if rest == "" {
		return true
	}
	rest = str.TrimLeft(rest)
	if rest == "" {
		return true
	}
	_, ok := stripComment(rest)
	return ok
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
		b.ensureRequest(line)
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
		scopeToken, args = lex.SplitFirst(rest)
		if scopeToken == "" {
			return false
		}
	}

	scopeStr, secret := dscope.ParseToken(scopeToken)
	name, value := options.ParseNameValue(args)

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

func (b *documentBuilder) addConstant(name, value string, line int) {
	constant := restfile.Constant{
		Name:  name,
		Value: value,
		Line:  line,
	}
	b.consts = append(b.consts, constant)
}

func (b *documentBuilder) handleBodyLine(line string) {
	if b.request.graphql.HandleBodyLine(line) {
		return
	}
	if b.request.grpc.HandleBodyLine(line) {
		return
	}

	compat := bodyref.AllowNoSpace
	if isXMLContentType(b.request.http.MimeType()) {
		compat = bodyref.ExplicitOnly
	}
	if file, ok := bodyref.Parse(line, bodyref.Line, compat); ok {
		b.request.http.SetBodyFromFile(file)
		return
	}
	if file, ok := bodyref.Parse(line, bodyref.Inline, compat); ok {
		b.request.http.SetBodyFromFile(file)
		return
	}
	b.request.http.AppendBodyLine(line)
}

func isXMLContentType(s string) bool {
	return strings.Contains(strings.ToLower(s), "xml")
}

func (b *documentBuilder) ensureRequest(line int) {
	if b.inRequest {
		return
	}

	if b.workflow != nil {
		b.flushWorkflow(line - 1)
	}

	b.inRequest = true
	b.request = &requestBuilder{
		startLine:         line,
		metadata:          restfile.RequestMetadata{Tags: []string{}},
		currentScriptKind: defaultScriptKind,
		currentScriptLang: defaultScriptLang,
		http:              httpbuilder.New(),
		graphql:           graphqlbuilder.New(),
		grpc:              grpcbuilder.New(),
		sse:               ssebuilder.New(),
		websocket:         wsbuilder.New(),
	}
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

	b.endSB(true)

	b.request.flushPendingScript()

	req := b.request.build()
	b.lintRequestCaptures(req)
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
		maps.Copy(b.doc.Settings, b.fileSettings)
	}
	b.doc.Variables = append(b.doc.Variables, b.fileVars...)
	b.doc.Globals = append(b.doc.Globals, b.globalVars...)
	b.doc.Constants = append(b.doc.Constants, b.consts...)
	b.doc.Auth = append(b.doc.Auth, b.authDefs...)
	b.doc.Uses = append(b.doc.Uses, b.fileUses...)
	b.doc.SSH = append(b.doc.SSH, b.sshDefs...)
	b.doc.K8s = append(b.doc.K8s, b.k8sDefs...)
	b.doc.Patches = append(b.doc.Patches, b.patchDefs...)
}

func (b *documentBuilder) handleFileSetting(rest string) {
	keyName, value := lex.SplitDirective(rest)
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
	maps.Copy(b.doc.Settings, b.fileSettings)
	b.fileSettings = nil
}

func (b *documentBuilder) startWorkflow(line int, rest string) {
	if b.inRequest {
		b.flushRequest(line - 1)
	}
	nameToken, remainder := lex.SplitFirst(rest)
	if nameToken == "" || strings.Contains(nameToken, "=") {
		return
	}
	b.flushWorkflow(line - 1)
	sb := newWorkflowBuilder(line, nameToken)
	sb.applyOptions(options.Parse(remainder))
	sb.touch(line)
	b.workflow = sb
}
