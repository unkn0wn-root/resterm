package ui

import (
	"context"
	"fmt"
	"strings"

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
	site := tag + " " + expr
	pos := m.rtsPosForLine(doc, req, spec.Line)
	val, err := m.rtsEvalValue(ctx, doc, req, envName, base, expr, site, pos, vars, extraVals)
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

func (m *Model) evalForEachItems(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	envName, base string,
	spec forEachSpec,
	vars map[string]string,
	extraVals map[string]rts.Value,
) ([]rts.Value, error) {
	expr := strings.TrimSpace(spec.Expr)
	if expr == "" {
		return nil, fmt.Errorf("@for-each expression missing")
	}
	pos := m.rtsPosForLine(doc, req, spec.Line)
	val, err := m.rtsEvalValue(
		ctx,
		doc,
		req,
		envName,
		base,
		expr,
		"@for-each "+expr,
		pos,
		vars,
		extraVals,
	)
	if err != nil {
		return nil, err
	}
	if val.K != rts.VList {
		return nil, fmt.Errorf("@for-each expects list result")
	}
	return val.L, nil
}

func (m *Model) rtsValueString(ctx context.Context, pos rts.Pos, v rts.Value) (string, error) {
	if m.rtsEng == nil {
		m.rtsEng = rts.NewEng()
	}
	cx := rts.NewCtx(ctx, m.rtsEng.Lim)
	return rts.ValueString(cx, pos, v)
}
