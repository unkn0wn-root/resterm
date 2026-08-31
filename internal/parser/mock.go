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

func (b *documentBuilder) addMockError(line int, message string) {
	b.doc.Errors = append(b.doc.Errors, restfile.ParseError{Line: line, Message: message, Mock: true})
}

// One error per key, so a typo next to a usable option still names what broke.
func (b *documentBuilder) checkMockOptions(
	line int,
	name directive.Name,
	vals directive.Options,
	known ...string,
) {
	for _, key := range vals.Keys() {
		if !slices.Contains(known, key) {
			b.addMockError(line, fmt.Sprintf("unknown %s option %q", name.Tag(), key))
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
			b.addMockError(d.lines.Start, "@mock must start a new block after a ### separator")
			return directiveRejected
		}
		if b.workflow != nil {
			b.addMockError(d.lines.Start, "@mock cannot be declared inside a workflow")
			return directiveRejected
		}
		b.startMock(d.lines.Start, d.Args)
		return directiveApplied
	case directive.Match, directive.Expect:
		b.addMockError(d.lines.Start, d.Name.Tag()+" must follow an @mock directive")
		return directiveRejected
	default:
		return directiveIgnored
	}
}

func (b *documentBuilder) startMock(line int, raw string) {
	vals, err := directive.ParseOptions(directive.Mock, raw)
	if err != nil {
		b.addMockError(line, err.Error())
	}
	b.checkMockOptions(
		line, directive.Mock, vals,
		"method", "path", "name", "sequence", "sequence-key", "default", "latency", "interpolate",
	)

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
	b.checkMockRoute(line, m)
	if vals.Has("sequence") && m.sequence == "" {
		b.addMockError(line, "@mock sequence name cannot be empty")
	}
	if raw, ok := vals.Lookup("sequence-key"); ok {
		if m.sequence == "" {
			b.addMockError(line, "@mock sequence-key requires sequence")
		} else if key, err := parseMockSequenceKey(raw, m.path); err != nil {
			b.addMockError(line, "@mock sequence-key "+err.Error())
		} else {
			m.sequenceKey = key
		}
	}

	if v, ok := b.mockBool(line, vals, "default"); ok {
		m.isDefault = v
	}
	if raw, ok := vals.Lookup("latency"); ok {
		spec, err := delay.Parse(raw)
		if err != nil {
			b.addMockError(line, "@mock latency "+err.Error())
		} else {
			m.latency = spec
		}
	}
	if v, ok := b.mockBool(line, vals, "interpolate"); ok {
		m.disableInterpolation = !v
	}
	b.mock = m
}

func (b *documentBuilder) mockBool(line int, vals directive.Options, key string) (bool, bool) {
	raw, ok := vals.Lookup(key)
	if !ok {
		return false, false
	}
	v, ok := directive.ParseBool(raw)
	if !ok {
		b.addMockError(line, fmt.Sprintf("@mock %s must be true or false", key))
		return false, false
	}
	return v, true
}

func (b *documentBuilder) checkMockRoute(line int, m *mockBuilder) {
	if m.method == "" {
		b.addMockError(line, "@mock method is required")
	} else if !httpguts.ValidHeaderFieldName(m.method) {
		b.addMockError(line, fmt.Sprintf("invalid @mock method %q", m.method))
	}
	if m.path == "" {
		b.addMockError(line, "@mock path is required")
	} else if err := restfile.ValidateMockPath(m.path); err != nil {
		b.addMockError(line, err.Error())
	}
	if m.name != "" && !restfile.ValidMockName(m.name) {
		b.addMockError(line, "@mock name may contain only letters, digits, '.', '_' and '-'")
	}
	if m.sequence != "" && !restfile.ValidMockName(m.sequence) {
		b.addMockError(line, "@mock sequence may contain only letters, digits, '.', '_' and '-'")
	}
	if m.name != "" && m.sequence != "" {
		b.addMockError(line, "@mock name and sequence cannot be combined")
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
		if d, ok := b.readDirective(ln.no, c.col(), c.text); ok {
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
		m.addMatch(b, d.lines.Start, d.Args)
	case d.Name == directive.Expect && len(m.responses) == 0:
		m.addExpectation(b, d.lines.Start, d.Args)
	case d.Name == directive.Match || d.Name == directive.Expect:
		b.addMockError(d.lines.Start, d.Name.Tag()+" must be declared before the first sequence response")
	default:
		b.addMockError(
			d.lines.Start,
			fmt.Sprintf("directive %s is not valid before a mock response", d.Spelling.Tag()),
		)
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

func (m *mockBuilder) addMatch(b *documentBuilder, line int, raw string) {
	vals, err := directive.ParseOptions(directive.Match, raw)
	if err != nil {
		b.addMockError(line, err.Error())
	}
	b.checkMockOptions(line, directive.Match, vals, "query", "headers", "json", "json-rules")

	if raw, ok := vals.Lookup("query"); ok {
		addMatchers(b, line, "query", raw, m.match.Query, canonQueryMatcher)
	}
	if raw, ok := vals.Lookup("headers"); ok {
		addMatchers(b, line, "headers", raw, m.match.Headers, canonHeaderMatcher)
	}
	if raw, ok := vals.Lookup("json"); ok {
		compact, err := compactJSON(raw)
		b.setMockJSON(line, "json", &m.match.JSON, compact, jsonValueError(raw, err))
	}
	if raw, ok := vals.Lookup("json-rules"); ok {
		compact, err := compactJSONObject(raw)
		b.setMockJSON(line, "json-rules", &m.match.JSONRules, compact, err)
	}
}

func (b *documentBuilder) setMockJSON(line int, opt string, dst *[]byte, compact []byte, err error) {
	if err != nil {
		b.addMockError(line, "invalid @match "+opt+": "+err.Error())
		return
	}
	if len(*dst) == 0 {
		*dst = sortMockJSONFields(compact)
		return
	}
	merged, err := mergeMockJSON(*dst, compact)
	if err != nil {
		b.addMockError(line, "@match "+opt+" "+err.Error())
		return
	}
	*dst = merged
}

func (m *mockBuilder) addExpectation(b *documentBuilder, line int, raw string) {
	vals, err := directive.ParseOptions(directive.Expect, raw)
	if err != nil {
		b.addMockError(line, err.Error())
	}
	b.checkMockOptions(line, directive.Expect, vals, "calls")

	if m.expectation != nil {
		b.addMockError(line, "@expect is already defined for this mock")
		return
	}
	calls := vals.Get("calls")
	if calls == "" {
		b.addMockError(line, "@expect calls is required")
		return
	}
	n, err := strconv.ParseUint(calls, 10, 64)
	if err != nil {
		b.addMockError(line, "@expect calls must be a non-negative integer")
		return
	}
	m.expectation = &restfile.MockExpectation{Calls: n, Line: line}
}

// Rules are decoded independently so one bad matcher does not discard the valid
// ones. Sort the keys first so reparsing produces diagnostics in the same order
// instead of exposing map iteration order.
func addMatchers[T restfile.MockQueryRule | restfile.MockHeaderRule](
	b *documentBuilder,
	line int,
	opt string,
	raw string,
	dst map[string]T,
	canon func(string) (string, error),
) {
	fields, err := parseJSONObject(raw)
	if err != nil {
		b.addMockError(line, fmt.Sprintf("invalid @match %s: %s", opt, err))
		return
	}
	for _, key := range util.SortedKeys(fields) {
		name := strings.TrimSpace(key)
		if name == "" {
			b.addMockError(line, fmt.Sprintf("@match %s name cannot be empty", opt))
			continue
		}
		name, err := canon(name)
		if err != nil {
			b.addMockError(line, err.Error())
			continue
		}
		if _, exists := dst[name]; exists {
			b.addMockError(line, fmt.Sprintf("@match %s %q is repeated", opt, name))
			continue
		}
		var rule T
		if err := json.Unmarshal(fields[key], &rule); err != nil {
			b.addMockError(line, fmt.Sprintf("invalid @match %s: matcher for %q: %s", opt, key, err))
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
