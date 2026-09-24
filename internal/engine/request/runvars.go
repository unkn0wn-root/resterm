package request

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// RunScope carries workflow and loop values, plus run values already fixed.
type RunScope struct {
	Overlay vars.NameMap[string]
	RunVars vars.NameMap[string]
}

// EvalRunVars returns the new values alongside those already in sc.RunVars.
func (e *Engine) EvalRunVars(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	src vars.Environment,
	sc RunScope,
	decls []restfile.RunVar,
) (vars.NameMap[string], error) {
	if len(decls) == 0 {
		return sc.RunVars, nil
	}
	env := ResolveEnvironment(src, doc, req)
	vals, err := e.evalRunVars(ctx, doc, req, env, e.fileDir(doc), sc, decls)
	return vals, e.withSource(err, doc)
}

// Declarations are evaluated top to bottom against values that are already
// fixed. A value can use the declarations above it and the run value it
// replaces, and it never sees the raw text of a declaration still waiting.
func (e *Engine) evalRunVars(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.ResolvedEnv,
	base string,
	sc RunScope,
	decls []restfile.RunVar,
) (vars.NameMap[string], error) {
	globs := e.collectStoredGlobalValues(env)
	out := sc.RunVars.Clone()
	for _, d := range decls {
		run := sc
		run.RunVars = out
		res := e.buildResolver(ctx, doc, req, env, base, globs, rts.Locals{}, execVars{RunScope: run})
		val, err := res.ExpandTemplatesAt(d.Value, vars.ExprPos{Path: e.filePath(doc), Line: d.Line, Col: d.Col})
		if err != nil {
			return vars.NameMap[string]{}, diag.WrapAs(diag.ClassScript, err, directive.RunVarTag+" "+d.Name)
		}
		out.Set(d.Name, val)
	}
	return out, nil
}
