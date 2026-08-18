package request

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/unkn0wn-root/resterm/internal/capture"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rtshost"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	str "github.com/unkn0wn-root/resterm/internal/util"
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
	requestVars vars.NameMap[restfile.Variable]
	fileVars    vars.NameMap[restfile.Variable]
	globals     vars.Globals
}

type captureRun struct {
	ctx     context.Context
	base    string
	doc     *restfile.Document
	req     *restfile.Request
	res     *vars.Resolver
	resp    *scripts.Response
	stream  *scripts.StreamInfo
	out     *captureResult
	env     vars.ResolvedEnv
	store   vars.Globals
	secrets *vars.Secrets
	locals  rts.Locals
	run     runVars
}

type captureExpr struct {
	raw  string
	norm string
	mode restfile.CaptureExprMode
}

type captureScope struct {
	ctx  context.Context
	base string
	doc  *restfile.Document
	req  *restfile.Request
	env  vars.ResolvedEnv
	evalScope
	locals rts.Locals
	rr     *rtshost.Response
	rs     *rtshost.Stream
}

type captureValueIn struct {
	captureScope
	resolver *vars.Resolver
	spec     restfile.CaptureSpec
	lc       *captureContext
}

type captureRTSIn struct {
	captureScope
	spec restfile.CaptureSpec
	ex   string
}

func (r *captureResult) addRequest(name, value string, secret bool) {
	r.requestVars.Set(name, restfile.Variable{
		Name:   strings.TrimSpace(name),
		Value:  value,
		Secret: secret,
		Scope:  directive.ScopeRequest,
	})
}

func (r *captureResult) addFile(name, value string, secret bool) {
	r.fileVars.Set(name, restfile.Variable{
		Name:   strings.TrimSpace(name),
		Value:  value,
		Secret: secret,
		Scope:  directive.ScopeFile,
	})
}

// Captures are evaluated in declaration order against staged values and commit
// only after the entire batch succeeds. Staged values retain their normal
// precedence. Reusing the resolver memo keeps dynamic values stable between the
// request and its captures. Secret values enter the redaction set before commit
// so a later failure cannot expose them.
func (e *Engine) applyCaptures(in captureRun) error {
	if in.req == nil || in.resp == nil || len(in.req.Metadata.Captures) == 0 {
		return nil
	}
	rr, err := rtsScriptResp(in.resp)
	if err != nil {
		return err
	}

	lc := newCaptureContext(in.resp, in.stream, capture.StrictEnabled(in.req.Settings))
	sc := captureScope{
		ctx:    in.ctx,
		base:   in.base,
		doc:    in.doc,
		req:    in.req,
		env:    in.env,
		locals: in.locals,
		rr:     rr,
		rs:     rtsStream(in.stream),
	}
	work := &captureResult{}
	for _, c := range in.req.Metadata.Captures {
		res := e.refreshCaptureScope(in, &sc, work)
		val, ex, err := e.captureValue(captureValueIn{
			captureScope: sc,
			resolver:     res,
			spec:         c,
			lc:           lc,
		})
		if err != nil {
			return diag.WrapAsf(diag.ClassScript, err, "%s", captureErrCtx(in.req, c, ex))
		}
		if c.Secret {
			in.secrets.Add(val)
		}
		switch c.Scope {
		case directive.ScopeRequest:
			work.addRequest(c.Name, val, c.Secret)
		case directive.ScopeFile:
			work.addFile(c.Name, val, c.Secret)
		case directive.ScopeGlobal:
			work.globals.Set(c.Name, vars.GlobalMutation{Name: c.Name, Value: val, Secret: c.Secret})
		}
	}

	e.commitCaptures(in, work)
	if in.out != nil {
		*in.out = *work
	}
	return nil
}

func (e *Engine) refreshCaptureScope(
	in captureRun,
	sc *captureScope,
	work *captureResult,
) *vars.Resolver {
	runtime := mergeGlobalValues(in.store, work.globals)
	plan := e.buildVariablePlan(varSources{
		doc:     in.doc,
		req:     in.req,
		env:     in.env,
		globals: runtime,
		sec:     keepSecrets,
		run:     in.run,
	})
	plan.overlay(sourceRequestCapture, capturedValues(work.requestVars))
	plan.overlay(sourceRuntimeFile, capturedValues(work.fileVars))
	sc.vars = plan.values()
	sc.globals = effectiveGlobalValues(in.doc, runtime, in.env.Refs())

	ei := ExprInput{
		Doc:      sc.doc,
		Req:      sc.req,
		Env:      sc.env,
		Base:     sc.base,
		Vars:     sc.vars,
		Response: sc.rr,
		Stream:   sc.rs,
		Locals:   sc.locals,
		globals:  sc.globals,
	}
	if in.res == nil {
		return e.planResolver(sc.ctx, plan, ei, ExprEvalOptions{})
	}
	res := in.res.WithProviders(plan.providers()...)
	return res.WithExprEval(e.ExprEval(sc.ctx, ei))
}

func capturedValues(src vars.NameMap[restfile.Variable]) vars.NameMap[string] {
	var out vars.NameMap[string]
	for name, v := range src.All() {
		out.Set(name, v.Value)
	}
	return out
}

func (e *Engine) commitCaptures(in captureRun, work *captureResult) {
	for _, c := range in.req.Metadata.Captures {
		switch c.Scope {
		case directive.ScopeRequest:
			if v, ok := work.requestVars.Get(c.Name); ok {
				upsertVariable(&in.req.Variables, directive.ScopeRequest, v.Name, v.Value, v.Secret)
			}
		case directive.ScopeFile:
			if in.doc == nil {
				continue
			}
			if v, ok := work.fileVars.Get(c.Name); ok {
				upsertVariable(&in.doc.Variables, directive.ScopeFile, v.Name, v.Value, v.Secret)
			}
		}
	}

	fs := e.rt.Files()
	if fs == nil || work.fileVars.Len() == 0 {
		return
	}
	path := e.filePath(in.doc)
	for _, v := range work.fileVars.All() {
		fs.Set(in.env.Scope(), path, v.Name, v.Value, v.Secret)
	}
}

func (e *Engine) captureValue(in captureValueIn) (string, captureExpr, error) {
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
	val, err := e.captureRTSValue(captureRTSIn{captureScope: in.captureScope, spec: in.spec, ex: ex.norm})
	return val, ex, err
}

func (e *Engine) captureRTSValue(in captureRTSIn) (string, error) {
	ps := e.rtsPosForLineCol(in.doc, in.req, in.spec.Line, in.spec.Col)
	rt, err := e.buildRT(rtIn{
		doc:     in.doc,
		req:     in.req,
		env:     in.env,
		base:    in.base,
		vars:    in.vars,
		globals: in.globals,
		locals:  in.locals,
		site:    directive.Capture.Tag() + " " + str.FoldLines(in.ex),
		resp:    in.rr,
		res:     in.rr,
		st:      in.rs,
		secrets: rtshost.IncludeSecrets,
	})
	if err != nil {
		return "", err
	}
	return e.evalRTSString(in.ctx, in.doc, rt, in.ex, ps)
}

func parseCaptureExpr(raw string, mode restfile.CaptureExprMode) captureExpr {
	ex := strings.TrimSpace(raw)
	if ex == "" {
		return captureExpr{}
	}
	switch mode {
	case restfile.CaptureExprModeTemplate:
		return captureExpr{raw: ex, norm: ex, mode: restfile.CaptureExprModeTemplate}
	case restfile.CaptureExprModeRTS:
		return captureExpr{raw: ex, norm: normCaptureRTSExpr(ex), mode: restfile.CaptureExprModeRTS}
	default:
		if capture.HasUnquotedTemplateMarker(ex) {
			return captureExpr{raw: ex, norm: ex, mode: restfile.CaptureExprModeTemplate}
		}
		return captureExpr{raw: ex, norm: normCaptureRTSExpr(ex), mode: restfile.CaptureExprModeRTS}
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
	mtd := strings.ToUpper(strings.TrimSpace(req.Method))
	url := strings.TrimSpace(req.URL)
	switch {
	case mtd == "" && url == "":
		return ""
	case mtd == "":
		return url
	case url == "":
		return mtd
	default:
		return mtd + " " + url
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
	for _, p := range []string{"response.json", "last.json"} {
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
	switch {
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return true
	default:
		return b == '_'
	}
}

func cutFoldPrefix(s, pfx string) (string, bool) {
	if len(s) <= len(pfx) {
		return "", false
	}
	if !strings.EqualFold(s[:len(pfx)], pfx) {
		return "", false
	}
	return s[len(pfx):], true
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
	var hdr http.Header
	if resp != nil {
		hdr = resp.Header.Clone()
	}
	return &captureContext{
		response: resp,
		body:     body,
		headers:  hdr,
		stream:   stream,
		strict:   strict,
	}
}

func (c *captureContext) evaluate(ex string, res *vars.Resolver) (string, error) {
	var first error
	out := vars.ReplaceTemplateVars(ex, func(match, name string) string {
		if name == "" {
			return match
		}
		if rest, ok := cutFoldPrefix(name, captureResponsePrefix); ok {
			val, err := c.lookupResponse(strings.TrimSpace(rest))
			if err != nil && first == nil {
				first = err
			}
			if err == nil {
				return val
			}
			return match
		}
		if rest, ok := cutFoldPrefix(name, captureStreamPrefix); ok {
			val, err := c.lookupStream(strings.TrimSpace(rest))
			if err != nil && first == nil {
				first = err
			}
			if err == nil {
				return val
			}
			return match
		}
		if res != nil {
			val, err := res.ExpandTemplates(match)
			if err == nil {
				return val
			}
			if first == nil {
				first = err
			}
		}
		return match
	})
	if first != nil {
		return "", first
	}
	return out, nil
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
		vs := c.headers.Values(key)
		if len(vs) == 0 {
			return "", fmt.Errorf("header %s not found", key)
		}
		return strings.Join(vs, ", "), nil
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
	trim := strings.TrimSpace(path)
	if trim == "" {
		return "", fmt.Errorf("stream reference empty")
	}
	if strings.EqualFold(trim, streamKindField) {
		return c.stream.Kind, nil
	}
	if rest, ok := cutFoldPrefix(trim, streamSummaryPrefix); ok {
		key := strings.TrimSpace(rest)
		val, ok := caseLookup(c.stream.Summary, key)
		if !ok {
			return "", fmt.Errorf("stream summary field %s not found", key)
		}
		return formatCaptureValue(val)
	}
	if inner, ok := cutFoldPrefix(trim, streamEventsPrefix); ok {
		closeIdx := strings.Index(inner, "]")
		if closeIdx <= 0 {
			return "", fmt.Errorf("invalid stream events reference")
		}
		cnt := len(c.stream.Events)
		if cnt == 0 {
			return "", fmt.Errorf("stream events empty")
		}
		idxText := strings.TrimSpace(inner[:closeIdx])
		idx, err := strconv.Atoi(idxText)
		if err != nil {
			return "", fmt.Errorf("stream event index %s invalid", idxText)
		}
		if idx < 0 {
			idx = cnt + idx
		}
		if idx < 0 || idx >= cnt {
			return "", fmt.Errorf("stream event index %s out of range", idxText)
		}
		ev := c.stream.Events[idx]
		rest := strings.TrimSpace(inner[closeIdx+1:])
		rest = strings.TrimPrefix(rest, ".")
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return formatCaptureValue(ev)
		}
		val, ok := caseLookup(ev, rest)
		if !ok {
			return "", fmt.Errorf("stream event field %s not found", rest)
		}
		return formatCaptureValue(val)
	}
	return "", fmt.Errorf("unsupported stream reference %q", path)
}

func (c *captureContext) lookupJSON(path string) (string, error) {
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
		if c.strict {
			return "", fmt.Errorf("json unavailable: %w", c.jsonErr)
		}
		return "", nil
	}

	trim := strings.TrimSpace(path[len(captureJSONPrefix):])
	if trim == "" {
		return c.body, nil
	}
	full := captureJSONPrefix + trim
	trim = strings.TrimPrefix(trim, ".")
	segs, err := splitJSONPath(trim)
	if err != nil {
		return c.jsonPathFail(full, captureJSONPrefix, err.Error())
	}
	cur := c.jsonValue
	seen := captureJSONPrefix
	for _, seg := range segs {
		seen = jsonPathAppend(seen, seg)
		switch typed := cur.(type) {
		case map[string]any:
			if seg.name == "" {
				return c.jsonPathFail(full, seen, "missing object key")
			}
			val, ok := typed[seg.name]
			if !ok {
				return c.jsonPathFail(full, seen, "segment not found")
			}
			cur = val
		case []any:
			if seg.name != "" {
				return c.jsonPathFail(full, seen, "cannot access object key on array")
			}
			if !seg.hasIndex {
				return c.jsonPathFail(full, seen, "missing array index")
			}
			idx := seg.index
			if idx < 0 {
				idx = len(typed) + idx
			}
			if idx < 0 || idx >= len(typed) {
				return c.jsonPathFail(full, seen, fmt.Sprintf("index %d out of range", seg.index))
			}
			cur = typed[idx]
		default:
			return c.jsonPathFail(full, seen, "cannot traverse non-container value")
		}
	}
	return stringifyJSONValue(cur), nil
}

type jsonPathSegment struct {
	name     string
	index    int
	hasIndex bool
}

func splitJSONPath(path string) ([]jsonPathSegment, error) {
	segs := make([]jsonPathSegment, 0, 8)
	for i := 0; i < len(path); {
		switch path[i] {
		case '.':
			return nil, fmt.Errorf("empty path segment")
		case '[':
			seg, next, err := parseJSONIndex(path, i)
			if err != nil {
				return nil, err
			}
			segs = append(segs, seg)
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
			segs = append(segs, jsonPathSegment{name: name})
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
		default:
			return nil, fmt.Errorf("expected '.' or '[' between path segments, got %q", path[i])
		}
	}
	return segs, nil
}

func parseJSONIndex(path string, start int) (jsonPathSegment, int, error) {
	closeIdx := strings.IndexByte(path[start:], ']')
	if closeIdx < 0 {
		return jsonPathSegment{}, 0, fmt.Errorf("missing closing bracket")
	}
	closeIdx += start
	idxText := strings.TrimSpace(path[start+1 : closeIdx])
	if idxText == "" {
		return jsonPathSegment{}, 0, fmt.Errorf("empty array index")
	}
	idx, err := strconv.Atoi(idxText)
	if err != nil {
		return jsonPathSegment{}, 0, fmt.Errorf("invalid array index %q", idxText)
	}
	return jsonPathSegment{index: idx, hasIndex: true}, closeIdx + 1, nil
}

func jsonPathAppend(base string, seg jsonPathSegment) string {
	idx := ""
	if seg.hasIndex {
		idx = strconv.Itoa(seg.index)
	}
	var b strings.Builder
	b.Grow(len(base) + len(seg.name) + len(idx) + 3)
	b.WriteString(base)
	if seg.name != "" {
		if base != "" {
			b.WriteByte('.')
		}
		b.WriteString(seg.name)
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
	if v, ok := m[key]; ok {
		return v, true
	}
	low := strings.ToLower(key)
	for k, v := range m {
		if strings.ToLower(k) == low {
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
	scope directive.Scope,
	name, value string,
	secret bool,
) {
	low := strings.ToLower(name)
	xs := *list
	for i := range xs {
		if strings.ToLower(xs[i].Name) == low {
			xs[i].SetRuntimeValue(value)
			xs[i].Scope = scope
			xs[i].Secret = secret
			return
		}
	}
	*list = append(xs, restfile.Variable{Name: name, Value: value, Scope: scope, Secret: secret})
}
