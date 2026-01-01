package curl

import (
	"encoding/base64"
	"strings"
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

func TestParseCommandDataBinaryFile(t *testing.T) {
	req, err := ParseCommand("curl https://example.com --data-binary @payload.bin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Body.FilePath != "payload.bin" {
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

func TestParseCommandPromptPrefix(t *testing.T) {
	req, err := ParseCommand("$ curl https://api.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.URL != "https://api.example.com" {
		t.Fatalf("unexpected url %q", req.URL)
	}
}

func TestParseCommandSudoHead(t *testing.T) {
	req, err := ParseCommand("sudo curl -I https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "HEAD" {
		t.Fatalf("expected HEAD, got %s", req.Method)
	}
}

func TestParseCommandFormEncoded(t *testing.T) {
	req, err := ParseCommand("curl https://example.com --data foo=bar --data baz=qux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "POST" {
		t.Fatalf("expected POST, got %s", req.Method)
	}
	if req.Headers.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("expected form urlencoded header, got %q", req.Headers.Get("Content-Type"))
	}
	if req.Body.Text != "foo=bar&baz=qux" {
		t.Fatalf("unexpected form body %q", req.Body.Text)
	}
}

func TestParseCommandDataUrlencode(t *testing.T) {
	req, err := ParseCommand("curl https://example.com --data-urlencode 'note=hello world'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Headers.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("expected form urlencoded header, got %q", req.Headers.Get("Content-Type"))
	}
	if req.Body.Text != "note=hello+world" {
		t.Fatalf("unexpected urlencode body %q", req.Body.Text)
	}
}

func TestParseCommandMultipart(t *testing.T) {
	req, err := ParseCommand("curl https://example.com -F file=@payload.json -F caption=hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ct := req.Headers.Get("Content-Type")
	if !strings.HasPrefix(ct, "multipart/form-data; boundary=") {
		t.Fatalf("expected multipart content type, got %q", ct)
	}
	boundary := strings.TrimPrefix(ct, "multipart/form-data; boundary=")
	if boundary == "" {
		t.Fatalf("boundary not set")
	}
	body := req.Body.Text
	if !strings.Contains(body, "@payload.json") {
		t.Fatalf("expected file placeholder in body: %q", body)
	}
	if !strings.Contains(body, "Content-Disposition: form-data; name=\"caption\"") {
		t.Fatalf("expected caption part in body: %q", body)
	}
	if !strings.Contains(body, "hello") {
		t.Fatalf("expected caption value in body: %q", body)
	}
}

func TestParseCommandMultilineJSON(t *testing.T) {
	cmd := "curl https://example.com -d '{\n  \"foo\": \"bar\"\n}'"
	req, err := ParseCommand(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "POST" {
		t.Fatalf("expected POST, got %s", req.Method)
	}
	if !strings.Contains(req.Body.Text, "\n  \"foo\": \"bar\"\n") {
		t.Fatalf("expected multiline body, got %q", req.Body.Text)
	}
}

func TestParseCommandDataRawSegments(t *testing.T) {
	req, err := ParseCommand("curl https://example.com --data-raw alpha --data-raw beta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Body.Text != "alpha\nbeta" {
		t.Fatalf("unexpected raw body %q", req.Body.Text)
	}
}

func TestParseCommandJsonShortcut(t *testing.T) {
	req, err := ParseCommand("curl https://example.com --json '{\"ok\":true}'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Headers.Get("Content-Type") != "application/json" {
		t.Fatalf("expected json content type, got %q", req.Headers.Get("Content-Type"))
	}
	if req.Body.Text != "{\"ok\":true}" {
		t.Fatalf("unexpected json body %q", req.Body.Text)
	}
}
