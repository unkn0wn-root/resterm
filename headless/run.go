package headless

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/runner"
)

// Run executes a request or workflow file and returns a stable public report.
// ctx must not be nil; otherwise Run returns ErrNilContext.
func Run(ctx context.Context, opt Options) (*Report, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	pl, err := Build(opt)
	if err != nil {
		return nil, err
	}
	return RunPlan(ctx, pl)
}

// RunPlan executes a prepared plan and returns a stable public report.
// ctx must not be nil; otherwise RunPlan returns ErrNilContext.
func RunPlan(ctx context.Context, pl Plan) (*Report, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if pl.pl == nil {
		return nil, UsageError{err: ErrInvalidPlan}
	}
	rep, err := runner.RunPlan(ctx, pl.pl)
	if err != nil {
		if runner.IsUsageError(err) {
			return nil, UsageError{err: err}
		}
		return nil, err
	}
	return reportFromRunner(rep), nil
}
