package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type captureResult struct {
	requestVars map[string]restfile.Variable
	fileVars    map[string]restfile.Variable
}

type captureRun struct {
	doc    *restfile.Document
	req    *restfile.Request
	res    *vars.Resolver
	resp   *scripts.Response
	stream *scripts.StreamInfo
	out    *captureResult
	env    string
	v      map[string]string
	x      map[string]rts.Value
}

func (r *captureResult) addRequest(name, value string, secret bool) {
	if r == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if r.requestVars == nil {
		r.requestVars = make(map[string]restfile.Variable)
	}
	key := strings.ToLower(name)
	r.requestVars[key] = restfile.Variable{
		Name:   name,
		Value:  value,
		Secret: secret,
		Scope:  restfile.ScopeRequest,
	}
}

func (r *captureResult) addFile(name, value string, secret bool) {
	if r == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if r.fileVars == nil {
		r.fileVars = make(map[string]restfile.Variable)
	}
	key := strings.ToLower(name)
	r.fileVars[key] = restfile.Variable{
		Name:   name,
		Value:  value,
		Secret: secret,
		Scope:  restfile.ScopeFile,
	}
}

func (m *Model) applyCaptures(in captureRun) error {
	if in.req == nil || in.resp == nil {
		return nil
	}
	if len(in.req.Metadata.Captures) == 0 {
		return nil
	}

	envKey := vars.SelectEnv(m.cfg.EnvironmentSet, in.env, m.cfg.EnvironmentName)
	lc := newCaptureContext(in.resp, in.stream)
	rr := rtsScriptResp(in.resp)
	rs := rtsStream(in.stream)
	if in.v == nil {
		in.v = m.collectVariables(in.doc, in.req, in.env)
	}
	for _, c := range in.req.Metadata.Captures {
		value, err := m.captureValue(in.doc, in.req, in.res, in.env, c, in.v, in.x, rr, rs, lc)
		if err != nil {
			return errdef.Wrap(errdef.CodeScript, err, "evaluate capture %s", c.Name)
		}
		switch c.Scope {
		case restfile.CaptureScopeRequest:
			upsertVariable(&in.req.Variables, restfile.ScopeRequest, c.Name, value, c.Secret)
			if in.out != nil {
				in.out.addRequest(c.Name, value, c.Secret)
			}
		case restfile.CaptureScopeFile:
			if in.doc != nil {
				upsertVariable(&in.doc.Variables, restfile.ScopeFile, c.Name, value, c.Secret)
			}
			if in.out != nil {
				in.out.addFile(c.Name, value, c.Secret)
			}
		case restfile.CaptureScopeGlobal:
			if m.globals != nil {
				m.globals.set(envKey, c.Name, value, c.Secret)
			}
		}
	}

	if in.out != nil && len(in.out.fileVars) > 0 && m.fileVars != nil {
		path := m.documentRuntimePath(in.doc)
		for _, e := range in.out.fileVars {
			m.fileVars.set(envKey, path, e.Name, e.Value, e.Secret)
		}
	}

	return nil
}

func (m *Model) captureValue(
	doc *restfile.Document,
	req *restfile.Request,
	resolver *vars.Resolver,
	env string,
	c restfile.CaptureSpec,
	v map[string]string,
	x map[string]rts.Value,
	rr *rts.Resp,
	rs *rts.Stream,
	lc *captureContext,
) (string, error) {
	ex := strings.TrimSpace(c.Expression)
	if ex == "" {
		return "", nil
	}
	if legacyCaptureExpr(ex) {
		if lc == nil {
			return "", fmt.Errorf("capture context not available")
		}
		return lc.evaluate(ex, resolver)
	}
	return m.captureRSTValue(doc, req, env, c, ex, v, x, rr, rs)
}

func (m *Model) captureRSTValue(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	c restfile.CaptureSpec,
	ex string,
	v map[string]string,
	x map[string]rts.Value,
	rr *rts.Resp,
	rs *rts.Stream,
) (string, error) {
	if m.rtsEng == nil {
		m.rtsEng = rts.NewEng()
	}
	ex = normCaptureRSTExpr(ex)
	ps := m.rtsPosForLine(doc, req, c.Line)
	rt := m.rtsRT(
		doc,
		req,
		env,
		"",
		v,
		x,
		"@capture "+ex,
		false,
		rr,
		rr,
		nil,
		rs,
	)
	return m.rtsEng.EvalStr(context.Background(), rt, ex, ps)
}

func legacyCaptureExpr(ex string) bool {
	return strings.Contains(ex, "{{") && strings.Contains(ex, "}}")
}

func normCaptureRSTExpr(ex string) string {
	if ex == "" {
		return ex
	}
	var b strings.Builder
	b.Grow(len(ex) + 8)
	var q byte
	esc := false
	for i := 0; i < len(ex); {
		ch := ex[i]
		if q != 0 {
			b.WriteByte(ch)
			if esc {
				esc = false
				i++
				continue
			}
			if ch == '\\' {
				esc = true
				i++
				continue
			}
			if ch == q {
				q = 0
			}
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			q = ch
			b.WriteByte(ch)
			i++
			continue
		}
		if n, p, ok := captureJSONPathPrefix(ex, i); ok {
			b.WriteString(p)
			if n < len(ex) {
				switch ex[n] {
				case '.', '[':
					b.WriteString("()")
				}
			}
			i = n
			continue
		}
		b.WriteByte(ch)
		i++
	}
	return b.String()
}

func captureJSONPathPrefix(ex string, i int) (int, string, bool) {
	ps := []string{"response.json", "last.json"}
	for _, p := range ps {
		n := len(p)
		if i+n > len(ex) || ex[i:i+n] != p {
			continue
		}
		if i > 0 {
			c := ex[i-1]
			if captureIdentByte(c) || c == '.' {
				continue
			}
		}
		if i+n < len(ex) && captureIdentByte(ex[i+n]) {
			continue
		}
		if i+n < len(ex) && ex[i+n] == '(' {
			continue
		}
		return i + n, p, true
	}
	return 0, "", false
}

func captureIdentByte(b byte) bool {
	if b >= 'a' && b <= 'z' {
		return true
	}
	if b >= 'A' && b <= 'Z' {
		return true
	}
	if b >= '0' && b <= '9' {
		return true
	}
	return b == '_'
}

type captureContext struct {
	response  *scripts.Response
	body      string
	headers   http.Header
	stream    *scripts.StreamInfo
	jsonOnce  sync.Once
	jsonValue any
	jsonErr   error
}

var captureTemplatePattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func newCaptureContext(resp *scripts.Response, stream *scripts.StreamInfo) *captureContext {
	body := ""
	if resp != nil {
		body = string(resp.Body)
	}

	var headers http.Header
	if resp != nil {
		headers = cloneHeader(resp.Header)
	}
	return &captureContext{response: resp, body: body, headers: headers, stream: stream}
}

func (c *captureContext) evaluate(ex string, resolver *vars.Resolver) (string, error) {
	var firstErr error
	expanded := captureTemplatePattern.ReplaceAllStringFunc(ex, func(match string) string {
		name := strings.TrimSpace(captureTemplatePattern.FindStringSubmatch(match)[1])
		if name == "" {
			return match
		}

		if strings.HasPrefix(strings.ToLower(name), "response.") {
			value, err := c.lookupResponse(strings.TrimSpace(name[len("response."):]))
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return match
			}
			return value
		}

		if strings.HasPrefix(strings.ToLower(name), "stream.") {
			value, err := c.lookupStream(strings.TrimSpace(name[len("stream."):]))
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return match
			}
			return value
		}

		if resolver != nil {
			res, err := resolver.ExpandTemplates(match)
			if err == nil {
				return res
			}
			if firstErr == nil {
				firstErr = err
			}
		}
		return match
	})

	if firstErr != nil {
		return "", firstErr
	}
	return expanded, nil
}

func (c *captureContext) lookupResponse(path string) (string, error) {
	lp := strings.ToLower(path)
	switch lp {
	case "body":
		return c.body, nil
	case "status":
		if c.response != nil {
			return c.response.Status, nil
		}
		return "", nil
	case "statuscode":
		if c.response != nil {
			return strconv.Itoa(c.response.Code), nil
		}
		return "", nil
	}
	if strings.HasPrefix(lp, "headers.") {
		key := path[len("headers."):]
		if c.headers == nil {
			return "", fmt.Errorf("header %s not available", key)
		}
		values := c.headers.Values(key)
		if len(values) == 0 {
			values = c.headers.Values(http.CanonicalHeaderKey(key))
		}
		if len(values) == 0 {
			return "", fmt.Errorf("header %s not found", key)
		}
		return strings.Join(values, ", "), nil
	}
	if strings.HasPrefix(lp, "json") {
		return c.lookupJSON(path), nil
	}
	return "", fmt.Errorf("unsupported response reference %q", path)
}

func (c *captureContext) lookupStream(path string) (string, error) {
	if c.stream == nil {
		return "", fmt.Errorf("stream data not available")
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("stream reference empty")
	}

	lower := strings.ToLower(trimmed)
	if lower == "kind" {
		return c.stream.Kind, nil
	}
	if strings.HasPrefix(lower, "summary.") {
		key := strings.TrimSpace(trimmed[len("summary."):])
		value, ok := caseLookup(c.stream.Summary, key)
		if !ok {
			return "", fmt.Errorf("stream summary field %s not found", key)
		}
		return formatCaptureValue(value)
	}
	if strings.HasPrefix(lower, "events[") {
		inner := trimmed[len("events["):]
		closeIdx := strings.Index(inner, "]")
		if closeIdx <= 0 {
			return "", fmt.Errorf("invalid stream events reference")
		}
		count := len(c.stream.Events)
		if count == 0 {
			return "", fmt.Errorf("stream events empty")
		}
		indexText := strings.TrimSpace(inner[:closeIdx])
		idx, err := strconv.Atoi(indexText)
		if err != nil {
			return "", fmt.Errorf("stream event index %s invalid", indexText)
		}
		if idx < 0 {
			idx = count + idx
		}
		if idx < 0 || idx >= count {
			return "", fmt.Errorf("stream event index %s out of range", indexText)
		}
		event := c.stream.Events[idx]
		remainder := strings.TrimSpace(inner[closeIdx+1:])
		remainder = strings.TrimPrefix(remainder, ".")
		remainder = strings.TrimSpace(remainder)
		if remainder == "" {
			return formatCaptureValue(event)
		}
		value, ok := caseLookup(event, remainder)
		if !ok {
			return "", fmt.Errorf("stream event field %s not found", remainder)
		}
		return formatCaptureValue(value)
	}
	return "", fmt.Errorf("unsupported stream reference %q", path)
}

func (c *captureContext) lookupJSON(path string) string {
	c.jsonOnce.Do(func() {
		if strings.TrimSpace(c.body) == "" {
			c.jsonErr = fmt.Errorf("response body empty")
			return
		}
		var data any
		if err := json.Unmarshal([]byte(c.body), &data); err != nil {
			c.jsonErr = err
			return
		}
		c.jsonValue = data
	})
	if c.jsonErr != nil {
		return ""
	}

	trimmed := strings.TrimSpace(path[len("json"):])
	if trimmed == "" {
		return c.body
	}

	trimmed = strings.TrimPrefix(trimmed, ".")
	current := c.jsonValue
	for _, segment := range splitJSONPath(trimmed) {
		switch typed := current.(type) {
		case map[string]any:
			val, ok := typed[segment.name]
			if !ok {
				return ""
			}
			current = val
		case []any:
			if segment.index == nil {
				return ""
			}
			idx := *segment.index
			if idx < 0 || idx >= len(typed) {
				return ""
			}
			current = typed[idx]
		default:
			return ""
		}
	}
	return stringifyJSONValue(current)
}

type jsonPathSegment struct {
	name  string
	index *int
}

func splitJSONPath(path string) []jsonPathSegment {
	parts := strings.Split(path, ".")
	segments := make([]jsonPathSegment, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		segment := jsonPathSegment{}
		if bracket := strings.Index(part, "["); bracket != -1 {
			segment.name = part[:bracket]
			end := strings.Index(part[bracket:], "]")
			if end > 1 {
				idxStr := part[bracket+1 : bracket+end]
				if n, err := strconv.Atoi(idxStr); err == nil {
					segment.index = &n
				}
			}
		} else {
			segment.name = part
		}
		segments = append(segments, segment)
	}
	return segments
}

func stringifyJSONValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		if float64(int64(v)) == v {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(data)
	}
}

func caseLookup(m map[string]any, key string) (any, bool) {
	if m == nil {
		return nil, false
	}
	if value, ok := m[key]; ok {
		return value, true
	}
	lower := strings.ToLower(key)
	for k, v := range m {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return nil, false
}

func formatCaptureValue(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	switch v := value.(type) {
	case string:
		return v, nil
	case fmt.Stringer:
		return v.String(), nil
	case bool:
		return strconv.FormatBool(v), nil
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v), nil
	case float32, float64:
		return fmt.Sprintf("%v", v), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v), nil
		}
		return string(data), nil
	}
}

func upsertVariable(
	list *[]restfile.Variable,
	scope restfile.VariableScope,
	name, value string,
	secret bool,
) {
	lower := strings.ToLower(name)
	vars := *list
	for i := range vars {
		if strings.ToLower(vars[i].Name) == lower {
			vars[i].Value = value
			vars[i].Scope = scope
			vars[i].Secret = secret
			return
		}
	}
	*list = append(vars, restfile.Variable{Name: name, Value: value, Scope: scope, Secret: secret})
}
