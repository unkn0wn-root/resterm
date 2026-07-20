package parser

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestParseMockBlock(t *testing.T) {
	src := `### Payment accepted
# @mock method=POST path=/payments name=accepted default=true latency=250ms
# @match query={"mode":"test"} headers={"X-Tenant":["acme","west"]} json={"amount":100}
HTTP/1.1 202 Accepted
Content-Type: application/json
Set-Cookie: one=1
Set-Cookie: two=2

{"id":"pay_123","status":"pending"}

### Request
GET https://example.com
`
	doc := Parse("mocks.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}
	if len(doc.Mocks) != 1 {
		t.Fatalf("mocks = %d, want 1", len(doc.Mocks))
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(doc.Requests))
	}
	m := doc.Mocks[0]
	if m.Title != "Payment accepted" || m.Method != "POST" || m.Path != "/payments" {
		t.Fatalf("unexpected mock route: %+v", m)
	}
	if m.Name != "accepted" || !m.Default || m.Latency != 250*time.Millisecond {
		t.Fatalf("unexpected mock options: %+v", m)
	}
	if len(m.Responses) != 1 {
		t.Fatalf("responses = %d, want 1", len(m.Responses))
	}
	resp := m.Responses[0]
	if resp.Status != 202 || resp.Body.Text != "{\"id\":\"pay_123\",\"status\":\"pending\"}" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got := resp.Headers.Values("Set-Cookie"); !reflect.DeepEqual(got, []string{"one=1", "two=2"}) {
		t.Fatalf("set-cookie = %#v", got)
	}
	if got := m.Match.Query["mode"]; !reflect.DeepEqual(got, restfile.StringList{"test"}) {
		t.Fatalf("query matcher = %#v", got)
	}
	if got := m.Match.Headers["X-Tenant"]; got.Op != restfile.MockHeaderOpExact ||
		!reflect.DeepEqual(got.Values, []string{"acme", "west"}) {
		t.Fatalf("header matcher = %#v", got)
	}
	if string(m.Match.JSON) != `{"amount":100}` {
		t.Fatalf("json matcher = %s", m.Match.JSON)
	}
}

func TestParseMockSequenceKeyExpectationAndHeaderRules(t *testing.T) {
	src := `# @mock method=POST path=/payments/{id} sequence=polling sequence-key=path.id
# @expect calls=2
# @match headers={"X-Tenant":{"exact":"acme"},"Authorization":{"prefix":"Bearer "},"X-Request-ID":{"present":true},"X-Debug":{"absent":true}}
HTTP/1.1 503 Service Unavailable

pending
---
HTTP/1.1 200 OK

done`
	doc := Parse("mocks.http", []byte(src))
	if len(doc.Errors) != 0 || len(doc.Mocks) != 1 {
		t.Fatalf("errors=%+v mocks=%d", doc.Errors, len(doc.Mocks))
	}
	mock := doc.Mocks[0]
	if mock.SequenceKey.Source != restfile.MockSequenceKeySourcePath ||
		mock.SequenceKey.Name != "id" || mock.SequenceKey.String() != "path.id" {
		t.Fatalf("sequence key = %+v", mock.SequenceKey)
	}
	if mock.Expectation == nil || mock.Expectation.Calls != 2 || mock.Expectation.Line != 2 {
		t.Fatalf("expectation = %+v", mock.Expectation)
	}
	wantOps := map[string]restfile.MockHeaderOp{
		"X-Tenant":      restfile.MockHeaderOpExact,
		"Authorization": restfile.MockHeaderOpPrefix,
		"X-Request-Id":  restfile.MockHeaderOpPresent,
		"X-Debug":       restfile.MockHeaderOpAbsent,
	}
	for name, want := range wantOps {
		if got := mock.Match.Headers[name].Op; got != want {
			t.Fatalf("header %s op = %v, want %v", name, got, want)
		}
	}
}

func TestParseMockBodyIsIsolated(t *testing.T) {
	src := `### Looks like resterm syntax
# @mock method=GET path=/docs
HTTP/1.1 200 OK
Content-Type: text/plain

POST https://not-a-request.example
# @name not-a-directive
@file not-a-variable
`
	doc := Parse("mocks.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}
	if len(doc.Requests) != 0 || len(doc.Mocks) != 1 {
		t.Fatalf("requests=%d mocks=%d", len(doc.Requests), len(doc.Mocks))
	}
	want := "POST https://not-a-request.example\n# @name not-a-directive\n@file not-a-variable"
	if got := doc.Mocks[0].Responses[0].Body.Text; got != want {
		t.Fatalf("body:\n%q\nwant:\n%q", got, want)
	}
}

func TestParseMockSequence(t *testing.T) {
	src := `# @mock method=GET path=/payments/{id} sequence=polling interpolate=false
HTTP/1.1 503 Service Unavailable
Retry-After: 1

pending

---
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"completed"}
`
	doc := Parse("mocks.http", []byte(src))
	if len(doc.Errors) != 0 || len(doc.Mocks) != 1 {
		t.Fatalf("errors=%+v mocks=%d", doc.Errors, len(doc.Mocks))
	}
	m := doc.Mocks[0]
	if m.Sequence != "polling" || !m.DisableInterpolation || len(m.Responses) != 2 {
		t.Fatalf("sequence mock = %+v", m)
	}
	if first := m.Responses[0]; first.Status != 503 || first.Body.Text != "pending" ||
		first.Headers.Get("Retry-After") != "1" {
		t.Fatalf("first response = %+v", first)
	}
	if second := m.Responses[1]; second.Status != 200 || second.Body.Text != `{"status":"completed"}` {
		t.Fatalf("second response = %+v", second)
	}
}

func TestParseMockResponseDelimiterIsLiteralWithoutSequence(t *testing.T) {
	src := `# @mock method=GET path=/text
HTTP/1.1 200 OK

before
---
after
`
	doc := Parse("mocks.http", []byte(src))
	if len(doc.Errors) != 0 || len(doc.Mocks) != 1 {
		t.Fatalf("errors=%+v mocks=%d", doc.Errors, len(doc.Mocks))
	}
	if got := doc.Mocks[0].Responses[0].Body.Text; got != "before\n---\nafter" {
		t.Fatalf("body = %q", got)
	}
}

func TestParseMockSequenceTrailingDelimiterErrors(t *testing.T) {
	src := `# @mock method=GET path=/x sequence=poll
HTTP/1.1 503 Service Unavailable

pending
---
HTTP/1.1 200 OK

done
---
`
	doc := Parse("mocks.http", []byte(src))
	if len(doc.Errors) != 1 || !strings.Contains(doc.Errors[0].Message, "dangling delimiter") {
		t.Fatalf("errors = %+v, want one dangling delimiter error", doc.Errors)
	}
	if got := doc.Errors[0].Line; got != 9 {
		t.Fatalf("error line = %d, want 9 (the trailing delimiter)", got)
	}
	if got := len(doc.Mocks[0].Responses); got != 2 {
		t.Fatalf("responses = %d, want 2 (no phantom from trailing delimiter)", got)
	}
}

func TestParseMockSequenceDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "name and sequence",
			source: "# @mock method=GET path=/x name=one sequence=two\nHTTP/1.1 200 OK",
			want:   "name and sequence cannot be combined",
		},
		{
			name:   "one response",
			source: "# @mock method=GET path=/x sequence=one\nHTTP/1.1 200 OK",
			want:   "at least two responses",
		},
		{
			name:   "empty sequence",
			source: "# @mock method=GET path=/x sequence=\nHTTP/1.1 200 OK",
			want:   "sequence name cannot be empty",
		},
		{
			name:   "invalid interpolation option",
			source: "# @mock method=GET path=/x interpolate=maybe\nHTTP/1.1 200 OK",
			want:   "interpolate must be true or false",
		},
		{
			name:   "status without code",
			source: "# @mock method=GET path=/x\nHTTP/1.1",
			want:   "invalid mock response status line",
		},
		{
			name:   "key without sequence",
			source: "# @mock method=GET path=/x sequence-key=query.job\nHTTP/1.1 200 OK",
			want:   "sequence-key requires sequence",
		},
		{
			name:   "unknown key source",
			source: "# @mock method=GET path=/x sequence=poll sequence-key=body.id\nHTTP/1.1 503 Nope\n---\nHTTP/1.1 200 OK",
			want:   "source \"body\" is not supported",
		},
		{
			name:   "unknown path key",
			source: "# @mock method=GET path=/x/{id} sequence=poll sequence-key=path.job\nHTTP/1.1 503 Nope\n---\nHTTP/1.1 200 OK",
			want:   "path wildcard \"job\" is not declared",
		},
		{
			name:   "negative expected calls",
			source: "# @mock method=GET path=/x\n# @expect calls=-1\nHTTP/1.1 200 OK",
			want:   "calls must be a non-negative integer",
		},
		{
			name:   "null query matchers",
			source: "# @mock method=GET path=/x\n# @match query=null\nHTTP/1.1 200 OK",
			want:   "expected a JSON object",
		},
		{
			name:   "duplicate expectation",
			source: "# @mock method=GET path=/x\n# @expect calls=1\n# @expect calls=2\nHTTP/1.1 200 OK",
			want:   "@expect is already defined",
		},
		{
			name:   "multiple header operations",
			source: "# @mock method=GET path=/x\n# @match headers={\"X-Test\":{\"present\":true,\"absent\":true}}\nHTTP/1.1 200 OK",
			want:   "must contain exactly one operator",
		},
		{
			name:   "empty header prefix",
			source: "# @mock method=GET path=/x\n# @match headers={\"X-Test\":{\"prefix\":\"\"}}\nHTTP/1.1 200 OK",
			want:   "must be a non-empty string",
		},
		{
			name:   "false header presence",
			source: "# @mock method=GET path=/x\n# @match headers={\"X-Test\":{\"present\":false}}\nHTTP/1.1 200 OK",
			want:   "must be true",
		},
		{
			name:   "null header matcher",
			source: "# @mock method=GET path=/x\n# @match headers={\"X-Test\":null}\nHTTP/1.1 200 OK",
			want:   "cannot be null",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Parse("bad.http", []byte(tt.source))
			if len(doc.Errors) == 0 {
				t.Fatalf("expected %q error", tt.want)
			}
			found := false
			for _, err := range doc.Errors {
				if strings.Contains(err.Message, tt.want) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("errors=%+v, want %q", doc.Errors, tt.want)
			}
		})
	}
}

func TestParseMockDiagnostics(t *testing.T) {
	src := `# @mock method=POST path=/payments status=202 default=maybe
# @match query={"mode":1} json={bad}
	HTTP/2 199 Nope
`
	doc := Parse("bad.http", []byte(src))
	if len(doc.Errors) == 0 || len(doc.Requests) != 0 || len(doc.Mocks) != 1 {
		t.Fatalf("errors=%+v requests=%d mocks=%d", doc.Errors, len(doc.Requests), len(doc.Mocks))
	}
}
