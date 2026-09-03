package parser

import (
	"strings"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/directive"
	grpcbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/grpc"
	httpbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/http"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type SourceLineKind uint8

const (
	SourceLineCode SourceLineKind = iota
	SourceLineComment
	SourceLineDirective
	SourceLineDirectiveValue
	SourceLineRequestSeparator
	SourceLineLiteral
)

func (k SourceLineKind) String() string {
	switch k {
	case SourceLineCode:
		return "code"
	case SourceLineComment:
		return "comment"
	case SourceLineDirective:
		return "directive"
	case SourceLineDirectiveValue:
		return "directive value"
	case SourceLineRequestSeparator:
		return "request separator"
	case SourceLineLiteral:
		return "literal"
	default:
		return "unknown"
	}
}

// ContentStart and ContentEnd are rune offsets for comment text without its marker or spaces.
type SourceLine struct {
	Kind         SourceLineKind
	Args         directive.ArgKind
	ContentStart int
	ContentEnd   int
}

func (l SourceLine) ContentRange(n int) (start, end int, ok bool) {
	start, end = min(l.ContentStart, n), min(l.ContentEnd, n)
	return start, end, start < end
}

// SourceSyntax uses the parser's context rules to classify each line.
type SourceSyntax struct {
	lines   []SourceLine
	effects map[directiveKey]directiveEffect
}

// A trailing newline creates an empty final line to match the editor.
func (s *SourceSyntax) Classify(source string) {
	count := strings.Count(source, "\n") + 1
	if cap(s.lines) < count {
		s.lines = make([]SourceLine, count)
	}
	s.lines = s.lines[:count]

	// Reuse directive results across edits, but cap the cache.
	if s.effects == nil || len(s.effects) > maxDirectiveEffects {
		s.effects = make(map[directiveKey]directiveEffect)
	}

	scan := sourceScan{effects: s.effects}
	i := 0
	for raw := range strings.SplitSeq(source, "\n") {
		s.lines[i] = scan.classify(makeLine(i+1, strings.TrimSuffix(raw, "\r"), ""))
		i++
	}
}

func (s *SourceSyntax) Len() int {
	return len(s.lines)
}

func (s *SourceSyntax) Line(i int) SourceLine {
	if i < 0 || i >= len(s.lines) {
		return SourceLine{}
	}
	return s.lines[i]
}

type sourceScan struct {
	inBlock  bool
	inScript bool
	workflow bool
	request  requestScan
	mock     mockScan
	reader   directiveReader
	effects  map[directiveKey]directiveEffect
}

type directiveKey struct {
	name       directive.Name
	spelling   directive.Name
	args       string
	inWorkflow bool
}

// clone prevents the cache from retaining the source buffer.
func (k directiveKey) clone() directiveKey {
	k.name = directive.Name(strings.Clone(string(k.name)))
	k.spelling = directive.Name(strings.Clone(string(k.spelling)))
	k.args = strings.Clone(k.args)
	return k
}

type directiveEffect struct {
	opensRequest   bool
	startsWorkflow bool
	startsMock     bool
	mockSequence   bool
}

const maxDirectiveEffects = 4096

type requestScan struct {
	open        bool
	hasMethod   bool
	headersDone bool
	contentType string
	multipart   *multipartSpan
}

type mockScan struct {
	active    bool
	sequence  bool
	hasStatus bool
}

// Keep this order in sync with documentBuilder.processLine.
func (s *sourceScan) classify(ln line) SourceLine {
	s.reader.close(ln, s.inBlock)

	switch {
	case s.mock.active:
		return s.mockLine(ln)
	case s.inBlock:
		return s.blockCommentLine(ln, false)
	case s.inScript:
		return s.scriptBlockLine(ln)
	}

	if ln.isScriptBlockStart() {
		s.openRequest()
		s.inScript = true
		return SourceLine{Kind: SourceLineLiteral}
	}

	if ln.isSeparator() {
		s.endSection()
		return SourceLine{Kind: SourceLineRequestSeparator}
	}

	if s.multipartBodyLine(ln) {
		return SourceLine{Kind: SourceLineLiteral}
	}

	if ln.isBlockCommentStart() {
		return s.blockCommentLine(ln, true)
	}

	if c, ok := ln.comment(); ok {
		syntax, call, ok := s.commentLine(ln, c)
		if ok {
			s.applyDirective(call)
		}
		return syntax
	}

	if ln.hasScriptMarker() {
		s.openRequest()
		return SourceLine{}
	}

	if s.variableLine(ln) {
		return SourceLine{}
	}

	if ln.text == "" {
		s.endHeaders()
		return SourceLine{}
	}

	if s.request.hasMethod && s.request.headersDone {
		return SourceLine{Kind: SourceLineLiteral}
	}

	switch readMethodLine(ln.raw) {
	case methodLineOpens:
		s.openMethodLine()
		return SourceLine{}
	case methodLineRejected:
		return SourceLine{}
	}

	if s.request.hasMethod {
		s.observeHeader(ln.raw)
		return SourceLine{}
	}

	s.openRequest()
	return SourceLine{}
}

func (s *sourceScan) blockCommentLine(ln line, opening bool) SourceLine {
	c, closed := ln.blockComment(opening)
	s.inBlock = !closed
	if c.text == "" {
		return sourceComment(ln, c, SourceLineComment, 0)
	}

	syntax, call, ok := s.commentLine(ln, c)
	if ok {
		s.applyDirective(call)
	}
	return syntax
}

func (s *sourceScan) scriptBlockLine(ln line) SourceLine {
	switch {
	case ln.isScriptBlockEnd():
		s.inScript = false
	case ln.isSeparator():
		s.endSection()
		return SourceLine{Kind: SourceLineRequestSeparator}
	}
	return SourceLine{Kind: SourceLineLiteral}
}

func (s *sourceScan) mockLine(ln line) SourceLine {
	if ln.isSeparator() {
		s.endSection()
		return SourceLine{Kind: SourceLineRequestSeparator}
	}

	if s.mock.sequence && s.mock.hasStatus && restfile.IsMockSequenceDelimiter(ln.text) {
		s.mock.hasStatus = false
		return SourceLine{Kind: SourceLineLiteral}
	}

	if !s.mock.hasStatus {
		return s.mockPreambleLine(ln)
	}

	return SourceLine{Kind: SourceLineLiteral}
}

func (s *sourceScan) mockPreambleLine(ln line) SourceLine {
	if ln.text == "" {
		return SourceLine{}
	}
	// Preamble directives are classified but do not change surrounding state.
	if c, ok := ln.comment(); ok {
		syntax, _, _ := s.commentLine(ln, c)
		return syntax
	}
	if status, recognized, err := parseMockStatusLine(ln.text); recognized && err == nil && status != 0 {
		s.mock.hasStatus = true
	}
	return SourceLine{Kind: SourceLineLiteral}
}

func (s *sourceScan) multipartBodyLine(ln line) bool {
	m := s.request.multipart
	return ln.text != "" && m != nil && m.bodyLine(ln.text)
}

// An unscoped variable uses request scope inside a request and file scope elsewhere.
func (s *sourceScan) variableLine(ln line) bool {
	matches := variableLineRe.FindStringSubmatch(ln.text)
	if matches == nil {
		return false
	}

	scope, _, ok := directive.ParseSecretScope(matches[1])
	if !ok {
		scope = directive.ScopeFile
		if s.request.open {
			scope = directive.ScopeRequest
		}
	}
	if scope == directive.ScopeRequest {
		s.openRequest()
	}
	return true
}

func (s *sourceScan) observeHeader(raw string) {
	name, value, ok := strings.Cut(raw, ":")
	if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Type") {
		s.request.contentType = strings.TrimSpace(value)
	}
}

func (s *sourceScan) commentLine(ln line, c commentText) (SourceLine, directive.Call, bool) {
	result := s.reader.read(ln.no, c.col(), c.text)
	switch result.kind {
	case directiveReadStarted:
		return sourceComment(ln, c, SourceLineDirective, argKind(result.owner)), directive.Call{}, false
	case directiveReadContinued:
		return sourceComment(ln, c, SourceLineDirectiveValue, argKind(result.owner)), directive.Call{}, false
	case directiveReadCompleted:
		return sourceComment(ln, c, SourceLineDirective, argKind(result.owner)), result.directive.Call, true
	case directiveReadContinuationCompleted:
		return sourceComment(ln, c, SourceLineDirectiveValue, argKind(result.owner)), result.directive.Call, true
	default:
		return sourceComment(ln, c, SourceLineComment, 0), directive.Call{}, false
	}
}

func argKind(name directive.Name) directive.ArgKind {
	spec, ok := directive.Lookup(name)
	if !ok {
		return directive.ArgToken
	}
	return spec.Args
}

func (s *sourceScan) applyDirective(call directive.Call) {
	// @workflow can close an open request; other directives cannot.
	if call.Name == directive.Workflow {
		if s.effectOf(call, false).startsWorkflow {
			s.request = requestScan{}
			s.workflow = true
		}
		return
	}

	if s.request.open {
		return
	}

	effect := s.effectOf(call, s.workflow)
	switch {
	case effect.startsMock:
		s.mock = mockScan{active: true, sequence: effect.mockSequence}
	case effect.opensRequest:
		s.openRequest()
	}
}

func (s *sourceScan) effectOf(call directive.Call, inWorkflow bool) directiveEffect {
	if !call.Name.Known() {
		return directiveEffect{}
	}

	if strings.Contains(call.Args, "\n") {
		return probeEffect(call, inWorkflow)
	}

	key := directiveKey{
		name:       call.Name,
		spelling:   call.Spelling,
		args:       call.Args,
		inWorkflow: inWorkflow,
	}
	if effect, ok := s.effects[key]; ok {
		return effect
	}

	effect := probeEffect(call, inWorkflow)
	s.effects[key.clone()] = effect
	return effect
}

// Run directives through the parser so classification follows the same rules.
func probeEffect(call directive.Call, inWorkflow bool) directiveEffect {
	probe := documentBuilder{doc: &restfile.Document{}}
	if inWorkflow {
		probe.workflow = newWorkflowBuilder(1, "probe")
	}
	probe.routeDirective(parsedDirective{
		Call:  call,
		lines: restfile.LineRange{Start: 1, End: 1},
	})

	effect := directiveEffect{
		opensRequest:   probe.inRequest,
		startsWorkflow: !inWorkflow && probe.workflow != nil,
		startsMock:     probe.mock != nil,
	}
	if effect.startsMock {
		effect.mockSequence = probe.mock.sequence != ""
	}
	return effect
}

func (s *sourceScan) openRequest() {
	s.request.open = true
	s.workflow = false
}

func (s *sourceScan) openMethodLine() {
	s.openRequest()
	s.request.hasMethod = true
	s.request.headersDone = false
	s.request.contentType = ""
	s.request.multipart = nil
}

func (s *sourceScan) endHeaders() {
	if !s.request.hasMethod || s.request.headersDone {
		return
	}
	s.request.headersDone = true
	if restfile.IsMultipartMime(s.request.contentType) {
		s.request.multipart = newMultipartSpan(s.request.contentType)
	}
}

// Keep cached directive results between sections.
func (s *sourceScan) endSection() {
	*s = sourceScan{effects: s.effects}
}

type methodLineKind uint8

const (
	notMethodLine methodLineKind = iota
	methodLineOpens
	methodLineRejected
)

// Match handleMethodLine, including request lines that are rejected.
func readMethodLine(raw string) methodLineKind {
	if grpcbuilder.IsMethodLine(raw) {
		return methodLineOpens
	}

	_, ok, err := httpbuilder.ParseMethodLine(raw)
	if !ok && err == nil {
		_, ok, err = httpbuilder.ParseWebSocketURLLine(raw)
	}
	switch {
	case err != nil:
		return methodLineRejected
	case ok:
		return methodLineOpens
	default:
		return notMethodLine
	}
}

func sourceComment(ln line, c commentText, kind SourceLineKind, args directive.ArgKind) SourceLine {
	start := utf8.RuneCountInString(ln.raw[:c.start])
	return SourceLine{
		Kind:         kind,
		Args:         args,
		ContentStart: start,
		ContentEnd:   start + utf8.RuneCountInString(ln.raw[c.start:c.end]),
	}
}
