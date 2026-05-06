package request

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rtspre"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/urltpl"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (e *Engine) rtsReq(req *restfile.Request) *rts.Req {
	if req == nil {
		return nil
	}
	out := &rts.Req{
		Method: strings.TrimSpace(req.Method),
		URL:    strings.TrimSpace(req.URL),
	}
	if len(req.Headers) > 0 {
		h := make(map[string][]string, len(req.Headers))
		for k, vs := range req.Headers {
			if len(vs) == 0 {
				continue
			}
			h[strings.ToLower(k)] = append([]string(nil), vs...)
		}
		if len(h) > 0 {
			out.H = h
		}
	}
	if q := requestQuery(out.URL); len(q) > 0 {
		out.Q = q
	}
	return out
}

func requestQuery(raw string) map[string][]string {
	if raw == "" {
		return nil
	}
	idx := strings.Index(raw, "?")
	if idx < 0 {
		return nil
	}
	q := raw[idx+1:]
	if cut := strings.Index(q, "#"); cut >= 0 {
		q = q[:cut]
	}
	if strings.TrimSpace(q) == "" {
		return nil
	}
	vals, err := url.ParseQuery(q)
	if err != nil || len(vals) == 0 {
		return nil
	}
	out := make(map[string][]string, len(vals))
	for k, vs := range vals {
		if len(vs) > 0 {
			out[k] = append([]string(nil), vs...)
		}
	}
	return out
}

func (e *Engine) rtsUses(doc *restfile.Document, req *restfile.Request) []rts.Use {
	var out []rts.Use
	if doc != nil {
		for _, spec := range doc.Uses {
			path := strings.TrimSpace(spec.Path)
			alias := strings.TrimSpace(spec.Alias)
			if path != "" {
				out = append(out, rts.Use{Path: path, Alias: alias})
			}
		}
	}
	if req != nil {
		for _, spec := range req.Metadata.Uses {
			path := strings.TrimSpace(spec.Path)
			alias := strings.TrimSpace(spec.Alias)
			if path != "" {
				out = append(out, rts.Use{Path: path, Alias: alias})
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (e *Engine) rtsLast() *rts.Resp {
	switch {
	case e.last.http != nil:
		return rtsHTTP(e.last.http)
	case e.last.grpc != nil:
		return rtsGRPC(e.last.grpc)
	default:
		return nil
	}
}

func rtsHTTP(resp *httpclient.Response) *rts.Resp {
	if resp == nil {
		return nil
	}
	h := make(map[string][]string, len(resp.Headers))
	for k, vs := range resp.Headers {
		h[k] = append([]string(nil), vs...)
	}
	return &rts.Resp{
		Status: resp.Status,
		Code:   resp.StatusCode,
		H:      h,
		Body:   resp.Body,
		URL:    resp.EffectiveURL,
	}
}

func rtsGRPC(resp *grpcclient.Response) *rts.Resp {
	if resp == nil {
		return nil
	}
	h := make(map[string][]string, len(resp.Headers)+len(resp.Trailers))
	for k, vs := range resp.Headers {
		h[k] = append([]string(nil), vs...)
	}
	for k, vs := range resp.Trailers {
		h[k] = append([]string(nil), vs...)
	}
	return &rts.Resp{
		Status: resp.StatusMessage,
		Code:   int(resp.StatusCode),
		H:      h,
		Body:   resp.Body,
	}
}

func rtsTrace(resp *httpclient.Response) *rts.Trace {
	if resp == nil || resp.TraceReport == nil {
		return nil
	}
	return &rts.Trace{Rep: resp.TraceReport.Clone()}
}

func rtsScriptResp(resp *scripts.Response) *rts.Resp {
	if resp == nil {
		return nil
	}
	h := make(map[string][]string, len(resp.Header))
	for k, vs := range resp.Header {
		h[k] = append([]string(nil), vs...)
	}
	return &rts.Resp{
		Status: resp.Status,
		Code:   resp.Code,
		H:      h,
		Body:   append([]byte(nil), resp.Body...),
		URL:    resp.URL,
	}
}

func rtsStream(info *scripts.StreamInfo) *rts.Stream {
	if info == nil {
		return nil
	}
	sum := make(map[string]any, len(info.Summary))
	for k, v := range info.Summary {
		sum[k] = v
	}
	evs := make([]map[string]any, len(info.Events))
	for i, item := range info.Events {
		if item == nil {
			continue
		}
		cp := make(map[string]any, len(item))
		for k, v := range item {
			cp[k] = v
		}
		evs[i] = cp
	}
	return &rts.Stream{Kind: info.Kind, Summary: sum, Events: evs}
}

type rtIn struct {
	doc  *restfile.Document
	req  *restfile.Request
	env  string
	base string
	vars map[string]string
	site string
	resp *rts.Resp
	res  *rts.Resp
	tr   *rts.Trace
	st   *rts.Stream
	x    map[string]rts.Value
}

func (e *Engine) buildRT(in rtIn) rts.RT {
	base := e.rtsBase(in.doc, in.base)
	resp := in.resp
	if resp == nil {
		resp = e.rtsLast()
	}
	res := in.res
	if res == nil {
		res = resp
	}
	return rts.RT{
		Env:         e.rtsEnv(in.env),
		Vars:        in.vars,
		Globals:     rtspre.RuntimeGlobals(e.collectGlobalValues(in.doc, in.env), false),
		Resp:        resp,
		Res:         res,
		Trace:       in.tr,
		Stream:      in.st,
		Req:         e.rtsReq(in.req),
		BaseDir:     base,
		ReadFile:    os.ReadFile,
		AllowRandom: true,
		Site:        in.site,
		Uses:        e.rtsUses(in.doc, in.req),
		Extra:       in.x,
	}
}

func (e *Engine) rtsEval(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env, base string,
	extra map[string]rts.Value,
	extras ...map[string]string,
) vars.ExprEval {
	vv := e.collectVariables(doc, req, env)
	for _, extra := range extras {
		for k, v := range extra {
			vv[k] = v
		}
	}
	return func(expr string, pos vars.ExprPos) (string, error) {
		rt := e.buildRT(rtIn{
			doc:  doc,
			req:  req,
			env:  env,
			base: base,
			vars: vv,
			site: "{{= " + expr + " }}",
			x:    extra,
		})
		return e.re.EvalStr(ctx, rt, expr, rts.Pos{Path: pos.Path, Line: pos.Line, Col: pos.Col})
	}
}

func (e *Engine) rtsEvalValue(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env, base, expr, site string,
	pos rts.Pos,
	vv map[string]string,
	extra map[string]rts.Value,
) (rts.Value, error) {
	if vv == nil {
		vv = e.collectVariables(doc, req, env)
	}
	return e.re.Eval(ctx, e.buildRT(rtIn{
		doc:  doc,
		req:  req,
		env:  env,
		base: base,
		vars: vv,
		site: site,
		x:    extra,
	}), expr, pos)
}

func (e *Engine) PosForLine(doc *restfile.Document, req *restfile.Request, line int) rts.Pos {
	return e.rtsPosForLine(doc, req, line)
}

func (e *Engine) CollectVariables(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	extras ...map[string]string,
) map[string]string {
	return e.collectVariables(doc, req, env, extras...)
}

func (e *Engine) EvalValue(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env, base, expr, site string,
	pos rts.Pos,
	vv map[string]string,
	extra map[string]rts.Value,
) (rts.Value, error) {
	return e.rtsEvalValue(ctx, doc, req, env, base, expr, site, pos, vv, extra)
}

type ForEachSpec struct {
	Expr string
	Var  string
	Line int
}

func (e *Engine) EvalCondition(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env, base string,
	spec *restfile.ConditionSpec,
	vv map[string]string,
	extra map[string]rts.Value,
) (bool, string, error) {
	if spec == nil {
		return true, "", nil
	}
	expr := strings.TrimSpace(spec.Expression)
	if expr == "" {
		return true, "", nil
	}
	tag := "@when"
	if spec.Negate {
		tag = "@skip-if"
	}
	val, err := e.rtsEvalValue(
		ctx,
		doc,
		req,
		env,
		base,
		expr,
		tag+" "+expr,
		e.rtsPosForLine(doc, req, spec.Line),
		vv,
		extra,
	)
	if err != nil {
		return false, "", err
	}
	truthy := val.IsTruthy()
	shouldRun := truthy
	if spec.Negate {
		shouldRun = !truthy
	}
	if shouldRun {
		return true, "", nil
	}
	if spec.Negate {
		return false, fmt.Sprintf("@skip-if evaluated to true: %s", expr), nil
	}
	return false, fmt.Sprintf("@when evaluated to false: %s", expr), nil
}

func (e *Engine) EvalForEachItems(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env, base string,
	spec ForEachSpec,
	vv map[string]string,
	extra map[string]rts.Value,
) ([]rts.Value, error) {
	expr := strings.TrimSpace(spec.Expr)
	if expr == "" {
		return nil, fmt.Errorf("@for-each expression missing")
	}
	val, err := e.rtsEvalValue(
		ctx,
		doc,
		req,
		env,
		base,
		expr,
		"@for-each "+expr,
		e.rtsPosForLine(doc, req, spec.Line),
		vv,
		extra,
	)
	if err != nil {
		return nil, err
	}
	if val.K != rts.VList {
		return nil, fmt.Errorf("@for-each expects list result")
	}
	return val.L, nil
}

func (e *Engine) ValueString(ctx context.Context, pos rts.Pos, v rts.Value) (string, error) {
	cx := rts.NewCtx(ctx, e.re.Lim)
	return rts.ValueString(cx, pos, v)
}

func (e *Engine) runAsserts(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env, base string,
	vv map[string]string,
	extra map[string]rts.Value,
	resp *rts.Resp,
	tr *rts.Trace,
	st *rts.Stream,
) ([]scripts.TestResult, error) {
	if req == nil || len(req.Metadata.Asserts) == 0 {
		return nil, nil
	}
	if vv == nil {
		vv = e.collectVariables(doc, req, env)
	}
	ex := make(map[string]rts.Value)
	for k, v := range extra {
		if k == "" {
			continue
		}
		ex[k] = v
	}
	for k, v := range rts.AssertExtra(resp) {
		ex[k] = v
	}
	rt := e.buildRT(rtIn{
		doc:  doc,
		req:  req,
		env:  env,
		base: base,
		vars: vv,
		resp: resp,
		res:  resp,
		tr:   tr,
		st:   st,
		x:    ex,
	})
	out := make([]scripts.TestResult, 0, len(req.Metadata.Asserts))
	for _, as := range req.Metadata.Asserts {
		expr := strings.TrimSpace(as.Expression)
		if expr == "" {
			continue
		}
		rt.Site = "@assert " + expr
		start := time.Now()
		val, err := e.re.Eval(ctx, rt, expr, e.rtsPosForLine(doc, req, as.Line))
		if err != nil {
			return out, err
		}
		out = append(out, scripts.TestResult{
			Name:    expr,
			Message: strings.TrimSpace(as.Message),
			Passed:  val.IsTruthy(),
			Elapsed: time.Since(start),
		})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (e *Engine) runRTSPreRequest(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env, base string,
	vv map[string]string,
	globs map[string]prerequest.GlobalValue,
) (prerequest.Output, error) {
	var out prerequest.Output
	if req == nil {
		return out, nil
	}
	uses := e.rtsUses(doc, req)
	envs := e.rtsEnv(env)
	base = e.rtsBase(doc, base)
	gv := rtspre.RuntimeGlobals(globs, false)
	mut := rtspre.NewMutator(&out, e.rtsReq(req), vv, gv)
	empty := &rts.Resp{}

	err := rtspre.Run(ctx, e.re, rtspre.ExecInput{
		Doc:     doc,
		Scripts: req.Metadata.Scripts,
		BaseDir: base,
		BuildRT: func() rts.RT {
			return rts.RT{
				Env:         envs,
				Vars:        vv,
				Globals:     gv,
				Resp:        e.rtsLast(),
				Res:         empty,
				Req:         mut.Request(),
				ReqMut:      mut,
				VarsMut:     mut,
				GlobalMut:   mut,
				Uses:        uses,
				BaseDir:     base,
				ReadFile:    os.ReadFile,
				AllowRandom: true,
				Site:        "@script pre-request",
			}
		},
	})
	if err != nil {
		return out, err
	}
	prerequest.Normalize(&out)
	return out, nil
}

type applyPatch struct {
	method     *string
	url        *string
	headers    map[string][]string
	headerDels map[string]struct{}
	query      map[string]*string
	body       *string
	auth       *restfile.AuthSpec
	authSet    bool
	settings   map[string]*string
	vars       map[string]string
}

func (e *Engine) parseApplyPatch(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (applyPatch, error) {
	if v.K != rts.VDict {
		return applyPatch{}, applyErr("", "expects dict")
	}
	var p applyPatch
	for k, val := range v.M {
		switch key := applyKey(k); key {
		case "method":
			s, err := e.applyScalar(ctx, pos, val, "method")
			if err != nil {
				return applyPatch{}, err
			}
			s = strings.TrimSpace(s)
			if s == "" {
				return applyPatch{}, applyErr("method", "expects non-empty value")
			}
			p.method = &s
		case "url":
			s, err := e.applyScalar(ctx, pos, val, "url")
			if err != nil {
				return applyPatch{}, err
			}
			s = strings.TrimSpace(s)
			if s == "" {
				return applyPatch{}, applyErr("url", "expects non-empty value")
			}
			p.url = &s
		case "headers":
			set, del, err := e.parseApplyHeaders(ctx, pos, val)
			if err != nil {
				return applyPatch{}, err
			}
			p.headers, p.headerDels = set, del
		case "query":
			out, err := e.parseApplyQuery(ctx, pos, val)
			if err != nil {
				return applyPatch{}, err
			}
			p.query = out
		case "body":
			s, err := e.rtsValueString(ctx, pos, val)
			if err != nil {
				return applyPatch{}, applyErr("body", err.Error())
			}
			p.body = &s
		case "auth":
			if val.K == rts.VNull {
				p.authSet = true
				break
			}
			out, err := e.parseApplyAuth(ctx, pos, val)
			if err != nil {
				return applyPatch{}, err
			}
			p.auth, p.authSet = out, true
		case "settings":
			out, err := e.parseApplySettings(ctx, pos, val)
			if err != nil {
				return applyPatch{}, err
			}
			p.settings = out
		case "vars":
			out, err := e.parseApplyVars(ctx, pos, val)
			if err != nil {
				return applyPatch{}, err
			}
			p.vars = out
		default:
			if key == "" {
				return applyPatch{}, applyErr("", "empty field")
			}
			return applyPatch{}, applyErr("", fmt.Sprintf("unknown field %q", key))
		}
	}
	return p, nil
}

func (e *Engine) applyScalar(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
	field string,
) (string, error) {
	switch v.K {
	case rts.VStr, rts.VNum, rts.VBool:
		s, err := e.rtsValueString(ctx, pos, v)
		if err != nil {
			return "", applyErr(field, err.Error())
		}
		return s, nil
	default:
		return "", applyErr(field, "expects string/number/bool")
	}
}

func (e *Engine) rtsValueString(ctx context.Context, pos rts.Pos, v rts.Value) (string, error) {
	cx := rts.NewCtx(ctx, e.re.Lim)
	return rts.ValueString(cx, pos, v)
}

func (e *Engine) parseApplyHeaders(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (map[string][]string, map[string]struct{}, error) {
	if v.K != rts.VDict {
		return nil, nil, applyErr("headers", "expects dict")
	}
	set := make(map[string][]string)
	del := make(map[string]struct{})
	for key, val := range v.M {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, nil, applyErr("headers", "expects non-empty header name")
		}
		switch val.K {
		case rts.VNull:
			delete(set, name)
			del[name] = struct{}{}
		case rts.VList:
			vs, err := e.applyList(ctx, pos, val, "headers."+name)
			if err != nil {
				return nil, nil, err
			}
			set[name] = vs
			delete(del, name)
		default:
			s, err := e.applyScalar(ctx, pos, val, "headers."+name)
			if err != nil {
				return nil, nil, err
			}
			set[name] = []string{s}
			delete(del, name)
		}
	}
	if len(set) == 0 {
		set = nil
	}
	if len(del) == 0 {
		del = nil
	}
	return set, del, nil
}

func (e *Engine) parseApplyQuery(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (map[string]*string, error) {
	if v.K != rts.VDict {
		return nil, applyErr("query", "expects dict")
	}
	out := make(map[string]*string)
	for key, val := range v.M {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, applyErr("query", "expects non-empty key")
		}
		if val.K == rts.VNull {
			out[name] = nil
			continue
		}
		s, err := e.applyScalar(ctx, pos, val, "query."+name)
		if err != nil {
			return nil, err
		}
		cp := s
		out[name] = &cp
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (e *Engine) parseApplyVars(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (map[string]string, error) {
	if v.K != rts.VDict {
		return nil, applyErr("vars", "expects dict")
	}
	out := make(map[string]string)
	for key, val := range v.M {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, applyErr("vars", "expects non-empty name")
		}
		s, err := e.applyScalar(ctx, pos, val, "vars."+name)
		if err != nil {
			return nil, err
		}
		out[name] = s
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (e *Engine) parseApplyAuth(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (*restfile.AuthSpec, error) {
	if v.K != rts.VDict {
		return nil, applyErr("auth", "expects dict")
	}
	var typ string
	pm := make(map[string]string)
	for k, val := range v.M {
		key := applyKey(k)
		if key == "" {
			return nil, applyErr("auth", "expects non-empty key")
		}
		s, err := e.applyScalar(ctx, pos, val, "auth."+key)
		if err != nil {
			return nil, err
		}
		if key == "type" {
			typ = strings.ToLower(strings.TrimSpace(s))
			continue
		}
		pm[key] = s
	}
	if strings.TrimSpace(typ) == "" {
		return nil, applyErr("auth", "requires type")
	}
	if len(pm) == 0 {
		pm = nil
	}
	return &restfile.AuthSpec{
		Type:       typ,
		Params:     pm,
		SourcePath: pos.Path,
	}, nil
}

func (e *Engine) parseApplySettings(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (map[string]*string, error) {
	if v.K != rts.VDict {
		return nil, applyErr("settings", "expects dict")
	}
	out := make(map[string]*string)
	for k, val := range v.M {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			return nil, applyErr("settings", "expects non-empty key")
		}
		if val.K == rts.VNull {
			out[key] = nil
			continue
		}
		s, err := e.applyScalar(ctx, pos, val, "settings."+key)
		if err != nil {
			return nil, err
		}
		cp := s
		out[key] = &cp
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func (e *Engine) applyList(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
	field string,
) ([]string, error) {
	if v.K != rts.VList {
		return nil, applyErr(field, "expects list")
	}
	if len(v.L) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(v.L))
	for _, item := range v.L {
		s, err := e.applyScalar(ctx, pos, item, field)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func applyPatchToRequest(req *restfile.Request, vv map[string]string, p applyPatch) error {
	if req == nil {
		return nil
	}
	applyPatchMethod(req, p.method)
	applyPatchURL(req, p.url)
	if err := applyPatchQuery(req, p.query); err != nil {
		return err
	}
	applyPatchHeaders(req, p.headers, p.headerDels)
	applyPatchBody(req, p.body)
	applyPatchAuth(req, p.auth, p.authSet)
	applyPatchSettings(req, p.settings)
	applyPatchVars(req, vv, p.vars)
	return nil
}

func applyPatchMethod(req *restfile.Request, val *string) {
	if val != nil && req != nil {
		req.Method = strings.ToUpper(strings.TrimSpace(*val))
	}
}

func applyPatchURL(req *restfile.Request, val *string) {
	if val != nil && req != nil {
		req.URL = strings.TrimSpace(*val)
	}
}

func applyPatchQuery(req *restfile.Request, q map[string]*string) error {
	if req == nil || len(q) == 0 {
		return nil
	}
	raw := strings.TrimSpace(req.URL)
	if raw == "" {
		return nil
	}
	out, err := urltpl.PatchQuery(raw, q)
	if err != nil {
		return fmt.Errorf("invalid url after @apply: %w", err)
	}
	req.URL = out
	return nil
}

func applyPatchHeaders(req *restfile.Request, set map[string][]string, del map[string]struct{}) {
	if req == nil || (len(set) == 0 && len(del) == 0) {
		return
	}
	if req.Headers == nil {
		req.Headers = make(http.Header)
	}
	for name := range del {
		req.Headers.Del(name)
	}
	for name, vs := range set {
		req.Headers.Del(name)
		for _, v := range vs {
			req.Headers.Add(name, v)
		}
	}
}

func applyPatchBody(req *restfile.Request, val *string) {
	if req == nil || val == nil {
		return
	}
	req.Body.FilePath = ""
	req.Body.Text = *val
	req.Body.GraphQL = nil
}

func applyPatchAuth(req *restfile.Request, auth *restfile.AuthSpec, set bool) {
	if req == nil || !set {
		return
	}
	if auth == nil {
		req.Metadata.Auth = nil
		req.Metadata.AuthDisabled = true
		return
	}
	req.Metadata.Auth = restfile.CloneAuthSpec(auth)
	req.Metadata.AuthDisabled = false
}

func applyPatchSettings(req *restfile.Request, in map[string]*string) {
	if req == nil || len(in) == 0 {
		return
	}
	if req.Settings == nil {
		req.Settings = make(map[string]string, len(in))
	}
	for k, v := range in {
		if v == nil {
			delete(req.Settings, k)
			continue
		}
		req.Settings[k] = *v
	}
}

func applyPatchVars(req *restfile.Request, vv map[string]string, in map[string]string) {
	if req == nil || len(in) == 0 {
		return
	}
	setRequestVars(req, in)
	for k, v := range in {
		vv[k] = v
	}
}

func setRequestVars(req *restfile.Request, vv map[string]string) {
	if req == nil || len(vv) == 0 {
		return
	}
	idxs := make(map[string]int)
	for i, v := range req.Variables {
		idxs[strings.ToLower(v.Name)] = i
	}
	for name, val := range vv {
		key := strings.ToLower(name)
		if idx, ok := idxs[key]; ok {
			req.Variables[idx].Value = val
			continue
		}
		req.Variables = append(req.Variables, restfile.Variable{
			Name:  name,
			Value: val,
			Scope: restfile.ScopeRequest,
		})
	}
}

func applyKey(key string) string { return strings.ToLower(strings.TrimSpace(key)) }

func applyErr(field, msg string) error {
	if field == "" {
		return fmt.Errorf("@apply %s", msg)
	}
	return fmt.Errorf("@apply %s: %s", field, msg)
}

type applyExpr struct {
	ex string
	ps rts.Pos
	st string
}

func (e *Engine) applyExprs(
	doc *restfile.Document,
	req *restfile.Request,
	sp restfile.ApplySpec,
	idx int,
) ([]applyExpr, error) {
	if len(sp.Uses) > 0 {
		out := make([]applyExpr, 0, len(sp.Uses))
		for _, name := range sp.Uses {
			pf, ok := e.findPatchProfile(doc, name)
			if !ok {
				return nil, fmt.Errorf("@apply use=%q not found", name)
			}
			ex := strings.TrimSpace(pf.Expression)
			if ex == "" {
				return nil, fmt.Errorf("@apply use=%q has an empty expression", name)
			}
			ps := e.rtsPosForLine(doc, req, pf.Line)
			if pf.Col > 0 {
				ps.Col = pf.Col
			}
			if pf.SourcePath != "" {
				ps.Path = pf.SourcePath
			}
			out = append(out, applyExpr{
				ex: ex,
				ps: ps,
				st: fmt.Sprintf("@apply %d use=%s", idx+1, name),
			})
		}
		return out, nil
	}
	ex := strings.TrimSpace(sp.Expression)
	if ex == "" {
		return nil, nil
	}
	ps := e.rtsPosForLine(doc, req, sp.Line)
	if sp.Col > 0 {
		ps.Col = sp.Col
	}
	return []applyExpr{{
		ex: ex,
		ps: ps,
		st: fmt.Sprintf("@apply %d", idx+1),
	}}, nil
}

func (e *Engine) findPatchProfile(
	doc *restfile.Document,
	name string,
) (*restfile.PatchProfile, bool) {
	return e.registryIndex().PatchNamed(doc, name)
}

func (e *Engine) runRTSApply(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env, base string,
	vv map[string]string,
) error {
	if req == nil || len(req.Metadata.Applies) == 0 {
		return nil
	}
	for idx, spec := range req.Metadata.Applies {
		if err := ctx.Err(); err != nil {
			return err
		}
		exprs, err := e.applyExprs(doc, req, spec, idx)
		if err != nil {
			return err
		}
		for _, ex := range exprs {
			if err := ctx.Err(); err != nil {
				return err
			}
			val, err := e.rtsEvalValue(ctx, doc, req, env, base, ex.ex, ex.st, ex.ps, vv, nil)
			if err != nil {
				return err
			}
			patch, err := e.parseApplyPatch(ctx, ex.ps, val)
			if err != nil {
				return err
			}
			if err := applyPatchToRequest(req, vv, patch); err != nil {
				return err
			}
		}
	}
	return nil
}

func joinErr(a, b error) error {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	return fmt.Errorf("%v; %v", a, b)
}
