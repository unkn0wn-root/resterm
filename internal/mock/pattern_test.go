package mock

import (
	"context"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestExpectationsShareNothingWithTheHandler(t *testing.T) {
	handler := compileSource(t, `# @mock method=POST path=/orders
# @match query={"kind":{"oneOf":["web","ios"]}} headers={"X-Env":"prod"} json={"status":"new"}
# @expect calls=1
HTTP/1.1 204 No Content`)

	got := handler.Expectations()
	if len(got) != 1 {
		t.Fatalf("expectations = %d, want 1", len(got))
	}
	p := got[0].Pattern
	if len(p.Query) == 0 || len(p.Headers) == 0 || len(p.JSON) == 0 {
		t.Fatalf("pattern = %+v, want the declared matchers", p)
	}

	p.Query["kind"].Values[0] = "clobbered"
	p.Query["injected"] = restfile.MockQueryRule{Op: restfile.MockOpPresent}
	p.Headers["X-Env"].Values[0] = "clobbered"
	p.JSON[0] = 'x'
	p.JSONRules = append(p.JSONRules, 'x')

	again := handler.Expectations()[0].Pattern
	if v := again.Query["kind"].Values[0]; v != "web" {
		t.Errorf("query operand = %q, want the declared value", v)
	}
	if _, ok := again.Query["injected"]; ok {
		t.Error("a key added to the returned map reached the handler")
	}
	if v := again.Headers["X-Env"].Values[0]; v != "prod" {
		t.Errorf("header operand = %q, want the declared value", v)
	}
	if !strings.HasPrefix(string(again.JSON), `{"status"`) {
		t.Errorf("json matcher = %s, want the declared body", again.JSON)
	}
}

func TestRequestPatternRejectsRepeatedJSONFields(t *testing.T) {
	handler := compileSource(t, `# @mock method=POST path=/orders
HTTP/1.1 204 No Content`)
	server, err := Start("127.0.0.1:0", handler, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	tests := []struct {
		name    string
		pattern RequestPattern
		want    string
	}{
		{
			name:    "json",
			pattern: RequestPattern{JSON: []byte(`{"status":"new","status":"done"}`)},
			want:    `invalid json matcher: field "status" is repeated`,
		},
		{
			name:    "json nested",
			pattern: RequestPattern{JSON: []byte(`{"order":{"id":1,"id":2}}`)},
			want:    `invalid json matcher: field "id" is repeated`,
		},
		{
			name:    "json rules",
			pattern: RequestPattern{JSONRules: []byte(`{"amount":{"gt":1},"amount":{"lt":9}}`)},
			want:    `invalid json-rules matcher: field "amount" is repeated`,
		},
		{
			name:    "json past a number too large for a float",
			pattern: RequestPattern{JSON: []byte(`{"n":1e1000,"status":"new","status":"done"}`)},
			want:    `invalid json matcher: field "status" is repeated`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.Count(context.Background(), tt.pattern)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Count() error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}
