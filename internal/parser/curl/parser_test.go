package curl

import (
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
	if req.Metadata.Auth == nil {
		t.Fatalf("expected auth spec")
	}
	if req.Metadata.Auth.Type != "basic" {
		t.Fatalf("expected basic auth, got %q", req.Metadata.Auth.Type)
	}
	if req.Metadata.Auth.Params["username"] != "user" ||
		req.Metadata.Auth.Params["password"] != "pass" {
		t.Fatalf("unexpected auth params: %#v", req.Metadata.Auth.Params)
	}
	if req.Headers.Get("Authorization") != "" {
		t.Fatalf("expected auth header to be empty")
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

func TestParseCommandMultipartStableBoundary(t *testing.T) {
	cmd := "curl https://example.com -F file=@payload.json -F caption=hello"
	req1, err := ParseCommand(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req2, err := ParseCommand(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ct1 := req1.Headers.Get("Content-Type")
	ct2 := req2.Headers.Get("Content-Type")
	if ct1 == "" || ct2 == "" {
		t.Fatalf("expected content type header to be set")
	}
	if ct1 != ct2 {
		t.Fatalf("expected stable boundary, got %q and %q", ct1, ct2)
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

func TestParseCommandUserAgent(t *testing.T) {
	req, err := ParseCommand("curl https://example.com -A agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Headers.Get("User-Agent") != "agent" {
		t.Fatalf("expected user-agent header, got %q", req.Headers.Get("User-Agent"))
	}
}

func TestParseCommandCookie(t *testing.T) {
	req, err := ParseCommand("curl https://example.com -b a=b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Headers.Get("Cookie") != "a=b" {
		t.Fatalf("expected cookie header, got %q", req.Headers.Get("Cookie"))
	}
}

func TestParseCommandUploadFile(t *testing.T) {
	req, err := ParseCommand("curl https://example.com -T payload.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "PUT" {
		t.Fatalf("expected PUT, got %s", req.Method)
	}
	if req.Body.FilePath != "payload.json" {
		t.Fatalf("expected file body, got %q", req.Body.FilePath)
	}
}

func TestParseCommandSettings(t *testing.T) {
	cmd := "curl https://example.com -k -L -x http://proxy --max-time 2.5 --connect-timeout 3 --max-redirs 7 --retry 2"
	req, err := ParseCommand(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Settings["http-insecure"] != "true" {
		t.Fatalf("expected http-insecure, got %q", req.Settings["http-insecure"])
	}
	if req.Settings["followredirects"] != "true" {
		t.Fatalf("expected followredirects, got %q", req.Settings["followredirects"])
	}
	if req.Settings["proxy"] != "http://proxy" {
		t.Fatalf("expected proxy, got %q", req.Settings["proxy"])
	}
	if req.Settings["timeout"] != "2.5s" {
		t.Fatalf("expected timeout 2.5s, got %q", req.Settings["timeout"])
	}
	if _, ok := req.Settings["connect-timeout"]; ok {
		t.Fatalf("expected connect-timeout to be ignored")
	}
	if _, ok := req.Settings["max-redirs"]; ok {
		t.Fatalf("expected max-redirs to be ignored")
	}
	if _, ok := req.Settings["retry"]; ok {
		t.Fatalf("expected retry to be ignored")
	}
}

func TestSplitTokensAnsiQuote(t *testing.T) {
	tok, err := splitTokens("curl $'foo\\nbar'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tok) != 2 {
		t.Fatalf("unexpected token count: %d", len(tok))
	}
	if tok[1] != "foo\nbar" {
		t.Fatalf("unexpected ansi token: %q", tok[1])
	}
}

func TestSplitTokensAnsiHex(t *testing.T) {
	tok, err := splitTokens("curl $'foo\\x41bar'")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tok) != 2 {
		t.Fatalf("unexpected token count: %d", len(tok))
	}
	if tok[1] != "fooAbar" {
		t.Fatalf("unexpected ansi hex token: %q", tok[1])
	}
}

func TestParseCommandLineContinuation(t *testing.T) {
	cmd := "curl https://example.com \\\n -H 'X-Test: 1'"
	req, err := ParseCommand(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Headers.Get("X-Test") != "1" {
		t.Fatalf("expected header from continuation, got %q", req.Headers.Get("X-Test"))
	}
}

func TestParseCommandGetQuery(t *testing.T) {
	req, err := ParseCommand("curl -G https://example.com -d foo=bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "GET" {
		t.Fatalf("expected GET, got %s", req.Method)
	}
	if req.URL != "https://example.com?foo=bar" {
		t.Fatalf("unexpected url %q", req.URL)
	}
	if req.Body.Text != "" {
		t.Fatalf("expected empty body, got %q", req.Body.Text)
	}
}

func TestParseCommandsNext(t *testing.T) {
	reqs, err := ParseCommands("curl https://a.test --next https://b.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	if reqs[0].URL != "https://a.test" {
		t.Fatalf("unexpected first url %q", reqs[0].URL)
	}
	if reqs[1].URL != "https://b.test" {
		t.Fatalf("unexpected second url %q", reqs[1].URL)
	}
}
