package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc/codes"
)

type ComparePlan struct {
	Run     RunMeta
	Doc     *restfile.Document
	Request *restfile.Request
	Spec    restfile.CompareSpec
}

type cmpRun struct {
	dep      Dep
	sink     Sink
	ectx     context.Context
	pl       *ComparePlan
	seen     bool
	skip     bool
	fail     bool
	done     bool
	canceled bool
}

func PrepareCompare(
	doc *restfile.Document,
	req *restfile.Request,
	spec *restfile.CompareSpec,
	run RunMeta,
) (*ComparePlan, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	spec = prepareCompareSpec(spec)
	if spec == nil || len(spec.Environments) < 2 {
		return nil, fmt.Errorf("compare requires at least two environments")
	}
	run = normRun(run, ModeCompare, engine.ReqTitle(req), run.Env)
	out := &ComparePlan{
		Run:     run,
		Doc:     doc,
		Request: req,
		Spec:    *spec,
	}
	return out, nil
}

func RunCompare(ctx context.Context, dep Dep, sink Sink, pl *ComparePlan) error {
	if dep == nil {
		return fmt.Errorf("compare dependency is nil")
	}
	if pl == nil {
		return fmt.Errorf("compare plan is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	r := &cmpRun{
		dep:  dep,
		sink: sink,
		ectx: emitCtx(ctx),
		pl:   pl,
		skip: true,
	}
	if err := r.emitRunStart(); err != nil {
		return err
	}
	err := r.run(ctx)
	if derr := r.emitRunDone(err); err == nil {
		err = derr
	}
	return err
}

func CompareBaseIndex(rows []engine.CompareRow, want string) int {
	if len(rows) == 0 {
		return 0
	}
	want = strings.TrimSpace(want)
	if want == "" {
		return 0
	}
	for i := range rows {
		if strings.EqualFold(rows[i].Environment, want) {
			return i
		}
	}
	return 0
}

func CompareBaseline(rows []engine.CompareRow, want string) string {
	want = strings.TrimSpace(want)
	if want != "" || len(rows) == 0 {
		return want
	}
	return rows[0].Environment
}

func (r *cmpRun) run(ctx context.Context) error {
	total := len(r.pl.Spec.Environments)
	for i, env := range r.pl.Spec.Environments {
		if ctx.Err() != nil {
			r.canceled = true
			break
		}
		req := request.CloneRequest(r.pl.Request)
		if err := r.emitRowStart(i, env, total, req); err != nil {
			return err
		}
		out, err := r.dep.ExecuteWith(
			r.pl.Doc,
			req,
			env,
			request.ExecOptions{Record: false, Ctx: ctx},
		)
		if err != nil {
			return err
		}
		if err := r.emitRowDone(i, env, total, out); err != nil {
			return err
		}
		ok, skip, cancel := compareOutcome(out)
		r.note(ok, skip, cancel)
		if cancel {
			break
		}
	}
	r.done = true
	return nil
}

func (r *cmpRun) note(ok, skip, cancel bool) {
	r.seen = true
	if !skip {
		r.skip = false
	}
	if !skip && !ok {
		r.fail = true
	}
	if cancel {
		r.canceled = true
	}
}

func (r *cmpRun) row(i int, env string, total int) RowMeta {
	name := strings.TrimSpace(env)
	return RowMeta{
		Index: i,
		Env:   name,
		Base:  r.base(i, name),
		Total: total,
	}
}

func (r *cmpRun) base(i int, env string) bool {
	if r == nil {
		return false
	}
	want := strings.TrimSpace(r.pl.Spec.Baseline)
	if want == "" {
		return i == 0
	}
	return strings.EqualFold(env, want)
}

func compareOutcome(out engine.RequestResult) (bool, bool, bool) {
	if out.Skipped {
		return false, true, false
	}
	if out.Err != nil {
		return false, false, errors.Is(out.Err, context.Canceled)
	}
	if out.ScriptErr != nil {
		return false, false, false
	}
	for _, t := range out.Tests {
		if !t.Passed {
			return false, false, false
		}
	}
	switch {
	case out.Response != nil:
		return out.Response.StatusCode < 400, false, false
	case out.GRPC != nil:
		return out.GRPC.StatusCode == codes.OK, false, false
	default:
		return false, false, false
	}
}
