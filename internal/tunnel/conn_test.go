package tunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestWaitWithContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := WaitWithContext(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestWaitWithContextNoDelay(t *testing.T) {
	if err := WaitWithContext(context.Background(), 0); err != nil {
		t.Fatalf("expected nil error for zero delay, got %v", err)
	}
}

func TestWrapConnCloseJoinsErrors(t *testing.T) {
	ce := errors.New("conn close")
	fe := errors.New("fn close")
	c := &tc{e: ce}
	fnCalled := 0
	w := WrapConn(c, func() error {
		fnCalled++
		return fe
	})
	err := w.Close()
	if c.n != 1 {
		t.Fatalf("expected conn close to be called once, got %d", c.n)
	}
	if fnCalled != 1 {
		t.Fatalf("expected closeFn to be called once, got %d", fnCalled)
	}
	if !errors.Is(err, ce) || !errors.Is(err, fe) {
		t.Fatalf("expected joined errors, got %v", err)
	}
}

func TestWrapConnCloseWithNilParts(t *testing.T) {
	w := WrapConn(nil, nil)
	if err := w.Close(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

type tc struct {
	e error
	n int
}

func (c *tc) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (c *tc) Write(p []byte) (int, error) { return len(p), nil }
func (c *tc) Close() error {
	c.n++
	return c.e
}
func (c *tc) LocalAddr() net.Addr                { return ta("l") }
func (c *tc) RemoteAddr() net.Addr               { return ta("r") }
func (c *tc) SetDeadline(_ time.Time) error      { return nil }
func (c *tc) SetReadDeadline(_ time.Time) error  { return nil }
func (c *tc) SetWriteDeadline(_ time.Time) error { return nil }

type ta string

func (a ta) Network() string { return string(a) }
func (a ta) String() string  { return string(a) }
