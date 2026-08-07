package mock

import (
	"context"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestJSONRulesMatchBodies(t *testing.T) {
	tests := []struct {
		name  string
		rules string
		body  string
		want  bool
	}{
		{"gt", `{"amount":{"gt":100}}`, `{"amount":101}`, true},
		{"gt at the boundary", `{"amount":{"gt":100}}`, `{"amount":100}`, false},
		{"gte at the boundary", `{"age":{"gte":18}}`, `{"age":18}`, true},
		{"lt", `{"price":{"lt":500}}`, `{"price":499.99}`, true},
		{"lte at the boundary", `{"price":{"lte":500}}`, `{"price":500.0}`, true},
		{"exponent form", `{"n":{"gte":100}}`, `{"n":1e2}`, true},
		{"large integers stay exact", `{"n":{"gt":9007199254740992}}`, `{"n":9007199254740993}`, true},
		{"a tiny value is still above zero", `{"n":{"gt":0}}`, `{"n":1e-999999999}`, true},
		{"a tiny value is not at or below zero", `{"n":{"lte":0}}`, `{"n":1e-999999999}`, false},
		{"negative operand", `{"n":{"lt":-5}}`, `{"n":-6}`, true},

		{"a quoted value is not a number", `{"amount":{"gt":100}}`, `{"amount":"101"}`, false},
		{"a missing field satisfies nothing", `{"amount":{"gt":0}}`, `{"other":1}`, false},
		{"null is not a number", `{"amount":{"gt":0}}`, `{"amount":null}`, false},
		{"an object is not a number", `{"amount":{"gt":0}}`, `{"amount":{"n":1}}`, false},
		{"a non-object body never matches a field", `{"amount":{"gt":0}}`, `5`, false},

		{"several operators at one leaf", `{"amount":{"gte":100,"lt":500}}`, `{"amount":100}`, true},
		{"lt rejects its boundary", `{"amount":{"gte":100,"lt":500}}`, `{"amount":500}`, false},
		{"nested fields", `{"user":{"age":{"gte":18}}}`, `{"user":{"age":21,"name":"x"}}`, true},
		{"nested field misses", `{"user":{"age":{"gte":18}}}`, `{"user":{"age":17}}`, false},
		{"two fields are ANDed", `{"a":{"gt":1},"b":{"lt":9}}`, `{"a":2,"b":8}`, true},
		{"one of two fields misses", `{"a":{"gt":1},"b":{"lt":9}}`, `{"a":2,"b":9}`, false},

		{"oneOf strings", `{"status":{"oneOf":["A","B"]}}`, `{"status":"B"}`, true},
		{"oneOf miss", `{"status":{"oneOf":["A","B"]}}`, `{"status":"C"}`, false},
		{"oneOf compares numbers by value", `{"n":{"oneOf":[1]}}`, `{"n":1.0}`, true},
		{"oneOf null", `{"v":{"oneOf":[null,"x"]}}`, `{"v":null}`, true},
		{"oneOf mixed types", `{"v":{"oneOf":[1,"1",true]}}`, `{"v":"1"}`, true},
		{"oneOf alternatives are whole", `{"v":{"oneOf":[{"a":1}]}}`, `{"v":{"a":1,"b":2}}`, false},
		{"oneOf object hit", `{"v":{"oneOf":[{"a":1}]}}`, `{"v":{"a":1}}`, true},
		{"oneOf arrays are exact", `{"roles":{"oneOf":[["admin"]]}}`, `{"roles":["admin"]}`, true},
		{"oneOf array order matters", `{"roles":{"oneOf":[["a","b"]]}}`, `{"roles":["b","a"]}`, false},
		{"oneOf with a bound", `{"n":{"oneOf":[1,2,3],"gt":1}}`, `{"n":2}`, true},

		{"a field named gt", `{"gt":{"gt":100}}`, `{"gt":125}`, true},
		{"a field named gt misses", `{"gt":{"gt":100}}`, `{"gt":100}`, false},
		{"a nested field named gt", `{"range":{"gt":{"gt":100}}}`, `{"range":{"gt":125}}`, true},
		{"a field named oneOf", `{"oneOf":{"oneOf":["x"]}}`, `{"oneOf":"x"}`, true},

		{"dotted name", `{"a.b":{"gt":1}}`, `{"a.b":2}`, true},
		{"dotted name is not a path", `{"a.b":{"gt":1}}`, `{"a":{"b":2}}`, false},
		{"slashed name", `{"a/b":{"gt":1}}`, `{"a/b":2}`, true},
		{"dollar name", `{"$amount":{"gt":1}}`, `{"$amount":2}`, true},
		{"unicode name", `{"цена":{"gt":1}}`, `{"цена":2}`, true},
		{"empty name", `{"":{"gt":1}}`, `{"":2}`, true},

		{"an array is not traversed", `{"roles":{"oneOf":[["admin","auditor"]]}}`,
			`{"roles":["admin","auditor"]}`, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			match := mustCompileBody(t, "", test.rules)
			if got := match(decodeBody(t, test.body)); got != test.want {
				t.Fatalf("%s against %s = %t, want %t", test.rules, test.body, got, test.want)
			}
		})
	}
}

func TestJSONRulesRejectInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		rules string
		want  string
	}{
		{"malformed JSON", `{"a":`, "invalid json-rules matcher"},
		{"not an object", `[1]`, "invalid json-rules matcher: must be a JSON object"},
		{"a bare scalar", `5`, "invalid json-rules matcher: must be a JSON object"},
		{"empty rule set", `{}`, "invalid json-rules matcher: rule object cannot be empty"},
		{"empty nested node", `{"a":{}}`, "invalid json-rules matcher at a: rule object cannot be empty"},
		{
			"misspelled operator", `{"amount":{"gtt":100}}`,
			"invalid json-rules matcher at amount.gtt: expected a rule object or a known operator",
		},
		{
			"a scalar where a rule belongs", `{"amount":100}`,
			"invalid json-rules matcher at amount: expected a rule object or a known operator",
		},
		{
			"an array where a rule belongs", `{"amount":[1]}`,
			"invalid json-rules matcher at amount: expected a rule object or a known operator",
		},
		{
			"a quoted number operand", `{"amount":{"gt":"100"}}`,
			"invalid json-rules matcher at amount.gt: operand must be a JSON number",
		},
		{
			"a null operand", `{"amount":{"gte":null}}`,
			"invalid json-rules matcher at amount.gte: operand must be a JSON number",
		},
		{
			"a scalar oneOf operand", `{"v":{"oneOf":"x"}}`,
			"invalid json-rules matcher at v.oneOf: operand must be an array",
		},
		{
			"an empty oneOf", `{"v":{"oneOf":[]}}`,
			"invalid json-rules matcher at v.oneOf: operand cannot be an empty array",
		},
		{
			"a rule mixed with a field", `{"gt":1,"user":{"age":{"gt":1}}}`,
			"invalid json-rules matcher at gt: expected a rule object or a known operator",
		},
		{
			"the deepest node names the whole path", `{"a":{"b":{"c":{"lt":"9"}}}}`,
			"invalid json-rules matcher at a.b.c.lt: operand must be a JSON number",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileJSONBody(nil, []byte(test.rules))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compileJSONBody(%s) error = %v, want %q", test.rules, err, test.want)
			}
		})
	}
}

func TestJSONRulesReportTheSameProblemEveryTime(t *testing.T) {
	const rules = `{"zeta":{"gt":"x"},"alpha":{"gt":"x"},"mid":{"gt":"x"}}`
	want := "invalid json-rules matcher at alpha.gt: operand must be a JSON number"
	for range 20 {
		_, err := compileJSONBody(nil, []byte(rules))
		if err == nil || err.Error() != want {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func TestJSONRuleOpsRejectInvalidOperands(t *testing.T) {
	invalid := map[string]string{
		"gt":    `"100"`,
		"gte":   `null`,
		"lt":    `[1]`,
		"lte":   `true`,
		"oneOf": `[]`,
	}
	if !slices.Equal(slices.Sorted(maps.Keys(invalid)), slices.Sorted(maps.Keys(jsonRuleOps))) {
		t.Fatalf("operators = %q, invalid operand cases = %q",
			slices.Sorted(maps.Keys(jsonRuleOps)), slices.Sorted(maps.Keys(invalid)))
	}
	for name, operand := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := compileJSONBody(nil, []byte(`{"x":{"`+name+`":`+operand+`}}`))
			if err == nil {
				t.Fatalf("%s accepted %s", name, operand)
			}
			if !strings.Contains(err.Error(), "at x."+name+":") {
				t.Fatalf("%s error does not name where it failed: %v", name, err)
			}
		})
	}
}

func TestJSONRulesAcrossRoutingJournalAndExpectations(t *testing.T) {
	handler := compileSource(t, `### Big order from a known channel
# @mock method=POST path=/orders name=priority
# @match query={"page":{"gte":2},"channel":{"oneOf":["web","ios"]},"trace":{"absent":true}}
# @match json={"type":"order"} json-rules={"amount":{"gt":100},"status":{"oneOf":["new","hold"]}}
# @expect calls=2
HTTP/1.1 200 OK

priority

### Everything else
# @mock method=POST path=/orders name=standard default=true
HTTP/1.1 200 OK

standard`)
	server, err := Start("127.0.0.1:0", handler, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	tests := []struct {
		name  string
		query string
		body  string
		want  string
	}{
		{
			name:  "all rules match",
			query: "page=2&channel=web",
			body:  `{"type":"order","amount":101,"status":"new"}`,
			want:  "priority",
		},
		{
			name:  "a repeated value only has to match once",
			query: "page=1&page=7&channel=qa&channel=ios",
			body:  `{"type":"order","amount":1e3,"status":"hold","extra":true}`,
			want:  "priority",
		},
		{
			name:  "a non-numeric query value never satisfies gte",
			query: "page=two&channel=web",
			body:  `{"type":"order","amount":101,"status":"new"}`,
			want:  "standard",
		},
		{
			name:  "absent fails once the parameter appears",
			query: "page=2&channel=web&trace=1",
			body:  `{"type":"order","amount":101,"status":"new"}`,
			want:  "standard",
		},
		{
			name:  "the literal body does not match",
			query: "page=2&channel=web",
			body:  `{"type":"refund","amount":101,"status":"new"}`,
			want:  "standard",
		},
		{
			name:  "a quoted JSON amount is not a number",
			query: "page=2&channel=web",
			body:  `{"type":"order","amount":"101","status":"new"}`,
			want:  "standard",
		},
		{
			name:  "the status is outside the allowed set",
			query: "page=2&channel=web",
			body:  `{"type":"order","amount":101,"status":"shipped"}`,
			want:  "standard",
		},
	}

	client := &http.Client{Timeout: 2 * time.Second}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			url := "http://" + server.Addr() + "/orders?" + test.query
			request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(string(body)); got != test.want {
				t.Fatalf("scenario = %q, want %q", got, test.want)
			}
		})
	}

	loose := RequestPattern{
		Query: map[string]restfile.MockQueryRule{
			"page":    queryRuleOf(restfile.MockQueryOpGTE, "2"),
			"channel": queryRuleOf(restfile.MockQueryOpOneOf, "web", "ios"),
		},
		JSONRules: []byte(`{"amount":{"gt":100}}`),
	}
	count, err := server.Count(context.Background(), loose)
	if err != nil || count != 5 {
		t.Fatalf("loose journal Count() = %d, %v, want 5", count, err)
	}

	tight := loose
	tight.Query = map[string]restfile.MockQueryRule{
		"page":    queryRuleOf(restfile.MockQueryOpGTE, "2"),
		"channel": queryRuleOf(restfile.MockQueryOpOneOf, "web", "ios"),
		"trace":   queryRuleOf(restfile.MockQueryOpAbsent),
	}
	tight.JSON = []byte(`{"type":"order"}`)
	tight.JSONRules = []byte(`{"amount":{"gt":100},"status":{"oneOf":["new","hold"]}}`)
	count, err = server.Count(context.Background(), tight)
	if err != nil || count != 2 {
		t.Fatalf("tight journal Count() = %d, %v, want 2", count, err)
	}
	results := Verify(context.Background(), server, handler.Expectations())
	if len(results) != 1 || !results[0].Passed || results[0].Actual != 2 {
		t.Fatalf("Verify() = %+v", results)
	}
}

func TestInvalidJSONRulesFailCompilation(t *testing.T) {
	_, err := Compile([]*restfile.Document{parser.Parse("mocks.http", []byte(`### Bad
# @mock method=GET path=/x
# @match json-rules={"n":{"gtt":1}}
HTTP/1.1 200 OK
`))})
	want := "invalid json-rules matcher at n.gtt"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Compile() error = %v, want %q", err, want)
	}
}

func FuzzCompileJSONRules(f *testing.F) {
	for _, seed := range []string{
		`{"amount":{"gt":100}}`,
		`{"a":{"gte":1,"lt":2}}`,
		`{"v":{"oneOf":[1,"x",null,[1],{"a":1}]}}`,
		`{"gt":{"gt":1}}`,
		`{"a":{"b":{"c":{"lte":0}}}}`,
		`{}`,
		`[]`,
		`{"a":1}`,
	} {
		f.Add(seed)
	}
	bodies := []string{`{}`, `{"amount":101}`, `{"a":{"b":{"c":-1}}}`, `[1,2]`, `null`, `"x"`, `5`, `true`}
	f.Fuzz(func(t *testing.T, rules string) {
		match, err := compileJSONRules([]byte(rules))
		switch {
		case err != nil && match != nil:
			t.Fatalf("compileJSONRules(%s) returned both a predicate and %v", rules, err)
		case err != nil:
			return
		case match == nil:
			t.Fatalf("compileJSONRules(%s) returned no predicate and no error", rules)
		}
		for _, body := range bodies {
			got, err := decodeJSON([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			match(got)
		}
	})
}
