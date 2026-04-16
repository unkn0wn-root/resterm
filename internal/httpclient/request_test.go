package httpclient

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/httpver"
	"github.com/unkn0wn-root/resterm/internal/restfile"
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
