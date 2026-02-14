package tunnel

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/unkn0wn-root/resterm/internal/httpver"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
)

type DialContextFunc func(context.Context, string, string) (net.Conn, error)

type DialManager[T any] interface {
	DialContext(context.Context, T, string, string) (net.Conn, error)
}

func HasConflict(sshOn bool, k8sOn bool) bool {
	return sshOn && k8sOn
}

func DialerFor[T any](manager DialManager[T], cfg T) DialContextFunc {
	return func(ctx context.Context, network string, addr string) (net.Conn, error) {
		return manager.DialContext(ctx, cfg, network, addr)
	}
}

func ApplyHTTPTransport(
	transport *http.Transport,
	version httpver.Version,
	dialer DialContextFunc,
) error {
	if transport == nil {
		return errors.New("transport is required")
	}
	if dialer == nil {
		return errors.New("dialer is required")
	}
	transport.Proxy = nil
	transport.DialContext = dialer
	if version == httpver.V10 || version == httpver.V11 {
		return nil
	}
	return http2.ConfigureTransport(transport)
}

func GRPCDialOption(dialer DialContextFunc) grpc.DialOption {
	return grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
		return dialer(ctx, "tcp", addr)
	})
}
