package scripts

import (
	"context"
	"errors"
	"time"

	"github.com/dop251/goja"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

const DefaultScriptTimeout = 30 * time.Second

func guardVM(ctx context.Context, vm *goja.Runtime, budget time.Duration) func() {
	ctx, cancel := context.WithTimeoutCause(
		ctx,
		budget,
		diag.Newf(diag.ClassTimeout, "script exceeded the %s time limit", budget),
	)
	stop := context.AfterFunc(ctx, func() { vm.Interrupt(context.Cause(ctx)) })
	return func() {
		stop()
		cancel()
	}
}

func stopReason(ctx context.Context, err error) error {
	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) {
		if reason, ok := interrupted.Value().(error); ok {
			return reason
		}
	}
	return context.Cause(ctx)
}
