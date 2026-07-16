package parser

import (
	"reflect"
	"testing"
	"time"
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
	if m.Response.Status != 202 || m.Response.Body.Text != "{\"id\":\"pay_123\",\"status\":\"pending\"}" {
		t.Fatalf("unexpected response: %+v", m.Response)
	}
	if got := m.Response.Headers.Values("Set-Cookie"); !reflect.DeepEqual(got, []string{"one=1", "two=2"}) {
		t.Fatalf("set-cookie = %#v", got)
	}
	if got := m.Match.Query["mode"]; !reflect.DeepEqual(got, []string{"test"}) {
		t.Fatalf("query matcher = %#v", got)
	}
	if got := m.Match.Headers["X-Tenant"]; !reflect.DeepEqual(got, []string{"acme", "west"}) {
		t.Fatalf("header matcher = %#v", got)
	}
	if string(m.Match.JSON) != `{"amount":100}` {
		t.Fatalf("json matcher = %s", m.Match.JSON)
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
	if got := doc.Mocks[0].Response.Body.Text; got != want {
		t.Fatalf("body:\n%q\nwant:\n%q", got, want)
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
