package grpcx

import (
	"context"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestUnaryAppliesTimeout(t *testing.T) {
	gr, opt := descriptorRequest(t)
	opt.Timeout = 250 * time.Millisecond

	var deadline time.Time
	conn := &stubConn{
		invoke: func(ctx context.Context, _ string, _, _ any) error {
			deadline, _ = ctx.Deadline()
			return nil
		},
	}

	if _, err := stubClient(conn).Execute(
		context.Background(),
		&restfile.Request{GRPC: gr},
		opt,
		nil,
	); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if deadline.IsZero() {
		t.Fatal("unary call ran without a deadline")
	}
	if left := time.Until(deadline); left > opt.Timeout {
		t.Fatalf("deadline is %v out, want at most %v", left, opt.Timeout)
	}
}

func TestStreamHasNoDeadline(t *testing.T) {
	gr, opt := streamingRequest(t)
	opt.Timeout = 250 * time.Millisecond
	opt.DialTimeout = 250 * time.Millisecond

	conn := &stubConn{stream: &stubStream{recvCount: 1}}
	if _, err := stubClient(conn).Execute(
		context.Background(),
		&restfile.Request{GRPC: gr},
		opt,
		nil,
	); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if _, ok := conn.stream.ctx.Deadline(); ok {
		t.Fatal("stream ran under a deadline")
	}
}

func TestStreamAppliesExplicitStreamTimeout(t *testing.T) {
	gr, opt := streamingRequest(t)
	opt.Timeout = 5 * time.Second
	opt.StreamTimeout = 250 * time.Millisecond

	conn := &stubConn{stream: &stubStream{recvCount: 1}}
	if _, err := stubClient(conn).Execute(
		context.Background(),
		&restfile.Request{GRPC: gr},
		opt,
		nil,
	); err != nil {
		t.Fatalf("execute: %v", err)
	}

	deadline, ok := conn.stream.ctx.Deadline()
	if !ok {
		t.Fatal("stream ran without the explicit deadline")
	}
	if left := time.Until(deadline); left > opt.StreamTimeout {
		t.Fatalf("deadline is %v out, want at most %v", left, opt.StreamTimeout)
	}
}

func TestStreamTimeoutCancelsBlockedStream(t *testing.T) {
	gr, opt := streamingRequest(t)
	opt.StreamTimeout = 50 * time.Millisecond

	conn := &stubConn{stream: &stubStream{blockCtx: true}}
	resp, err := stubClient(conn).Execute(
		context.Background(),
		&restfile.Request{GRPC: gr},
		opt,
		nil,
	)
	if err == nil {
		t.Fatal("expected the expired stream to surface an error")
	}
	if resp == nil {
		t.Fatal("expected a response alongside the timeout error")
	}
}

func TestDialTimeoutBoundsSetupOnly(t *testing.T) {
	gr, opt := descriptorRequest(t)
	opt.DialTimeout = 5 * time.Second

	var callDeadline time.Time
	conn := &stubConn{
		invoke: func(ctx context.Context, _ string, _, _ any) error {
			callDeadline, _ = ctx.Deadline()
			return nil
		},
	}

	if _, err := stubClient(conn).Execute(
		context.Background(),
		&restfile.Request{GRPC: gr},
		opt,
		nil,
	); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !callDeadline.IsZero() {
		t.Fatal("DialTimeout leaked into the call deadline")
	}
}

func TestWithTimeout(t *testing.T) {
	ctx, cancel := withTimeout(context.Background(), 0)
	defer cancel()
	if _, ok := ctx.Deadline(); ok {
		t.Fatal("a zero timeout should leave the context alone")
	}

	ctx, cancel = withTimeout(context.Background(), time.Second)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("a positive timeout should set a deadline")
	}
}
