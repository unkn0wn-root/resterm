package mock

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestQueryAndJSONOperatorsAcrossRoutingJournalAndExpectations(t *testing.T) {
	handler := compileSource(t, `### Big order from a known channel
# @mock method=POST path=/orders name=priority
# @match query={"page":{"gte":2},"channel":{"oneOf":["web","ios"]},"trace":{"absent":true}} json={"amount":{"$gt":100},"status":{"$oneOf":["new","hold"]}}
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
			name:  "every rule holds",
			query: "page=2&channel=web",
			body:  `{"amount":101,"status":"new"}`,
			want:  "priority",
		},
		{
			name:  "a repeated value only has to match once",
			query: "page=1&page=7&channel=qa&channel=ios",
			body:  `{"amount":1e3,"status":"hold","extra":true}`,
			want:  "priority",
		},
		{
			name:  "a non-numeric query value never satisfies gte",
			query: "page=two&channel=web",
			body:  `{"amount":101,"status":"new"}`,
			want:  "standard",
		},
		{
			name:  "absent fails once the parameter appears",
			query: "page=2&channel=web&trace=1",
			body:  `{"amount":101,"status":"new"}`,
			want:  "standard",
		},
		{
			name:  "a quoted JSON amount is not a number",
			query: "page=2&channel=web",
			body:  `{"amount":"101","status":"new"}`,
			want:  "standard",
		},
		{
			name:  "the status is outside the allowed set",
			query: "page=2&channel=web",
			body:  `{"amount":101,"status":"shipped"}`,
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

	// the journal counts every request the server saw, so a pattern narrower
	// than the mock's own @match block is what isolates the routed ones
	loose := RequestPattern{
		Query: map[string]restfile.MockQueryRule{
			"page":    queryRuleOf(restfile.MockQueryOpGTE, "2"),
			"channel": queryRuleOf(restfile.MockQueryOpOneOf, "web", "ios"),
		},
		JSON: []byte(`{"amount":{"$gt":100}}`),
	}
	count, err := server.Count(context.Background(), loose)
	if err != nil || count != 4 {
		t.Fatalf("loose journal Count() = %d, %v, want 4", count, err)
	}

	tight := loose
	tight.Query = map[string]restfile.MockQueryRule{
		"page":    queryRuleOf(restfile.MockQueryOpGTE, "2"),
		"channel": queryRuleOf(restfile.MockQueryOpOneOf, "web", "ios"),
		"trace":   queryRuleOf(restfile.MockQueryOpAbsent),
	}
	tight.JSON = []byte(`{"amount":{"$gt":100},"status":{"$oneOf":["new","hold"]}}`)
	count, err = server.Count(context.Background(), tight)
	if err != nil || count != 2 {
		t.Fatalf("tight journal Count() = %d, %v, want 2", count, err)
	}
	results := Verify(context.Background(), server, handler.Expectations())
	if len(results) != 1 || !results[0].Passed || results[0].Actual != 2 {
		t.Fatalf("Verify() = %+v", results)
	}
}

func TestInvalidQueryRuleFailsCompilation(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "malformed regex",
			source: `### Bad
# @mock method=GET path=/x
# @match query={"v":{"regex":"^v[0-9"}}
HTTP/1.1 200 OK
`,
			want: "not a valid regular expression",
		},
		{
			name: "quoted number",
			source: `### Bad
# @mock method=GET path=/x
# @match query={"page":{"gt":"10"}}
HTTP/1.1 200 OK
`,
			want: "must be a number, not a quoted string",
		},
		{
			name: "quoted JSON number",
			source: `### Bad
# @mock method=GET path=/x
# @match json={"n":{"$gt":"1"}}
HTTP/1.1 200 OK
`,
			want: "$gt matcher requires a number",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile([]*restfile.Document{parser.Parse("mocks.http", []byte(test.source))})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Compile() error = %v, want %q", err, test.want)
			}
		})
	}
}

// Integers longer than a fixed 256-bit mantissa used to round together, which
// made gt miss, gte match anyway, and $oneOf accept a different number.
func TestNumbersStayDistinctPastFixedPrecision(t *testing.T) {
	for _, digits := range []int{78, maxNumberText - 1} {
		t.Run(strconv.Itoa(digits)+" digits", func(t *testing.T) {
			high := strings.Repeat("9", digits)
			low := high[:digits-1] + "8"

			gt, err := compileQueryRule(queryRuleOf(restfile.MockQueryOpGT, low))
			if err != nil {
				t.Fatal(err)
			}
			if !gt([]string{high}) {
				t.Fatal("gt missed a value one larger than the operand")
			}
			gte, err := compileQueryRule(queryRuleOf(restfile.MockQueryOpGTE, high))
			if err != nil {
				t.Fatal(err)
			}
			if gte([]string{low}) {
				t.Fatal("gte accepted a value smaller than the operand")
			}

			match, err := compileJSONMatcher([]byte(`{"n":{"$gte":` + high + `}}`))
			if err != nil {
				t.Fatal(err)
			}
			body, err := decodeJSON([]byte(`{"n":` + low + `}`))
			if err != nil {
				t.Fatal(err)
			}
			if match(body) {
				t.Fatal("$gte accepted a value smaller than the operand")
			}
			oneOf, err := compileJSONMatcher([]byte(`{"n":{"$oneOf":[` + high + `]}}`))
			if err != nil {
				t.Fatal(err)
			}
			if oneOf(body) {
				t.Fatal("$oneOf accepted a numerically different value")
			}
		})
	}
}

// The same number written at very different lengths has to compare equal. 0.1
// has no exact binary form, so parsing it at two widths gives two values that
// are not equal.
func TestEquivalentNumberFormsCompareEqual(t *testing.T) {
	pairs := []struct{ short, long string }{
		{"0.1", "0.1" + strings.Repeat("0", 300)},
		{"-2.5", "-2.5" + strings.Repeat("0", 500)},
	}
	for _, pair := range pairs {
		t.Run(pair.short, func(t *testing.T) {
			gte, err := compileQueryRule(queryRuleOf(restfile.MockQueryOpGTE, pair.short))
			if err != nil {
				t.Fatal(err)
			}
			lte, err := compileQueryRule(queryRuleOf(restfile.MockQueryOpLTE, pair.short))
			if err != nil {
				t.Fatal(err)
			}
			// only a value equal to the operand satisfies both bounds at once
			if !gte([]string{pair.long}) || !lte([]string{pair.long}) {
				t.Fatalf("query %s is not equal to its padded form", pair.short)
			}
			gt, err := compileQueryRule(queryRuleOf(restfile.MockQueryOpGT, pair.short))
			if err != nil {
				t.Fatal(err)
			}
			if gt([]string{pair.long}) {
				t.Fatalf("query %s compares greater than its padded form", pair.short)
			}

			body, err := decodeJSON([]byte(`{"n":` + pair.long + `}`))
			if err != nil {
				t.Fatal(err)
			}
			for _, pattern := range []string{
				`{"n":{"$gte":` + pair.short + `}}`,
				`{"n":{"$lte":` + pair.short + `}}`,
				`{"n":{"$oneOf":[` + pair.short + `]}}`,
				`{"n":` + pair.short + `}`,
			} {
				match, err := compileJSONMatcher([]byte(pattern))
				if err != nil {
					t.Fatal(err)
				}
				if !match(body) {
					t.Fatalf("%s did not match the padded form of %s", pattern, pair.short)
				}
			}
		})
	}
}

// Past the cap a value stops matching, so a body cannot ask for an unbounded
// mantissa.
func TestNumbersBeyondTheTextCapAreNonMatches(t *testing.T) {
	huge := strings.Repeat("9", maxNumberText+1)
	match, err := compileQueryRule(queryRuleOf(restfile.MockQueryOpGT, "1"))
	if err != nil {
		t.Fatal(err)
	}
	if match([]string{huge}) {
		t.Fatal("a value past the text cap should not match")
	}
	if _, err := compileQueryRule(queryRuleOf(restfile.MockQueryOpGT, huge)); err == nil {
		t.Fatal("an operand past the text cap should fail to compile")
	}
	if _, err := compileJSONMatcher([]byte(`{"n":{"$gt":` + huge + `}}`)); err == nil {
		t.Fatal("a JSON operand past the text cap should fail to compile")
	}
}
