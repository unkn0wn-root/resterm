package core

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type ProfilePlan struct {
	Run     RunMeta
	Doc     *restfile.Document
	Request *restfile.Request
	Spec    restfile.ProfileSpec
	Total   int
}

type proRun struct {
	dep      Dep
	sink     Sink
	ectx     context.Context
	pl       *ProfilePlan
	done     bool
	seen     bool
	skip     bool
	canceled bool
	ok       int
}

func PrepareProfile(
	doc *restfile.Document,
	req *restfile.Request,
	run RunMeta,
) (*ProfilePlan, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.GRPC != nil {
		return nil, fmt.Errorf("profiling is not supported for gRPC requests")
	}
	spec := profileSpec(req)
	total := spec.Count + spec.Warmup
	run = normRun(run, ModeProfile, engine.ReqTitle(req))
	return &ProfilePlan{
		Run:     run,
		Doc:     doc,
		Request: req,
		Spec:    spec,
		Total:   total,
	}, nil
}

func RunProfile(ctx context.Context, dep Dep, sink Sink, pl *ProfilePlan) error {
	if dep == nil {
		return fmt.Errorf("profile dependency is nil")
	}
	if pl == nil {
		return fmt.Errorf("profile plan is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	r := &proRun{
		dep:  dep,
		sink: sink,
		ectx: emitCtx(ctx),
		pl:   pl,
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

// A zero count means the default, so callers can enable profiling with an empty spec.
func profileSpec(req *restfile.Request) restfile.ProfileSpec {
	var spec restfile.ProfileSpec
	if req.Metadata.Profile != nil {
		spec = *req.Metadata.Profile
	}
	spec.Count = cmp.Or(spec.Count, restfile.DefaultProfileCount)
	return spec
}

func (r *proRun) run(ctx context.Context) error {
	for i := 0; i < r.pl.Total; i++ {
		if ctx.Err() != nil {
			r.canceled = true
			break
		}
		it := r.iter(i)
		req := request.CloneRequest(r.pl.Request)
		if err := r.emitIterStart(it, req); err != nil {
			return err
		}
		out, err := r.dep.ExecuteWith(
			r.pl.Doc,
			req,
			r.pl.Run.Env,
			request.ExecOptions{Record: false, Ctx: ctx},
		)
		if err != nil {
			return err
		}
		if err := r.emitIterDone(it, out); err != nil {
			return err
		}
		r.seen = true
		if out.Err != nil && errors.Is(out.Err, context.Canceled) {
			r.canceled = true
			break
		}
		if out.Skipped {
			r.skip = true
			break
		}
		if _, failed := iterFailure(it, out); !failed && !it.Warmup {
			r.ok++
		}
		if r.pl.Spec.Delay > 0 && i+1 < r.pl.Total {
			tm := time.NewTimer(r.pl.Spec.Delay)
			select {
			case <-tm.C:
			case <-ctx.Done():
				if !tm.Stop() {
					select {
					case <-tm.C:
					default:
					}
				}
				r.canceled = true
			}
			if r.canceled {
				break
			}
		}
	}
	r.done = true
	return nil
}

func (r *proRun) iter(i int) IterMeta {
	warm := i < r.pl.Spec.Warmup
	meta := IterMeta{
		Index:       i,
		Total:       r.pl.Total,
		Warmup:      warm,
		WarmupTotal: r.pl.Spec.Warmup,
		RunTotal:    r.pl.Spec.Count,
		Delay:       r.pl.Spec.Delay,
	}
	if warm {
		meta.WarmupIndex = i + 1
		return meta
	}
	meta.RunIndex = i - r.pl.Spec.Warmup + 1
	return meta
}
