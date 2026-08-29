package telemetry

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func startedSpan(t *testing.T, target string) (*tracetest.SpanRecorder, RequestSpan) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	inst, err := New(Config{ServiceName: "resterm-test"}, WithSpanProcessor(recorder))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = inst.Shutdown(context.Background()) })

	httpReq, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	_, span := inst.Start(context.Background(), RequestStart{
		Request:     &restfile.Request{Method: http.MethodGet, URL: target},
		HTTPRequest: httpReq,
	})
	return recorder, span
}

func assertNoSecrets(t *testing.T, recorder *tracetest.SpanRecorder, secrets ...string) {
	t.Helper()
	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			assertClean(t, string(attr.Key), attr.Value.AsString(), secrets)
		}
		assertClean(t, "status", span.Status().Description, secrets)
		for _, evt := range span.Events() {
			for _, attr := range evt.Attributes {
				assertClean(t, evt.Name+"/"+string(attr.Key), attr.Value.AsString(), secrets)
			}
		}
	}
}

func assertClean(t *testing.T, where, text string, secrets []string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(text, secret) {
			t.Errorf("%s carries %q: %s", where, secret, text)
		}
	}
}

func TestSpanKeepsQuerySecretsOffTheWire(t *testing.T) {
	const target = "https://api.example.com/v1/things?api_key=SECRETKEY&sig=SIGNED&page=2"

	recorder, span := startedSpan(t, target)
	span.End(RequestResult{StatusCode: 200})

	assertNoSecrets(t, recorder, "SECRETKEY", "SIGNED")

	attrs := spanAttributes(t, recorder)
	if got := attrs["http.url"]; got != "https://api.example.com/v1/things?api_key=REDACTED&sig=REDACTED&page=REDACTED" {
		t.Fatalf("http.url = %q", got)
	}
	if got := attrs["http.target"]; got != "/v1/things?api_key=REDACTED&sig=REDACTED&page=REDACTED" {
		t.Fatalf("http.target = %q", got)
	}
}

func TestSpanNeverCarriesURLCredentials(t *testing.T) {
	const target = "https://alice:hunter2@api.example.com/v1/things"

	recorder, span := startedSpan(t, target)
	span.End(RequestResult{StatusCode: 200})

	assertNoSecrets(t, recorder, "alice", "hunter2")
}

func TestSpanScrubsTheErrorItRecords(t *testing.T) {
	const target = "https://alice:hunter2@api.example.com/v1?api_key=SECRETKEY"

	recorder, span := startedSpan(t, target)
	span.End(RequestResult{Err: errors.New(
		`perform request: Get "https://alice:***@api.example.com/v1?api_key=SECRETKEY": dial tcp: refused`,
	)})

	assertNoSecrets(t, recorder, "SECRETKEY", "hunter2")
}

func TestSpanScrubsTraceErrors(t *testing.T) {
	const target = "https://api.example.com/v1?token=SECRETKEY"

	recorder, span := startedSpan(t, target)
	span.RecordTrace(&nettrace.Timeline{
		Err: `Get "https://api.example.com/v1?token=SECRETKEY": timeout`,
		Phases: []nettrace.Phase{{
			Kind: nettrace.PhaseDNS,
			Err:  `lookup for https://api.example.com/v1?token=SECRETKEY failed`,
		}},
	}, nil)
	span.End(RequestResult{StatusCode: 500})

	assertNoSecrets(t, recorder, "SECRETKEY")
}

func TestRedactQuery(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty"},
		{name: "single value", raw: "api_key=secret", want: "api_key=REDACTED"},
		{name: "several values", raw: "a=1&b=2", want: "a=REDACTED&b=REDACTED"},
		{name: "empty value", raw: "a=", want: "a=REDACTED"},
		{name: "bare key carries no value", raw: "verbose", want: "verbose"},
		{name: "mixed", raw: "verbose&token=abc", want: "verbose&token=REDACTED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactQuery(tt.raw); got != tt.want {
				t.Fatalf("redactQuery(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestSanitizeURLDropsFragmentsAndUserinfo(t *testing.T) {
	u, err := url.Parse("https://alice:hunter2@example.com/p?a=1#access_token=SECRET")
	if err != nil {
		t.Fatal(err)
	}
	safe := sanitizeURL(u)
	if safe.full != "https://example.com/p?a=REDACTED" {
		t.Fatalf("full = %q", safe.full)
	}
	if safe.target != "/p?a=REDACTED" {
		t.Fatalf("target = %q", safe.target)
	}
}

func spanAttributes(t *testing.T, recorder *tracetest.SpanRecorder) map[string]string {
	t.Helper()
	out := make(map[string]string)
	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			out[string(attr.Key)] = attr.Value.AsString()
		}
	}
	return out
}

func TestSpanScrubsARedirectedURLItNeverSaw(t *testing.T) {
	recorder, span := startedSpan(t, "https://api.example.com/v1")
	span.End(RequestResult{Err: errors.New(
		`perform request: Get "https://cdn.example.net/blob?X-Amz-Signature=SIGNED&token=SECRETKEY": timeout`,
	)})

	assertNoSecrets(t, recorder, "SIGNED", "SECRETKEY")
}

func TestSpanScrubsRedirectCredentialsInTraceErrors(t *testing.T) {
	recorder, span := startedSpan(t, "https://api.example.com/v1")
	span.RecordTrace(&nettrace.Timeline{
		Err: `dial https://bob:hunter2@evil.example.net/p?sig=SIGNED failed`,
	}, nil)
	span.End(RequestResult{StatusCode: 502})

	assertNoSecrets(t, recorder, "hunter2", "SIGNED", "bob")
}

func TestScrubText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no url", in: "dial tcp: connection refused", want: "dial tcp: connection refused"},
		{
			name: "quoted url",
			in:   `Get "https://h/p?a=1": timeout`,
			want: `Get "https://h/p?a=REDACTED": timeout`,
		},
		{
			name: "trailing colon is not part of the url",
			in:   "fetch https://h/p?a=1: timeout",
			want: "fetch https://h/p?a=REDACTED: timeout",
		},
		{
			name: "userinfo removed",
			in:   "get https://u:p@h/x now",
			want: "get https://h/x now",
		},
		{
			name: "two urls",
			in:   "https://a/x?k=1 redirected to https://b/y?k=2",
			want: "https://a/x?k=REDACTED redirected to https://b/y?k=REDACTED",
		},
		{
			name: "uppercase scheme",
			in:   "HTTPS://h/p?a=1 failed",
			want: "https://h/p?a=REDACTED failed",
		},
		{
			name: "fragment dropped",
			in:   "opened https://h/p#access_token=SECRET ok",
			want: "opened https://h/p ok",
		},
		{
			name: "scheme without a host is left alone",
			in:   "see http:// for details",
			want: "see http:// for details",
		},
		{
			name: "already redacted text is unchanged",
			in:   "https://h/p?a=REDACTED",
			want: "https://h/p?a=REDACTED",
		},
		{
			name: "word ending in http is not a scheme",
			in:   "xhttp://h/p?a=1",
			want: "xhttp://h/p?a=1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scrubText(tt.in); got != tt.want {
				t.Fatalf("scrubText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestScrubTextCoversWebSocketURLs(t *testing.T) {
	in := `dial wss://api.example.com/socket?token=SECRETKEY: handshake failed`
	want := `dial wss://api.example.com/socket?token=REDACTED: handshake failed`
	if got := scrubText(in); got != want {
		t.Fatalf("scrubText() = %q, want %q", got, want)
	}
}

func FuzzScrubText(f *testing.F) {
	for _, seed := range []string{
		"",
		"https://h/p?a=1",
		`Get "https://u:p@h/p?a=1": timeout`,
		"http://",
		"xhttps://h",
		"https://h/p#f",
		"::://",
		"ws://h/s?t=1",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, in string) {
		once := scrubText(in)
		if twice := scrubText(once); twice != once {
			t.Fatalf("scrubText is not settled: %q then %q", once, twice)
		}
	})
}
