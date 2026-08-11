package core

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Dep interface {
	// overlay contains workflow step and @for-each bindings. Its values rank
	// below script values and above request variables.
	CollectVariables(
		doc *restfile.Document,
		req *restfile.Request,
		env vars.Environment,
		overlay map[string]string,
	) map[string]string
	ExecuteWith(
		doc *restfile.Document,
		req *restfile.Request,
		env vars.Environment,
		opt request.ExecOptions,
	) (engine.RequestResult, error)
	EvalCondition(
		ctx context.Context,
		doc *restfile.Document,
		req *restfile.Request,
		env vars.Environment,
		base string,
		spec *restfile.ConditionSpec,
		vv map[string]string,
		locals rts.Locals,
	) (bool, string, error)
	EvalForEachItems(
		ctx context.Context,
		doc *restfile.Document,
		req *restfile.Request,
		env vars.Environment,
		base string,
		spec request.ForEachSpec,
		vv map[string]string,
		locals rts.Locals,
	) ([]rts.Value, error)
	EvalValue(ctx context.Context, in request.EvalInput) (rts.Value, error)
	PosForLine(doc *restfile.Document, req *restfile.Request, line int) rts.Pos
	ValueString(ctx context.Context, pos rts.Pos, v rts.Value) (string, error)
}
