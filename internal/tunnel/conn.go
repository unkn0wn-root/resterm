package tunnel

import (
	"context"
	"errors"
	"net"
	"time"
)

type wrappedConn struct {
	net.Conn
	closeFn func() error
}

func WrapConn(conn net.Conn, closeFn func() error) net.Conn {
	return &wrappedConn{Conn: conn, closeFn: closeFn}
}

func (c *wrappedConn) Close() error {
	var errs []error
	if c.Conn != nil {
		if err := c.Conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if c.closeFn != nil {
		if err := c.closeFn(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func WaitWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
