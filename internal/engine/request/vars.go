package request

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/prerequest"
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
		if raw, ok := req.Settings["timeout"]; ok {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				return d
			}
		}
	}
	return base
}

func (e *Engine) resolveHTTPOptions(
	doc *restfile.Document,
	opts httpclient.Options,
) httpclient.Options {
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

func (e *Engine) envName(name string) string {
	return vars.SelectEnv(e.cfg.EnvironmentSet, name, e.cfg.EnvironmentName)
}

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

func (e *Engine) rtsBase(doc *restfile.Document, base string) string {
	if strings.TrimSpace(base) != "" {
		return base
	}
	return e.fileDir(doc)
}

func (e *Engine) rtsEnv(name string) map[string]string {
	out := make(map[string]string)
	if env := vars.EnvValues(e.cfg.EnvironmentSet, name); len(env) > 0 {
		for k, v := range env {
			out[k] = v
		}
	}
	if strings.TrimSpace(name) != "" {
		out["name"] = name
	}
	return out
}

func (e *Engine) buildResolver(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env, base string,
	globs map[string]prerequest.GlobalValue,
	extra map[string]rts.Value,
	extras ...map[string]string,
) *vars.Resolver {
	env = e.envName(env)
	ps := make([]vars.Provider, 0, 8)
	if doc != nil && len(doc.Constants) > 0 {
		vals := make(map[string]string, len(doc.Constants))
		for _, c := range doc.Constants {
			vals[c.Name] = c.Value
		}
		ps = append(ps, vars.NewMapProvider("const", vals))
	}
	for _, extra := range extras {
		if len(extra) > 0 {
			ps = append(ps, vars.NewMapProvider("script", extra))
		}
	}
	if req != nil && len(req.Variables) > 0 {
		vals := make(map[string]string, len(req.Variables))
		for _, v := range req.Variables {
			vals[v.Name] = v.Value
		}
		ps = append(ps, vars.NewMapProvider("request", vals))
	}
	if vals := globalValueMap(globs); len(vals) > 0 {
		ps = append(ps, vars.NewMapProvider("global", vals))
	}
	if doc != nil && len(doc.Globals) > 0 {
		vals := make(map[string]string, len(doc.Globals))
		for _, v := range doc.Globals {
			vals[v.Name] = v.Value
		}
		ps = append(ps, vars.NewMapProvider("document-global", vals))
	}
	fv := make(map[string]string)
	if doc != nil {
		for _, v := range doc.Variables {
			fv[v.Name] = v.Value
		}
	}
	e.mergeFileRuntimeVars(fv, doc, env)
	if len(fv) > 0 {
		ps = append(ps, vars.NewMapProvider("file", fv))
	}
	if envVals := vars.EnvValues(e.cfg.EnvironmentSet, env); len(envVals) > 0 {
		ps = append(ps, vars.NewMapProvider("environment", envVals))
	}
	ps = append(ps, vars.EnvProvider{})

	res := vars.NewResolver(ps...)
	res.AddRefResolver(vars.EnvRefResolver)
	res.SetExprEval(e.rtsEval(ctx, doc, req, env, base, extra, extras...))
	res.SetExprPos(e.rtsPos(doc, req))
	return res
}

func (e *Engine) mergeFileRuntimeVars(dst map[string]string, doc *restfile.Document, env string) {
	fs := e.rt.Files()
	if dst == nil || fs == nil {
		return
	}
	env = e.envName(env)
	if snap := fs.Snapshot(env, e.filePath(doc)); len(snap) > 0 {
		for k, v := range snap {
			name := strings.TrimSpace(v.Name)
			if name == "" {
				name = k
			}
			dst[name] = v.Value
		}
	}
}

func (e *Engine) collectVariables(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	extras ...map[string]string,
) map[string]string {
	return e.collectVariablesWithGlobals(
		doc,
		req,
		env,
		e.collectStoredGlobalValues(env),
		extras...,
	)
}

func (e *Engine) collectVariablesWithGlobals(
	doc *restfile.Document,
	req *restfile.Request,
	env string,
	globs map[string]prerequest.GlobalValue,
	extras ...map[string]string,
) map[string]string {
	env = e.envName(env)
	out := make(map[string]string)
	if vals := vars.EnvValues(e.cfg.EnvironmentSet, env); len(vals) > 0 {
		for k, v := range vals {
			out[k] = v
		}
	}
	if doc != nil {
		for _, v := range doc.Variables {
			out[v.Name] = v.Value
		}
		for _, v := range doc.Globals {
			out[v.Name] = v.Value
		}
	}
	e.mergeFileRuntimeVars(out, doc, env)
	for k, v := range globalValueMap(globs) {
		out[k] = v
	}
	if req != nil {
		for _, v := range req.Variables {
			out[v.Name] = v.Value
		}
	}
	for _, extra := range extras {
		for k, v := range extra {
			out[k] = v
		}
	}
	return out
}

func (e *Engine) collectGlobalValues(
	doc *restfile.Document,
	env string,
) map[string]prerequest.GlobalValue {
	return effectiveGlobalValues(doc, e.collectStoredGlobalValues(env))
}

func (e *Engine) collectStoredGlobalValues(env string) map[string]prerequest.GlobalValue {
	env = e.envName(env)
	gs := e.rt.Globals()
	if gs == nil {
		return nil
	}
	out := make(map[string]prerequest.GlobalValue)
	if snap := gs.Snapshot(env); len(snap) > 0 {
		for k, v := range snap {
			name := strings.TrimSpace(v.Name)
			if name == "" {
				name = k
			}
			out[name] = prerequest.GlobalValue{Name: name, Value: v.Value, Secret: v.Secret}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectDocumentGlobalValues(doc *restfile.Document) map[string]prerequest.GlobalValue {
	if doc == nil || len(doc.Globals) == 0 {
		return nil
	}
	out := make(map[string]prerequest.GlobalValue)
	for _, v := range doc.Globals {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			continue
		}
		out[name] = prerequest.GlobalValue{Name: name, Value: v.Value, Secret: v.Secret}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func effectiveGlobalValues(
	doc *restfile.Document,
	globs map[string]prerequest.GlobalValue,
) map[string]prerequest.GlobalValue {
	return mergeGlobalValues(collectDocumentGlobalValues(doc), globs)
}

func cloneGlobalValues(src map[string]prerequest.GlobalValue) map[string]prerequest.GlobalValue {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]prerequest.GlobalValue, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeGlobalValues(
	base map[string]prerequest.GlobalValue,
	changes map[string]prerequest.GlobalValue,
) map[string]prerequest.GlobalValue {
	if len(base) == 0 && len(changes) == 0 {
		return nil
	}
	out := cloneGlobalValues(base)
	if out == nil {
		out = make(map[string]prerequest.GlobalValue, len(changes))
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

func globalValueMap(globs map[string]prerequest.GlobalValue) map[string]string {
	if len(globs) == 0 {
		return nil
	}
	out := make(map[string]string, len(globs))
	for k, v := range globs {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			name = strings.TrimSpace(k)
		}
		if name == "" || v.Delete {
			continue
		}
		out[name] = v.Value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (e *Engine) applyGlobalMutations(changes map[string]prerequest.GlobalValue, env string) {
	gs := e.rt.Globals()
	if len(changes) == 0 || gs == nil {
		return
	}
	env = e.envName(env)
	for _, ch := range changes {
		name := strings.TrimSpace(ch.Name)
		if name == "" {
			continue
		}
		if ch.Delete {
			gs.Delete(env, name)
			continue
		}
		gs.Set(env, name, ch.Value, ch.Secret)
	}
}

func (e *Engine) resolveSSH(
	doc *restfile.Document,
	req *restfile.Request,
	res *vars.Resolver,
	env string,
) (*ssh.Plan, error) {
	if req == nil || req.SSH == nil {
		return nil, nil
	}
	ix := e.registryIndex()
	fileProfiles, globalProfiles := ix.SSH(doc)
	cfg, err := ssh.Resolve(req.SSH, fileProfiles, globalProfiles, res, env)
	if err != nil {
		return nil, err
	}
	return &ssh.Plan{Manager: e.rt.SSH(), Config: cfg}, nil
}

func (e *Engine) resolveK8s(
	doc *restfile.Document,
	req *restfile.Request,
	res *vars.Resolver,
	env string,
) (*k8s.Plan, error) {
	if req == nil || req.K8s == nil {
		return nil, nil
	}
	ix := e.registryIndex()
	fileProfiles, globalProfiles := ix.K8s(doc)
	cfg, err := k8s.Resolve(req.K8s, fileProfiles, globalProfiles, res, env)
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
}

func (x *execCtx) applySettings() *xrunResult {
	x.configureGRPC()

	gset := settings.FromEnv(x.eng.cfg.EnvironmentSet, x.env)
	fset := map[string]string{}
	if x.doc != nil && x.doc.Settings != nil {
		fset = x.doc.Settings
	}
	before := cloneRequest(x.req)
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
