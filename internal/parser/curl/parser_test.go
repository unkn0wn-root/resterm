package curl

import (
	"encoding/base64"
	"testing"
)

func TestParseCommandSimpleGET(t *testing.T) {
	req, err := ParseCommand("curl https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "GET" {
		t.Fatalf("expected GET, got %s", req.Method)
	}
	if req.URL != "https://example.com" {
		t.Fatalf("unexpected url %q", req.URL)
	}
}

func TestParseCommandWithHeadersAndBody(t *testing.T) {
	cmd := "curl -X POST https://api.example.com/users -H 'Content-Type: application/json' --data '{\"name\":\"Sam\"}'"
	req, err := ParseCommand(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "POST" {
		t.Fatalf("expected POST, got %s", req.Method)
	}
	header := req.Headers.Get("Content-Type")
	if header != "application/json" {
		t.Fatalf("expected json content type, got %q", header)
	}
	if req.Body.Text != "{\"name\":\"Sam\"}" {
		t.Fatalf("unexpected body %q", req.Body.Text)
	}
}

func TestParseCommandImplicitPost(t *testing.T) {
	req, err := ParseCommand("curl https://example.com --data foo=bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "POST" {
		t.Fatalf("expected POST fallback when data provided, got %s", req.Method)
	}
}

func TestParseCommandBasicAuth(t *testing.T) {
	req, err := ParseCommand("curl https://example.com -u user:pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := req.Headers.Get("Authorization")
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	if got != expected {
		t.Fatalf("expected basic auth header %q, got %q", expected, got)
	}
}

func TestParseCommandDataFile(t *testing.T) {
	req, err := ParseCommand("curl https://example.com --data @payload.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Body.FilePath != "payload.json" {
		t.Fatalf("expected file body, got %q", req.Body.FilePath)
	}
}

func TestParseCommandCompressedAddsHeader(t *testing.T) {
	req, err := ParseCommand("curl --compressed https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Headers.Get("Accept-Encoding") == "" {
		t.Fatalf("expected accept-encoding header to be set")
	}
}
