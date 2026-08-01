package ssh

import (
	"context"
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
	delay := o.retryDelay
	if delay <= 0 {
		delay = dialRetryDelay
	}

	cli, err := tunnel.OpenRetry(ctx, cfg.Retries+1, delay,
		func(ctx context.Context) (sshClient, error) {
			return o.dial(ctx, cfg)
		})
	if err != nil {
		return nil, err
	}

	ka := time.Duration(0)
	if cached {
		ka = cfg.KeepAlive
	}
	return newSession(cli, ka), nil
}
