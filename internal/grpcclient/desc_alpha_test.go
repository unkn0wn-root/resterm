package grpcclient

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	reflectalpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

//nolint:staticcheck // v1alpha is deprecated, but this verifies compatibility with v1alpha-only servers.
func startAlphaReflectionServer(t *testing.T) (string, func()) {
	t.Helper()

	return startTestServerWith(t, func(srv *grpc.Server) {
		reflectalpha.RegisterServerReflectionServer(
			srv,
			reflection.NewServer(reflection.ServerOptions{Services: srv}),
		)
	})
}
