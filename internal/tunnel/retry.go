package tunnel

import (
	"context"
	"time"
)

func OpenRetry[S any](
	ctx context.Context,
	attempts int,
	delay time.Duration,
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

		if i+1 < attempts {
			if err := WaitWithContext(ctx, delay); err != nil {
				return zero, err
			}
		}
	}
	return zero, lastErr
}
