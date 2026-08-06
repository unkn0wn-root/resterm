package mock

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

type recordingInspector struct {
	pattern RequestPattern
	count   uint64
}

func (i *recordingInspector) Count(_ context.Context, pattern RequestPattern) (uint64, error) {
	i.pattern = pattern
	return i.count, nil
}

type unavailableInspector struct{}

func (unavailableInspector) Count(context.Context, RequestPattern) (uint64, error) {
	return 0, ErrInspectorUnavailable
}

func TestRTSInspectorCountAndReceived(t *testing.T) {
	inspector := &recordingInspector{count: 3}
	engine := rts.NewEng(nil)
	runtime := rts.RT{Extra: map[string]rts.Value{"mock": RTSValue(inspector)}}
	value, err := engine.Eval(context.Background(), runtime, `mock.count({
  method: "POST",
  path: "/webhooks/{id}",
  query: {kind: "payment", page: {gte: 10}},
  headers: {
    Authorization: {prefix: "Bearer "},
    "X-Trace": {present: true},
    "X-Debug": {absent: true},
    "User-Agent": {contains: "Chrome"},
    "X-Version": {regex: "^v[0-9]+$"},
    "X-Env": {oneOf: ["dev", "prod"]}
  },
  json: {status: "completed", amount: {"$gt": 100}}
})`, rts.Pos{Path: "test.http", Line: 1, Col: 1})
	if err != nil {
		t.Fatal(err)
	}
	if value.K != rts.VNum || value.N != 3 {
		t.Fatalf("mock.count value = %+v", value)
	}
	if inspector.pattern.Method != http.MethodPost || inspector.pattern.Path != "/webhooks/{id}" {
		t.Fatalf("pattern = %+v", inspector.pattern)
	}
	if got := inspector.pattern.Query["kind"]; got.Op != restfile.MockQueryOpExact ||
		!slices.Equal(got.Values, []string{"payment"}) {
		t.Fatalf("query rule = %+v", got)
	}
	if got := inspector.pattern.Query["page"]; got.Op != restfile.MockQueryOpGTE ||
		!slices.Equal(got.Values, []string{"10"}) {
		t.Fatalf("page rule = %+v", got)
	}
	if got := inspector.pattern.Headers["Authorization"]; got.Op != restfile.MockHeaderOpPrefix ||
		got.Values[0] != "Bearer " {
		t.Fatalf("Authorization rule = %+v", got)
	}
	if got := inspector.pattern.Headers["X-Debug"]; got.Op != restfile.MockHeaderOpAbsent {
		t.Fatalf("X-Debug rule = %+v", got)
	}
	// scripts, the control API, and .http files share one JSON schema, so every
	// operator has to decode the same way from an RTS dict
	if got := inspector.pattern.Headers["User-Agent"]; got.Op != restfile.MockHeaderOpContains ||
		got.Values[0] != "Chrome" {
		t.Fatalf("User-Agent rule = %+v", got)
	}
	if got := inspector.pattern.Headers["X-Version"]; got.Op != restfile.MockHeaderOpRegex ||
		got.Values[0] != "^v[0-9]+$" {
		t.Fatalf("X-Version rule = %+v", got)
	}
	if got := inspector.pattern.Headers["X-Env"]; got.Op != restfile.MockHeaderOpOneOf ||
		!slices.Equal(got.Values, []string{"dev", "prod"}) {
		t.Fatalf("X-Env rule = %+v", got)
	}
	// $ does not start an RTS identifier, so an operator key has to be quoted
	if string(inspector.pattern.JSON) != `{"amount":{"$gt":100},"status":"completed"}` {
		t.Fatalf("JSON pattern = %s", inspector.pattern.JSON)
	}
	if _, err := compileRequestPattern(inspector.pattern); err != nil {
		t.Fatalf("scripted pattern does not compile: %v", err)
	}

	inspector.count = 0
	value, err = engine.Eval(
		context.Background(),
		runtime,
		`mock.received({method: "GET"})`,
		rts.Pos{Line: 1, Col: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if value.K != rts.VBool || value.B {
		t.Fatalf("mock.received value = %+v", value)
	}
	if inspector.pattern.Method != http.MethodGet {
		t.Fatalf("received pattern method = %q", inspector.pattern.Method)
	}
}

func TestRTSInspectorReportsUnavailableAndInvalidPatterns(t *testing.T) {
	engine := rts.NewEng(nil)
	for _, test := range []struct {
		name string
		rt   rts.RT
		expr string
	}{
		{
			name: "unavailable",
			rt:   rts.RT{Extra: map[string]rts.Value{"mock": RTSValue(unavailableInspector{})}},
			expr: `mock.count({})`,
		},
		{
			name: "invalid header rule",
			rt: rts.RT{Extra: map[string]rts.Value{
				"mock": RTSValue(&recordingInspector{}),
			}},
			expr: `mock.count({headers: {Authorization: {prefix: ""}}})`,
		},
		{
			name: "unknown field",
			rt: rts.RT{Extra: map[string]rts.Value{
				"mock": RTSValue(&recordingInspector{}),
			}},
			expr: `mock.count({verb: "GET"})`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := engine.Eval(context.Background(), test.rt, test.expr, rts.Pos{}); err == nil {
				t.Fatal("expected RTS error")
			}
		})
	}

	if _, err := requestPatternFromValue(rts.Str("pattern")); err == nil {
		t.Fatal("expected non-dict pattern to be rejected")
	}
	value := rts.Dict(map[string]rts.Value{"json": {K: rts.VNative}})
	if _, err := requestPatternFromValue(value); err == nil {
		t.Fatal("expected non-JSON RTS value to be rejected")
	}
}
