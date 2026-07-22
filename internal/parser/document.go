package parser

import (
	"maps"
	"strings"

	graphqlbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/graphql"
	grpcbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/grpc"
	httpbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/http"
	ssebuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/sse"
	wsbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/websocket"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	"github.com/unkn0wn-root/resterm/internal/parser/lexer"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type documentBuilder struct {
	doc                  *restfile.Document
	inRequest            bool
	request              *requestBuilder
	mock                 *mockBuilder
	workflow             *workflowBuilder
	pendingTitle         string
	file                 fileScope
	inBlock              bool
	inScriptBlock        bool
	scriptBlockStartLine int
}

// fileScope accumulates declarations made at file level, outside any request,
// until they are copied into the document.
type fileScope struct {
	vars     []restfile.Variable
	globals  []restfile.Variable
	settings map[string]string
	consts   []restfile.Constant
	auth     []restfile.AuthProfile
	ssh      []restfile.SSHProfile
	k8s      []restfile.K8sProfile
	patches  []restfile.PatchProfile
	uses     []restfile.UseSpec
}

// flushSettings copies pending settings into the document. Settings apply per
// section, so this runs at every ### separator, not only at finish.
func (s *fileScope) flushSettings(doc *restfile.Document) {
	if len(s.settings) == 0 {
		return
	}
	if doc.Settings == nil {
		doc.Settings = make(map[string]string, len(s.settings))
	}
	maps.Copy(doc.Settings, s.settings)
	s.settings = nil
}

func (s *fileScope) apply(doc *restfile.Document) {
	doc.Variables = append(doc.Variables, s.vars...)
	doc.Globals = append(doc.Globals, s.globals...)
	doc.Constants = append(doc.Constants, s.consts...)
	doc.Auth = append(doc.Auth, s.auth...)
	doc.Uses = append(doc.Uses, s.uses...)
	doc.SSH = append(doc.SSH, s.ssh...)
	doc.K8s = append(doc.K8s, s.k8s...)
	doc.Patches = append(doc.Patches, s.patches...)
}

func (b *documentBuilder) addError(line int, message string) {
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
	msg := strings.TrimSpace(message)
	if msg == "" {
		return
	}
	b.doc.Warnings = append(b.doc.Warnings, restfile.ParseError{
		Line:    line,
		Message: msg,
	})
}

func (b *documentBuilder) processLine(no int, raw string) {
	ln := makeLine(no, raw)

	if b.mock != nil {
		b.handleMockBlockLine(ln)
		return
	}

	if b.inBlock {
		if b.handleBlockComment(ln) {
			return
		}
	}

	if b.inScriptBlock {
		if b.handleScriptBlockLine(ln) {
			return
		}
	} else if b.handleScriptBlockStart(ln) {
		return
	}

	b.flushScriptIfNeeded(ln)

	if b.handleSeparator(ln) {
		return
	}
	if b.handleMultipartBodyLine(ln) {
		return
	}
	if b.handleBlockComment(ln) {
		return
	}
	if b.handleCommentLine(ln) {
		return
	}
	if b.handleScriptLine(ln) {
		return
	}
	if b.handleVariableLine(ln) {
		return
	}
	if b.handleBlankLine(ln) {
		return
	}
	if b.handleBodyContinuation(ln) {
		return
	}
	if b.handleMethodLine(ln) {
		return
	}
	if b.handleHeaderLine(ln) {
		return
	}
	b.handleDescriptionLine(ln)
}

func (b *documentBuilder) handleSeparator(ln line) bool {
	if !ln.isSeparator() {
		return false
	}
	b.flushMock()
	if b.workflow != nil {
		b.flushWorkflow(ln.no - 1)
	}
	b.flushRequest(ln.no - 1)
	b.file.flushSettings(b.doc)
	b.pendingTitle = strings.TrimSpace(strings.TrimPrefix(ln.text, "###"))
	return true
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

func (b *documentBuilder) appendLine(raw string) {
	if b.inRequest {
		b.request.originalLines = append(b.request.originalLines, raw)
	}
}

func (b *documentBuilder) flushRequest(_ int) {
	if !b.inRequest {
		return
	}

	if b.inScriptBlock {
		b.addError(b.scriptBlockStartLine, "script block missing %}")
	}
	b.endScriptBlock()

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
	if err := b.workflow.flushFlow(line); err != nil {
		b.addError(line, err.Error())
	}
	if err := b.workflow.requireNoPending(); err != nil {
		b.addError(line, err.Error())
	}
	scene := b.workflow.build(line)
	if len(scene.Steps) > 0 {
		b.doc.Workflows = append(b.doc.Workflows, scene)
	}
	b.workflow = nil
}

func (b *documentBuilder) finish() {
	b.flushMock()
	b.flushRequest(0)
	b.flushWorkflow(0)
	b.file.flushSettings(b.doc)
	b.file.apply(b.doc)
}

func (b *documentBuilder) startWorkflow(line int, rest string) {
	if b.inRequest {
		b.flushRequest(line - 1)
	}
	nameToken, remainder := lexer.SplitFirst(rest)
	if nameToken == "" || strings.Contains(nameToken, "=") {
		return
	}
	b.flushWorkflow(line - 1)
	sb := newWorkflowBuilder(line, nameToken)
	sb.applyOptions(options.Parse(remainder))
	sb.touch(line)
	b.workflow = sb
}
