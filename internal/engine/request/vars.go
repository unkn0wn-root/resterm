package request

import (
	"context"
	"maps"
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

func (e *Engine) rtsEnv(env vars.Environment) map[string]string {
	out := make(map[string]string, len(env.Values())+1)
	maps.Copy(out, env.Values())
	if env.Label() != "" {
		out["name"] = env.Label()
	}
	return out
}

func (e *Engine) buildResolver(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
	base string,
	globs map[string]vars.GlobalMutation,
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
	in := ExprInput{Doc: doc, Req: req, Env: env, Base: base, Locals: locals}
	return e.planResolver(ctx, plan, in, ExprEvalOptions{})
}

// DisplayResolver uses normal variable precedence for UI text but leaves out
// known secret values so labels and previews do not expose them.
func (e *Engine) DisplayResolver(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
	base string,
	locals rts.Locals,
) *vars.Resolver {
	globs := e.collectStoredGlobalValues(env)
	maps.DeleteFunc(globs, func(_ string, v vars.GlobalMutation) bool { return v.Secret })

	plan := e.buildVariablePlan(varSources{
		doc:     doc,
		req:     req,
		env:     env,
		globals: globs,
		sec:     omitSecrets,
	})
	in := ExprInput{Doc: doc, Req: req, Env: env, Base: e.rtsBase(doc, base), Locals: locals}
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
	res.AddRefResolver(vars.EnvRefResolver)
	res.SetExprEval(e.ExprEvalWithOptions(ctx, in, opt))
	res.SetExprPos(e.rtsPos(in.Doc, in.Req))
	return res
}

func (e *Engine) collectVariables(
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
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
	env vars.Environment,
	globs map[string]vars.GlobalMutation,
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

func (e *Engine) collectGlobalValues(
	doc *restfile.Document,
	env vars.Environment,
) map[string]vars.GlobalMutation {
	return effectiveGlobalValues(doc, e.collectStoredGlobalValues(env))
}

func (e *Engine) collectStoredGlobalValues(env vars.Environment) map[string]vars.GlobalMutation {
	gs := e.rt.Globals()
	if gs == nil {
		return nil
	}
	out := make(map[string]vars.GlobalMutation)
	if snap := gs.Snapshot(env.Scope()); len(snap) > 0 {
		for k, v := range snap {
			name := strings.TrimSpace(v.Name)
			if name == "" {
				name = k
			}
			out[name] = vars.GlobalMutation{Name: name, Value: v.Value, Secret: v.Secret}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectDocumentGlobalValues(doc *restfile.Document) map[string]vars.GlobalMutation {
	if doc == nil || len(doc.Globals) == 0 {
		return nil
	}
	out := make(map[string]vars.GlobalMutation)
	for _, v := range doc.Globals {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			continue
		}
		out[name] = vars.GlobalMutation{Name: name, Value: v.Value, Secret: v.Secret}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func effectiveGlobalValues(
	doc *restfile.Document,
	globs map[string]vars.GlobalMutation,
) map[string]vars.GlobalMutation {
	return mergeGlobalValues(collectDocumentGlobalValues(doc), globs)
}

func cloneGlobalValues(src map[string]vars.GlobalMutation) map[string]vars.GlobalMutation {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]vars.GlobalMutation, len(src))
	maps.Copy(dst, src)
	return dst
}

func mergeGlobalValues(
	base map[string]vars.GlobalMutation,
	changes map[string]vars.GlobalMutation,
) map[string]vars.GlobalMutation {
	if len(base) == 0 && len(changes) == 0 {
		return nil
	}
	out := cloneGlobalValues(base)
	if out == nil {
		out = make(map[string]vars.GlobalMutation, len(changes))
	}
	for k, v := range changes {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			name = strings.TrimSpace(k)
		}
		if name == "" {
			continue
		}
		for cur := range out {
			if strings.EqualFold(strings.TrimSpace(cur), name) {
				delete(out, cur)
			}
		}
		if v.Delete {
			continue
		}
		v.Name = name
		out[name] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (e *Engine) applyGlobalMutations(
	changes map[string]vars.GlobalMutation,
	env vars.Environment,
) {
	gs := e.rt.Globals()
	if len(changes) == 0 || gs == nil {
		return
	}
	for _, ch := range changes {
		name := strings.TrimSpace(ch.Name)
		if name == "" {
			continue
		}
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
	env vars.Environment,
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
	env vars.Environment,
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
