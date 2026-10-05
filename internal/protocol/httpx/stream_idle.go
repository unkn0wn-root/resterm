package httpx

import (
	"context"
	"io"
	"time"
)

// watchIdle returns r wrapped so that every read that returns data restarts
// the timer. onTimeout runs once if d passes without data. stop ends the watch.
func watchIdle(ctx context.Context, r io.Reader, d time.Duration, onTimeout func()) (io.Reader, func()) {
	if d <= 0 {
		return r, func() {}
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
				timer.Reset(d)
			}
		}
	}()

	return idleReader{r: r, reset: reset}, func() { close(stop) }
}

type idleReader struct {
	r     io.Reader
	reset chan<- struct{}
}

func (r idleReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		select {
		case r.reset <- struct{}{}:
		default:
		}
	}
	return n, err
}
