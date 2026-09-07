package parser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/delay"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
)

func (b *documentBuilder) addMockError(line int, msg string) {
	b.pushMockError(lineDiagnostic(line, msg))
}

func (b *documentBuilder) failMock(d parsedDirective, msg string) {
	b.pushMockError(d.diagnostic(msg, nil))
}

func (b *documentBuilder) reportMock(d parsedDirective, err error) {
	b.pushMockError(d.diagnostic(err.Error(), err))
}

func (b *documentBuilder) pushMockError(item restfile.ParseDiagnostic) {
	item.Mock = true
	b.pushError(item)
}

// One error per key, so a typo next to a usable option still names what broke.
func (b *documentBuilder) checkMockOptions(d parsedDirective, vals directive.Options, known ...string) {
	for _, key := range vals.Keys() {
		if !slices.Contains(known, key) {
			b.reportMock(d, directive.UnknownOption(d.Name, key))
		}
	}
}

type mockBuilder struct {
	startLine            int
	endLine              int
	title                string
	method               string
	path                 string
	name                 string
	sequence             string
	sequenceKey          restfile.MockSequenceKey
	latency              delay.Spec
	isDefault            bool
	disableInterpolation bool
	match                restfile.MockMatch
	expectation          *restfile.MockExpectation
	responses            []restfile.MockResponse
	status               int
	headers              http.Header
	inBody               bool
	body                 []string
	delimLine            int
}

func (b *documentBuilder) handleMockDirective(d parsedDirective) directiveOutcome {
	switch d.Name {
	case directive.Mock:
		if b.inRequest {
			b.failMock(d, "@mock must start a new block after a ### separator")
			return directiveRejected
		}
		if b.workflow != nil {
			b.failMock(d, "@mock cannot be declared inside a workflow")
			return directiveRejected
		}
		b.startMock(d)
		return directiveApplied
	case directive.Match, directive.Expect:
		b.failMock(d, d.Name.Tag()+" must follow an @mock directive")
		return directiveRejected
	default:
		return directiveIgnored
	}
}

func (b *documentBuilder) startMock(d parsedDirective) {
	vals, err := directive.ParseOptions(directive.Mock, d.Args)
	if err != nil {
		b.reportMock(d, err)
	}
	b.checkMockOptions(
		d, vals,
		"method", "path", "name", "sequence", "sequence-key", "default", "latency", "interpolate",
	)

	line := d.lines.Start
	m := &mockBuilder{
		startLine: line,
		endLine:   line,
		title:     b.pendingTitle,
		method:    strings.ToUpper(vals.Get("method")),
		path:      vals.Get("path"),
		name:      vals.Get("name"),
		sequence:  vals.Get("sequence"),
		headers:   make(http.Header),
		match: restfile.MockMatch{
			Query:   make(map[string]restfile.MockQueryRule),
			Headers: make(map[string]restfile.MockHeaderRule),
		},
	}
	b.pendingTitle = ""
	b.checkMockRoute(d, m)
	if vals.Has("sequence") && m.sequence == "" {
		b.failMock(d, "@mock sequence name cannot be empty")
	}
	if raw, ok := vals.Lookup("sequence-key"); ok {
		if m.sequence == "" {
			b.failMock(d, "@mock sequence-key requires sequence")
		} else if key, err := parseMockSequenceKey(raw, m.path); err != nil {
			b.failMock(d, "@mock sequence-key "+err.Error())
		} else {
			m.sequenceKey = key
		}
	}

	if v, ok := b.mockBool(d, vals, "default"); ok {
		m.isDefault = v
	}
	if raw, ok := vals.Lookup("latency"); ok {
		spec, err := delay.Parse(raw)
		if err != nil {
			b.failMock(d, "@mock latency "+err.Error())
		} else {
			m.latency = spec
		}
	}
	if v, ok := b.mockBool(d, vals, "interpolate"); ok {
		m.disableInterpolation = !v
	}
	b.mock = m
}

func (b *documentBuilder) mockBool(d parsedDirective, vals directive.Options, key string) (bool, bool) {
	raw, ok := vals.Lookup(key)
	if !ok {
		return false, false
	}
	v, ok := directive.ParseBool(raw)
	if !ok {
		b.failMock(d, fmt.Sprintf("@mock %s must be true or false", key))
		return false, false
	}
	return v, true
}

func (b *documentBuilder) checkMockRoute(d parsedDirective, m *mockBuilder) {
	if m.method == "" {
		b.failMock(d, "@mock method is required")
	} else if !httpguts.ValidHeaderFieldName(m.method) {
		b.failMock(d, fmt.Sprintf("invalid @mock method %q", m.method))
	}
	if m.path == "" {
		b.failMock(d, "@mock path is required")
	} else if err := restfile.ValidateMockPath(m.path); err != nil {
		b.reportMock(d, err)
	}
	if m.name != "" && !restfile.ValidMockName(m.name) {
		b.failMock(d, "@mock name may contain only letters, digits, '.', '_' and '-'")
	}
	if m.sequence != "" && !restfile.ValidMockName(m.sequence) {
		b.failMock(d, "@mock sequence may contain only letters, digits, '.', '_' and '-'")
	}
	if m.name != "" && m.sequence != "" {
		b.failMock(d, "@mock name and sequence cannot be combined")
	}
}

func (b *documentBuilder) handleMockBlockLine(ln line) {
	m := b.mock
	if ln.isSeparator() {
		m.trimStructuralBlankLine()
		b.handleSeparator(ln)
		return
	}

	m.endLine = ln.no
	if m.sequence != "" && restfile.IsMockSequenceDelimiter(ln.text) {
		m.delimLine = ln.no
		if !m.started() {
			b.addMockError(ln.no, "@mock sequence has an empty response")
			return
		}
		m.trimStructuralBlankLine()
		m.finishResponse(b, ln.no)
		return
	}
	switch {
	case m.inBody:
		m.body = append(m.body, ln.raw)
	case m.status == 0:
		m.parsePreamble(b, ln)
	case ln.text == "":
		m.inBody = true
	default:
		m.addHeader(b, ln.no, ln.raw)
	}
}

func (m *mockBuilder) parsePreamble(b *documentBuilder, ln line) {
	if ln.text == "" {
		return
	}
	if c, ok := ln.comment(); ok {
		if d, ok := b.readDirective(ln.no, c); ok {
			m.declare(b, d)
		}
		return
	}

	status, recognized, err := parseMockStatusLine(ln.text)
	if !recognized {
		b.addMockError(ln.no, "expected an HTTP response status line in @mock block")
	} else if err != nil {
		b.addMockError(ln.no, err.Error())
	} else {
		m.status = status
	}
}

func (m *mockBuilder) declare(b *documentBuilder, d parsedDirective) {
	switch {
	case d.Name == directive.Match && len(m.responses) == 0:
		m.addMatch(b, d)
	case d.Name == directive.Expect && len(m.responses) == 0:
		m.addExpectation(b, d)
	case d.Name == directive.Match || d.Name == directive.Expect:
		b.failMock(d, d.Name.Tag()+" must be declared before the first sequence response")
	default:
		b.failMock(d, fmt.Sprintf("directive %s is not valid before a mock response", d.Spelling.Tag()))
	}
}

func (m *mockBuilder) addHeader(b *documentBuilder, ln int, line string) {
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		b.addMockError(ln, "folded response headers are not supported")
		return
	}
	name, value, ok := strings.Cut(line, ":")
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if !ok || !httpguts.ValidHeaderFieldName(name) {
		b.addMockError(ln, "invalid mock response header")
		return
	}
	if !httpguts.ValidHeaderFieldValue(value) {
		b.addMockError(ln, fmt.Sprintf("invalid value for mock response header %q", name))
		return
	}
	m.headers.Add(name, value)
}

func (m *mockBuilder) addMatch(b *documentBuilder, d parsedDirective) {
	vals, err := directive.ParseOptions(directive.Match, d.Args)
	if err != nil {
		b.reportMock(d, err)
	}
	b.checkMockOptions(d, vals, "query", "headers", "json", "json-rules")

	if raw, ok := vals.Lookup("query"); ok {
		addMatchers(b, d, "query", raw, m.match.Query, canonQueryMatcher)
	}
	if raw, ok := vals.Lookup("headers"); ok {
		addMatchers(b, d, "headers", raw, m.match.Headers, canonHeaderMatcher)
	}
	if raw, ok := vals.Lookup("json"); ok {
		compact, err := compactJSON(raw)
		b.setMockJSON(d, "json", &m.match.JSON, compact, jsonValueError(raw, err))
	}
	if raw, ok := vals.Lookup("json-rules"); ok {
		compact, err := compactJSONObject(raw)
		b.setMockJSON(d, "json-rules", &m.match.JSONRules, compact, err)
	}
}

func (b *documentBuilder) setMockJSON(d parsedDirective, opt string, dst *[]byte, compact []byte, err error) {
	if err != nil {
		b.failMock(d, "invalid @match "+opt+": "+err.Error())
		return
	}
	if len(*dst) == 0 {
		*dst = sortMockJSONFields(compact)
		return
	}
	merged, err := mergeMockJSON(*dst, compact)
	if err != nil {
		b.failMock(d, "@match "+opt+" "+err.Error())
		return
	}
	*dst = merged
}

func (m *mockBuilder) addExpectation(b *documentBuilder, d parsedDirective) {
	vals, err := directive.ParseOptions(directive.Expect, d.Args)
	if err != nil {
		b.reportMock(d, err)
	}
	b.checkMockOptions(d, vals, "calls")

	if m.expectation != nil {
		b.failMock(d, "@expect is already defined for this mock")
		return
	}
	calls := vals.Get("calls")
	if calls == "" {
		b.failMock(d, "@expect calls is required")
		return
	}
	n, err := strconv.ParseUint(calls, 10, 64)
	if err != nil {
		b.failMock(d, "@expect calls must be a non-negative integer")
		return
	}
	m.expectation = &restfile.MockExpectation{Calls: n, Line: d.lines.Start}
}

// Rules are decoded independently so one bad matcher does not discard the valid
// ones. Sort the keys first so reparsing produces diagnostics in the same order
// instead of exposing map iteration order.
func addMatchers[T restfile.MockQueryRule | restfile.MockHeaderRule](
	b *documentBuilder,
	d parsedDirective,
	opt string,
	raw string,
	dst map[string]T,
	canon func(string) (string, error),
) {
	fields, err := parseJSONObject(raw)
	if err != nil {
		b.failMock(d, fmt.Sprintf("invalid @match %s: %s", opt, err))
		return
	}
	for _, key := range util.SortedKeys(fields) {
		name := strings.TrimSpace(key)
		if name == "" {
			b.failMock(d, fmt.Sprintf("@match %s name cannot be empty", opt))
			continue
		}
		name, err := canon(name)
		if err != nil {
			b.reportMock(d, err)
			continue
		}
		if _, exists := dst[name]; exists {
			b.failMock(d, fmt.Sprintf("@match %s %q is repeated", opt, name))
			continue
		}
		var rule T
		if err := json.Unmarshal(fields[key], &rule); err != nil {
			b.failMock(d, fmt.Sprintf("invalid @match %s: matcher for %q: %s", opt, key, err))
			continue
		}
		dst[name] = rule
	}
}

// Query names are case sensitive, so they are stored as written.
func canonQueryMatcher(name string) (string, error) {
	return name, nil
}

func canonHeaderMatcher(name string) (string, error) {
	if !httpguts.ValidHeaderFieldName(name) {
		return "", fmt.Errorf("invalid @match header name %q", name)
	}
	return http.CanonicalHeaderKey(name), nil
}

func (b *documentBuilder) flushMock() {
	if b.mock == nil {
		return
	}
	m := b.mock
	if m.delimLine > 0 && !m.started() {
		b.addMockError(m.delimLine, "@mock sequence ends with a dangling delimiter")
	}
	if m.started() || len(m.responses) == 0 {
		m.finishResponse(b, m.endLine)
	}
	if m.sequence != "" && len(m.responses) < 2 {
		b.addMockError(m.endLine, "@mock sequence must define at least two responses")
	}
	b.doc.Mocks = append(b.doc.Mocks, &restfile.Mock{
		Title:                m.title,
		Name:                 m.name,
		Sequence:             m.sequence,
		SequenceKey:          m.sequenceKey,
		Method:               m.method,
		Path:                 m.path,
		Latency:              m.latency,
		Default:              m.isDefault,
		Match:                m.match,
		Expectation:          m.expectation,
		Responses:            m.responses,
		DisableInterpolation: m.disableInterpolation,
		LineRange:            restfile.LineRange{Start: m.startLine, End: m.endLine},
	})
	b.mock = nil
}

func (m *mockBuilder) finishResponse(b *documentBuilder, line int) {
	if m.status == 0 {
		b.addMockError(line, "@mock response status line is missing")
	}
	body := restfile.BodySource{MimeType: m.headers.Get("Content-Type")}
	if len(m.body) > 0 {
		file, ok := parseHTTPBodyFile(m.body[0], false)
		if ok && util.AllBlank(m.body[1:]) {
			body.FilePath = file
		} else {
			body.Text = strings.Join(m.body, "\n")
		}
	}
	m.responses = append(m.responses, restfile.MockResponse{
		Status:  m.status,
		Headers: m.headers,
		Body:    body,
	})
	m.status = 0
	m.headers = make(http.Header)
	m.inBody = false
	m.body = nil
}

// started reports whether the current response has begun accumulating, so a
// stray '---' is reported instead of finalizing a phantom empty response.
func (m *mockBuilder) started() bool {
	return m.status != 0 || len(m.body) > 0
}

func (m *mockBuilder) trimStructuralBlankLine() {
	if last := len(m.body) - 1; last >= 0 && m.body[last] == "" {
		m.body = m.body[:last]
	}
}

func parseMockStatusLine(line string) (int, bool, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 || !strings.HasPrefix(strings.ToUpper(fields[0]), "HTTP/") {
		return 0, false, nil
	}
	version := strings.ToUpper(fields[0])
	if len(fields) < 2 || version != "HTTP/1.0" && version != "HTTP/1.1" {
		return 0, true, fmt.Errorf("invalid mock response status line")
	}
	if len(fields[1]) != 3 {
		return 0, true, fmt.Errorf("mock response status must be a three-digit number between 200 and 599")
	}
	status, err := strconv.Atoi(fields[1])
	if err != nil || !restfile.ValidMockStatus(status) {
		return 0, true, fmt.Errorf("mock response status must be between 200 and 599")
	}
	return status, true, nil
}

func parseMockSequenceKey(raw, path string) (restfile.MockSequenceKey, error) {
	key, err := restfile.ParseMockSequenceKey(raw)
	if err != nil {
		return restfile.MockSequenceKey{}, err
	}
	var params map[string]string
	if key.Source == restfile.MockSequenceKeySourcePath {
		if _, params, err = restfile.CompileMockPath(path); err != nil {
			return restfile.MockSequenceKey{}, err
		}
	}
	return key.Check(params)
}
