package grpcx

import (
	"context"
	"net"
	"sync"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// tlsFailure keeps the TLS error behind a failed connection. gRPC reports a
// connection error as status text, which loses the type that tells a
// certificate failure from a network one.
type tlsFailure struct {
	mu  sync.Mutex
	err error
}

func (f *tlsFailure) record(err error) {
	if diag.ClassOf(err) != diag.ClassTLS {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err == nil {
		f.err = err
	}
}

// classify reports a call that could not connect because of TLS as a TLS
// failure. The result reads as the call error but holds only the TLS error, so
// the Unavailable status cannot rank it as a network failure.
func (f *tlsFailure) classify(err error) error {
	if err == nil || status.Code(err) != codes.Unavailable {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err == nil {
		return err
	}
	return diag.WrapAs(diag.ClassTLS, tlsCallError{call: err, cause: f.err}, "tls handshake", grpcComponent)
}

type tlsCallError struct {
	call  error
	cause error
}

func (e tlsCallError) Error() string { return e.call.Error() }
func (e tlsCallError) Unwrap() error { return e.cause }

type tlsCreds struct {
	credentials.TransportCredentials
	failure *tlsFailure
}

func (c tlsCreds) ClientHandshake(
	ctx context.Context,
	authority string,
	raw net.Conn,
) (net.Conn, credentials.AuthInfo, error) {
	conn, info, err := c.TransportCredentials.ClientHandshake(ctx, authority, raw)
	if err != nil {
		c.failure.record(err)
		return nil, nil, err
	}
	return tlsConn{Conn: conn, failure: c.failure}, info, nil
}

func (c tlsCreds) Clone() credentials.TransportCredentials {
	return tlsCreds{TransportCredentials: c.TransportCredentials.Clone(), failure: c.failure}
}

// tlsConn records a TLS alert that arrives after the handshake. Over TLS 1.3 a
// server rejects a missing client certificate this way.
type tlsConn struct {
	net.Conn
	failure *tlsFailure
}

func (c tlsConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if err != nil {
		c.failure.record(err)
	}
	return n, err
}
