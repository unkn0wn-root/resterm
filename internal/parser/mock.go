package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/value"
	"github.com/unkn0wn-root/resterm/internal/parser/lexer"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
)

func (b *documentBuilder) addMockError(line int, message string) {
	b.doc.Errors = append(b.doc.Errors, restfile.ParseError{Line: line, Message: message, Mock: true})
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
	latency              time.Duration
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

func (b *documentBuilder) handleMockDirective(line int, key, raw string) bool {
	switch key {
	case "mock":
		if b.inRequest {
			b.addMockError(line, "@mock must start a new block after a ### separator")
			return true
		}
		if b.workflow != nil {
			b.addMockError(line, "@mock cannot be declared inside a workflow")
			return true
		}
		b.startMock(line, raw)
		return true
	case "match", "expect":
		b.addMockError(line, "@"+key+" must follow an @mock directive")
		return true
	default:
		return false
	}
}

func (b *documentBuilder) startMock(line int, raw string) {
	vals := options.Parse(raw)
	for _, key := range util.SortedKeys(vals) {
		switch key {
		case "method", "path", "name", "sequence", "sequence-key", "default", "latency", "interpolate":
		default:
			b.addMockError(line, fmt.Sprintf("unknown @mock option %q", key))
		}
	}

	m := &mockBuilder{
		startLine: line,
		endLine:   line,
		title:     b.pendingTitle,
		method:    strings.ToUpper(strings.TrimSpace(vals["method"])),
		path:      strings.TrimSpace(vals["path"]),
		name:      strings.TrimSpace(vals["name"]),
		sequence:  strings.TrimSpace(vals["sequence"]),
		headers:   make(http.Header),
		match: restfile.MockMatch{
			Query:   make(map[string]restfile.StringList),
			Headers: make(map[string]restfile.MockHeaderRule),
		},
	}
	b.pendingTitle = ""
	b.checkMockRoute(line, m)
	if _, ok := vals["sequence"]; ok && m.sequence == "" {
		b.addMockError(line, "@mock sequence name cannot be empty")
	}
	if raw, ok := vals["sequence-key"]; ok {
		if m.sequence == "" {
			b.addMockError(line, "@mock sequence-key requires sequence")
		} else if key, err := parseMockSequenceKey(raw, m.path); err != nil {
			b.addMockError(line, "@mock sequence-key "+err.Error())
		} else {
			m.sequenceKey = key
		}
	}

	if raw, ok := vals["default"]; ok {
		if v, ok := value.ParseBool(raw); ok {
			m.isDefault = v
		} else {
			b.addMockError(line, "@mock default must be true or false")
		}
	}
	if raw, ok := vals["latency"]; ok {
		v, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil || v < 0 {
			b.addMockError(line, "@mock latency must be a non-negative Go duration")
		} else {
			m.latency = v
		}
	}
	if raw, ok := vals["interpolate"]; ok {
		if v, ok := value.ParseBool(raw); ok {
			m.disableInterpolation = !v
		} else {
			b.addMockError(line, "@mock interpolate must be true or false")
		}
	}
	b.mock = m
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
		m.parsePreamble(b, ln.no, ln.text)
	case ln.text == "":
		m.inBody = true
	default:
		m.addHeader(b, ln.no, ln.raw)
	}
}

func (m *mockBuilder) parsePreamble(b *documentBuilder, line int, s string) {
	if s == "" {
		return
	}
	if text, _, ok := stripComment(s); ok {
		if !strings.HasPrefix(text, "@") {
			return
		}
		key, raw := lexer.SplitDirective(strings.TrimSpace(text[1:]))
		switch {
		case key == "match" && len(m.responses) == 0:
			m.addMatch(b, line, raw)
		case key == "expect" && len(m.responses) == 0:
			m.addExpectation(b, line, raw)
		case key == "match" || key == "expect":
			b.addMockError(line, "@"+key+" must be declared before the first sequence response")
		default:
			b.addMockError(line, fmt.Sprintf("directive @%s is not valid before a mock response", key))
		}
		return
	}

	status, recognized, err := parseMockStatusLine(s)
	if !recognized {
		b.addMockError(line, "expected an HTTP response status line in @mock block")
	} else if err != nil {
		b.addMockError(line, err.Error())
	} else {
		m.status = status
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
	vals := options.Parse(raw)
	for _, key := range util.SortedKeys(vals) {
		switch key {
		case "query", "headers", "json":
		default:
			b.addMockError(line, fmt.Sprintf("unknown @match option %q", key))
		}
	}
	if raw, ok := vals["query"]; ok {
		m.addStringMatchers(b, line, "query", raw, m.match.Query)
	}
	if raw, ok := vals["headers"]; ok {
		m.addHeaderMatchers(b, line, raw)
	}
	if raw, ok := vals["json"]; ok {
		if len(m.match.JSON) > 0 {
			b.addMockError(line, "@match json is already defined for this mock")
		} else if compact, err := compactJSON(raw); err != nil {
			b.addMockError(line, "invalid @match json: "+err.Error())
		} else {
			m.match.JSON = compact
		}
	}
}

func (m *mockBuilder) addExpectation(b *documentBuilder, line int, raw string) {
	vals := options.Parse(raw)
	for _, key := range util.SortedKeys(vals) {
		if key != "calls" {
			b.addMockError(line, fmt.Sprintf("unknown @expect option %q", key))
		}
	}
	if m.expectation != nil {
		b.addMockError(line, "@expect is already defined for this mock")
		return
	}
	calls := strings.TrimSpace(vals["calls"])
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

func (m *mockBuilder) addStringMatchers(
	b *documentBuilder,
	line int,
	kind, raw string,
	dst map[string]restfile.StringList,
) {
	vals, err := parseStringListMap(raw)
	if err != nil {
		b.addMockError(line, fmt.Sprintf("invalid @match %s: %s", kind, err))
		return
	}
	for _, k := range util.SortedKeys(vals) {
		name := strings.TrimSpace(k)
		if name == "" {
			b.addMockError(line, fmt.Sprintf("@match %s name cannot be empty", kind))
			continue
		}
		if _, ok := dst[name]; ok {
			b.addMockError(line, fmt.Sprintf("@match %s %q is repeated", kind, name))
			continue
		}
		dst[name] = vals[k]
	}
}

func (m *mockBuilder) addHeaderMatchers(b *documentBuilder, line int, raw string) {
	vals, err := parseMockHeaderRules(raw)
	if err != nil {
		b.addMockError(line, "invalid @match headers: "+err.Error())
		return
	}
	for _, key := range util.SortedKeys(vals) {
		name := strings.TrimSpace(key)
		if name == "" {
			b.addMockError(line, "@match headers name cannot be empty")
			continue
		}
		canon, ok := b.canonMatchHeader(line, name, vals[key])
		if !ok {
			continue
		}
		if _, exists := m.match.Headers[canon]; exists {
			b.addMockError(line, fmt.Sprintf("@match headers %q is repeated", canon))
			continue
		}
		m.match.Headers[canon] = vals[key]
	}
}

func (b *documentBuilder) canonMatchHeader(
	line int,
	key string,
	rule restfile.MockHeaderRule,
) (string, bool) {
	if !httpguts.ValidHeaderFieldName(key) {
		b.addMockError(line, fmt.Sprintf("invalid @match header name %q", key))
		return "", false
	}
	key = http.CanonicalHeaderKey(key)
	for _, v := range rule.Values {
		if !httpguts.ValidHeaderFieldValue(v) {
			b.addMockError(line, fmt.Sprintf("invalid @match header value for %q", key))
			return "", false
		}
	}
	return key, true
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

func parseStringListMap(raw string) (map[string]restfile.StringList, error) {
	fields, err := parseJSONObject(raw)
	if err != nil {
		return nil, err
	}
	out := make(map[string]restfile.StringList, len(fields))
	for key, value := range fields {
		var values restfile.StringList
		if err := json.Unmarshal(value, &values); err != nil {
			return nil, fmt.Errorf("value for %q must be a string or string array", key)
		}
		out[key] = values
	}
	return out, nil
}

func parseMockHeaderRules(raw string) (map[string]restfile.MockHeaderRule, error) {
	fields, err := parseJSONObject(raw)
	if err != nil {
		return nil, err
	}
	out := make(map[string]restfile.MockHeaderRule, len(fields))
	for name, value := range fields {
		var rule restfile.MockHeaderRule
		if err := json.Unmarshal(value, &rule); err != nil {
			return nil, fmt.Errorf("matcher for %q: %w", name, err)
		}
		out[name] = rule
	}
	return out, nil
}

func parseJSONObject(raw string) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	return obj, nil
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

func compactJSON(raw string) ([]byte, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(raw)); err != nil {
		return nil, err
	}
	return compact.Bytes(), nil
}
