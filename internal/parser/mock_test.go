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
# @match json-rules={ "score": { "gte": 10 } }
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
	if got := m.Match.Query["mode"]; got.Op != restfile.MockQueryOpExact ||
		!reflect.DeepEqual(got.Values, []string{"test"}) {
		t.Fatalf("query matcher = %#v", got)
	}
	if got := m.Match.Headers["X-Tenant"]; got.Op != restfile.MockHeaderOpExact ||
		!reflect.DeepEqual(got.Values, []string{"acme", "west"}) {
		t.Fatalf("header matcher = %#v", got)
	}
	if string(m.Match.JSON) != `{"amount":100}` {
		t.Fatalf("json matcher = %s", m.Match.JSON)
	}
	if string(m.Match.JSONRules) != `{"score":{"gte":10}}` {
		t.Fatalf("json-rules matcher = %s", m.Match.JSONRules)
	}
}

func TestParseMockJSONMatchOptions(t *testing.T) {
	tests := []struct {
		name  string
		match string
		want  string
	}{
		{
			name:  "json takes any JSON value",
			match: `json=[1,2]`,
		},
		{
			name:  "json-rules has to be an object",
			match: `json-rules=[1,2]`,
			want:  "invalid @match json-rules: must be a JSON object",
		},
		{
			name:  "json-rules cannot be a scalar",
			match: `json-rules=5`,
			want:  "invalid @match json-rules: must be a JSON object",
		},
		{
			name:  "malformed json-rules",
			match: `json-rules={bad}`,
			want:  "invalid @match json-rules:",
		},
		{
			name:  "a field cannot be declared twice",
			match: "json-rules={\"a\":{\"gt\":1}}\n# @match json-rules={\"a\":{\"lt\":9}}",
			want:  `@match json-rules field "a" is repeated`,
		},
		{
			name:  "only objects can be repeated",
			match: "json=[1,2]\n# @match json={\"b\":2}",
			want:  "@match json can only be repeated when every declaration is a JSON object",
		},
		{
			name:  "a field cannot be repeated inside one declaration",
			match: `json-rules={"amount":{"gt":100},"amount":{"lt":500}}`,
			want:  `invalid @match json-rules: field "amount" is repeated`,
		},
		{
			name:  "a nested field cannot be repeated either",
			match: `json-rules={"user":{"age":{"gt":1},"age":{"lt":9}}}`,
			want:  `invalid @match json-rules: field "age" is repeated`,
		},
		{
			name:  "inside an array counts too",
			match: `json=[{"a":1,"a":2}]`,
			want:  `invalid @match json: field "a" is repeated`,
		},
		{
			name:  "nesting a name under itself is fine",
			match: `json-rules={"a":{"a":{"gt":1}}}`,
		},
		{
			name:  "query names cannot be repeated",
			match: `query={"page":"1","page":"2"}`,
			want:  `invalid @match query: field "page" is repeated`,
		},
		{
			name:  "header names cannot be repeated",
			match: `headers={"X-Env":"a","X-Env":"b"}`,
			want:  `invalid @match headers: field "X-Env" is repeated`,
		},
		{
			name:  "every repeated field is named",
			match: "json-rules={\"a\":{\"gt\":1},\"b\":{\"gt\":1}}\n# @match json-rules={\"b\":{\"lt\":9},\"a\":{\"lt\":9}}",
			want:  `@match json-rules fields "a", "b" are repeated`,
		},
		{
			name:  "repeating the option on one line is not a merge",
			match: `json={"a":1} json={"b":2}`,
			want:  `@match option "json" is repeated`,
		},
		{
			name:  "an unterminated bracket is reported",
			match: "json-rules={\"a\":{\"gt\":1}",
			want:  "@match option is missing a closing bracket",
		},
		{
			name:  "unknown option",
			match: `jsonRules={"a":{"gt":1}}`,
			want:  `unknown @match option "jsonrules"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			doc := Parse("mocks.http", []byte("# @mock method=GET path=/x\n# @match "+test.match+"\nHTTP/1.1 200 OK\n"))
			if test.want == "" {
				if len(doc.Errors) != 0 {
					t.Fatalf("parse errors: %+v", doc.Errors)
				}
				return
			}
			var found bool
			for _, err := range doc.Errors {
				found = found || strings.Contains(err.Message, test.want)
			}
			if !found {
				t.Fatalf("errors = %+v, want %q", doc.Errors, test.want)
			}
		})
	}
}

func TestParseMockJSONMatchSpansLines(t *testing.T) {
	const wantJSON = `{"kind":"personal"}`
	const wantRules = `{"amount":{"gte":100,"lt":500},"status":{"oneOf":["new","hold"]},` +
		`"user":{"age":{"gte":18},"tier":{"oneOf":["gold","silver"]}}}`

	sources := map[string]string{
		"one line": `# @match json={"kind":"personal"}
# @match json-rules={"amount":{"gte":100,"lt":500},"status":{"oneOf":["new","hold"]},"user":{"age":{"gte":18},"tier":{"oneOf":["gold","silver"]}}}`,

		"indented": `# @match json={"kind":"personal"}
# @match json-rules={
#   "amount": {"gte": 100, "lt": 500},
#   "status": {"oneOf": ["new", "hold"]},
#   "user": {
#     "age":  {"gte": 18},
#     "tier": {"oneOf": ["gold", "silver"]}
#   }
# }`,

		"repeated": `# @match json={"kind":"personal"}
# @match json-rules={"user":{"age":{"gte":18},"tier":{"oneOf":["gold","silver"]}}}
# @match json-rules={"status":{"oneOf":["new","hold"]}}
# @match json-rules={"amount":{"gte":100,"lt":500}}`,

		"indented and repeated": `# @match json={
#   "kind": "personal"
# }
# @match json-rules={
#   "amount": {"gte": 100, "lt": 500}
# }
# @match json-rules={
#   "status": {"oneOf": ["new", "hold"]},
#   "user":   {"age": {"gte": 18}, "tier": {"oneOf": ["gold", "silver"]}}
# }`,

		"beside other options": `# @match json={"kind":"personal"} query={"page":"2"}
# @match json-rules={
#   "amount": {"gte": 100, "lt": 500}
# } headers={"X-Env":"prod"}
# @match json-rules={"status":{"oneOf":["new","hold"]},"user":{"age":{"gte":18},"tier":{"oneOf":["gold","silver"]}}}`,
	}

	for name, match := range sources {
		t.Run(name, func(t *testing.T) {
			doc := Parse("mocks.http", []byte("# @mock method=POST path=/accounts\n"+match+"\nHTTP/1.1 200 OK\n"))
			if len(doc.Errors) != 0 {
				t.Fatalf("parse errors: %+v", doc.Errors)
			}
			m := doc.Mocks[0].Match
			if string(m.JSON) != wantJSON {
				t.Fatalf("json = %s, want %s", m.JSON, wantJSON)
			}
			if string(m.JSONRules) != wantRules {
				t.Fatalf("json-rules = %s, want %s", m.JSONRules, wantRules)
			}
		})
	}
}

func TestParseMockJSONMatchKeepsMarkupCharacters(t *testing.T) {
	const src = `# @mock method=POST path=/accounts
# @match json={"note":"a<b & c>d"}
# @match json-rules={"tag":{"eq":"<b>"}}
# @match json-rules={"other":{"gt":1}}
HTTP/1.1 200 OK
`
	doc := Parse("mocks.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}
	m := doc.Mocks[0].Match
	if want := `{"note":"a<b & c>d"}`; string(m.JSON) != want {
		t.Fatalf("json = %s, want %s", m.JSON, want)
	}
	if want := `{"other":{"gt":1},"tag":{"eq":"<b>"}}`; string(m.JSONRules) != want {
		t.Fatalf("json-rules = %s, want %s", m.JSONRules, want)
	}
}

func TestParseMockSequenceKeyExpectationAndHeaderRules(t *testing.T) {
	src := `# @mock method=POST path=/payments/{id} sequence=polling sequence-key=path.id
# @expect calls=2
# @match headers={"X-Tenant":{"exact":"acme"},"Authorization":{"prefix":"Bearer "},"X-Request-ID":{"present":true},"X-Debug":{"absent":true},"User-Agent":{"contains":"Chrome"},"X-Version":{"regex":"^v[0-9]+$"},"X-Env":{"oneOf":["dev","prod"]}}
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
		"User-Agent":    restfile.MockHeaderOpContains,
		"X-Version":     restfile.MockHeaderOpRegex,
		"X-Env":         restfile.MockHeaderOpOneOf,
	}
	if len(mock.Match.Headers) != len(wantOps) {
		t.Fatalf("headers = %+v", mock.Match.Headers)
	}
	for name, want := range wantOps {
		if got := mock.Match.Headers[name].Op; got != want {
			t.Fatalf("header %s op = %v, want %v", name, got, want)
		}
	}
	if got := mock.Match.Headers["X-Env"].Values; !reflect.DeepEqual(got, []string{"dev", "prod"}) {
		t.Fatalf("oneOf values = %#v", got)
	}
}

// TestParseMockRulesReportsTheSameRuleEveryTime pins the diagnostic for a block
// with several broken rules. Map order would otherwise pick a different one on
// each parse, and the editor reparses on every keystroke.
func TestParseMockRulesReportsTheSameRuleEveryTime(t *testing.T) {
	sources := map[string]string{
		"query": "# @mock method=GET path=/x\n" +
			`# @match query={"ccc":{"oneOf":[]},"aaa":{"gt":"1"},"bbb":{"prefix":""}}` + "\nHTTP/1.1 200 OK",
		"headers": "# @mock method=GET path=/x\n" +
			`# @match headers={"X-C":{"oneOf":[]},"X-A":{"regex":"^v[0-9"},"X-B":{"prefix":""}}` +
			"\nHTTP/1.1 200 OK",
	}
	for kind, source := range sources {
		t.Run(kind, func(t *testing.T) {
			first := Parse("bad.http", []byte(source)).Errors
			if len(first) == 0 {
				t.Fatal("expected a diagnostic")
			}
			for range 50 {
				got := Parse("bad.http", []byte(source)).Errors
				if len(got) != len(first) || got[0].Message != first[0].Message {
					t.Fatalf("diagnostic varies between parses:\n  %s\n  %s", first[0].Message, got[0].Message)
				}
			}
			// sorted order means the alphabetically first broken key wins
			if !strings.Contains(first[0].Message, "aaa") && !strings.Contains(first[0].Message, "X-A") {
				t.Fatalf("diagnostic = %q, want the first key in sorted order", first[0].Message)
			}
		})
	}
}

func TestParseMockQueryRules(t *testing.T) {
	src := `# @mock method=GET path=/orders
# @match query={"mode":"live","tags":["a","b"],"job":{"prefix":"pay_"},"trace":{"present":true},"debug":{"absent":true},"note":{"contains":"abc"},"v":{"regex":"^v[0-9]+$"},"env":{"oneOf":["dev","prod"]},"page":{"gt":10},"size":{"gte":1},"skip":{"lt":100},"limit":{"lte":50}}
HTTP/1.1 200 OK

ok`
	doc := Parse("mocks.http", []byte(src))
	if len(doc.Errors) != 0 || len(doc.Mocks) != 1 {
		t.Fatalf("errors=%+v mocks=%d", doc.Errors, len(doc.Mocks))
	}
	wantOps := map[string]restfile.MockQueryOp{
		"mode":  restfile.MockQueryOpExact,
		"tags":  restfile.MockQueryOpExact,
		"job":   restfile.MockQueryOpPrefix,
		"trace": restfile.MockQueryOpPresent,
		"debug": restfile.MockQueryOpAbsent,
		"note":  restfile.MockQueryOpContains,
		"v":     restfile.MockQueryOpRegex,
		"env":   restfile.MockQueryOpOneOf,
		"page":  restfile.MockQueryOpGT,
		"size":  restfile.MockQueryOpGTE,
		"skip":  restfile.MockQueryOpLT,
		"limit": restfile.MockQueryOpLTE,
	}
	query := doc.Mocks[0].Match.Query
	if len(query) != len(wantOps) {
		t.Fatalf("query = %+v", query)
	}
	for name, want := range wantOps {
		if got := query[name].Op; got != want {
			t.Fatalf("query %s op = %v, want %v", name, got, want)
		}
	}
	if got := query["page"].Values; !reflect.DeepEqual(got, []string{"10"}) {
		t.Fatalf("gt operand = %#v", got)
	}
	if got := query["tags"].Values; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("exact array operand = %#v", got)
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
			want:   "prefix matcher requires one non-empty value",
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
		{
			name:   "unknown header operator",
			source: "# @mock method=GET path=/x\n# @match headers={\"X-Test\":{\"matches\":\"v1\"}}\nHTTP/1.1 200 OK",
			want:   "unknown matcher operator \"matches\"",
		},
		{
			name:   "empty header contains",
			source: "# @mock method=GET path=/x\n# @match headers={\"X-Test\":{\"contains\":\"\"}}\nHTTP/1.1 200 OK",
			want:   "contains matcher requires one non-empty value",
		},
		{
			name:   "header contains wrong type",
			source: "# @mock method=GET path=/x\n# @match headers={\"X-Test\":{\"contains\":[\"a\"]}}\nHTTP/1.1 200 OK",
			want:   "contains matcher must be a non-empty string",
		},
		{
			name:   "malformed header regex",
			source: "# @mock method=GET path=/x\n# @match headers={\"X-Test\":{\"regex\":\"^v[0-9\"}}\nHTTP/1.1 200 OK",
			want:   "regex matcher is not a valid regular expression",
		},
		{
			name:   "empty header oneOf",
			source: "# @mock method=GET path=/x\n# @match headers={\"X-Test\":{\"oneOf\":[]}}\nHTTP/1.1 200 OK",
			want:   "oneOf matcher requires at least one value",
		},
		{
			name:   "header oneOf rejects a scalar",
			source: "# @mock method=GET path=/x\n# @match headers={\"X-Test\":{\"oneOf\":\"dev\"}}\nHTTP/1.1 200 OK",
			want:   "oneOf matcher must be a non-empty string array",
		},
		{
			name:   "null query matcher",
			source: "# @mock method=GET path=/x\n# @match query={\"page\":null}\nHTTP/1.1 200 OK",
			want:   "cannot be null",
		},
		{
			name:   "quoted query number",
			source: "# @mock method=GET path=/x\n# @match query={\"page\":{\"gte\":\"10\"}}\nHTTP/1.1 200 OK",
			want:   "gte matcher must be a number, not a quoted string",
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
