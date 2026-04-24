package ssh

import (
	"context"
	"errors"
	"time"

	"github.com/unkn0wn-root/resterm/internal/tunnel"
)

const dialRetryDelay = 150 * time.Millisecond

type clientDialer func(context.Context, execConfig) (sshClient, error)

type sessionOpener struct {
	dial       clientDialer
	retryDelay time.Duration
}

func newSessionOpener(dial clientDialer, retryDelay time.Duration) sessionOpener {
	return sessionOpener{dial: dial, retryDelay: retryDelay}
}

func (o sessionOpener) ready() bool {
	return o.dial != nil
}

func (o sessionOpener) open(
	ctx context.Context,
	cfg execConfig,
	cached bool,
) (*session, error) {
	attempts := cfg.Retries + 1
	if attempts < 1 {
		attempts = 1
	}

	delay := o.retryDelay
	if delay <= 0 {
		delay = dialRetryDelay
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		cli, err := o.dial(ctx, cfg)
		if err == nil {
			ka := time.Duration(0)
			if cached {
				ka = cfg.KeepAlive
			}
			return newSession(cli, ka), nil
		}

		lastErr = err
		if i+1 < attempts {
			if err := tunnel.WaitWithContext(ctx, delay); err != nil {
				return nil, err
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("ssh dial failed")
	}
	return nil, lastErr
}
