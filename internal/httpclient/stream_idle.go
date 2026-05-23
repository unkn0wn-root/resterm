package httpclient

import (
	"context"
	"time"
)

func startIdleWatch(
	ctx context.Context,
	d time.Duration,
	onTimeout func(),
) (chan<- struct{}, func()) {
	if d <= 0 {
		return nil, func() {}
	}
	if onTimeout == nil {
		onTimeout = func() {}
	}

	reset := make(chan struct{}, 1)
	stop := make(chan struct{})
	timer := time.NewTimer(d)

	go func() {
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-timer.C:
				onTimeout()
				return
			case <-reset:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(d)
			}
		}
	}()

	return reset, func() { close(stop) }
}
