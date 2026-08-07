package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"

	"github.com/unkn0wn-root/resterm/internal/directive"
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
	pending              *pendingMatch
	expectation          *restfile.MockExpectation
	responses            []restfile.MockResponse
	status               int
	headers              http.Header
	inBody               bool
	body                 []string
	delimLine            int
}

func (b *documentBuilder) handleMockDirective(
	line int,
	name directive.Name,
	raw string,
) bool {
	switch name {
	case directive.Mock:
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
	case directive.Match, directive.Expect:
		b.addMockError(line, name.Tag()+" must follow an @mock directive")
		return true
	default:
		return false
	}
}

func (b *documentBuilder) startMock(line int, raw string) {
	vals, err := directive.ParseOptions(directive.Mock, raw)
	if err != nil {
		b.addMockError(line, err.Error())
	}
	for _, key := range vals.Keys() {
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

	if raw, ok := vals.Lookup("default"); ok {
		if v, ok := directive.ParseBool(raw); ok {
			m.isDefault = v
		} else {
			b.addMockError(line, "@mock default must be true or false")
		}
	}
	if raw, ok := vals.Lookup("latency"); ok {
		v, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil || v < 0 {
			b.addMockError(line, "@mock latency must be a non-negative Go duration")
		} else {
			m.latency = v
		}
	}
	if raw, ok := vals.Lookup("interpolate"); ok {
		if v, ok := directive.ParseBool(raw); ok {
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
	if m.pending != nil && m.continueMatch(b, s) {
		return
	}
	if s == "" {
		return
	}
	if text, _, ok := stripComment(s); ok {
		call, ok := directive.Parse(text)
		if !ok {
			return
		}
		switch {
		case call.Name == directive.Match && len(m.responses) == 0:
			m.addMatch(b, line, call.Args)
		case call.Name == directive.Expect && len(m.responses) == 0:
			m.addExpectation(b, line, call.Args)
		case call.Name == directive.Match || call.Name == directive.Expect:
			b.addMockError(line, call.Name.Tag()+" must be declared before the first sequence response")
		default:
			b.addMockError(
				line,
				fmt.Sprintf("directive %s is not valid before a mock response", call.Spelling.Tag()),
			)
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

type pendingMatch struct {
	line int
	args string
}

func (m *mockBuilder) continueMatch(b *documentBuilder, s string) bool {
	text, _, ok := stripComment(s)
	if !ok {
		b.addMockError(m.pending.line, "@match option is missing a closing bracket")
		m.pending = nil
		return false
	}
	open := m.pending
	m.pending = nil
	m.addMatch(b, open.line, open.args+" "+text)
	return true
}

func (m *mockBuilder) addMatch(b *documentBuilder, line int, raw string) {
	if directive.OptionsOpen(raw) {
		m.pending = &pendingMatch{line: line, args: raw}
		return
	}

	vals, err := directive.ParseOptions(directive.Match, raw)
	if err != nil {
		b.addMockError(line, err.Error())
	}
	for _, key := range vals.Keys() {
		switch key {
		case "query", "headers", "json", "json-rules":
		default:
			b.addMockError(line, fmt.Sprintf("unknown @match option %q", key))
		}
	}
	if raw, ok := vals.Lookup("query"); ok {
		m.addQueryMatchers(b, line, raw)
	}
	if raw, ok := vals.Lookup("headers"); ok {
		m.addHeaderMatchers(b, line, raw)
	}
	if raw, ok := vals.Lookup("json"); ok {
		compact, err := compactJSON(raw)
		b.setMockJSON(line, "json", &m.match.JSON, compact, err)
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
	for _, key := range vals.Keys() {
		if key != "calls" {
			b.addMockError(line, fmt.Sprintf("unknown @expect option %q", key))
		}
	}
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

func (m *mockBuilder) addQueryMatchers(b *documentBuilder, line int, raw string) {
	vals, err := parseMockRules[restfile.MockQueryRule](raw)
	if err != nil {
		b.addMockError(line, "invalid @match query: "+err.Error())
		return
	}
	for _, key := range util.SortedKeys(vals) {
		name := strings.TrimSpace(key)
		if name == "" {
			b.addMockError(line, "@match query name cannot be empty")
			continue
		}
		if _, exists := m.match.Query[name]; exists {
			b.addMockError(line, fmt.Sprintf("@match query %q is repeated", name))
			continue
		}
		m.match.Query[name] = vals[key]
	}
}

func (m *mockBuilder) addHeaderMatchers(b *documentBuilder, line int, raw string) {
	vals, err := parseMockRules[restfile.MockHeaderRule](raw)
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
		canon, ok := b.canonMatchHeader(line, name)
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

// canonMatchHeader only validates the name. The rule and its values are checked
// when they decode in parseMockRules.
func (b *documentBuilder) canonMatchHeader(line int, key string) (string, bool) {
	if !httpguts.ValidHeaderFieldName(key) {
		b.addMockError(line, fmt.Sprintf("invalid @match header name %q", key))
		return "", false
	}
	return http.CanonicalHeaderKey(key), true
}

func (b *documentBuilder) flushMock() {
	if b.mock == nil {
		return
	}
	m := b.mock
	if m.pending != nil {
		b.addMockError(m.pending.line, "@match option is missing a closing bracket")
	}
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

// parseMockRules visits keys in sorted order. Map order would report a different
// broken rule on each parse, and the editor reparses on every keystroke.
func parseMockRules[T restfile.MockQueryRule | restfile.MockHeaderRule](
	raw string,
) (map[string]T, error) {
	fields, err := parseJSONObject(raw)
	if err != nil {
		return nil, err
	}
	out := make(map[string]T, len(fields))
	for _, name := range util.SortedKeys(fields) {
		var rule T
		if err := json.Unmarshal(fields[name], &rule); err != nil {
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
	if key, dup := duplicateJSONKey([]byte(raw)); dup {
		return nil, fmt.Errorf("field %q is repeated", key)
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
	if key, dup := duplicateJSONKey(compact.Bytes()); dup {
		return nil, fmt.Errorf("field %q is repeated", key)
	}
	return compact.Bytes(), nil
}

// encoding/json keeps the last duplicate key, which could silently remove a
// match condition.
func duplicateJSONKey(raw []byte) (string, bool) {
	key, dup, _ := scanJSONKeys(json.NewDecoder(bytes.NewReader(raw)))
	return key, dup
}

func scanJSONKeys(dec *json.Decoder) (string, bool, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", false, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return "", false, nil
	}

	seen := map[string]bool{}
	for dec.More() {
		if delim == '{' {
			name, err := dec.Token()
			if err != nil {
				return "", false, err
			}
			key, _ := name.(string)
			if seen[key] {
				return key, true, nil
			}
			seen[key] = true
		}
		if key, dup, err := scanJSONKeys(dec); dup || err != nil {
			return key, dup, err
		}
	}
	_, err = dec.Token()
	return "", false, err
}

func mergeMockJSON(dst, src []byte) ([]byte, error) {
	into, ok := mockJSONFields(dst)
	from, alsoObject := mockJSONFields(src)
	if !ok || !alsoObject {
		return nil, errors.New("can only be repeated when every declaration is a JSON object")
	}

	var dup []string
	for _, key := range util.SortedKeys(from) {
		if _, seen := into[key]; seen {
			dup = append(dup, strconv.Quote(key))
			continue
		}
		into[key] = from[key]
	}
	switch len(dup) {
	case 0:
		return encodeMockJSON(into)
	case 1:
		return nil, fmt.Errorf("field %s is repeated", dup[0])
	default:
		return nil, fmt.Errorf("fields %s are repeated", strings.Join(dup, ", "))
	}
}

func mockJSONFields(raw []byte) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, false
	}
	return fields, true
}

// Sorting gives multiline and repeated matchers one stored form.
func sortMockJSONFields(compact []byte) []byte {
	fields, ok := mockJSONFields(compact)
	if !ok {
		return compact
	}
	sorted, err := encodeMockJSON(fields)
	if err != nil {
		return compact
	}
	return sorted
}

// Matchers are request data, so HTML escaping would change their stored form.
func encodeMockJSON(fields map[string]json.RawMessage) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(fields); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func compactJSONObject(raw string) ([]byte, error) {
	compact, err := compactJSON(raw)
	if err != nil {
		return nil, err
	}
	if len(compact) == 0 || compact[0] != '{' {
		return nil, errors.New("must be a JSON object")
	}
	return compact, nil
}
