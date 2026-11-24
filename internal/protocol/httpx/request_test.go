package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestPrepareHTTPRequestRejectsHTTP2OverHTTP(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{Method: "GET", URL: "http://example.com"}
	opts := optionsFrom(t, map[string]string{"http-version": "2"})

	_, _, err := c.prepareHTTPRequest(context.Background(), req, nil, opts)
	if err == nil {
		t.Fatalf("expected error for http-version=2 over http")
	}
	if !strings.Contains(err.Error(), "requires https") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareHTTPRequestAllowsHTTP2OverHTTPS(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{Method: "GET", URL: "https://example.com"}
	opts := optionsFrom(t, map[string]string{"http-version": "2"})

	_, _, err := c.prepareHTTPRequest(context.Background(), req, nil, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// The settings package applies req.Settings to the options before a send. A raw
// template that still parses must not replace the value it expanded to.
func TestBuildHTTPRequestReadsSettingsOnlyFromOptions(t *testing.T) {
	for _, tt := range []struct {
		name     string
		opts     Options
		settings map[string]string
	}{
		{
			name:     "proxy",
			opts:     Options{ProxyURL: "http://proxy:8080/edge"},
			settings: map[string]string{"proxy": "http://proxy:8080/{{x}}"},
		},
		{
			name:     "timeout",
			opts:     Options{Timeout: 5 * time.Second},
			settings: map[string]string{"timeout": "1s"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := &restfile.Request{Method: http.MethodGet, URL: "http://example.com", Settings: tt.settings}
			_, got, _, err := NewClient(nil).BuildHTTPRequest(t.Context(), req, nil, tt.opts)
			if err != nil {
				t.Fatalf("BuildHTTPRequest() error = %v", err)
			}
			if got.ProxyURL != tt.opts.ProxyURL || got.Timeout != tt.opts.Timeout {
				t.Fatalf(
					"options = %q %v, want %q %v",
					got.ProxyURL,
					got.Timeout,
					tt.opts.ProxyURL,
					tt.opts.Timeout,
				)
			}
		})
	}

	t.Run("execute", func(t *testing.T) {
		var hits atomic.Int32
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.IsAbs() {
				hits.Add(1)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer proxy.Close()

		req := &restfile.Request{
			Method:   http.MethodGet,
			URL:      "http://example.com/",
			Settings: map[string]string{"proxy": "http://127.0.0.1:1/{{x}}"},
		}
		resp, err := NewClient(nil).Execute(t.Context(), req, nil, Options{ProxyURL: proxy.URL})
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if resp.StatusCode != http.StatusNoContent || hits.Load() != 1 {
			t.Fatalf("status %d with %d proxy hits, want 204 with 1", resp.StatusCode, hits.Load())
		}
	})

	t.Run("websocket", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := websocket.Accept(w, r, nil); err == nil {
				_ = c.Close(websocket.StatusNormalClosure, "")
			}
		}))
		defer srv.Close()

		// HTTP/2 is rejected for websockets, so a re-read of this setting would fail the start.
		req := &restfile.Request{
			Method:    http.MethodGet,
			URL:       "ws" + strings.TrimPrefix(srv.URL, "http"),
			WebSocket: &restfile.WebSocketRequest{},
			Settings:  map[string]string{"http-version": "2"},
		}
		handle, _, err := NewClient(nil).StartWebSocket(t.Context(), req, nil, Options{})
		if err != nil {
			t.Fatalf("StartWebSocket() error = %v", err)
		}
		<-handle.Session.Done()
	})
}

func TestBuildHTTPRequestFailsOnUnresolvedHeader(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{
		Method:     "GET",
		URL:        "https://example.com",
		Headers:    http.Header{"X-Trace": []string{"{{traceID}}"}},
		SourcePath: "api.http",
		LineRange:  restfile.LineRange{Start: 10, End: 12},
	}

	_, _, _, err := c.BuildHTTPRequest(context.Background(), req, vars.NewResolver(), Options{})
	if err == nil {
		t.Fatalf("expected error for unresolved header variable")
	}
	if !strings.Contains(err.Error(), "expand header X-Trace (api.http:10)") ||
		!strings.Contains(err.Error(), "undefined variable: traceID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildHTTPRequestFailsOnUnresolvedBearerToken(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type:       "bearer",
				Params:     map[string]string{"token": "{{auth.globalToken}}"},
				SourcePath: "auth.http",
				Line:       3,
			},
		},
	}

	_, _, _, err := c.BuildHTTPRequest(context.Background(), req, vars.NewResolver(), Options{})
	if err == nil {
		t.Fatalf("expected error for unresolved bearer token")
	}
	if !strings.Contains(err.Error(), "expand bearer auth token (auth.http:3)") ||
		!strings.Contains(err.Error(), "undefined variable: auth.globalToken") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildHTTPRequestFailsOnUnresolvedBasicAuth(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type:   "basic",
				Params: map[string]string{"username": "svc", "password": "{{secret}}"},
			},
		},
	}

	_, _, _, err := c.BuildHTTPRequest(context.Background(), req, vars.NewResolver(), Options{})
	if err == nil {
		t.Fatalf("expected error for unresolved basic auth password")
	}
	if !strings.Contains(err.Error(), "expand basic auth password") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildHTTPRequestFailsOnUnresolvedAPIKeyQuery(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type: "apikey",
				Params: map[string]string{
					"placement": "query",
					"name":      "key",
					"value":     "{{apiKey}}",
				},
			},
		},
	}

	_, _, _, err := c.BuildHTTPRequest(context.Background(), req, vars.NewResolver(), Options{})
	if err == nil {
		t.Fatalf("expected error for unresolved apikey value")
	}
	if !strings.Contains(err.Error(), "expand apikey auth value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyAuthenticationSkipsUnusedParams(t *testing.T) {
	c := NewClient(nil)
	httpReq, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	httpReq.Header.Set("Authorization", "Bearer explicit")
	auth := &restfile.AuthSpec{
		Type:   "bearer",
		Params: map[string]string{"token": "{{missing}}"},
	}

	if _, err := c.applyAuthentication(httpReq, vars.NewResolver(), auth); err != nil {
		t.Fatalf("unexpected error for unused auth param: %v", err)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer explicit" {
		t.Fatalf("authorization header changed: %q", got)
	}
}

func TestApplyAuthenticationSkipsUnusedParamsForLowercaseHeader(t *testing.T) {
	c := NewClient(nil)
	httpReq, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	httpReq.Header = http.Header{"authorization": {"Bearer explicit"}}
	auth := &restfile.AuthSpec{
		Type:   "bearer",
		Params: map[string]string{"token": "{{missing}}"},
	}

	if _, err := c.applyAuthentication(httpReq, vars.NewResolver(), auth); err != nil {
		t.Fatalf("unexpected error for case-insensitive auth override: %v", err)
	}
	if got := header.Value(httpReq.Header, "Authorization"); got != "Bearer explicit" {
		t.Fatalf("authorization header changed: %q", got)
	}
	if len(httpReq.Header) != 1 {
		t.Fatalf("auth added a second authorization header: %v", httpReq.Header)
	}
}

func TestBuildHTTPRequestExpandsHeaderAndAuth(t *testing.T) {
	c := NewClient(nil)
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{
		"traceID": "abc123",
		"token":   "tok-1",
	}))
	req := &restfile.Request{
		Method:  "GET",
		URL:     "https://example.com",
		Headers: http.Header{"X-Trace": []string{"{{traceID}}"}},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "{{token}}"},
			},
		},
	}

	httpReq, _, _, err := c.BuildHTTPRequest(context.Background(), req, resolver, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := httpReq.Header.Get("X-Trace"); got != "abc123" {
		t.Fatalf("unexpected trace header: %q", got)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer tok-1" {
		t.Fatalf("unexpected authorization header: %q", got)
	}
}

func TestBuildHTTPRequestResolvesBaseURLSetting(t *testing.T) {
	c := NewClient(nil)
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{
		"api.base": "https://api.example.com/v1/",
	}))
	req := &restfile.Request{Method: http.MethodGet, URL: "users/42?view=full"}
	opts := optionsFrom(t, map[string]string{"base-url": "{{api.base}}"})

	httpReq, effective, _, err := c.BuildHTTPRequest(t.Context(), req, resolver, opts)
	if err != nil {
		t.Fatalf("BuildHTTPRequest() error = %v", err)
	}
	if got, want := httpReq.URL.String(), "https://api.example.com/v1/users/42?view=full"; got != want {
		t.Fatalf("request URL = %q, want %q", got, want)
	}
	if effective.BaseURL != "{{api.base}}" {
		t.Fatalf("effective BaseURL = %q, want the raw setting", effective.BaseURL)
	}
	if req.URL != "users/42?view=full" {
		t.Fatalf("source request URL was mutated to %q", req.URL)
	}
}

func TestBuildHTTPRequestResolvesGraphQLGETAgainstBaseURL(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "graphql?existing=1",
		Body: restfile.BodySource{GraphQL: &restfile.GraphQLBody{
			Query:         "query { ping }",
			OperationName: "Ping",
		}},
	}
	opts := optionsFrom(t, map[string]string{"base-url": "https://api.example.com/v1/"})

	httpReq, _, _, err := c.BuildHTTPRequest(t.Context(), req, vars.NewResolver(), opts)
	if err != nil {
		t.Fatalf("BuildHTTPRequest() error = %v", err)
	}
	if got, want := httpReq.URL.Path, "/v1/graphql"; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
	query := httpReq.URL.Query()
	if query.Get("existing") != "1" || query.Get("operationName") != "Ping" ||
		query.Get("query") == "" {
		t.Fatalf("request query = %#v, want existing GraphQL parameters", query)
	}
}

func TestBuildHTTPRequestUsesBaseURLForHTTPFamilyMetadata(t *testing.T) {
	tests := []struct {
		name string
		set  func(*restfile.Request)
		want string
	}{
		{name: "REST", want: "https://api.example.com/events"},
		{
			name: "SSE",
			set: func(req *restfile.Request) {
				req.SSE = &restfile.SSERequest{}
			},
			want: "https://api.example.com/events",
		},
		{
			name: "WebSocket",
			set: func(req *restfile.Request) {
				req.WebSocket = &restfile.WebSocketRequest{}
			},
			want: "wss://api.example.com/events",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &restfile.Request{Method: http.MethodGet, URL: "/events"}
			if tt.set != nil {
				tt.set(req)
			}
			httpReq, _, _, err := NewClient(nil).BuildHTTPRequest(
				t.Context(),
				req,
				nil,
				Options{BaseURL: "https://api.example.com/v1/"},
			)
			if err != nil {
				t.Fatalf("BuildHTTPRequest() error = %v", err)
			}
			if got := httpReq.URL.String(); got != tt.want {
				t.Fatalf("request URL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildHTTPRequestSendsSchemelessLocalTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/users?page=2" {
			t.Errorf("server request URI = %q", r.URL.RequestURI())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	target := strings.TrimPrefix(srv.URL, "http://") + "/users?page=2"
	req := &restfile.Request{Method: http.MethodGet, URL: target}

	httpReq, _, _, err := NewClient(nil).BuildHTTPRequest(t.Context(), req, nil, Options{})
	if err != nil {
		t.Fatalf("BuildHTTPRequest() error = %v", err)
	}
	if got, want := httpReq.URL.String(), srv.URL+"/users?page=2"; got != want {
		t.Fatalf("request URL = %q, want %q", got, want)
	}
	if req.URL != target {
		t.Fatalf("source request URL was mutated to %q", req.URL)
	}

	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %s, want 204", resp.Status)
	}
}

func TestBuildHTTPRequestSchemelessWebSocketNeedsMetadata(t *testing.T) {
	for _, tt := range []struct {
		name string
		ws   *restfile.WebSocketRequest
		want string
	}{
		{name: "with @websocket", ws: &restfile.WebSocketRequest{}, want: "ws://localhost:8080/socket"},
		{name: "without @websocket", want: "http://localhost:8080/socket"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := &restfile.Request{
				Method:    http.MethodGet,
				URL:       "localhost:8080/socket",
				WebSocket: tt.ws,
			}
			httpReq, _, _, err := NewClient(nil).BuildHTTPRequest(t.Context(), req, nil, Options{})
			if err != nil {
				t.Fatalf("BuildHTTPRequest() error = %v", err)
			}
			if got := httpReq.URL.String(); got != tt.want {
				t.Fatalf("request URL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildHTTPRequestLenientResolverKeepsPlaceholders(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{
		Method:  "GET",
		URL:     "https://example.com",
		Headers: http.Header{"X-Trace": []string{"{{traceID}}"}},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "{{tok}}"},
			},
		},
	}

	httpReq, _, _, err := c.BuildHTTPRequest(
		context.Background(),
		req,
		vars.NewResolver().Lenient(),
		Options{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := httpReq.Header.Get("X-Trace"); got != "{{traceID}}" {
		t.Fatalf("unexpected trace header: %q", got)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer {{tok}}" {
		t.Fatalf("unexpected authorization header: %q", got)
	}
}

func TestBuildHTTPRequestExpandsAPIKeyPlacement(t *testing.T) {
	c := NewClient(nil)
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{
		"apiKeyPlacement": " Query ",
		"apiKey":          "k-123",
	}))
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/path",
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type: "apikey",
				Params: map[string]string{
					"placement": "{{apiKeyPlacement}}",
					"name":      "key",
					"value":     "{{apiKey}}",
				},
			},
		},
	}

	httpReq, _, _, err := c.BuildHTTPRequest(context.Background(), req, resolver, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := httpReq.URL.Query().Get("key"); got != "k-123" {
		t.Fatalf("expected api key in query, got %q", got)
	}
	if got := httpReq.Header.Get("key"); got != "" {
		t.Fatalf("api key must not land in a header, got %q", got)
	}
}

func TestBuildHTTPRequestFailsOnUnresolvedAPIKeyPlacement(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type: "apikey",
				Params: map[string]string{
					"placement": "{{apiKeyPlacement}}",
					"name":      "key",
					"value":     "k-123",
				},
			},
		},
	}

	_, _, _, err := c.BuildHTTPRequest(context.Background(), req, vars.NewResolver(), Options{})
	if err == nil {
		t.Fatalf("expected error for unresolved placement")
	}
	if !strings.Contains(err.Error(), "expand apikey auth placement") ||
		!strings.Contains(err.Error(), "undefined variable: apiKeyPlacement") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyAuthenticationInvalidPlacementErrors(t *testing.T) {
	c := NewClient(nil)
	httpReq, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	auth := &restfile.AuthSpec{
		Type: "apikey",
		Params: map[string]string{
			"placement": "qurey",
			"name":      "X-Key",
			"value":     "k-123",
		},
	}

	_, err = c.applyAuthentication(httpReq, vars.NewResolver(), auth)
	if err == nil || !strings.Contains(err.Error(), `invalid apikey auth placement "qurey"`) {
		t.Fatalf("expected invalid placement error, got %v", err)
	}
	if got := httpReq.Header.Get("X-Key"); got != "" {
		t.Fatalf("api key must not be applied on invalid placement, got %q", got)
	}
}

func TestApplyAuthenticationEmptyPlacementDefaultsToHeader(t *testing.T) {
	c := NewClient(nil)
	httpReq, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	auth := &restfile.AuthSpec{
		Type:   "apikey",
		Params: map[string]string{"name": "X-Key", "value": "k-123"},
	}

	if _, err := c.applyAuthentication(httpReq, vars.NewResolver(), auth); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := httpReq.Header.Get("X-Key"); got != "k-123" {
		t.Fatalf("expected header placement by default, got %q", got)
	}
}

func TestApplyAuthenticationLenientAPIKeyPlacement(t *testing.T) {
	tests := []struct {
		name      string
		placement string
		wantError string
	}{
		{
			name:      "undefined variable skips key",
			placement: "{{apiKeyPlacement}}",
		},
		{
			name:      "malformed opening marker errors",
			placement: "header{{",
			wantError: `invalid apikey auth placement "header{{"`,
		},
		{
			name:      "blank placeholder errors",
			placement: "{{ }}",
			wantError: `invalid apikey auth placement "{{ }}"`,
		},
		{
			name:      "misspelled placement errors",
			placement: "qurey",
			wantError: `invalid apikey auth placement "qurey"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(nil)
			httpReq, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			auth := &restfile.AuthSpec{
				Type: "apikey",
				Params: map[string]string{
					"placement": tt.placement,
					"name":      "X-Key",
					"value":     "k-123",
				},
			}

			_, err = c.applyAuthentication(httpReq, vars.NewResolver().Lenient(), auth)
			if tt.wantError == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
			}
			if got := httpReq.Header.Get("X-Key"); got != "" {
				t.Fatalf("api key with unknown placement must not be applied, got %q", got)
			}
			if got := httpReq.URL.RawQuery; got != "" {
				t.Fatalf("api key with unknown placement must not touch the query, got %q", got)
			}
		})
	}
}

func TestBuildHTTPRequestLocatesBodyVariableAfterComment(t *testing.T) {
	req := &restfile.Request{
		Method:     "POST",
		URL:        "https://example.com",
		SourcePath: "requests.http",
		Body: restfile.BodySource{
			Text:  "{\n  \"token\": \"{{auth.token}}\"\n}",
			Lines: []int{3, 5, 6},
		},
	}

	_, _, _, err := NewClient(nil).BuildHTTPRequest(context.Background(), req, vars.NewResolver(), Options{})
	if err == nil {
		t.Fatal("expected an expansion error")
	}
	got := diag.Render(err)
	want := "error[protocol]: undefined variable: auth.token\n" +
		"--> requests.http:5:13\n" +
		"expand body template"
	if got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestBuildHTTPRequestUserAgent(t *testing.T) {
	const ua = "resterm/1.2.3"
	tests := []struct {
		name    string
		headers http.Header
		auth    *restfile.AuthSpec
		opts    Options
		want    []string
	}{
		{name: "default", opts: Options{UserAgent: ua}, want: []string{ua}},
		{name: "zero options"},
		{
			name:    "written",
			headers: http.Header{"User-Agent": {"mine/1"}},
			opts:    Options{UserAgent: ua},
			want:    []string{"mine/1"},
		},
		{
			name:    "written empty",
			headers: http.Header{"User-Agent": {""}},
			opts:    Options{UserAgent: ua},
			want:    []string{""},
		},
		{
			name:    "template empty",
			headers: http.Header{"User-Agent": {"{{ua}}"}},
			opts:    Options{UserAgent: ua},
			want:    []string{""},
		},
		{
			name: "auth header",
			auth: &restfile.AuthSpec{
				Type:   restfile.AuthHeader,
				Params: map[string]string{"header": "User-Agent", "value": "auth/1"},
			},
			opts: Options{UserAgent: ua},
			want: []string{"auth/1"},
		},
		{
			name: "apikey header",
			auth: &restfile.AuthSpec{
				Type:   restfile.AuthAPIKey,
				Params: map[string]string{"placement": "header", "name": "User-Agent", "value": "key/1"},
			},
			opts: Options{UserAgent: ua},
			want: []string{"key/1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &restfile.Request{Method: http.MethodGet, URL: "https://example.com", Headers: tt.headers}
			req.Metadata.Auth = tt.auth
			src := req.Headers.Clone()
			resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"ua": ""}))

			httpReq, _, _, err := NewClient(nil).BuildHTTPRequest(context.Background(), req, resolver, tt.opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, ok := httpReq.Header["User-Agent"]
			if ok != (tt.want != nil) || !slices.Equal(got, tt.want) {
				t.Fatalf("User-Agent = %q (set %v), want %q", got, ok, tt.want)
			}
			if !reflect.DeepEqual(req.Headers, src) {
				t.Fatalf("source headers = %v, want %v", req.Headers, src)
			}
		})
	}
}
