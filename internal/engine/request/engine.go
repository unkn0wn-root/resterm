package request

import (
	"context"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	xexec "github.com/unkn0wn-root/resterm/internal/exec"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/registry"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/tracebudget"
	"github.com/unkn0wn-root/resterm/internal/tunnel"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type xrunResult = xexec.RequestResult

type lastState struct {
	http *httpclient.Response
	grpc *grpcclient.Response
}

type Engine struct {
	cfg  engine.Config
	rt   *rtrun.Runtime
	hc   *httpclient.Client
	gc   *grpcclient.Client
	sc   *scripts.Runner
	re   *rts.Eng
	rg   *registry.Index
	last lastState
}

type ExecMode uint8

const (
	ExecModeSend ExecMode = iota
	ExecModePreview
)

type ExecOptions struct {
	Extra      map[string]string
	Values     map[string]rts.Value
	Record     bool
	Ctx        context.Context
	Mode       ExecMode
	AttachSSE  func(*httpclient.StreamHandle, *restfile.Request)
	AttachWS   func(*httpclient.WebSocketHandle, *restfile.Request)
	AttachGRPC func(*stream.Session, *restfile.Request)
	Release    func()
}

func New(cfg engine.Config, rt *rtrun.Runtime) *Engine {
	hc := cfg.Client
	if hc == nil {
		hc = httpclient.NewClient(nil)
	}
	if rt == nil {
		rt = rtrun.New(rtrun.Config{
			Client:     hc,
			History:    cfg.History,
			SSHManager: cfg.SSHManager,
			K8sManager: cfg.K8sManager,
		})
	}
	return &Engine{
		cfg: cfg,
		rt:  rt,
		hc:  hc,
		gc:  grpcclient.NewClient(),
		sc:  scripts.NewRunner(nil),
		re:  rts.NewEng(),
		rg:  cfg.Registry,
	}
}

func (e *Engine) SetConfig(cfg engine.Config) {
	if e == nil {
		return
	}
	prev := e.cfg
	e.cfg = cfg
	if cfg.Client != nil {
		e.hc = cfg.Client
	} else if e.hc == nil {
		e.hc = httpclient.NewClient(nil)
	}
	if cfg.Registry != nil {
		e.rg = cfg.Registry
	} else if prev.Registry != nil {
		e.rg = nil
	}
}

func (e *Engine) Execute(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
) (engine.RequestResult, error) {
	return e.ExecuteWith(doc, req, env, ExecOptions{Record: true})
}

func (e *Engine) ExecuteWith(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	opt ExecOptions,
) (engine.RequestResult, error) {
	if req == nil {
		return engine.RequestResult{}, errdef.New(errdef.CodeUI, "request is nil")
	}

	e.syncRegistry(doc)
	env = e.envName(env)
	req = cloneRequest(req)
	opts := e.resolveHTTPOptions(doc, e.cfg.HTTPOptions)
	if ck := e.rt.Cookies(); ck != nil {
		opts.CookieJar = ck.GetOrCreate(env)
	}
	if tunnel.HasConflict(req.SSH != nil, req.K8s != nil) {
		err := errdef.New(errdef.CodeHTTP, "@ssh cannot be combined with @k8s")
		exp := newExplainBuilder(e, doc, req, env, opt.Mode == ExecModePreview)
		exp.stage(
			xplain.StageRoute,
			xplain.StageError,
			xplain.SummaryRouteConfigInvalid,
			nil,
			nil,
			err.Error(),
		)
		out := engine.RequestResult{
			Err:         err,
			Executed:    req,
			Environment: env,
			Explain:     exp.finish(xplain.StatusError, "Route resolution failed", err),
		}
		return out, nil
	}
	if req.Metadata.Trace != nil && req.Metadata.Trace.Enabled {
		opts.Trace = true
		if budget, ok := tracebudget.FromSpec(req.Metadata.Trace); ok {
			opts.TraceBudget = &budget
		}
	}

	start := time.Now()
	x := newExec(e, doc, req, env, opts, opt)
	res := xexec.RunRequest(flow{ctx: x})
	end := time.Now()
	res.Timing = requestTiming(start, end, res)
	if opt.Record {
		e.record(doc, req, runResult{
			Response:       res.Response,
			GRPC:           res.GRPC,
			RuntimeSecrets: res.RuntimeSecrets,
			RequestText:    res.RequestText,
			Environment:    res.Environment,
			Skipped:        res.Skipped,
			SkipReason:     res.SkipReason,
			Executed:       res.Executed,
		})
	}
	e.store(res)
	return toResult(res), nil
}

func (e *Engine) store(res xrunResult) {
	switch {
	case res.GRPC != nil:
		e.last.grpc = res.GRPC
		e.last.http = nil
	case res.Err == nil && res.Response != nil:
		e.last.http = res.Response
		e.last.grpc = nil
	}
}

func toResult(res xrunResult) engine.RequestResult {
	return engine.RequestResult{
		Response:       res.Response,
		GRPC:           res.GRPC,
		Stream:         res.Stream,
		Transcript:     copyBytes(res.Transcript),
		Err:            res.Err,
		Tests:          append([]scripts.TestResult(nil), res.Tests...),
		ScriptErr:      res.ScriptErr,
		Executed:       res.Executed,
		RequestText:    res.RequestText,
		RuntimeSecrets: append([]string(nil), res.RuntimeSecrets...),
		Environment:    res.Environment,
		Skipped:        res.Skipped,
		SkipReason:     res.SkipReason,
		Preview:        res.Preview,
		Explain:        res.Explain,
		Timing:         res.Timing,
	}
}

type execCtx struct {
	eng *Engine
	doc *restfile.Document
	req *restfile.Request
	env string
	mod ExecMode

	opts httpclient.Options

	sendCtx context.Context
	cancel  context.CancelFunc
	rel     func()

	baseVars  map[string]string
	storeG    map[string]scripts.GlobalValue
	hasRTSPre bool
	hasJSPre  bool
	scriptV   map[string]string
	extraV    map[string]string
	extraX    map[string]rts.Value

	res     *vars.Resolver
	mset    map[string]string
	sshPlan *ssh.Plan
	k8sPlan *k8s.Plan

	useGRPC  bool
	grpcOpts grpcclient.Options
	timeout  time.Duration

	runtimeSecrets []string
	trace          *vars.Trace
	exp            *explainBuilder
	onSSE          func(*httpclient.StreamHandle, *restfile.Request)
	onWS           func(*httpclient.WebSocketHandle, *restfile.Request)
	onGRPC         func(*stream.Session, *restfile.Request)
}

func newExec(
	e *Engine,
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	opts httpclient.Options,
	opt ExecOptions,
) *execCtx {
	baseCtx := opt.Ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(baseCtx)
	base := e.collectVariables(doc, req, env)
	for k, v := range opt.Extra {
		base[k] = v
	}
	hasRTS, hasJS := detectPreRequestScripts(req)
	exp := newExplainBuilder(e, doc, req, env, opt.Mode == ExecModePreview)
	if req != nil &&
		(req.Metadata.When != nil || len(req.Metadata.Applies) > 0 || hasRTS || hasJS) {
		exp.warn(
			"Variable trace covers template resolution only; RTS/JS script internals are not traced",
		)
	}
	return &execCtx{
		eng:       e,
		doc:       doc,
		req:       req,
		env:       env,
		mod:       opt.Mode,
		opts:      opts,
		sendCtx:   ctx,
		cancel:    cancel,
		rel:       opt.Release,
		baseVars:  base,
		storeG:    e.collectStoredGlobalValues(env),
		hasRTSPre: hasRTS,
		hasJSPre:  hasJS,
		extraV:    cloneStringMap(opt.Extra),
		extraX:    cloneValueMap(opt.Values),
		exp:       exp,
		onSSE:     opt.AttachSSE,
		onWS:      opt.AttachWS,
		onGRPC:    opt.AttachGRPC,
	}
}

func detectPreRequestScripts(req *restfile.Request) (bool, bool) {
	if req == nil {
		return false, false
	}
	var hasRTS, hasJS bool
	for _, blk := range req.Metadata.Scripts {
		if isRTSPre(blk) {
			hasRTS = true
		}
		if strings.ToLower(blk.Kind) == "pre-request" && scriptLang(blk.Lang) == "js" {
			hasJS = true
		}
	}
	return hasRTS, hasJS
}

func (x *execCtx) base() xrunResult {
	return xrunResult{
		Executed:       x.req,
		Environment:    x.env,
		RuntimeSecrets: append([]string(nil), x.runtimeSecrets...),
	}
}

func (x *execCtx) fail(err error, dec string, cause ...error) *xrunResult {
	res := x.base()
	res.Err = err
	why := err
	if len(cause) > 0 && cause[0] != nil {
		why = cause[0]
	}
	if x.exp != nil {
		res.Explain = x.exp.finish(xplain.StatusError, dec, why)
	}
	return &res
}

func (x *execCtx) skip(reason string) *xrunResult {
	res := x.base()
	res.RequestText = x.reqText()
	res.Skipped = true
	res.SkipReason = reason
	if x.exp != nil {
		res.Explain = x.exp.finish(xplain.StatusSkipped, reason, nil)
	}
	return &res
}

func (x *execCtx) canceled(err error) *xrunResult {
	return x.fail(err, "Request canceled")
}

func (x *execCtx) reqText() string { return renderRequestText(x.req) }

func (x *execCtx) currentVariables() map[string]string {
	return x.eng.collectVariablesWithGlobals(x.doc, x.req, x.env, x.storeG, x.extraV)
}

func (x *execCtx) currentGlobals() map[string]scripts.GlobalValue {
	return effectiveGlobalValues(x.doc, x.storeG)
}

func (x *execCtx) captureVariables() map[string]string {
	return mergeStringMaps(x.eng.collectVariables(x.doc, x.req, x.env, x.extraV), x.scriptV)
}

func (x *execCtx) applyRuntimeGlobals(ch map[string]scripts.GlobalValue) {
	if len(ch) == 0 {
		return
	}
	if x.preview() {
		x.storeG = mergeGlobalValues(x.storeG, ch)
	} else {
		x.eng.applyGlobalMutations(ch, x.env)
		x.storeG = x.eng.collectStoredGlobalValues(x.env)
	}
	if x.exp != nil {
		x.exp.globals = effectiveGlobalValues(x.doc, x.storeG)
	}
}

func (x *execCtx) preview() bool {
	return x != nil && x.mod == ExecModePreview
}

func requestTiming(start, end time.Time, res xrunResult) engine.Timing {
	tm := engine.Timing{Start: start, End: end}
	if !start.IsZero() && !end.IsZero() && !end.Before(start) {
		tm.Total = end.Sub(start)
	}
	switch {
	case res.Response != nil && res.Response.Duration > 0:
		tm.Transport = res.Response.Duration
	case res.GRPC != nil && res.GRPC.Duration > 0:
		tm.Transport = res.GRPC.Duration
	}
	return tm
}

type flow struct{ ctx *execCtx }

func (f flow) PendingCancel() *xexec.RequestResult {
	if f.ctx == nil {
		return nil
	}
	select {
	case <-f.ctx.sendCtx.Done():
		return f.ctx.canceled(context.Canceled)
	default:
		return nil
	}
}

func (f flow) Finish() {
	if f.ctx != nil && f.ctx.cancel != nil {
		f.ctx.cancel()
	}
}

func (f flow) EvaluateCondition() *xexec.RequestResult {
	if f.ctx == nil {
		return nil
	}
	x := f.ctx
	if x.req.Metadata.When == nil {
		return nil
	}
	ok, reason, err := x.eng.EvalCondition(
		x.sendCtx,
		x.doc,
		x.req,
		x.env,
		x.opts.BaseDir,
		x.req.Metadata.When,
		x.baseVars,
		x.extraX,
	)
	if err != nil {
		tag := "@when"
		if x.req.Metadata.When != nil && x.req.Metadata.When.Negate {
			tag = "@skip-if"
		}
		x.exp.stage(
			tag,
			xplain.StageError,
			xplain.SummaryConditionEvaluationFailed,
			nil,
			nil,
			err.Error(),
		)
		wrap := errdef.Wrap(errdef.CodeScript, err, "%s", tag)
		return x.fail(wrap, "Condition evaluation failed", err)
	}
	st := xplain.StageOK
	sum := xplain.SummaryConditionPassed
	if ok {
		x.exp.stage(xplain.StageCondition, st, sum, nil, nil, reason)
		return nil
	}
	st = xplain.StageSkipped
	sum = xplain.SummaryConditionBlockedRequest
	x.exp.stage(xplain.StageCondition, st, sum, nil, nil, reason)
	return x.skip(reason)
}

func (f flow) RunPreRequest() *xexec.RequestResult {
	if f.ctx == nil {
		return nil
	}
	x := f.ctx
	x.eng.resolveInheritedAuth(x.doc, x.req)

	vv := cloneStringMap(x.baseVars)
	var before *restfile.Request
	if len(x.req.Metadata.Applies) > 0 {
		before = cloneRequest(x.req)
	}
	if err := x.eng.runRTSApply(x.sendCtx, x.doc, x.req, x.env, x.opts.BaseDir, vv); err != nil {
		x.exp.stage(
			xplain.StageApply,
			xplain.StageError,
			xplain.SummaryApplyFailed,
			before,
			x.req,
			err.Error(),
		)
		wrap := errdef.Wrap(errdef.CodeScript, err, "@apply")
		return x.fail(wrap, "Apply failed", err)
	}
	if before != nil {
		x.exp.stage(
			xplain.StageApply,
			xplain.StageOK,
			xplain.SummaryApplyComplete,
			before,
			x.req,
		)
	}

	before = nil
	if x.hasRTSPre {
		before = cloneRequest(x.req)
	}
	rtsOut, err := x.eng.runRTSPreRequest(
		x.sendCtx,
		x.doc,
		x.req,
		x.env,
		x.opts.BaseDir,
		vv,
		cloneGlobalValues(x.currentGlobals()),
	)
	if err != nil {
		x.exp.stage(
			xplain.StageRTSPreRequest,
			xplain.StageError,
			xplain.SummaryRTSPreRequestFailed,
			before,
			x.req,
			err.Error(),
		)
		wrap := errdef.Wrap(errdef.CodeScript, err, "pre-request rts script")
		return x.fail(wrap, "RTS pre-request failed", err)
	}
	if err := applyPreRequestOutput(x.req, rtsOut); err != nil {
		x.exp.stage(
			xplain.StageRTSPreRequest,
			xplain.StageError,
			xplain.SummaryRTSPreRequestOutputBad,
			before,
			x.req,
			err.Error(),
		)
		return x.fail(err, "RTS pre-request failed")
	}
	if before != nil {
		x.exp.stage(
			xplain.StageRTSPreRequest,
			xplain.StageOK,
			xplain.SummaryRTSPreRequestComplete,
			before,
			x.req,
		)
	}
	if err := x.sendCtx.Err(); err != nil {
		return x.canceled(err)
	}

	x.applyRuntimeGlobals(rtsOut.Globals)
	if len(rtsOut.Globals) > 0 || len(rtsOut.Variables) > 0 {
		vv = x.currentVariables()
	}

	before = nil
	if x.hasJSPre {
		before = cloneRequest(x.req)
	}
	jsOut, err := x.eng.sc.RunPreRequest(x.req.Metadata.Scripts, scripts.PreRequestInput{
		Request:   x.req,
		Variables: vv,
		Globals:   cloneGlobalValues(x.currentGlobals()),
		BaseDir:   x.opts.BaseDir,
		Context:   x.sendCtx,
	})
	if err != nil {
		x.exp.stage(
			xplain.StageJSPreRequest,
			xplain.StageError,
			xplain.SummaryJSPreRequestFailed,
			before,
			x.req,
			err.Error(),
		)
		wrap := errdef.Wrap(errdef.CodeScript, err, "pre-request script")
		return x.fail(wrap, "JS pre-request failed", err)
	}
	if err := applyPreRequestOutput(x.req, jsOut); err != nil {
		x.exp.stage(
			xplain.StageJSPreRequest,
			xplain.StageError,
			xplain.SummaryJSPreRequestOutputBad,
			before,
			x.req,
			err.Error(),
		)
		return x.fail(err, "JS pre-request failed")
	}
	if before != nil {
		x.exp.stage(
			xplain.StageJSPreRequest,
			xplain.StageOK,
			xplain.SummaryJSPreRequestComplete,
			before,
			x.req,
		)
	}
	if err := x.sendCtx.Err(); err != nil {
		return x.canceled(err)
	}

	x.applyRuntimeGlobals(jsOut.Globals)
	x.scriptV = mergeStringMaps(rtsOut.Variables, jsOut.Variables)
	return nil
}

func (x *execCtx) buildResolver() {
	extra := make([]map[string]string, 0, 2)
	if len(x.scriptV) > 0 {
		extra = append(extra, x.scriptV)
	}
	if len(x.extraV) > 0 {
		extra = append(extra, x.extraV)
	}
	x.res = x.eng.buildResolver(
		x.sendCtx,
		x.doc,
		x.req,
		x.env,
		x.opts.BaseDir,
		x.storeG,
		x.extraX,
		extra...,
	)
	x.trace = vars.NewTrace()
	x.res.SetTrace(x.trace)
	x.exp.trace = x.trace
}

func (x *execCtx) resolveRoute() *xrunResult {
	sp, err := x.eng.resolveSSH(x.doc, x.req, x.res, x.env)
	if err != nil {
		x.exp.stage(
			xplain.StageRoute,
			xplain.StageError,
			xplain.SummaryRouteSSHResolutionFailed,
			nil,
			nil,
			err.Error(),
		)
		wrap := errdef.Wrap(errdef.CodeHTTP, err, "resolve ssh")
		return x.fail(wrap, "Route resolution failed", err)
	}
	kp, err := x.eng.resolveK8s(x.doc, x.req, x.res, x.env)
	if err != nil {
		x.exp.stage(
			xplain.StageRoute,
			xplain.StageError,
			xplain.SummaryRouteK8sResolutionFailed,
			nil,
			nil,
			err.Error(),
		)
		wrap := errdef.Wrap(errdef.CodeHTTP, err, "resolve k8s")
		return x.fail(wrap, "Route resolution failed", err)
	}
	x.sshPlan = sp
	x.k8sPlan = kp
	x.opts.SSH = sp
	x.opts.K8s = kp
	x.exp.setRoute(sp, kp)
	if rt := explainRoute(sp, kp); rt != nil {
		notes := append([]string{rt.Summary}, rt.Notes...)
		x.exp.stage(xplain.StageRoute, xplain.StageOK, rt.Kind, nil, nil, notes...)
		if sp != nil && sp.Active() && sp.Config != nil && !sp.Config.Strict {
			x.exp.warn("@ssh strict_hostkey=false (insecure)")
		}
	}
	return nil
}

func (x *execCtx) prepareAuth() *xrunResult {
	if x.req.Metadata.Auth == nil {
		return nil
	}
	x.exp.addSecrets(authSecretValues(x.req.Metadata.Auth, x.res)...)
	before := cloneRequest(x.req)
	if x.preview() {
		out, err := x.eng.prepareExplainAuthPreview(x.doc, x.req, x.res, x.env)
		if err != nil {
			x.exp.stage(
				xplain.StageAuth,
				xplain.StageError,
				xplain.SummaryAuthInjectionFailed,
				before,
				x.req,
				err.Error(),
			)
			return x.fail(err, "Auth preparation failed")
		}
		x.exp.addSecrets(out.extraSecrets...)
		x.exp.stage(
			xplain.StageAuth,
			out.status,
			out.summary,
			before,
			x.req,
			out.notes...,
		)
		return nil
	}

	var secs []string
	switch strings.ToLower(strings.TrimSpace(x.req.Metadata.Auth.Type)) {
	case "command":
		out, err := x.eng.ensureCommandAuth(x.sendCtx, x.doc, x.req, x.res, x.env, x.timeout)
		if err != nil {
			x.exp.stage(
				xplain.StageAuth,
				xplain.StageError,
				xplain.SummaryAuthInjectionFailed,
				before,
				x.req,
				err.Error(),
			)
			return x.fail(err, "Auth preparation failed")
		}
		secs = commandAuthSecrets(out)
	default:
		if err := x.eng.ensureOAuth(x.sendCtx, x.req, x.res, x.opts, x.env, x.timeout); err != nil {
			x.exp.stage(
				xplain.StageAuth,
				xplain.StageError,
				xplain.SummaryAuthInjectionFailed,
				before,
				x.req,
				err.Error(),
			)
			return x.fail(err, "Auth preparation failed")
		}
		secs = injectedAuthSecrets(x.req.Metadata.Auth, before, x.req)
	}
	x.exp.addSecrets(secs...)
	x.runtimeSecrets = append(x.runtimeSecrets, secs...)
	x.exp.stage(xplain.StageAuth, xplain.StageOK, xplain.SummaryAuthPrepared, before, x.req)
	return nil
}

func (x *execCtx) prepareProto() *xrunResult {
	if x.req.GRPC != nil {
		before := cloneRequest(x.req)
		if err := x.eng.prepareGRPCRequest(x.req, x.res, x.grpcOpts.BaseDir); err != nil {
			x.exp.stage(
				xplain.StageGRPCPrepare,
				xplain.StageError,
				xplain.SummaryGRPCPrepareFailed,
				before,
				x.req,
				err.Error(),
			)
			return x.fail(err, "gRPC preparation failed")
		}
		x.exp.stage(
			xplain.StageGRPCPrepare,
			xplain.StageOK,
			xplain.SummaryGRPCRequestPrepared,
			before,
			x.req,
		)
	}
	if x.req.WebSocket != nil {
		before := cloneRequest(x.req)
		if err := x.eng.expandWebSocketSteps(x.req, x.res); err != nil {
			x.exp.stage(
				xplain.StageWebSocketPrepare,
				xplain.StageError,
				xplain.SummaryWebSocketPrepareFailed,
				before,
				x.req,
				err.Error(),
			)
			return x.fail(err, "WebSocket preparation failed")
		}
		x.exp.stage(
			xplain.StageWebSocketPrepare,
			xplain.StageOK,
			xplain.SummaryWebSocketRequestPrepared,
			before,
			x.req,
		)
	}
	return nil
}

func (f flow) PrepareRequest() *xexec.RequestResult {
	if f.ctx == nil {
		return nil
	}
	x := f.ctx
	x.buildResolver()
	if out := x.resolveRoute(); out != nil {
		return out
	}
	if out := x.applySettings(); out != nil {
		return out
	}
	if out := x.prepareAuth(); out != nil {
		return out
	}
	return x.prepareProto()
}

func (f flow) PreviewResult() xexec.RequestResult {
	if f.ctx == nil || !f.ctx.preview() {
		return xexec.RequestResult{}
	}
	x := f.ctx
	x.exp.setPrepared(x.req)
	if x.req.GRPC == nil {
		if err := x.eng.prepareExplainHTTPPreview(
			x.sendCtx,
			x.exp.report,
			x.req,
			x.res,
			x.opts,
		); err != nil {
			x.exp.stage(
				xplain.StageHTTPPrepare,
				xplain.StageError,
				xplain.SummaryHTTPRequestBuildFailed,
				nil,
				nil,
				err.Error(),
			)
			out := x.base()
			out.Err = err
			out.RequestText = x.reqText()
			out.Explain = x.exp.finish(xplain.StatusError, "HTTP preparation failed", err)
			return out
		}
	}
	out := x.base()
	out.RequestText = x.reqText()
	out.Preview = true
	out.Explain = x.exp.finish(
		xplain.StatusReady,
		"Explain preview ready. No request was sent.",
		nil,
	)
	return out
}

func (f flow) UseGRPC() bool { return f.ctx != nil && f.ctx.useGRPC }

func (f flow) IsInteractiveWebSocket() bool { return false }

func (f flow) ExecuteInteractiveWebSocket() xexec.RequestResult { return xexec.RequestResult{} }

func (f flow) ExecuteGRPC() xexec.RequestResult {
	if f.ctx == nil {
		return xexec.RequestResult{}
	}
	x := f.ctx
	if x.eng.gc == nil {
		err := errdef.New(errdef.CodeHTTP, "gRPC client is not initialised")
		x.exp.setPrepared(x.req)
		res := x.base()
		res.Err = err
		res.RequestText = x.reqText()
		res.Explain = x.exp.finish(xplain.StatusError, "gRPC request failed", err)
		return res
	}

	ctx, cancel := context.WithTimeout(x.sendCtx, x.timeout)
	defer cancel()

	if x.grpcOpts.DialTimeout == 0 {
		x.grpcOpts.DialTimeout = x.timeout
	}
	x.grpcOpts.SSH = x.sshPlan
	x.grpcOpts.K8s = x.k8sPlan

	var sess *stream.Session
	resp, err := x.eng.gc.Execute(ctx, x.req, x.req.GRPC, x.grpcOpts, func(s *stream.Session) {
		sess = s
		if x.onGRPC != nil {
			x.onGRPC(s, x.req)
		}
	})
	info, raw, sErr := grpcStreamInfoFromSession(sess)
	if sErr != nil {
		x.exp.setPrepared(x.req)
		res := x.base()
		res.Err = sErr
		res.GRPC = resp
		res.RequestText = x.reqText()
		res.Explain = x.exp.finish(xplain.StatusError, "gRPC request failed", sErr)
		return res
	}
	if err != nil {
		x.exp.setPrepared(x.req)
		res := x.base()
		res.Err = err
		res.GRPC = resp
		res.Stream = info
		res.Transcript = copyBytes(raw)
		res.RequestText = x.reqText()
		res.Explain = x.exp.finish(xplain.StatusError, "gRPC request failed", err)
		return res
	}

	respForScripts := grpcScriptResponse(x.req, resp)
	var caps captureResult
	if err := x.eng.applyCaptures(captureRun{
		doc:    x.doc,
		req:    x.req,
		res:    x.res,
		resp:   respForScripts,
		stream: info,
		out:    &caps,
		env:    x.env,
		v:      x.captureVariables(),
		x:      x.extraX,
	}); err != nil {
		x.exp.stage(
			xplain.StageCaptures,
			xplain.StageError,
			xplain.SummaryCaptureEvaluationFailed,
			nil,
			nil,
			err.Error(),
		)
		x.exp.setPrepared(x.req)
		res := x.base()
		res.Err = err
		res.RequestText = x.reqText()
		res.Explain = x.exp.finish(xplain.StatusError, "Capture evaluation failed", err)
		return res
	}

	vv := x.eng.collectVariables(x.doc, x.req, x.env, x.extraV)
	testV := mergeStringMaps(vv, x.scriptV)
	testG := x.eng.collectGlobalValues(x.doc, x.env)
	asserts, assertErr := x.eng.runAsserts(
		ctx,
		x.doc,
		x.req,
		x.env,
		x.opts.BaseDir,
		testV,
		x.extraX,
		rtsGRPC(resp),
		nil,
		rtsStream(info),
	)
	tests, globs, testErr := x.eng.sc.RunTests(
		x.req.Metadata.Scripts,
		scripts.TestInput{
			Response:  respForScripts,
			Variables: testV,
			Globals:   testG,
			BaseDir:   x.opts.BaseDir,
			Stream:    info,
		},
	)
	x.applyRuntimeGlobals(globs)
	x.exp.setPrepared(x.req)
	x.exp.setGRPC(x.req)

	res := x.base()
	res.GRPC = resp
	res.Stream = info
	res.Transcript = copyBytes(raw)
	res.Tests = append(asserts, tests...)
	res.ScriptErr = joinErr(assertErr, testErr)
	res.RequestText = x.reqText()
	res.Explain = x.exp.finish(xplain.StatusReady, "gRPC request sent", nil)
	return res
}

func (f flow) ExecuteHTTP() xexec.RequestResult {
	if f.ctx == nil {
		return xexec.RequestResult{}
	}
	x := f.ctx
	res := x.httpRunner().RunHTTP(xexec.HTTPInput{
		Client:           x.eng.hc,
		Scripts:          x.eng.sc,
		Context:          x.sendCtx,
		Doc:              x.doc,
		Req:              x.req,
		Resolver:         x.res,
		Options:          x.opts,
		EnvName:          x.env,
		EffectiveTimeout: x.timeout,
		ScriptVars:       x.scriptV,
		ExtraVals:        x.extraX,
	})
	return x.finishHTTP(res)
}

func (x *execCtx) httpRunner() xexec.Runner {
	return xexec.Runner{
		Hooks: xexec.HTTPHooks{
			AttachSSEHandle:       x.onSSE,
			AttachWebSocketHandle: x.onWS,
			ApplyCaptures: func(in xexec.CaptureInput) error {
				var caps captureResult
				return x.eng.applyCaptures(captureRun{
					doc:    in.Doc,
					req:    in.Req,
					res:    in.Resolver,
					resp:   in.Response,
					stream: in.Stream,
					out:    &caps,
					env:    in.EnvName,
					v:      in.Vars,
					x:      in.ExtraVals,
				})
			},
			CollectVariables: func(
				doc *restfile.Document,
				req *restfile.Request,
				env string,
			) map[string]string {
				return x.eng.collectVariables(doc, req, env, x.extraV)
			},
			CollectGlobalValues: func(doc *restfile.Document, env string) map[string]scripts.GlobalValue {
				return x.eng.collectGlobalValues(doc, env)
			},
			RunAsserts: func(in xexec.AssertInput) ([]scripts.TestResult, error) {
				return x.eng.runAsserts(
					in.Context,
					in.Doc,
					in.Req,
					in.EnvName,
					in.BaseDir,
					in.Vars,
					in.ExtraVals,
					rtsHTTP(in.HTTP),
					rtsTrace(in.HTTP),
					rtsStream(in.Stream),
				)
			},
			ApplyRuntimeGlobals: x.applyRuntimeGlobals,
		},
	}
}

func (x *execCtx) finishHTTP(res xexec.HTTPResult) xexec.RequestResult {
	out := x.base()
	out.Response = res.Response
	out.Stream = res.Stream
	if res.Stream != nil && res.Response != nil {
		out.Transcript = copyBytes(res.Response.Body)
	}
	if res.Response != nil {
		x.exp.sentHTTP(x.req, res.Response)
	}
	if res.Err != nil {
		if res.ErrStage == xexec.StageCaptures {
			x.exp.stage(
				xplain.StageCaptures,
				xplain.StageError,
				xplain.SummaryCaptureEvaluationFailed,
				nil,
				nil,
				res.Err.Error(),
			)
			out.Response = nil
			out.Stream = nil
			out.Transcript = nil
		}
		x.exp.setPrepared(x.req)
		if res.Response != nil {
			x.exp.setHTTP(res.Response)
		}
		out.Err = res.Err
		out.RequestText = x.reqText()
		out.Explain = x.exp.finish(xplain.StatusError, res.Decision, res.Err)
		return out
	}
	x.exp.setPrepared(x.req)
	if res.Response != nil {
		x.exp.setHTTP(res.Response)
	}
	out.Tests = res.Tests
	out.ScriptErr = res.ScriptErr
	out.RequestText = x.reqText()
	out.Explain = x.exp.finish(xplain.StatusReady, res.Decision, nil)
	return out
}
