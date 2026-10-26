package parser

import (
	"cmp"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	graphqlbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/graphql"
	grpcbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/grpc"
	httpbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/http"
	ssebuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/sse"
	wsbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/websocket"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type documentBuilder struct {
	doc                  *restfile.Document
	inRequest            bool
	separated            bool // set at the first ### line, where file scope ends
	request              *requestBuilder
	mock                 *mockBuilder
	workflow             *workflowBuilder
	reader               directiveReader
	openLines            []string
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

func (b *documentBuilder) addError(line int, msg string) {
	b.pushError(lineDiagnostic(line, msg))
}

func (b *documentBuilder) addWarning(line int, msg string) {
	b.pushWarning(lineDiagnostic(line, msg))
}

func (b *documentBuilder) pushError(item restfile.ParseDiagnostic) {
	b.doc.Errors = pushDiagnostic(b.doc.Errors, item)
}

func (b *documentBuilder) pushWarning(item restfile.ParseDiagnostic) {
	b.doc.Warnings = pushDiagnostic(b.doc.Warnings, item)
}

func pushDiagnostic(items []restfile.ParseDiagnostic, item restfile.ParseDiagnostic) []restfile.ParseDiagnostic {
	item.Message = strings.TrimSpace(item.Message)
	if item.Message == "" {
		return items
	}
	return append(items, item)
}

func lineDiagnostic(line int, msg string) restfile.ParseDiagnostic {
	return restfile.ParseDiagnostic{Message: msg, Span: diag.Span{Start: diag.Pos{Line: line}}}
}

// leaves flattens joined errors but keeps a single-error wrapper as one leaf.
func leaves(err error) []error {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var errs []error
		for _, err := range joined.Unwrap() {
			errs = append(errs, leaves(err)...)
		}
		return errs
	}
	return []error{err}
}

// Report each joined error separately so the editor can mark every problem.
func (b *documentBuilder) report(d parsedDirective, err error) {
	for _, err := range leaves(err) {
		item := d.diagnostic(err.Error(), err)
		if isWarning(err) {
			b.pushWarning(item)
			continue
		}
		b.pushError(item)
	}
}

func isWarning(err error) bool {
	var unknown *directive.UnknownOptionsError
	var quoted *directive.QuotedOptionError
	var script *scriptFieldError
	return errors.As(err, &unknown) || errors.As(err, &quoted) || errors.As(err, &script)
}

func withoutWarnings(err error) error {
	return errors.Join(slices.DeleteFunc(leaves(err), isWarning)...)
}

// reportStrict treats unknown options as errors because they can change what runs.
func (b *documentBuilder) reportStrict(d parsedDirective, err error) {
	for _, err := range leaves(err) {
		b.pushError(d.diagnostic(err.Error(), err))
	}
}

// Keep valid options when the only errors are unknown keys, quoting, spacing,
// or unsupported operators.
func fatalErr(err error) bool {
	for _, err := range leaves(err) {
		var unknown *directive.UnknownOptionsError
		var spaced *directive.SpacedOptionsError
		var op *directive.OpOptionError
		var quoted *directive.QuotedOptionError
		if !errors.As(err, &unknown) && !errors.As(err, &spaced) && !errors.As(err, &op) && !errors.As(err, &quoted) {
			return true
		}
	}
	return false
}

func (b *documentBuilder) processLine(no int, raw, term string) {
	ln := makeLine(no, raw, term)
	b.closeOpenDirective(ln)

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
	b.separated = true
	return true
}

func (b *documentBuilder) ensureRequest(line int) {
	if b.inRequest {
		return
	}
	b.openRequest(&requestBuilder{
		startLine:  line,
		sourcePath: b.doc.Path,
		metadata:   restfile.RequestMetadata{Tags: []string{}},
		declared:   make(map[directive.Name]bool),
		script:     defaultScriptMode,
		http:       httpbuilder.New(),
		graphql:    graphqlbuilder.New(),
		grpc:       grpcbuilder.New(),
		sse:        ssebuilder.New(),
		websocket:  wsbuilder.New(),
	})
}

func (b *documentBuilder) openRequest(r *requestBuilder) {
	if b.workflow != nil {
		b.flushWorkflow(r.startLine - 1)
		r.endsWorkflow = true
	}
	b.inRequest = true
	b.request = r
}

// Record active request lines first so a failed continuation cannot shorten the
// range. Before a request starts, hold continuation lines in case it opens one.
func (b *documentBuilder) appendLine(raw string) {
	if b.inRequest {
		b.request.originalLines = append(b.request.originalLines, raw)
		return
	}
	if _, open := b.reader.pending(); open {
		b.openLines = append(b.openLines, raw)
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
	b.finalizeRetry()

	req := b.request.build()
	b.warnUnclosedBody(req)
	b.lintRequestCaptures(req)
	b.lintRequestPolicies(req)
	b.lintProfile(req)
	if req.Method != "" && req.URL != "" {
		// Directives may come before the request line or @grpc-proto, so check here.
		if b.request.isGRPC() {
			for _, d := range b.request.protocols {
				b.fail(d, d.Spelling.Tag()+" cannot be combined with a GRPC request line")
			}
		} else {
			for _, d := range b.request.grpcDirectives {
				b.fail(d, d.Spelling.Tag()+" requires a GRPC request line")
			}
			var on directive.Name
			switch {
			case req.WebSocket != nil:
				on = directive.WebSocket
			case req.SSE != nil:
				on = directive.SSE
			case req.Body.GraphQL != nil:
				on = directive.GraphQL
			}
			for _, d := range b.request.protocols {
				if protocolOf(d.Name) != on {
					b.fail(d, d.Spelling.Tag()+" cannot be combined with "+on.Tag()+" on the same request")
				}
			}
		}
		if gr := req.GRPC; gr != nil && len(gr.ProtoFiles) == 0 {
			for _, d := range b.request.grpcDirectives {
				if d.Name == directive.GRPCImportPath {
					b.warn(d, d.Spelling.Tag()+" without "+directive.GRPCProto.Tag()+" was ignored")
				}
			}
			gr.ImportPaths = nil
		}
		b.doc.Requests = append(b.doc.Requests, req)
	} else if b.separated {
		for _, u := range req.Metadata.Uses {
			b.addWarning(
				u.Line,
				"@use in a block without a request was ignored. Move it above the first ### to share it",
			)
		}
		for _, d := range b.request.settingDirectives {
			b.warn(
				d,
				d.Spelling.Tag()+" in a block without a request was ignored. Move it above the first ### to share it",
			)
		}
	}

	b.inRequest = false
	b.request = nil
	b.inBlock = false
}

func (b *documentBuilder) flushWorkflow(line int) {
	if b.workflow == nil {
		return
	}
	if at, err := b.workflow.flushFlow(line); err != nil {
		b.addError(at, err.Error())
	}
	if at, err := b.workflow.requireNoPending(); err != nil {
		b.addError(at, err.Error())
	}
	scene := b.workflow.build(line)
	if len(scene.Steps) > 0 {
		b.doc.Workflows = append(b.doc.Workflows, scene)
	}
	b.workflow = nil
}

func (b *documentBuilder) finish() {
	if cut := b.reader.abandon(); cut != nil {
		b.failOpenDirective(cut)
	}
	b.flushMock()
	b.flushRequest(0)
	b.flushWorkflow(0)
	b.file.flushSettings(b.doc)
	b.file.apply(b.doc)

	// Checks at the end of a request, workflow or mock add findings out of order.
	byLine := func(x, y restfile.ParseDiagnostic) int {
		return cmp.Compare(x.Span.Start.Line, y.Span.Start.Line)
	}
	slices.SortStableFunc(b.doc.Errors, byLine)
	slices.SortStableFunc(b.doc.Warnings, byLine)
}

func (b *documentBuilder) startWorkflow(d parsedDirective) error {
	nameToken, remainder := directive.CutToken(d.Args)
	if nameToken == "" || strings.Contains(nameToken, "=") {
		return errors.New("@workflow name missing")
	}
	line := d.lines.Start
	if b.inRequest {
		b.flushRequest(line - 1)
	}
	b.flushWorkflow(line - 1)
	sb := newWorkflowBuilder(line, nameToken)
	opts, err := directive.ParseOptions(directive.Workflow, remainder)
	b.report(d, errors.Join(err, sb.applyOptions(opts)))
	sb.touch(line)
	b.workflow = sb
	return nil
}
