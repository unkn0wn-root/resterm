package httpx

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestResolveRequestTargetRFCReferences(t *testing.T) {
	const base = "https://api.example.com/v1/"
	tests := []struct {
		name   string
		target string
		base   string
		scheme requestScheme
		want   string
	}{
		{name: "path relative", target: "users", base: base, want: "https://api.example.com/v1/users"},
		{name: "root relative", target: "/users", base: base, want: "https://api.example.com/users"},
		{name: "parent segment", target: "../health", base: base, want: "https://api.example.com/health"},
		{name: "query only", target: "?page=2", base: base, want: "https://api.example.com/v1/?page=2"},
		{name: "network path", target: "//uploads.example.com/x", base: base, want: "https://uploads.example.com/x"},
		{name: "base without trailing slash", target: "users", base: "https://api.example.com/v1", want: "https://api.example.com/users"},
		{name: "encoded path", target: "files/a%2Fb", base: base, want: "https://api.example.com/v1/files/a%2Fb"},
		{name: "uppercase base scheme", target: "users", base: "HTTPS://api.example.com/v1/", want: "https://api.example.com/v1/users"},
		{name: "absolute ignores invalid base", target: "https://other.example/x%2Fy?z=%2F", base: "://invalid", want: "https://other.example/x%2Fy?z=%2F"},
		{name: "uppercase absolute scheme", target: "HTTPS://other.example/x", want: "https://other.example/x"},
		{name: "websocket relative", target: "socket", base: base, scheme: schemeWebSocket, want: "wss://api.example.com/v1/socket"},
		{name: "websocket explicit HTTP", target: "http://socket.example/x", base: "://invalid", scheme: schemeWebSocket, want: "ws://socket.example/x"},
		{name: "websocket explicit WSS", target: "wss://socket.example/x", scheme: schemeWebSocket, want: "wss://socket.example/x"},
		{name: "websocket uppercase WSS", target: "WSS://socket.example/x", scheme: schemeWebSocket, want: "wss://socket.example/x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRequestTarget(tt.target, tt.base, nil, tt.scheme)
			if err != nil {
				t.Fatalf("resolveRequestTarget() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveRequestTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveRequestTargetExpandsTargetAndBase(t *testing.T) {
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{
		"api":      "https://api.example.com/v2/",
		"resource": "users/42",
	}))

	got, err := resolveRequestTarget("{{resource}}", "{{api}}", resolver, schemeHTTP)
	if err != nil {
		t.Fatalf("resolveRequestTarget() error = %v", err)
	}
	if want := "https://api.example.com/v2/users/42"; got != want {
		t.Fatalf("resolveRequestTarget() = %q, want %q", got, want)
	}
}

func TestResolveRequestTargetValidatesBaseWhenNeeded(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "missing", want: "requires a base-url setting"},
		{name: "relative base", base: "api.example.com/v1/", want: "scheme must be http or https"},
		{name: "websocket scheme", base: "wss://api.example.com/", want: "scheme must be http or https"},
		{name: "missing host", base: "https:///v1/", want: "host is required"},
		{name: "userinfo", base: "https://user:pass@api.example.com/", want: "userinfo is not allowed"},
		{name: "query", base: "https://api.example.com/v1/?tenant=one", want: "query is not allowed"},
		{name: "empty query", base: "https://api.example.com/v1/?", want: "query is not allowed"},
		{name: "fragment", base: "https://api.example.com/v1/#section", want: "fragment is not allowed"},
		{name: "empty fragment", base: "https://api.example.com/v1/#", want: "fragment is not allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveRequestTarget("users", tt.base, nil, schemeHTTP)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestResolveRequestTargetReportsExpansionAndProtocolErrors(t *testing.T) {
	tests := []struct {
		name     string
		target   string
		base     string
		resolver *vars.Resolver
		scheme   requestScheme
		want     string
	}{
		{name: "missing target variable", target: "{{path}}", base: "https://api.example.com/", resolver: vars.NewResolver(), want: "expand url"},
		{name: "empty expanded target", target: "{{path}}", base: "https://api.example.com/", resolver: vars.NewResolver(vars.NewMapProvider("env", map[string]string{"path": ""})), want: "request url is empty"},
		{name: "missing base variable", target: "users", base: "{{api}}", resolver: vars.NewResolver(), want: "expand base-url"},
		{name: "unsupported HTTP scheme", target: "ftp://api.example.com/x", want: "scheme must be http or https"},
		{name: "missing target host", target: "https:///x", want: "request url host is empty"},
		{name: "base expands to empty", target: "users", base: "{{api}}", resolver: vars.NewResolver(vars.NewMapProvider("env", map[string]string{"api": ""})), want: "requires a base-url setting"},
		{name: "websocket fragment", target: "socket#section", base: "https://api.example.com/", scheme: schemeWebSocket, want: "must not contain a fragment"},
		{name: "websocket empty fragment", target: "socket#", base: "https://api.example.com/", scheme: schemeWebSocket, want: "must not contain a fragment"},
		{name: "unsupported websocket scheme", target: "ftp://api.example.com/socket", scheme: schemeWebSocket, want: "scheme must be ws, wss, http or https"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveRequestTarget(
				tt.target,
				tt.base,
				tt.resolver,
				tt.scheme,
			)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}
