package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/unkn0wn-root/resterm/internal/capture"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/stdlib"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	captureResponsePrefix = "response."
	captureStreamPrefix   = "stream."
	captureHeadersPrefix  = "headers."
	captureJSONPrefix     = "json"
	streamKindField       = "kind"
	streamSummaryPrefix   = "summary."
	streamEventsPrefix    = "events["
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

type captureExpr struct {
	raw  string
	norm string
	mode restfile.CaptureExprMode
}

type captureValueIn struct {
	doc      *restfile.Document
	req      *restfile.Request
	resolver *vars.Resolver
	env      string
	spec     restfile.CaptureSpec
	v        map[string]string
	x        map[string]rts.Value
	rr       *rts.Resp
	rs       *rts.Stream
	lc       *captureContext
}

type captureRTSIn struct {
	doc  *restfile.Document
	req  *restfile.Request
	env  string
	spec restfile.CaptureSpec
	ex   string
	v    map[string]string
	x    map[string]rts.Value
	rr   *rts.Resp
	rs   *rts.Stream
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
	st := capture.StrictEnabled(in.req.Settings)
	lc := newCaptureContext(in.resp, in.stream, st)
	rr := rtsScriptResp(in.resp)
	rs := rtsStream(in.stream)
	if in.v == nil {
		in.v = m.collectVariables(in.doc, in.req, in.env)
	}
	for _, c := range in.req.Metadata.Captures {
		value, ex, err := m.captureValue(captureValueIn{
			doc:      in.doc,
			req:      in.req,
			resolver: in.res,
			env:      in.env,
			spec:     c,
			v:        in.v,
			x:        in.x,
			rr:       rr,
			rs:       rs,
			lc:       lc,
		})
		if err != nil {
			return errdef.Wrap(errdef.CodeScript, err, "%s", captureErrCtx(in.req, c, ex))
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
			if gs := m.globalsStore(); gs != nil {
				gs.Set(envKey, c.Name, value, c.Secret)
			}
		}
	}

	if fs := m.fileStore(); in.out != nil && len(in.out.fileVars) > 0 && fs != nil {
		path := m.documentRuntimePath(in.doc)
		for _, e := range in.out.fileVars {
			fs.Set(envKey, path, e.Name, e.Value, e.Secret)
		}
	}

	return nil
}

func (m *Model) captureValue(in captureValueIn) (string, captureExpr, error) {
	ex := parseCaptureExpr(in.spec.Expression, in.spec.Mode)
	if ex.raw == "" {
		return "", ex, nil
	}
	if ex.mode == restfile.CaptureExprModeTemplate {
		if capture.MixedTemplateRTSCall(ex.raw) {
			return "", ex, fmt.Errorf(
				"mixed capture syntax is not supported; use pure RTS or {{= ... }}",
			)
		}
		if in.lc == nil {
			return "", ex, fmt.Errorf("capture context not available")
		}
		val, err := in.lc.evaluate(ex.raw, in.resolver)
		return val, ex, err
	}
	val, err := m.captureRTSValue(captureRTSIn{
		doc:  in.doc,
		req:  in.req,
		env:  in.env,
		spec: in.spec,
		ex:   ex.norm,
		v:    in.v,
		x:    in.x,
		rr:   in.rr,
		rs:   in.rs,
	})
	return val, ex, err
}

func (m *Model) captureRTSValue(in captureRTSIn) (string, error) {
	if m.rtsEng == nil {
		m.rtsEng = rts.NewEng(stdlib.New)
	}
	ps := m.rtsPosForLine(in.doc, in.req, in.spec.Line)
	rt := m.rtsRT(rtsRTIn{
		doc:  in.doc,
		req:  in.req,
		env:  in.env,
		v:    in.v,
		x:    in.x,
		site: "@capture " + in.ex,
		resp: in.rr,
		res:  in.rr,
		st:   in.rs,
	})
	return m.rtsEng.EvalStr(context.Background(), rt, in.ex, ps)
}

func parseCaptureExpr(raw string, mode restfile.CaptureExprMode) captureExpr {
	ex := strings.TrimSpace(raw)
	if ex == "" {
		return captureExpr{}
	}
	switch mode {
	case restfile.CaptureExprModeTemplate:
		return captureExpr{
			raw:  ex,
			norm: ex,
			mode: restfile.CaptureExprModeTemplate,
		}
	case restfile.CaptureExprModeRTS:
		return captureExpr{
			raw:  ex,
			norm: normCaptureRTSExpr(ex),
			mode: restfile.CaptureExprModeRTS,
		}
	default:
		if capture.HasUnquotedTemplateMarker(ex) {
			return captureExpr{
				raw:  ex,
				norm: ex,
				mode: restfile.CaptureExprModeTemplate,
			}
		}
		return captureExpr{
			raw:  ex,
			norm: normCaptureRTSExpr(ex),
			mode: restfile.CaptureExprModeRTS,
		}
	}
}

func captureErrCtx(req *restfile.Request, c restfile.CaptureSpec, ex captureExpr) string {
	lbl := captureReqLabel(req)
	if ex.norm != "" && ex.norm != ex.raw {
		return fmt.Sprintf(
			"evaluate capture %q (request=%q line=%d expr=%q norm=%q)",
			c.Name,
			lbl,
			c.Line,
			ex.raw,
			ex.norm,
		)
	}
	return fmt.Sprintf(
		"evaluate capture %q (request=%q line=%d expr=%q)",
		c.Name,
		lbl,
		c.Line,
		ex.raw,
	)
}

func captureReqLabel(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if name := strings.TrimSpace(req.Metadata.Name); name != "" {
		return name
	}
	m := strings.ToUpper(strings.TrimSpace(req.Method))
	u := strings.TrimSpace(req.URL)
	switch {
	case m == "" && u == "":
		return ""
	case m == "":
		return u
	case u == "":
		return m
	default:
		return m + " " + u
	}
}

func normCaptureRTSExpr(ex string) string {
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

func cutFoldPrefix(s, prefix string) (string, bool) {
	if len(s) <= len(prefix) {
		return "", false
	}
	if !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

type captureContext struct {
	response  *scripts.Response
	body      string
	headers   http.Header
	stream    *scripts.StreamInfo
	strict    bool
	jsonOnce  sync.Once
	jsonValue any
	jsonErr   error
}

func newCaptureContext(
	resp *scripts.Response,
	stream *scripts.StreamInfo,
	strict bool,
) *captureContext {
	body := ""
	if resp != nil {
		body = string(resp.Body)
	}

	var headers http.Header
	if resp != nil {
		headers = cloneHeader(resp.Header)
	}
	return &captureContext{
		response: resp,
		body:     body,
		headers:  headers,
		stream:   stream,
		strict:   strict,
	}
}

func (c *captureContext) evaluate(ex string, resolver *vars.Resolver) (string, error) {
	var firstErr error
	expanded := vars.ReplaceTemplateVars(ex, func(match, name string) string {
		if name == "" {
			return match
		}

		if rest, ok := cutFoldPrefix(name, captureResponsePrefix); ok {
			value, err := c.lookupResponse(strings.TrimSpace(rest))
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return match
			}
			return value
		}

		if rest, ok := cutFoldPrefix(name, captureStreamPrefix); ok {
			value, err := c.lookupStream(strings.TrimSpace(rest))
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
	if strings.HasPrefix(lp, captureHeadersPrefix) {
		key := path[len(captureHeadersPrefix):]
		if c.headers == nil {
			return "", fmt.Errorf("header %s not available", key)
		}
		values := c.headers.Values(key)
		if len(values) == 0 {
			return "", fmt.Errorf("header %s not found", key)
		}
		return strings.Join(values, ", "), nil
	}
	if strings.HasPrefix(lp, captureJSONPrefix) {
		return c.lookupJSON(path)
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

	if strings.EqualFold(trimmed, streamKindField) {
		return c.stream.Kind, nil
	}
	if rest, ok := cutFoldPrefix(trimmed, streamSummaryPrefix); ok {
		key := strings.TrimSpace(rest)
		value, ok := caseLookup(c.stream.Summary, key)
		if !ok {
			return "", fmt.Errorf("stream summary field %s not found", key)
		}
		return formatCaptureValue(value)
	}
	if inner, ok := cutFoldPrefix(trimmed, streamEventsPrefix); ok {
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

func (c *captureContext) lookupJSON(path string) (string, error) {
	c.jsonOnce.Do(func() {
		if strings.TrimSpace(c.body) == "" {
			c.jsonErr = fmt.Errorf("response body empty")
		} else {
			var data any
			if err := json.Unmarshal([]byte(c.body), &data); err != nil {
				c.jsonErr = err
			} else {
				c.jsonValue = data
			}
		}
	})
	if c.jsonErr != nil {
		if c.strict {
			return "", fmt.Errorf("json unavailable: %w", c.jsonErr)
		}
		return "", nil
	}

	trimmed := strings.TrimSpace(path[len(captureJSONPrefix):])
	if trimmed == "" {
		return c.body, nil
	}

	full := captureJSONPrefix + trimmed
	trimmed = strings.TrimPrefix(trimmed, ".")
	segments, err := splitJSONPath(trimmed)
	if err != nil {
		return c.jsonPathFail(full, captureJSONPrefix, err.Error())
	}
	current := c.jsonValue
	seen := captureJSONPrefix
	for _, segment := range segments {
		seen = jsonPathAppend(seen, segment)
		switch typed := current.(type) {
		case map[string]any:
			if segment.name == "" {
				return c.jsonPathFail(full, seen, "missing object key")
			}
			val, ok := typed[segment.name]
			if !ok {
				return c.jsonPathFail(full, seen, "segment not found")
			}
			current = val
		case []any:
			if segment.name != "" {
				return c.jsonPathFail(full, seen, "cannot access object key on array")
			}
			if !segment.hasIndex {
				return c.jsonPathFail(full, seen, "missing array index")
			}
			idx := segment.index
			if idx < 0 {
				idx = len(typed) + idx
			}
			if idx < 0 || idx >= len(typed) {
				return c.jsonPathFail(
					full,
					seen,
					fmt.Sprintf("index %d out of range", segment.index),
				)
			}
			current = typed[idx]
		default:
			return c.jsonPathFail(full, seen, "cannot traverse non-container value")
		}
	}
	return stringifyJSONValue(current), nil
}

type jsonPathSegment struct {
	name     string
	index    int
	hasIndex bool
}

func splitJSONPath(path string) ([]jsonPathSegment, error) {
	segments := make([]jsonPathSegment, 0, 8)
	for i := 0; i < len(path); {
		switch path[i] {
		case '.':
			return nil, fmt.Errorf("empty path segment")
		case '[':
			segment, next, err := parseJSONIndex(path, i)
			if err != nil {
				return nil, err
			}
			segments = append(segments, segment)
			i = next
		default:
			start := i
			for i < len(path) && path[i] != '.' && path[i] != '[' && path[i] != ']' {
				i++
			}
			name := strings.TrimSpace(path[start:i])
			if name == "" {
				return nil, fmt.Errorf("empty path segment")
			}
			segments = append(segments, jsonPathSegment{name: name})
		}
		if i >= len(path) {
			break
		}
		switch path[i] {
		case '.':
			i++
			if i >= len(path) {
				return nil, fmt.Errorf("empty path segment")
			}
		case '[':
			// Index segments can chain, e.g. a[0][1]
		default:
			return nil, fmt.Errorf(
				"expected '.' or '[' between path segments, got %q",
				path[i],
			)
		}
	}
	return segments, nil
}

func parseJSONIndex(path string, start int) (jsonPathSegment, int, error) {
	closeIdx := strings.IndexByte(path[start:], ']')
	if closeIdx < 0 {
		return jsonPathSegment{}, 0, fmt.Errorf("missing closing bracket")
	}
	closeIdx += start
	idxStr := strings.TrimSpace(path[start+1 : closeIdx])
	if idxStr == "" {
		return jsonPathSegment{}, 0, fmt.Errorf("empty array index")
	}
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		return jsonPathSegment{}, 0, fmt.Errorf("invalid array index %q", idxStr)
	}
	return jsonPathSegment{index: idx, hasIndex: true}, closeIdx + 1, nil
}

func jsonPathAppend(base string, segment jsonPathSegment) string {
	idx := ""
	if segment.hasIndex {
		idx = strconv.Itoa(segment.index)
	}
	g := len(base) + len(segment.name)
	if segment.name != "" && base != "" {
		g++
	}
	if idx != "" {
		g += len(idx) + 2
	}
	var b strings.Builder
	b.Grow(g)
	b.WriteString(base)
	if segment.name != "" {
		if base != "" {
			b.WriteByte('.')
		}
		b.WriteString(segment.name)
	}
	if idx != "" {
		b.WriteByte('[')
		b.WriteString(idx)
		b.WriteByte(']')
	}
	return b.String()
}

func (c *captureContext) jsonPathFail(full, seen, msg string) (string, error) {
	if !c.strict {
		return "", nil
	}
	return "", fmt.Errorf("json path %q failed at %q: %s", full, seen, msg)
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
