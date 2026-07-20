package mock

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestControlAPIRejectsBrowserAndNonControlRequests(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/value default=true
HTTP/1.1 200 OK`)
	server, err := Start("127.0.0.1:0", handler, Options{EnableControl: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	client := &http.Client{Timeout: 2 * time.Second}
	request := func(origin string, control bool) int {
		t.Helper()
		req, err := http.NewRequest(
			http.MethodPost,
			"http://"+server.Addr()+controlCountPath,
			strings.NewReader(`{}`),
		)
		if err != nil {
			t.Fatal(err)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if control {
			req.Header.Set(controlHeader, "1")
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	if got := request("", false); got != http.StatusForbidden {
		t.Fatalf("missing control header status = %d", got)
	}
	if got := request("https://example.test", true); got != http.StatusForbidden {
		t.Fatalf("browser origin status = %d", got)
	}
	remote, err := http.NewRequest(http.MethodPost, "http://example.test"+controlCountPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	remote.RemoteAddr = "203.0.113.10:1234"
	remote.Header.Set(controlHeader, "1")
	if controlRequestAllowed(remote) {
		t.Fatal("non-loopback request was allowed")
	}
}

func TestControlAPIMustBeExplicitlyEnabled(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/value default=true
HTTP/1.1 200 OK`)
	server, err := Start("127.0.0.1:0", handler, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	req, err := http.NewRequest(
		http.MethodPost,
		"http://"+server.Addr()+controlCountPath,
		strings.NewReader(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(controlHeader, "1")
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want ordinary mock 404", response.StatusCode)
	}
	if server.Stats().Calls != 1 || server.JournalStats().Entries != 1 {
		t.Fatalf(
			"disabled control request was not ordinary traffic: stats=%+v journal=%+v",
			server.Stats(),
			server.JournalStats(),
		)
	}
}

func TestCompileReservesControlNamespaceForLiteralPaths(t *testing.T) {
	for _, path := range []string{
		controlResetPath,
		"/.resterm/mock/{operation...}",
		"/.resterm/other",
	} {
		t.Run(path, func(t *testing.T) {
			source := "# @mock method=POST path=" + path + "\nHTTP/1.1 200 OK"
			_, err := Compile([]*restfile.Document{parser.Parse("bad.http", []byte(source))})
			if err == nil || !strings.Contains(err.Error(), "reserved") {
				t.Fatalf("Compile() error = %v", err)
			}
		})
	}
	for _, path := range []string{"/{all...}", "/.well-known/{rest...}"} {
		t.Run(path, func(t *testing.T) {
			source := "# @mock method=POST path=" + path + "\nHTTP/1.1 200 OK"
			if _, err := Compile([]*restfile.Document{parser.Parse("ok.http", []byte(source))}); err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
		})
	}
}

func TestControlEndpointsShadowCatchAllMocks(t *testing.T) {
	handler := compileSource(t, `# @mock method=POST path=/{all...} default=true
HTTP/1.1 200 OK

mock`)
	server, err := Start("127.0.0.1:0", handler, Options{EnableControl: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	client, err := NewClient("http://"+server.Addr(), ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Count(context.Background(), RequestPattern{}); err != nil {
		t.Fatalf("control Count() through catch-all mock = %v", err)
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Post(
		"http://"+server.Addr()+"/other",
		"text/plain",
		strings.NewReader("x"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("mock response = %d", response.StatusCode)
	}
}

func TestNewClientValidatesURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "http host", raw: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{name: "https root path", raw: " https://localhost:9443/ ", want: "https://localhost:9443"},
		{name: "ipv6 host", raw: "http://[::1]:8080", want: "http://[::1]:8080"},
		{name: "malformed url", raw: "http://[::1", wantErr: true},
		{name: "unsupported scheme", raw: "ftp://127.0.0.1:8080", wantErr: true},
		{name: "missing host", raw: "http://", wantErr: true},
		{name: "empty hostname", raw: "http://:8080", wantErr: true},
		{name: "user info", raw: "http://user@127.0.0.1:8080", wantErr: true},
		{name: "path", raw: "http://127.0.0.1:8080/proxy", wantErr: true},
		{name: "query", raw: "http://127.0.0.1:8080?query=1", wantErr: true},
		{name: "empty query", raw: "http://127.0.0.1:8080?", wantErr: true},
		{name: "fragment", raw: "http://127.0.0.1:8080#fragment", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient(tt.raw, ClientOptions{})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewClient(%q) accepted an ambiguous URL", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewClient(%q) error = %v", tt.raw, err)
			}
			if got := c.baseURL.String(); got != tt.want {
				t.Fatalf("NewClient(%q) URL = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
