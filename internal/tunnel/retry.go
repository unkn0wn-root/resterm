package tunnel

import (
	"context"
	"time"

	"github.com/unkn0wn-root/resterm/internal/delay"
)

func OpenRetry[S any](
	ctx context.Context,
	attempts int,
	pause time.Duration,
	open func(context.Context) (S, error),
) (S, error) {
	var zero S
	attempts = max(attempts, 1)

	var lastErr error
	for i := range attempts {
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		s, err := open(ctx)
		if err == nil {
			return s, nil
		}
		lastErr = err

		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		if i+1 < attempts {
			if err := delay.Wait(ctx, pause); err != nil {
				return zero, err
			}
		}
	}
	return zero, lastErr
}
