package httpclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/vars"
	"github.com/unkn0wn-root/resterm/pkg/restfile"
)

func TestApplyRequestSettings(t *testing.T) {
	opts := Options{Timeout: 5 * time.Second, FollowRedirects: true}
	settings := map[string]string{
		"timeout":         "10s",
		"proxy":           "http://localhost:8080",
		"followredirects": "false",
		"insecure":        "true",
	}

	effective := applyRequestSettings(opts, settings)
	if effective.Timeout != 10*time.Second {
		t.Fatalf("expected timeout 10s, got %s", effective.Timeout)
	}
	if !effective.InsecureSkipVerify {
		t.Fatalf("expected insecure skip verify to be true")
	}
	if effective.FollowRedirects {
		t.Fatalf("expected redirects disabled")
	}
	if effective.ProxyURL != "http://localhost:8080" {
		t.Fatalf("unexpected proxy url: %s", effective.ProxyURL)
	}
}

func TestInjectBodyIncludes(t *testing.T) {
	client := &Client{fs: OSFileSystem{}}
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "payload.json")
	if err := os.WriteFile(path, []byte(`{"status":"ok"}`), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	body := "part1\n@payload.json\n@{notIncluded}\n"
	processed, err := client.injectBodyIncludes(body, baseDir)
	if err != nil {
		t.Fatalf("inject body includes: %v", err)
	}
	if !strings.Contains(processed, `{"status":"ok"}`) {
		t.Fatalf("expected file contents to be embedded, got %q", processed)
	}
	if !strings.Contains(processed, "@{notIncluded}") {
		t.Fatalf("expected handlebars directive to remain untouched")
	}
}

func TestApplyAuthenticationBasic(t *testing.T) {
	client := NewClient(nil)
	req := restfile.Request{Method: "GET", URL: "https://example.com"}
	httpReq, err := http.NewRequestWithContext(context.Background(), req.Method, req.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	auth := &restfile.AuthSpec{Type: "basic", Params: map[string]string{"username": "alice", "password": "secret"}}
	resolver := vars.NewResolver()
	client.applyAuthentication(httpReq, resolver, auth)
	if got := httpReq.Header.Get("Authorization"); !strings.HasPrefix(got, "Basic ") {
		t.Fatalf("expected basic auth header, got %s", got)
	}
}

func TestPrepareGraphQLPostBody(t *testing.T) {
	client := NewClient(nil)
	req := &restfile.Request{Method: "POST", URL: "https://example.com/graphql"}
	req.Body.GraphQL = &restfile.GraphQLBody{
		Query:     "query Ping($id: ID!){ ping(id: $id) }",
		Variables: "{ \"id\": \"{{id}}\" }",
	}
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"id": "123"}))
	reader, err := client.prepareBody(req, resolver, Options{})
	if err != nil {
		t.Fatalf("prepare graphQL body: %v", err)
	}
	if reader == nil {
		t.Fatalf("expected reader for POST graphQL body")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["query"] != "query Ping($id: ID!){ ping(id: $id) }" {
		t.Fatalf("unexpected query: %v", payload["query"])
	}
	varsField, ok := payload["variables"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected variables object, got %T", payload["variables"])
	}
	if varsField["id"] != "123" {
		t.Fatalf("expected variables to expand templates, got %v", varsField["id"])
	}
}

func TestPrepareGraphQLGetQueryParameters(t *testing.T) {
	client := NewClient(nil)
	req := &restfile.Request{Method: "GET", URL: "https://example.com/graphql?existing=1"}
	req.Body.GraphQL = &restfile.GraphQLBody{
		Query:         "query { ping }",
		Variables:     "{ \"flag\": true }",
		OperationName: "Ping",
	}
	reader, err := client.prepareBody(req, vars.NewResolver(), Options{})
	if err != nil {
		t.Fatalf("prepare graphQL body (GET): %v", err)
	}
	if reader != nil {
		t.Fatalf("expected nil reader for GET graphql request")
	}
	parsed, err := url.Parse(req.URL)
	if err != nil {
		t.Fatalf("parse mutated url: %v", err)
	}
	values := parsed.Query()
	if values.Get("existing") != "1" {
		t.Fatalf("expected existing query param to persist, got %q", values.Get("existing"))
	}
	if values.Get("operationName") != "Ping" {
		t.Fatalf("expected operationName query param, got %q", values.Get("operationName"))
	}
	if values.Get("query") == "" {
		t.Fatalf("expected query parameter to be set")
	}
	if values.Get("variables") == "" {
		t.Fatalf("expected variables parameter to be set")
	}
}
