package request

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/settings"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func defaultTimeout(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return 30 * time.Second
}

func resolveRequestTimeout(req *restfile.Request, base time.Duration) time.Duration {
	if req != nil {
		if d, ok := settingTimeout(req.Settings); ok {
			return d
		}
	}
	return base
}

func settingTimeout(set map[string]string) (time.Duration, bool) {
	raw, ok := set["timeout"]
	if !ok {
		return 0, false
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

func (e *Engine) resolveHTTPOptions(
	doc *restfile.Document,
	opts httpx.Options,
) httpx.Options {
	if opts.BaseDir == "" && e.filePath(doc) != "" {
		opts.BaseDir = filepath.Dir(e.filePath(doc))
	}
	if !fallbackEnabled() {
		opts.FallbackBaseDirs = nil
		opts.NoFallback = true
		return opts
	}

	fbs := make([]string, 0, len(opts.FallbackBaseDirs)+3)
	fbs = append(fbs, opts.FallbackBaseDirs...)
	fbs = append(fbs, opts.BaseDir)
	if e.cfg.WorkspaceRoot != "" {
		fbs = append(fbs, e.cfg.WorkspaceRoot)
	}
	if cwd, err := os.Getwd(); err == nil {
		fbs = append(fbs, cwd)
	}
	opts.FallbackBaseDirs = util.DedupeNonEmptyStrings(fbs)
	opts.NoFallback = false
	return opts
}

func fallbackEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RESTERM_ENABLE_FALLBACK")))
	return v == "1" || v == "true" || v == "yes"
}

func (e *Engine) filePath(doc *restfile.Document) string {
	if doc != nil && strings.TrimSpace(doc.Path) != "" {
		return doc.Path
	}
	return strings.TrimSpace(e.cfg.FilePath)
}

func (e *Engine) fileDir(doc *restfile.Document) string {
	if path := e.filePath(doc); path != "" {
		return filepath.Dir(path)
	}
	return ""
}

// Naming these makes the call sites readable. A bare true/false here decides
// whether secrets reach the screen, which is not something to guess at.
type secrecy bool

const (
	keepSecrets secrecy = false
	omitSecrets secrecy = true
)

func (e *Engine) rtsPos(doc *restfile.Document, req *restfile.Request) vars.ExprPos {
	path := e.filePath(doc)
	line := 1
	if req != nil && req.LineRange.Start > 0 {
		line = req.LineRange.Start
	}
	return vars.ExprPos{Path: path, Line: line, Col: 1}
}

func (e *Engine) rtsPosForLine(doc *restfile.Document, req *restfile.Request, line int) rts.Pos {
	path := e.filePath(doc)
	if line <= 0 && req != nil && req.LineRange.Start > 0 {
		line = req.LineRange.Start
	}
	if line <= 0 {
		line = 1
	}
	return rts.Pos{Path: path, Line: line, Col: 1}
}

func (e *Engine) rtsPosForLineCol(doc *restfile.Document, req *restfile.Request, line, col int) rts.Pos {
	ps := e.rtsPosForLine(doc, req, line)
	if col > 0 && e.cfg.SourceDiagnostics {
		ps.Col = col
	}
	return ps
}

func (e *Engine) rtsErr(err error, doc *restfile.Document) error {
	if err == nil || !e.cfg.SourceDiagnostics {
		return err
	}
	rep := diag.ReportOf(err)
	if !canAttachSource(rep, doc) {
		return err
	}
	// empty operation preserves err.Error() (with position) and adds no chain entry
	return diag.Wrap(err, "", diag.WithSource(doc.Path, doc.Raw))
}

func canAttachSource(rep diag.Report, doc *restfile.Document) bool {
	if doc == nil || len(doc.Raw) == 0 || len(rep.Items) == 0 {
		return false
	}
	p := rep.Items[0].Span.Start.Path
	return p == "" || p == doc.Path
}

func (e *Engine) rtsBase(doc *restfile.Document, base string) string {
	if strings.TrimSpace(base) != "" {
		return base
	}
	return e.fileDir(doc)
}

func (e *Engine) buildResolver(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.ResolvedEnv,
	base string,
	globs vars.Globals,
	locals rts.Locals,
	run runVars,
) *vars.Resolver {
	plan := e.buildVariablePlan(varSources{
		doc:     doc,
		req:     req,
		env:     env,
		globals: globs,
		sec:     keepSecrets,
		run:     run,
	})
	in := ExprInput{
		Doc:     doc,
		Req:     req,
		Env:     env,
		Base:    base,
		Locals:  locals,
		globals: effectiveGlobalValues(doc, globs, env.Refs()),
	}
	return e.planResolver(ctx, plan, in, ExprEvalOptions{})
}

// DisplayResolver uses normal variable precedence for UI text but leaves out
// known secret values so labels and previews do not expose them.
func (e *Engine) DisplayResolver(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	src vars.Environment,
	base string,
	locals rts.Locals,
) *vars.Resolver {
	env := ResolveEnvironment(src, doc, req).WithoutRefValues()
	globs := e.collectStoredGlobalValues(env)
	globs.DeleteFunc(func(_ string, v vars.GlobalMutation) bool { return v.Secret })

	plan := e.buildVariablePlan(varSources{
		doc:     doc,
		req:     req,
		env:     env,
		globals: globs,
		sec:     omitSecrets,
	})
	in := ExprInput{
		Doc:     doc,
		Req:     req,
		Env:     env,
		Base:    e.rtsBase(doc, base),
		Locals:  locals,
		globals: effectiveGlobalValues(doc, globs, env.Refs()),
	}
	return e.planResolver(ctx, plan, in, ExprEvalOptions{OmitSecretGlobals: true})
}

// planResolver uses one plan for template and RTS lookups so they cannot
// disagree on precedence.
func (e *Engine) planResolver(
	ctx context.Context,
	plan variablePlan,
	in ExprInput,
	opt ExprEvalOptions,
) *vars.Resolver {
	in.Vars = plan.values()
	res := vars.NewResolver(plan.providers()...)
	res.SetExprEval(e.ExprEvalWithOptions(ctx, in, opt))
	res.SetExprPos(e.rtsPos(in.Doc, in.Req))
	return res
}

func (e *Engine) collectVariables(
	doc *restfile.Document,
	req *restfile.Request,
	env vars.ResolvedEnv,
	run runVars,
) map[string]string {
	return e.collectVariablesWithGlobals(
		doc,
		req,
		env,
		e.collectStoredGlobalValues(env),
		keepSecrets,
		run,
	)
}

func (e *Engine) collectVariablesWithGlobals(
	doc *restfile.Document,
	req *restfile.Request,
	env vars.ResolvedEnv,
	globs vars.Globals,
	sec secrecy,
	run runVars,
) map[string]string {
	return e.buildVariablePlan(varSources{
		doc:     doc,
		req:     req,
		env:     env,
		globals: globs,
		sec:     sec,
		run:     run,
	}).values()
}

func (e *Engine) collectGlobalValues(doc *restfile.Document, env vars.ResolvedEnv) vars.Globals {
	return effectiveGlobalValues(doc, e.collectStoredGlobalValues(env), env.Refs())
}

// ResolveEnvironment snapshots every declared env: reference for one run,
// including unused values that may need redaction.
func ResolveEnvironment(
	src vars.Environment,
	doc *restfile.Document,
	req *restfile.Request,
) vars.ResolvedEnv {
	env := src.ResolveWith(vars.NewEnvRefs(declaredNames(doc, req, src)))
	refs := env.Refs()
	read := func(authored bool, text string) { declaredValue(refs, authored, text) }
	if doc != nil {
		for _, c := range doc.Constants {
			read(c.Authored, c.Value)
		}
		for _, v := range doc.Variables {
			read(v.Authored, v.Value)
		}
		for _, v := range doc.Globals {
			read(v.Authored, v.Value)
		}
	}
	if req != nil {
		for _, v := range req.Variables {
			read(v.Authored, v.Value)
		}
	}
	return env
}

func (e *Engine) collectStoredGlobalValues(env vars.ResolvedEnv) vars.Globals {
	var out vars.Globals
	gs := e.rt.Globals()
	if gs == nil {
		return out
	}
	for key, v := range gs.Snapshot(env.Scope()) {
		name := storedName(v.Name, key)
		out.Set(name, vars.GlobalMutation{Name: name, Value: v.Value, Secret: v.Secret})
	}
	return out
}

func collectDocumentGlobalValues(doc *restfile.Document, refs *vars.EnvRefs) vars.Globals {
	var out vars.Globals
	if doc == nil {
		return out
	}
	for _, v := range doc.Globals {
		val := declaredValue(refs, v.Authored, v.Value)
		if val.Missing {
			continue
		}
		name := strings.TrimSpace(v.Name)
		out.Set(name, vars.GlobalMutation{
			Name:   name,
			Value:  val.Text,
			Secret: v.Secret || namesProcessVar(v.Authored, v.Value),
		})
	}
	return out
}

func effectiveGlobalValues(
	doc *restfile.Document,
	globs vars.Globals,
	refs *vars.EnvRefs,
) vars.Globals {
	return mergeGlobalValues(collectDocumentGlobalValues(doc, refs), globs)
}

func mergeGlobalValues(base, changes vars.Globals) vars.Globals {
	out := base.Clone()
	for name, ch := range changes.All() {
		if ch.Delete {
			out.Delete(name)
			continue
		}
		ch.Name = name
		out.Set(name, ch)
	}
	return out
}

// storedName prefers the recorded name and falls back to the storage key.
func storedName(name, key string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	return strings.TrimSpace(key)
}

func (e *Engine) applyGlobalMutations(changes vars.Globals, env vars.ResolvedEnv) {
	gs := e.rt.Globals()
	if gs == nil {
		return
	}
	for name, ch := range changes.All() {
		if ch.Delete {
			gs.Delete(env.Scope(), name)
			continue
		}
		gs.Set(env.Scope(), name, ch.Value, ch.Secret)
	}
}

func (e *Engine) resolveSSH(
	doc *restfile.Document,
	req *restfile.Request,
	res *vars.Resolver,
	env vars.ResolvedEnv,
) (*ssh.Plan, error) {
	if req == nil || req.SSH == nil {
		return nil, nil
	}
	ix := e.registryIndex()
	fileProfiles, globalProfiles := ix.SSH(doc)
	cfg, err := ssh.Resolve(req.SSH, fileProfiles, globalProfiles, res, env.Label())
	if err != nil {
		return nil, err
	}
	return &ssh.Plan{Manager: e.rt.SSH(), Config: cfg}, nil
}

func (e *Engine) resolveK8s(
	doc *restfile.Document,
	req *restfile.Request,
	res *vars.Resolver,
	env vars.ResolvedEnv,
) (*k8s.Plan, error) {
	if req == nil || req.K8s == nil {
		return nil, nil
	}
	ix := e.registryIndex()
	fileProfiles, globalProfiles := ix.K8s(doc)
	cfg, err := k8s.Resolve(req.K8s, fileProfiles, globalProfiles, res, env.Label())
	if err != nil {
		return nil, err
	}
	return &k8s.Plan{Manager: e.rt.K8s(), Config: cfg}, nil
}

func (x *execCtx) configureGRPC() {
	x.useGRPC = x.req.GRPC != nil
	if !x.useGRPC {
		return
	}
	x.grpcOpts = x.eng.cfg.GRPCOptions
	if x.grpcOpts.BaseDir == "" {
		x.grpcOpts.BaseDir = x.opts.BaseDir
		if x.grpcOpts.BaseDir == "" {
			x.grpcOpts.BaseDir = x.eng.fileDir(x.doc)
		}
	}
	// Use the same fallback roots as HTTP body files.
	x.grpcOpts.FallbackBaseDirs = x.opts.FallbackBaseDirs
	x.grpcOpts.NoFallback = x.opts.NoFallback

	// Only a timeout set on the request itself may bound a stream, so read
	// it before the settings merge adds file and env defaults.
	if d, ok := settingTimeout(x.req.Settings); ok {
		x.grpcOpts.StreamTimeout = d
	}
}

func (x *execCtx) applySettings() *xrunResult {
	x.configureGRPC()

	gset := settings.FromValues(x.env.Values())
	fset := map[string]string{}
	if x.doc != nil && x.doc.Settings != nil {
		fset = x.doc.Settings
	}
	before := CloneRequest(x.req)
	x.mset = settings.Merge(gset, fset, x.req.Settings)
	x.req.Settings = x.mset
	x.exp.setSettings(x.mset)
	x.exp.stage(
		xplain.StageSettings,
		xplain.StageOK,
		xplain.SummarySettingsMerged,
		before,
		x.req,
	)

	hs := []settings.Handler{settings.HTTPHandler(&x.opts, x.res)}
	if x.useGRPC {
		hs = append(hs, settings.GRPCHandler(&x.grpcOpts, x.res))
	}
	if _, err := settings.New(hs...).ApplyAll(x.mset); err != nil {
		x.exp.stage(
			xplain.StageSettings,
			xplain.StageError,
			xplain.SummarySettingsApplyFailed,
			nil,
			nil,
			err.Error(),
		)
		return x.fail(err, "Settings application failed")
	}
	x.timeout = defaultTimeout(resolveRequestTimeout(x.req, x.opts.Timeout))
	return nil
}
