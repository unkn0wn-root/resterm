package ui

import (
	"context"

	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

type forEachSpec struct {
	Expr string
	Var  string
	Line int
}

func (m *Model) evalCondition(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	spec *restfile.ConditionSpec,
	vars map[string]string,
	extraVals map[string]rts.Value,
) (bool, string, error) {
	return m.requestSvc(httpclient.Options{}).
		EvalCondition(ctx, doc, req, envName, base, spec, vars, extraVals)
}

func (m *Model) evalForEachItems(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	spec forEachSpec,
	vars map[string]string,
	extraVals map[string]rts.Value,
) ([]rts.Value, error) {
	return m.requestSvc(httpclient.Options{}).EvalForEachItems(
		ctx, doc, req, envName, base,
		rqeng.ForEachSpec{Expr: spec.Expr, Var: spec.Var, Line: spec.Line},
		vars, extraVals,
	)
}

func (m *Model) rtsEvalValue(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base, expr, site string,
	pos rts.Pos,
	vars map[string]string,
	extraVals map[string]rts.Value,
) (rts.Value, error) {
	return m.requestSvc(httpclient.Options{}).
		EvalValue(ctx, doc, req, envName, base, expr, site, pos, vars, extraVals)
}

func (m *Model) rtsValueString(ctx context.Context, pos rts.Pos, v rts.Value) (string, error) {
	return m.requestSvc(httpclient.Options{}).ValueString(ctx, pos, v)
}

func (m *Model) runRTSApply(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	vars map[string]string,
	extraVals map[string]rts.Value,
) error {
	return m.requestSvc(httpclient.Options{}).
		ApplyPatches(ctx, doc, req, envName, base, vars, extraVals)
}
