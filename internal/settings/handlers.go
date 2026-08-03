package settings

import (
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func HTTPHandler(opts *httpx.Options, resolver *vars.Resolver) Handler {
	return Handler{
		Match: IsHTTPKey,
		Apply: func(key, val string) error {
			m := map[string]string{key: val}
			return ApplyHTTPSettings(opts, m, resolver)
		},
	}
}

func GRPCHandler(opts *grpcx.Options, resolver *vars.Resolver) Handler {
	return Handler{
		Match: PrefixMatcher("grpc-"),
		Apply: func(key, val string) error {
			m := map[string]string{key: val}
			return ApplyGRPCSettings(opts, m, resolver)
		},
	}
}
