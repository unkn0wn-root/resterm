package core

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

type Dep interface {
	CollectVariables(
		*restfile.Document,
		*restfile.Request,
		string,
		...map[string]string,
	) map[string]string
	ExecuteWith(
		*restfile.Document,
		*restfile.Request,
		string,
		request.ExecOptions,
	) (engine.RequestResult, error)
	EvalCondition(
		context.Context,
		*restfile.Document,
		*restfile.Request,
		string,
		string,
		*restfile.ConditionSpec,
		map[string]string,
		map[string]rts.Value,
	) (bool, string, error)
	EvalForEachItems(
		context.Context,
		*restfile.Document,
		*restfile.Request,
		string,
		string,
		request.ForEachSpec,
		map[string]string,
		map[string]rts.Value,
	) ([]rts.Value, error)
	EvalValue(
		context.Context,
		*restfile.Document,
		*restfile.Request,
		string,
		string,
		string,
		string,
		rts.Pos,
		map[string]string,
		map[string]rts.Value,
	) (rts.Value, error)
	PosForLine(*restfile.Document, *restfile.Request, int) rts.Pos
	ValueString(context.Context, rts.Pos, rts.Value) (string, error)
}
