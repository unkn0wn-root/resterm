package httpx

import (
	"net/url"
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
		{
			name:   "base without trailing slash",
			target: "users",
			base:   "https://api.example.com/v1",
			want:   "https://api.example.com/users",
		},
		{name: "encoded path", target: "files/a%2Fb", base: base, want: "https://api.example.com/v1/files/a%2Fb"},
		{
			name:   "uppercase base scheme",
			target: "users",
			base:   "HTTPS://api.example.com/v1/",
			want:   "https://api.example.com/v1/users",
		},
		{
			name:   "absolute ignores invalid base",
			target: "https://other.example/x%2Fy?z=%2F",
			base:   "://invalid",
			want:   "https://other.example/x%2Fy?z=%2F",
		},
		{name: "uppercase absolute scheme", target: "HTTPS://other.example/x", want: "https://other.example/x"},
		{name: "host and port", target: "localhost:8080/users", want: "http://localhost:8080/users"},
		{name: "host and port only", target: "localhost:8080", want: "http://localhost:8080"},
		{name: "ipv4 host and port", target: "127.0.0.1:8080/users", want: "http://127.0.0.1:8080/users"},
		{name: "ipv6 host and port", target: "[::1]:8080/users", want: "http://[::1]:8080/users"},
		{name: "authority form", target: "proxy.example.com:443", want: "http://proxy.example.com:443"},
		{name: "host and port with query", target: "localhost:8080/a?b=1", want: "http://localhost:8080/a?b=1"},
		{
			name:   "url valued query",
			target: "localhost:8080/p?next=http://example.com",
			want:   "http://localhost:8080/p?next=http://example.com",
		},
		{
			name:   "url valued query with tls",
			target: "localhost:8080/p?next=https://example.com/x&a=1",
			want:   "http://localhost:8080/p?next=https://example.com/x&a=1",
		},
		{
			name:   "url valued fragment",
			target: "localhost:8080/p#next=http://example.com",
			want:   "http://localhost:8080/p#next=http://example.com",
		},
		{name: "ipv6 without port", target: "[::1]/path", want: "http://[::1]/path"},
		{name: "ipv6 bare", target: "[::1]", want: "http://[::1]"},
		{
			name:   "explicit userinfo is kept",
			target: "http://user:pass@example.com/x",
			want:   "http://user:pass@example.com/x",
		},
		{
			name:   "host and port ignores base",
			target: "localhost:8080/users",
			base:   base,
			want:   "http://localhost:8080/users",
		},
		{
			name:   "bare name stays relative",
			target: "example.com/users",
			base:   base,
			want:   "https://api.example.com/v1/example.com/users",
		},
		{
			name:   "at sign in a path stays relative",
			target: "user@example.com/path",
			base:   base,
			want:   "https://api.example.com/v1/user@example.com/path",
		},
		{
			name:   "at sign in a path segment",
			target: "users/user@example.com",
			base:   base,
			want:   "https://api.example.com/v1/users/user@example.com",
		},
		{
			name:   "websocket host and port",
			target: "localhost:8080/socket",
			scheme: schemeWebSocket,
			want:   "ws://localhost:8080/socket",
		},
		{
			name:   "websocket relative",
			target: "socket",
			base:   base,
			scheme: schemeWebSocket,
			want:   "wss://api.example.com/v1/socket",
		},
		{
			name:   "websocket explicit HTTP",
			target: "http://socket.example/x",
			base:   "://invalid",
			scheme: schemeWebSocket,
			want:   "ws://socket.example/x",
		},
		{
			name:   "websocket explicit WSS",
			target: "wss://socket.example/x",
			scheme: schemeWebSocket,
			want:   "wss://socket.example/x",
		},
		{
			name:   "websocket uppercase WSS",
			target: "WSS://socket.example/x",
			scheme: schemeWebSocket,
			want:   "wss://socket.example/x",
		},
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

func TestResolveRequestTargetExpandsBeforeFillingInScheme(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		target string
		base   string
		want   string
	}{
		{name: "host and port", host: "localhost:8080", target: "{{host}}/users", want: "http://localhost:8080/users"},
		{name: "ipv4 and port", host: "127.0.0.1:9000", target: "{{host}}/users", want: "http://127.0.0.1:9000/users"},
		{
			name:   "variable carries scheme",
			host:   "http://example.com",
			target: "{{host}}/users",
			want:   "http://example.com/users",
		},
		{
			name:   "bare name needs base",
			host:   "example.com",
			target: "{{host}}/users",
			base:   "https://api.example.com/v1/",
			want:   "https://api.example.com/v1/example.com/users",
		},
		{
			name:   "scheme outside the template",
			host:   "example.com",
			target: "http://{{host}}/users",
			want:   "http://example.com/users",
		},
		{
			name:   "port inside the template",
			host:   "8080",
			target: "localhost:{{host}}/users",
			want:   "http://localhost:8080/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"host": tt.host}))
			got, err := resolveRequestTarget(tt.target, tt.base, resolver, schemeHTTP)
			if err != nil {
				t.Fatalf("resolveRequestTarget() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveRequestTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveRequestTargetBareHostVariableNeedsBaseURL(t *testing.T) {
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"host": "example.com"}))

	_, err := resolveRequestTarget("{{host}}/users", "", resolver, schemeHTTP)
	if err == nil {
		t.Fatal("expected an error")
	}
	if want := "requires a base-url setting"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain %q", err, want)
	}
}

// Compare the parts sent over HTTP because net/url may normalize equivalent
// input when it serializes a URL.
func TestResolveRequestTargetPreservesAbsoluteURLs(t *testing.T) {
	for _, raw := range []string{
		"https://something:443/hello?name=david&hello#makesthat&not?",
		"https://x/p?a=1&b=2&b=3&c=&d#frag",
		"https://x/a%2Fb/c?q=%2F&r=a+b&s=a%20b#f%2Fg",
		"http://x/p?next=https://y/z?deep=1#anchor",
		"https://x/" + "über/päth?q=ü#ünicode",
		"https://user:pass@x:8443/p?q=1#f",
		"https://x/p;matrix=1/q?a[]=1&a[]=2",
		"https://x?onlyquery#onlyfrag",
		"https://x/p?q={{tpl}}&r=b",
		"http://x:8080",
	} {
		t.Run(raw, func(t *testing.T) {
			want, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("url.Parse(%q) error = %v", raw, err)
			}

			resolved, err := resolveRequestTarget(raw, "", nil, schemeHTTP)
			if err != nil {
				t.Fatalf("resolveRequestTarget() error = %v", err)
			}
			got, err := url.Parse(resolved)
			if err != nil {
				t.Fatalf("resolved %q does not parse: %v", resolved, err)
			}

			for _, f := range []struct {
				name      string
				want, got string
			}{
				{"scheme", want.Scheme, got.Scheme},
				{"host", want.Host, got.Host},
				{"escaped path", want.EscapedPath(), got.EscapedPath()},
				{"query", want.RawQuery, got.RawQuery},
				{"fragment", want.Fragment, got.Fragment},
				{"userinfo", want.User.String(), got.User.String()},
				{"request uri", want.RequestURI(), got.RequestURI()},
			} {
				if f.want != f.got {
					t.Errorf("%s = %q, want %q (resolved %q)", f.name, f.got, f.want, resolved)
				}
			}
		})
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
		{
			name:     "missing target variable",
			target:   "{{path}}",
			base:     "https://api.example.com/",
			resolver: vars.NewResolver(),
			want:     "expand url",
		},
		{
			name:     "empty expanded target",
			target:   "{{path}}",
			base:     "https://api.example.com/",
			resolver: vars.NewResolver(vars.NewMapProvider("env", map[string]string{"path": ""})),
			want:     "request url is empty",
		},
		{
			name:     "missing base variable",
			target:   "users",
			base:     "{{api}}",
			resolver: vars.NewResolver(),
			want:     "expand base-url",
		},
		{name: "unsupported HTTP scheme", target: "ftp://api.example.com/x", want: "scheme must be http or https"},
		{name: "mailto is not a host", target: "mailto:someone@example.com", want: "request url host is empty"},
		{
			name:   "credentials are not a host",
			target: "user:pass@example.com:8080/x",
			want:   "must not carry credentials",
		},
		{name: "non numeric port", target: "a:b/c", want: "request url host is empty"},
		{name: "data url", target: "data:text/html,x", want: "request url host is empty"},
		{name: "port without host", target: ":8080/users", want: "missing protocol scheme"},
		{name: "https without slashes", target: "https:443/path", want: "request url host is empty"},
		{name: "http without slashes", target: "http:8080/path", want: "request url host is empty"},
		{name: "ws without slashes", target: "ws:8080/path", want: "request url host is empty"},
		{name: "empty port", target: "localhost:/path", want: "request url host is empty"},
		{name: "non numeric port", target: "localhost:80a/x", want: "request url host is empty"},
		{name: "credentials before a host", target: "example.com:8080@evil.com/x", want: "request url host is empty"},
		{name: "userinfo without a password", target: "user@example.com:8080/path", want: "must not carry credentials"},
		{name: "userinfo before an ip", target: "user@127.0.0.1:8080/path", want: "must not carry credentials"},
		{name: "userinfo before an ipv6 host", target: "user@[::1]:8080/x", want: "must not carry credentials"},
		{
			name:   "websocket scheme without directive",
			target: "ws://example.com/s",
			want:   "needs a @websocket directive",
		},
		{
			name:   "secure websocket scheme without directive",
			target: "wss://example.com/s",
			want:   "needs a @websocket directive",
		},
		{name: "unexpanded host in scheme url", target: "http://{{host}}/status", want: "invalid character"},
		{name: "unexpanded host with port", target: "{{host}}:8080/x", want: "parse request url"},
		{name: "missing target host", target: "https:///x", want: "request url host is empty"},
		{
			name:     "base expands to empty",
			target:   "users",
			base:     "{{api}}",
			resolver: vars.NewResolver(vars.NewMapProvider("env", map[string]string{"api": ""})),
			want:     "requires a base-url setting",
		},
		{
			name:   "websocket fragment",
			target: "socket#section",
			base:   "https://api.example.com/",
			scheme: schemeWebSocket,
			want:   "must not contain a fragment",
		},
		{
			name:   "websocket empty fragment",
			target: "socket#",
			base:   "https://api.example.com/",
			scheme: schemeWebSocket,
			want:   "must not contain a fragment",
		},
		{
			name:   "unsupported websocket scheme",
			target: "ftp://api.example.com/socket",
			scheme: schemeWebSocket,
			want:   "scheme must be ws, wss, http or https",
		},
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
