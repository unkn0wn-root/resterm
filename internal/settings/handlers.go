package settings

import (
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func HTTPHandler(opts *httpclient.Options, resolver *vars.Resolver) Handler {
	return Handler{
		Match: IsHTTPKey,
		Apply: func(key, val string) error {
			m := map[string]string{key: val}
			return ApplyHTTPSettings(opts, m, resolver)
		},
	}
}

func GRPCHandler(opts *grpcclient.Options, resolver *vars.Resolver) Handler {
	return Handler{
		Match: PrefixMatcher("grpc-"),
		Apply: func(key, val string) error {
			m := map[string]string{key: val}
			return ApplyGRPCSettings(opts, m, resolver)
		},
	}
}
