package parser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/openapi"
)

const sampleSpec = `openapi: 3.0.3
info:
  title: Sample
  version: 1.0.0
servers:
  - url: /v2
paths:
  /widgets:
    get:
      operationId: listWidgets
      responses:
        '200':
          description: ok
`

const rootRefSpec = `openapi: 3.0.3
info:
  title: Ref
  version: 1.0.0
paths:
  /widgets:
    get:
      operationId: listWidgets
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                $ref: 'schemas/widget.yaml#/Widget'
`

const widgetSchema = `Widget:
  type: object
  properties:
    id:
      type: string
`

func TestParseSpecURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"openapi.yaml", false},
		{"./specs/openapi.json", false},
		{"/abs/openapi.yaml", false},
		{`C:\specs\openapi.yaml`, false},
		{"ftp://host/openapi.yaml", false},
		{"http://host/openapi.yaml", true},
		{" https://host/v1/openapi.json ", true},
		{"HTTP://HOST/openapi.yaml", true},
		{"http://", false},
		{"", false},
	}
	for _, tc := range cases {
		if _, ok := ParseSpecURL(tc.in); ok != tc.want {
			t.Errorf("ParseSpecURL(%q) = %v, want %v", tc.in, ok, tc.want)
		}
	}
}

func TestBaseDirURL(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"http://host/a/b/spec.yaml":             "http://host/a/b/",
		"http://host/spec.yaml":                 "http://host/",
		"http://host":                           "http://host/",
		"https://host/v1/openapi.json?x=1#frag": "https://host/v1/",
	}
	for in, want := range cases {
		u, ok := ParseSpecURL(in)
		if !ok {
			t.Fatalf("ParseSpecURL(%q) not a URL", in)
		}
		if got := baseDirURL(u).String(); got != want {
			t.Errorf("baseDirURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func newURLLoader(c *http.Client) *Loader {
	return NewLoader(WithHTTPClient(c))
}

func TestLoaderParseFromURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(sampleSpec))
	}))
	defer srv.Close()

	loader := newURLLoader(srv.Client())
	spec, err := loader.Parse(context.Background(), srv.URL+"/openapi.yaml", openapi.ParseOptions{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(spec.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(spec.Operations))
	}
	if spec.Operations[0].ID != "listWidgets" {
		t.Fatalf("unexpected operation: %s", spec.Operations[0].ID)
	}
}

func TestLoaderParseFromURLResolvesRelativeServers(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleSpec))
	}))
	defer srv.Close()

	loader := newURLLoader(srv.Client())
	spec, err := loader.Parse(context.Background(), srv.URL+"/openapi.yaml", openapi.ParseOptions{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := srv.URL + "/v2"
	if len(spec.Servers) != 1 || spec.Servers[0].URL != want {
		t.Fatalf("expected server %q, got %#v", want, spec.Servers)
	}
}

func TestLoaderParseFromURLStatusError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	loader := newURLLoader(srv.Client())
	_, err := loader.Parse(context.Background(), srv.URL+"/openapi.yaml", openapi.ParseOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status in error, got: %v", err)
	}
}

func TestLoaderParseFromURLEmptyBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	loader := newURLLoader(srv.Client())
	_, err := loader.Parse(context.Background(), srv.URL+"/openapi.yaml", openapi.ParseOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-body error, got: %v", err)
	}
}

func TestLoaderParseFromURLResolvesRemoteRef(t *testing.T) {
	t.Parallel()
	var schemaHits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(rootRefSpec))
	})
	mux.HandleFunc("/schemas/widget.yaml", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&schemaHits, 1)
		_, _ = w.Write([]byte(widgetSchema))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	loader := newURLLoader(srv.Client())
	spec, err := loader.Parse(
		context.Background(),
		srv.URL+"/openapi.yaml",
		openapi.ParseOptions{ResolveExternalRefs: true},
	)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if atomic.LoadInt32(&schemaHits) == 0 {
		t.Fatal("expected remote $ref document to be fetched")
	}
	if len(spec.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(spec.Operations))
	}
}

func TestRemoteHandlerRejectsStatusError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	loader := newURLLoader(srv.Client())
	handler := loader.remoteHandler(context.Background())
	resp, err := handler(srv.URL + "/schemas/widget.yaml")
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status in error, got: %v", err)
	}
}

func TestRemoteHandlerRejectsTooLargeRef(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", maxSpecBytes+1)))
	}))
	defer srv.Close()

	loader := newURLLoader(srv.Client())
	handler := loader.remoteHandler(context.Background())
	resp, err := handler(srv.URL + "/schemas/widget.yaml")
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "maximum size") {
		t.Fatalf("expected size-limit error, got: %v", err)
	}
}
