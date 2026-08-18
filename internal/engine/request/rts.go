package request

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/http/urltpl"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rtshost"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	str "github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (e *Engine) rtsReq(req *restfile.Request) (*rtshost.Request, error) {
	if req == nil {
		return nil, nil
	}
	out, err := rtshost.NewRequest(
		strings.TrimSpace(req.Method),
		strings.TrimSpace(req.URL),
		req.Headers,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return out, nil
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

func (e *Engine) rtsLast() (*rtshost.Response, error) {
	switch {
	case e.last.http != nil:
		return rtsHTTP(e.last.http)
	case e.last.grpc != nil:
		return rtsGRPC(e.last.grpc)
	default:
		return nil, nil
	}
}

// rtsLastTrace binds the most recent HTTP response's trace so RTS evaluation
// (trace.enabled(), trace.withinBudget(), trace.breaches()) resolves against it
// when no explicit trace is supplied.
func (e *Engine) rtsLastTrace() *rtshost.Trace {
	if e.last.http != nil {
		return rtsTrace(e.last.http)
	}
	return nil
}

func rtsHTTP(resp *httpx.Response) (*rtshost.Response, error) {
	if resp == nil {
		return nil, nil
	}
	return rtshost.NewResponse(
		resp.Status,
		resp.StatusCode,
		resp.Headers,
		resp.Body,
		resp.EffectiveURL,
	)
}

func rtsGRPC(resp *grpcx.Response) (*rtshost.Response, error) {
	if resp == nil {
		return nil, nil
	}
	// Same map scripts read, so an assertion and a script name a trailer the
	// same way: prefixed with Grpc-Trailer- and binary metadata base64-encoded.
	return rtshost.NewResponse(
		resp.StatusMessage,
		int(resp.StatusCode),
		resp.HeaderMap(),
		resp.Body,
		"",
	)
}

func rtsTrace(resp *httpx.Response) *rtshost.Trace {
	if resp == nil || resp.TraceReport == nil {
		return nil
	}
	return &rtshost.Trace{Report: resp.TraceReport.Clone()}
}

func rtsScriptResp(resp *scripts.Response) (*rtshost.Response, error) {
	if resp == nil {
		return nil, nil
	}
	return rtshost.NewResponse(resp.Status, resp.Code, resp.Header, resp.Body, resp.URL)
}

func rtsStream(info *scripts.StreamInfo) *rtshost.Stream {
	if info == nil {
		return nil
	}
	sum := make(map[string]any, len(info.Summary))
	maps.Copy(sum, info.Summary)
	evs := make([]map[string]any, len(info.Events))
	for i, item := range info.Events {
		if item == nil {
			continue
		}
		cp := make(map[string]any, len(item))
		maps.Copy(cp, item)
		evs[i] = cp
	}
	return &rtshost.Stream{Kind: info.Kind, Summary: sum, Events: evs}
}

type evalScope struct {
	vars    map[string]string
	globals vars.Globals
}

// Keep the caller-owned map so mutations and later expressions share one
// runtime view. The host validates its names when it builds the scope.
func newEvalScope(vv map[string]string, globals vars.Globals) evalScope {
	return evalScope{vars: vv, globals: globals}
}

func (e *Engine) storedScope(
	doc *restfile.Document,
	env vars.ResolvedEnv,
	vv map[string]string,
) evalScope {
	return newEvalScope(vv, e.collectGlobalValues(doc, env))
}

type rtIn struct {
	doc *restfile.Document
	req *restfile.Request
	// buildRT uses env, globals, and secrets to prepare the scope.
	// buildRTWithScope receives a prepared scope and ignores them.
	env     vars.ResolvedEnv
	base    string
	vars    map[string]string
	globals vars.Globals
	site    string
	resp    *rtshost.Response
	res     *rtshost.Response
	tr      *rtshost.Trace
	st      *rtshost.Stream
	locals  rts.Locals
	secrets rtshost.SecretPolicy
}

func prepareScope(
	env vars.ResolvedEnv,
	globals vars.Globals,
	secrets rtshost.SecretPolicy,
) (rtshost.PreparedScope, error) {
	return rtshost.PrepareScope(rtshost.ScopeInput{
		EnvName: env.Label(),
		Env:     env.Values(),
		Groups:  env.Selection().Groups(),
		Globals: rtshost.RuntimeGlobals(globals, secrets),
	})
}

func (e *Engine) buildRT(in rtIn) (rtshost.Runtime, error) {
	prep, err := prepareScope(in.env, in.globals, in.secrets)
	if err != nil {
		return rtshost.Runtime{}, err
	}
	return e.buildRTWithScope(in, prep)
}

// buildRTWithScope builds a runtime from prepared scope data. It still binds
// vars, the request, and the last response on each call because they may change
// between expressions.
func (e *Engine) buildRTWithScope(in rtIn, prep rtshost.PreparedScope) (rtshost.Runtime, error) {
	base := e.rtsBase(in.doc, in.base)
	resp := in.resp
	if resp == nil {
		var err error
		resp, err = e.rtsLast()
		if err != nil {
			return rtshost.Runtime{}, err
		}
	}
	// Keep in.res nil before the request runs. Falling back to resp would make
	// response refer to last.
	tr := in.tr
	if tr == nil {
		tr = e.rtsLastTrace()
	}
	scope, err := prep.BindVars(in.vars)
	if err != nil {
		return rtshost.Runtime{}, err
	}
	req, err := e.rtsReq(in.req)
	if err != nil {
		return rtshost.Runtime{}, err
	}
	return rtshost.Runtime{
		Scope:       scope,
		Last:        resp,
		Response:    in.res,
		Trace:       tr,
		Stream:      in.st,
		Request:     req,
		BaseDir:     base,
		AllowRandom: true,
		Site:        in.site,
		Uses:        e.rtsUses(in.doc, in.req),
		Extensions:  e.rtsExtensions(),
		Locals:      in.locals,
	}, nil
}

// The inspector is a host extension, so it is additive and never shadows a
// standard library name. Locals are overlaid after it, which is what lets an
// @for-each variable named mock win without a special case here.
func (e *Engine) rtsExtensions() rts.Extensions {
	if e.cfg.MockInspector == nil {
		return rts.Extensions{}
	}
	return rts.Extension("mock", mock.RTSValue(e.cfg.MockInspector))
}

// ExprInput contains the request data available to a {{= expr }} expression.
type ExprInput struct {
	Doc      *restfile.Document
	Req      *restfile.Request
	Env      vars.ResolvedEnv
	Base     string
	Vars     map[string]string
	Response *rtshost.Response
	Stream   *rtshost.Stream
	Locals   rts.Locals
	globals  vars.Globals
}

// EvalInput is one expression evaluation. Expr is the source to evaluate, Site
// is the directive text it came from, used in diagnostics.
type EvalInput struct {
	Doc     *restfile.Document
	Req     *restfile.Request
	Env     vars.ResolvedEnv
	Base    string
	Expr    string
	Site    string
	Pos     rts.Pos
	Vars    map[string]string
	Locals  rts.Locals
	globals vars.Globals
}

// ExprEvalOptions tunes how a {{= expr }} evaluator treats the RTS runtime.
type ExprEvalOptions struct {
	// OmitSecretGlobals drops secret global values from the runtime so preview
	// and status evaluation cannot expose them.
	OmitSecretGlobals bool
}

// ExprEval returns a {{= expr }} evaluator with default policy.
func (e *Engine) ExprEval(ctx context.Context, in ExprInput) vars.ExprEval {
	return e.ExprEvalWithOptions(ctx, in, ExprEvalOptions{})
}

// ExprEvalWithOptions is ExprEval with an explicit evaluation policy.
func (e *Engine) ExprEvalWithOptions(
	ctx context.Context,
	in ExprInput,
	opt ExprEvalOptions,
) vars.ExprEval {
	secrets := rtshost.IncludeSecrets
	if opt.OmitSecretGlobals {
		secrets = rtshost.OmitSecrets
	}
	// Keep Vars by reference so script writes are visible to later expressions.
	vv := in.Vars
	// Validate the environment and globals on the first RTS expression. Most
	// templates use only simple variables and never need this work. OnceValues
	// also ensures every expression receives the same validation result.
	prepare := sync.OnceValues(func() (rtshost.PreparedScope, error) {
		return prepareScope(in.Env, in.globals, secrets)
	})
	return func(expr string, pos vars.ExprPos) (string, error) {
		prep, err := prepare()
		if err != nil {
			return "", err
		}
		rt, err := e.buildRTWithScope(rtIn{
			doc:  in.Doc,
			req:  in.Req,
			base: in.Base,
			vars: vv,
			site: "{{= " + expr + " }}",
			// res binds response. resp remains the previous response used by last.
			res:    in.Response,
			st:     in.Stream,
			locals: in.Locals,
		}, prep)
		if err != nil {
			return "", err
		}
		return e.evalRTSString(
			ctx,
			in.Doc,
			rt,
			expr,
			rts.Pos{Path: pos.Path, Line: pos.Line, Col: pos.Col},
		)
	}
}

func (e *Engine) evalRTSValue(
	ctx context.Context,
	doc *restfile.Document,
	rt rtshost.Runtime,
	expr string,
	pos rts.Pos,
) (rts.Value, error) {
	v, err := e.re.Eval(ctx, rt, expr, pos)
	return v, e.rtsErr(err, doc)
}

func (e *Engine) evalRTSAssert(
	ctx context.Context,
	doc *restfile.Document,
	rt rtshost.Runtime,
	expr string,
	pos rts.Pos,
) (rts.Value, error) {
	v, err := e.re.EvalAssertion(ctx, rt, expr, pos)
	return v, e.rtsErr(err, doc)
}

func (e *Engine) evalRTSString(
	ctx context.Context,
	doc *restfile.Document,
	rt rtshost.Runtime,
	expr string,
	pos rts.Pos,
) (string, error) {
	s, err := e.re.EvalStr(ctx, rt, expr, pos)
	return s, e.rtsErr(err, doc)
}

func (e *Engine) rtsEvalValue(ctx context.Context, in EvalInput) (rts.Value, error) {
	vv := in.Vars
	if vv == nil {
		vv = e.collectVariables(in.Doc, in.Req, in.Env, runVars{})
	}
	rt, err := e.buildRT(rtIn{
		doc:     in.Doc,
		req:     in.Req,
		env:     in.Env,
		base:    in.Base,
		vars:    vv,
		globals: in.globals,
		site:    in.Site,
		locals:  in.Locals,
		secrets: rtshost.IncludeSecrets,
	})
	if err != nil {
		return rts.Null(), err
	}
	return e.evalRTSValue(ctx, in.Doc, rt, in.Expr, in.Pos)
}

func (e *Engine) PosForLine(doc *restfile.Document, req *restfile.Request, line int) rts.Pos {
	return e.rtsPosForLine(doc, req, line)
}

func (e *Engine) CollectVariables(
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
	overlay map[string]string,
) map[string]string {
	return e.collectVariables(
		doc,
		req,
		ResolveEnvironment(env, doc, req),
		runVars{overlay: vars.CollectNames(overlay)},
	)
}

func (e *Engine) EvalValue(ctx context.Context, in EvalInput) (rts.Value, error) {
	in.globals = e.collectGlobalValues(in.Doc, in.Env)
	return e.rtsEvalValue(ctx, in)
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
	src vars.Environment,
	base string,
	spec *restfile.ConditionSpec,
	vv map[string]string,
	locals rts.Locals,
) (bool, string, error) {
	env := ResolveEnvironment(src, doc, req)
	return e.evalCondition(ctx, doc, req, env, base, spec, e.storedScope(doc, env, vv), locals)
}

func (e *Engine) evalCondition(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.ResolvedEnv,
	base string,
	spec *restfile.ConditionSpec,
	sc evalScope,
	locals rts.Locals,
) (bool, string, error) {
	if spec == nil {
		return true, "", nil
	}
	expr := strings.TrimSpace(spec.Expression)
	if expr == "" {
		return true, "", nil
	}
	tag := directive.When.Tag()
	if spec.Negate {
		tag = directive.SkipIf.Tag()
	}
	flat := str.FoldLines(expr)
	val, err := e.rtsEvalValue(ctx, EvalInput{
		Doc:     doc,
		Req:     req,
		Env:     env,
		Base:    base,
		Expr:    expr,
		Site:    tag + " " + flat,
		Pos:     e.rtsPosForLineCol(doc, req, spec.Line, spec.Col),
		Vars:    sc.vars,
		Locals:  locals,
		globals: sc.globals,
	})
	if err != nil {
		return false, "", err
	}
	truthy := val.IsTruthy()
	if truthy != spec.Negate {
		return true, "", nil
	}
	reason := fmt.Sprintf("%s evaluated to %t: %s", tag, truthy, flat)
	return false, reason, nil
}

func (e *Engine) EvalForEachItems(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	src vars.Environment,
	base string,
	spec ForEachSpec,
	vv map[string]string,
	locals rts.Locals,
) ([]rts.Value, error) {
	expr := strings.TrimSpace(spec.Expr)
	if expr == "" {
		return nil, fmt.Errorf("@for-each expression missing")
	}
	env := ResolveEnvironment(src, doc, req)
	sc := e.storedScope(doc, env, vv)
	val, err := e.rtsEvalValue(ctx, EvalInput{
		Doc:     doc,
		Req:     req,
		Env:     env,
		Base:    base,
		Expr:    expr,
		Site:    directive.ForEach.Tag() + " " + str.FoldLines(expr),
		Pos:     e.rtsPosForLine(doc, req, spec.Line),
		Vars:    sc.vars,
		Locals:  locals,
		globals: sc.globals,
	})
	if err != nil {
		return nil, err
	}
	if val.K != rts.VList {
		return nil, fmt.Errorf("@for-each expects list result")
	}
	return val.L, nil
}

func (e *Engine) ValueString(ctx context.Context, pos rts.Pos, v rts.Value) (string, error) {
	cx := rts.NewCtx(ctx, e.re.Limits())
	return rts.ValueString(cx, pos, v)
}

func (e *Engine) runAsserts(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.ResolvedEnv,
	base string,
	sc evalScope,
	locals rts.Locals,
	resp *rtshost.Response,
	tr *rtshost.Trace,
	st *rtshost.Stream,
) ([]scripts.TestResult, error) {
	if req == nil || len(req.Metadata.Asserts) == 0 {
		return nil, nil
	}
	vv := sc.vars
	if vv == nil {
		vv = e.collectVariables(doc, req, env, runVars{})
	}
	rt, err := e.buildRT(rtIn{
		doc:     doc,
		req:     req,
		env:     env,
		base:    base,
		vars:    vv,
		globals: sc.globals,
		resp:    resp,
		res:     resp,
		tr:      tr,
		st:      st,
		locals:  locals,
		secrets: rtshost.IncludeSecrets,
	})
	if err != nil {
		return nil, err
	}
	out := make([]scripts.TestResult, 0, len(req.Metadata.Asserts))
	for _, as := range req.Metadata.Asserts {
		expr := strings.TrimSpace(as.Expression)
		if expr == "" {
			continue
		}
		name := str.FoldLines(expr)
		rt.Site = directive.Assert.Tag() + " " + name
		start := time.Now()
		val, err := e.evalRTSAssert(ctx, doc, rt, expr, e.rtsPosForLineCol(doc, req, as.Line, as.Col))
		if err != nil {
			return out, err
		}
		out = append(out, scripts.TestResult{
			Name:    name,
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

func (e *Engine) RunPreRequest(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
	base string,
	vv map[string]string,
	locals rts.Locals,
	globals vars.Globals,
) (prerequest.Output, error) {
	sc := newEvalScope(vv, globals)
	return e.runRTSPreRequest(ctx, doc, req, ResolveEnvironment(env, doc, req), base, sc, locals, nil)
}

func (e *Engine) runRTSPreRequest(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.ResolvedEnv,
	base string,
	sc evalScope,
	locals rts.Locals,
	secrets *vars.Secrets,
) (prerequest.Output, error) {
	vv := sc.vars
	var out prerequest.Output
	if req == nil {
		return out, nil
	}
	base = e.rtsBase(doc, base)
	gv := rtshost.RuntimeGlobals(sc.globals, rtshost.IncludeSecrets)
	// The mutator and each scope share gv so later blocks see global changes.
	si := rtshost.ScopeInput{
		EnvName: env.Label(),
		Env:     env.Values(),
		Groups:  env.Selection().Groups(),
		Globals: gv,
	}
	rt, err := e.buildRT(rtIn{
		doc:     doc,
		req:     req,
		env:     env,
		base:    base,
		vars:    vv,
		globals: sc.globals,
		site:    "@script pre-request",
		locals:  locals,
		secrets: rtshost.IncludeSecrets,
	})
	if err != nil {
		return out, err
	}
	// Use the mutator's request view so reads reflect earlier writes.
	mut := rtshost.NewMutator(&out, rt.Request, vv, gv, secrets)
	rt.Request = mut.Request()
	rt.Mutator = mut

	err = rtshost.RunPreRequest(ctx, e.re, rtshost.PreRequest{
		Doc:     doc,
		Scripts: req.Metadata.Scripts,
		BaseDir: base,
		Runtime: func() (rtshost.Runtime, error) {
			// Rebind the scope so each block sees earlier variable and global writes.
			scope, err := rtshost.NewScope(si, vv)
			if err != nil {
				return rtshost.Runtime{}, err
			}
			rt.Scope = scope
			return rt, nil
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
	vars       vars.NameMap[string]
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
	cx := rts.NewCtx(ctx, e.re.Limits())
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
	// A patch names headers the same way a script does, so it is held to the
	// same rule: the name has to be one the request can carry, and two forms of
	// one header are refused because http.Header would fold them and the winner
	// would depend on map order.
	keys, err := header.Keys(v.M)
	if err != nil {
		return nil, nil, applyErr("headers", err.Error())
	}
	set := make(map[string][]string)
	del := make(map[string]struct{})
	for _, k := range keys {
		name := k.Source
		val := v.M[name]
		switch val.K {
		case rts.VNull:
			del[name] = struct{}{}
		case rts.VList:
			vs, err := e.applyList(ctx, pos, val, "headers."+name)
			if err != nil {
				return nil, nil, err
			}
			set[name] = vs
		default:
			s, err := e.applyScalar(ctx, pos, val, "headers."+name)
			if err != nil {
				return nil, nil, err
			}
			set[name] = []string{s}
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
	// Query keys are exact strings, so a patch names a parameter the way
	// query.merge and request.setQueryParam do. Case, surrounding whitespace,
	// and the empty key are all data the URL can carry.
	out := make(map[string]*string)
	for key, val := range v.M {
		if val.K == rts.VNull {
			out[key] = nil
			continue
		}
		s, err := e.applyScalar(ctx, pos, val, "query."+key)
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

// parseApplyVars sorts keys so equivalent names and validation errors resolve deterministically.
func (e *Engine) parseApplyVars(
	ctx context.Context,
	pos rts.Pos,
	v rts.Value,
) (vars.NameMap[string], error) {
	var out vars.NameMap[string]
	if v.K != rts.VDict {
		return out, applyErr("vars", "expects dict")
	}
	for _, key := range slices.Sorted(maps.Keys(v.M)) {
		name := strings.TrimSpace(key)
		if name == "" {
			return out, applyErr("vars", "expects non-empty name")
		}
		s, err := e.applyScalar(ctx, pos, v.M[key], "vars."+name)
		if err != nil {
			return out, err
		}
		out.Set(name, s)
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
		Type:       restfile.AuthKind(typ),
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
	req.Metadata.Auth = auth.Clone()
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

func applyPatchVars(req *restfile.Request, vv map[string]string, in vars.NameMap[string]) {
	if req == nil || in.Len() == 0 {
		return
	}
	prerequest.SetRequestVars(req, in)
	if vv == nil {
		return
	}
	vars.MergeInto(vv, in)
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
			ps := e.rtsPosForLineCol(doc, req, pf.Line, pf.Col)
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
	ps := e.rtsPosForLineCol(doc, req, sp.Line, sp.Col)
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
	env vars.ResolvedEnv,
	base string,
	sc evalScope,
	locals rts.Locals,
) error {
	vv := sc.vars
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
			val, err := e.rtsEvalValue(ctx, EvalInput{
				Doc:     doc,
				Req:     req,
				Env:     env,
				Base:    base,
				Expr:    ex.ex,
				Site:    ex.st,
				Pos:     ex.ps,
				Vars:    vv,
				Locals:  locals,
				globals: sc.globals,
			})
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

func (e *Engine) ApplyPatches(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	src vars.Environment,
	base string,
	vv map[string]string,
	locals rts.Locals,
) error {
	env := ResolveEnvironment(src, doc, req)
	return e.runRTSApply(ctx, doc, req, env, base, e.storedScope(doc, env, vv), locals)
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
