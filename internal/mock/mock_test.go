package mock

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestHandlerSelectsScenariosAndRoutesRequests(t *testing.T) {
	handler := compileSource(t, `### Pending fallback
# @mock method=POST path=/payments name=pending default=true
HTTP/1.1 202 Accepted

pending
### Duplicate
# @mock method=POST path=/payments name=duplicate
# @match query={"mode":"test"} headers={"X-Tenant":"acme"} json={"amount":100}
HTTP/1.1 409 Conflict

duplicate
### User
# @mock method=GET path=/users/{id} default=true
HTTP/1.1 200 OK

user`)

	if handler.Routes() != 2 || handler.Scenarios() != 3 {
		t.Fatalf("routes=%d scenarios=%d", handler.Routes(), handler.Scenarios())
	}
	req := httptest.NewRequest(
		http.MethodPost,
		"/payments?mode=test",
		strings.NewReader(`{"amount":100,"currency":"USD"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant", "acme")
	assertResponse(t, handler, req, http.StatusConflict, "duplicate")

	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodPost, "/payments", nil),
		http.StatusAccepted,
		"pending",
	)
	req = httptest.NewRequest(http.MethodPost, "/payments", nil)
	req.Header.Set("X-Resterm-Mock", "duplicate")
	assertResponse(t, handler, req, http.StatusConflict, "duplicate")
	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/users/42", nil),
		http.StatusOK,
		"user",
	)
	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/missing", nil),
		http.StatusNotFound,
		"not found",
	)
}

func TestCompileRejectsInvalidMocks(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "relative path",
			source: "# @mock method=GET path=relative\nHTTP/1.1 200 OK",
			want:   "origin-form path",
		},
		{
			name: "duplicate default",
			source: `# @mock method=GET path=/x default=true
HTTP/1.1 200 OK
### Second
# @mock method=GET path=/x default=true
HTTP/1.1 201 Created`,
			want: "already has a default",
		},
		{
			name:   "managed header",
			source: "# @mock method=GET path=/x\nHTTP/1.1 200 OK\nContent-Length: 2\n\nok",
			want:   "managed by the HTTP server",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile([]*restfile.Document{parser.Parse("bad.http", []byte(test.source))})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Compile() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestReloaderReloadsFixtureChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "mocks.http")
	fixture := filepath.Join(root, "body.txt")
	writeFile(t, fixture, "one")
	writeFile(t, path, `# @mock method=GET path=/value
HTTP/1.1 200 OK

< ./body.txt`)

	reloader := NewReloader(root, false)
	handler, err := reloader.Reload("", nil)
	if err != nil || handler == nil {
		t.Fatalf("initial reload = %v, %v", handler, err)
	}
	assertResponse(t, handler, httptest.NewRequest(http.MethodGet, "/value", nil), http.StatusOK, "one")

	if handler, err = reloader.Reload("", nil); err != nil || handler != nil {
		t.Fatalf("unchanged reload = %v, %v", handler, err)
	}
	writeFile(t, fixture, "two")
	if handler, err = reloader.Reload("", nil); err != nil || handler == nil {
		t.Fatalf("fixture reload = %v, %v", handler, err)
	}
	assertResponse(t, handler, httptest.NewRequest(http.MethodGet, "/value", nil), http.StatusOK, "two")
}

func TestLoadConfinesFixturesToSourceRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(parent, "secret.txt"), "secret")
	writeFile(t, filepath.Join(root, "mocks.http"), `# @mock method=GET path=/value
HTTP/1.1 200 OK

< ../secret.txt`)

	_, err := Load(root, false, nil)
	if err == nil || !strings.Contains(err.Error(), "read mock response body") {
		t.Fatalf("Load() error = %v, want confined fixture rejection", err)
	}
}

func TestServerReloadsRoutesAndAppliesCORS(t *testing.T) {
	initial := compileSource(t, `# @mock method=GET path=/value
HTTP/1.1 200 OK

old`)
	updated := compileSource(t, `# @mock method=GET path=/value
HTTP/1.1 201 Created

new`)
	server, err := Start("127.0.0.1:0", initial, Options{CORS: WildcardCORS()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	client := &http.Client{Timeout: 2 * time.Second}
	response := get(t, client, "http://"+server.Addr()+"/value", "https://app.example")
	if body := readBody(t, response); response.StatusCode != http.StatusOK || body != "old" ||
		response.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("initial response = %d %q headers=%v", response.StatusCode, body, response.Header)
	}

	server.Reload(updated)
	response = get(t, client, "http://"+server.Addr()+"/value", "")
	if body := readBody(t, response); response.StatusCode != http.StatusCreated || body != "new" {
		t.Fatalf("reloaded response = %d %q", response.StatusCode, body)
	}
	if err := server.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-server.Done()
}

func compileSource(t *testing.T, source string) *Handler {
	t.Helper()
	handler, err := Compile([]*restfile.Document{parser.Parse("mocks.http", []byte(source))})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func assertResponse(t *testing.T, handler http.Handler, request *http.Request, status int, bodyContains string) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != status || !strings.Contains(response.Body.String(), bodyContains) {
		t.Fatalf(
			"response = %d %q, want %d containing %q",
			response.Code,
			response.Body.String(),
			status,
			bodyContains,
		)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, client *http.Client, url, origin string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return string(body)
}
