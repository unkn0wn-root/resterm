package httpclient

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/httpver"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestPrepareHTTPRequestRejectsHTTP2OverHTTP(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{
		Method:   "GET",
		URL:      "http://example.com",
		Settings: map[string]string{"http-version": "2"},
	}

	_, _, err := c.prepareHTTPRequest(context.Background(), req, nil, Options{})
	if err == nil {
		t.Fatalf("expected error for http-version=2 over http")
	}
	if !strings.Contains(err.Error(), "requires https") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareHTTPRequestAllowsHTTP2OverHTTPS(t *testing.T) {
	c := NewClient(nil)
	req := &restfile.Request{
		Method:   "GET",
		URL:      "https://example.com",
		Settings: map[string]string{"http-version": "2"},
	}

	_, _, err := c.prepareHTTPRequest(context.Background(), req, nil, Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyRequestSettingsIgnoresInvalidHTTPVersion(t *testing.T) {
	opts := Options{HTTPVersion: httpver.V11}

	effective := applyRequestSettings(opts, map[string]string{"http-version": "bogus"})

	if effective.HTTPVersion != httpver.V11 {
		t.Fatalf("expected HTTP version to remain unchanged, got %v", effective.HTTPVersion)
	}
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

	if err := c.applyAuthentication(httpReq, vars.NewResolver(), auth); err != nil {
		t.Fatalf("unexpected error for unused auth param: %v", err)
	}
	if got := httpReq.Header.Get("Authorization"); got != "Bearer explicit" {
		t.Fatalf("authorization header changed: %q", got)
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
