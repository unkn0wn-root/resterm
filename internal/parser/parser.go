package parser

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/parser/graphqlbuilder"
	"github.com/unkn0wn-root/resterm/internal/parser/grpcbuilder"
	"github.com/unkn0wn-root/resterm/internal/parser/httpbuilder"
	"github.com/unkn0wn-root/resterm/pkg/restfile"
)

var (
	variableLineRe = regexp.MustCompile(`^@([A-Za-z0-9_.-]+)\s*(?::|=)\s*(.+)$`)
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
	doc       *restfile.Document
	inRequest bool
	request   *requestBuilder
	fileVars  []restfile.Variable
	inBlock   bool
}

type requestBuilder struct {
	startLine         int
	endLine           int
	metadata          restfile.RequestMetadata
	variables         []restfile.Variable
	originalLines     []string
	currentScriptKind string
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
		b.handleScript(lineNumber, strings.TrimSpace(strings.TrimPrefix(line, ">")))
		b.appendLine(line)
		return
	}

	if matches := variableLineRe.FindStringSubmatch(trimmed); matches != nil {
		variable := restfile.Variable{
			Name:  matches[1],
			Value: strings.TrimSpace(matches[2]),
			Line:  lineNumber,
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

	if !b.ensureRequest(line) {
		return
	}

	key, rest := splitDirective(directive)
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
	case "script":
		if rest != "" {
			b.request.currentScriptKind = strings.ToLower(rest)
		}
	}
}

func (b *documentBuilder) handleScript(line int, body string) {
	if !b.ensureRequest(line) {
		return
	}
	kind := b.request.currentScriptKind
	if kind == "" {
		kind = "test"
	}
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}") {
		body = strings.TrimSpace(body)
	}
	b.request.metadata.Scripts = append(b.request.metadata.Scripts, restfile.ScriptBlock{Kind: kind, Body: body})
}

func parseAuthSpec(rest string) *restfile.AuthSpec {
	fields := strings.Fields(rest)
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
}

func (r *requestBuilder) build() *restfile.Request {
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
