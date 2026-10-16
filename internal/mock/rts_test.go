package mock

import (
	"context"
	"math"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rtshost"
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
	engine := rtshost.NewEngine(nil)
	runtime := rtshost.Runtime{Extensions: rts.Extension("mock", RTSValue(inspector))}
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
  json: {status: "completed"},
  jsonRules: {amount: {gt: 100}, tier: {oneOf: ["gold", "silver"]}}
})`, rts.NewFile("test.http", nil).Pos(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if value.K != rts.VNum || value.N != 3 {
		t.Fatalf("mock.count value = %+v", value)
	}
	if inspector.pattern.Method != http.MethodPost || inspector.pattern.Path != "/webhooks/{id}" {
		t.Fatalf("pattern = %+v", inspector.pattern)
	}
	if got := inspector.pattern.Query["kind"]; got.Op != restfile.MockOpExact ||
		!slices.Equal(got.Values, []string{"payment"}) {
		t.Fatalf("query rule = %+v", got)
	}
	if got := inspector.pattern.Query["page"]; got.Op != restfile.MockOpGTE ||
		!slices.Equal(got.Values, []string{"10"}) {
		t.Fatalf("page rule = %+v", got)
	}
	if got := inspector.pattern.Headers["Authorization"]; got.Op != restfile.MockOpPrefix ||
		got.Values[0] != "Bearer " {
		t.Fatalf("Authorization rule = %+v", got)
	}
	if got := inspector.pattern.Headers["X-Debug"]; got.Op != restfile.MockOpAbsent {
		t.Fatalf("X-Debug rule = %+v", got)
	}
	// scripts, the control API, and .http files share one JSON schema, so every
	// operator has to decode the same way from an RTS dict
	if got := inspector.pattern.Headers["User-Agent"]; got.Op != restfile.MockOpContains ||
		got.Values[0] != "Chrome" {
		t.Fatalf("User-Agent rule = %+v", got)
	}
	if got := inspector.pattern.Headers["X-Version"]; got.Op != restfile.MockOpRegex ||
		got.Values[0] != "^v[0-9]+$" {
		t.Fatalf("X-Version rule = %+v", got)
	}
	if got := inspector.pattern.Headers["X-Env"]; got.Op != restfile.MockOpOneOf ||
		!slices.Equal(got.Values, []string{"dev", "prod"}) {
		t.Fatalf("X-Env rule = %+v", got)
	}
	if string(inspector.pattern.JSON) != `{"status":"completed"}` {
		t.Fatalf("JSON pattern = %s", inspector.pattern.JSON)
	}
	if got := string(inspector.pattern.JSONRules); got != `{"amount":{"gt":100},"tier":{"oneOf":["gold","silver"]}}` {
		t.Fatalf("JSON rules pattern = %s", got)
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
	engine := rtshost.NewEngine(nil)
	for _, test := range []struct {
		name      string
		inspector Inspector
		expr      string
		want      string
	}{
		{
			name:      "unavailable",
			inspector: unavailableInspector{},
			expr:      `mock.count({})`,
			want:      "mock request inspector is unavailable",
		},
		{
			name:      "invalid header rule",
			inspector: &recordingInspector{},
			expr:      `mock.count({headers: {Authorization: {prefix: ""}}})`,
			want:      "prefix matcher requires one non-empty value",
		},
		{
			name:      "unknown field",
			inspector: &recordingInspector{},
			expr:      `mock.count({verb: "GET"})`,
			want:      `json: unknown field "verb"`,
		},
		{
			name:      "not a dict",
			inspector: &recordingInspector{},
			expr:      `mock.count("pattern")`,
			want:      "pattern must be a dict",
		},
		{
			name:      "function",
			inspector: &recordingInspector{},
			expr:      `mock.count({json: f})`,
			want:      "cannot convert function to a data value",
		},
		{
			name:      "native",
			inspector: &recordingInspector{},
			expr:      `mock.count({json: [mock.count]})`,
			want:      "cannot convert native to a data value",
		},
		{
			name:      "result",
			inspector: &recordingInspector{},
			expr:      `mock.count({json: {a: try 1}})`,
			want:      "cannot convert result to a data value",
		},
		{
			name:      "object",
			inspector: &recordingInspector{},
			expr:      `mock.count({json: mock})`,
			want:      "cannot convert mock to a data value",
		},
		{
			name:      "NaN",
			inspector: &recordingInspector{},
			expr:      `mock.count({json: [nan]})`,
			want:      "pattern numbers must be finite",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			rt := rtshost.Runtime{Extensions: rts.NewExtensions(map[string]rts.Value{
				"mock": RTSValue(test.inspector),
				"f":    rts.Fn(&rts.Func{Name: "f"}),
				"nan":  rts.Num(math.NaN()),
			})}
			_, err := engine.Eval(context.Background(), rt, test.expr, rts.Pos{})
			if err == nil || !strings.Contains(err.Error(), "mock.count(pattern): "+test.want) {
				t.Fatalf("%s error = %v, want %q", test.expr, err, test.want)
			}
		})
	}
}

func TestRTSInspectorKeepsPatternJSON(t *testing.T) {
	inspector := &recordingInspector{}
	doc := rts.Dict(map[string]rts.Value{
		"text": rts.Str("<&> é \u2028"),
		"nums": rts.List([]rts.Value{rts.Num(2.5), rts.Num(-0.5), rts.Num(1e21), rts.Num(1e-7)}),
	})
	rt := rtshost.Runtime{Extensions: rts.NewExtensions(map[string]rts.Value{"mock": RTSValue(inspector), "doc": doc})}
	if _, err := rtshost.NewEngine(nil).Eval(t.Context(), rt, "mock.count({json: doc})", rts.Pos{}); err != nil {
		t.Fatal(err)
	}
	want := `{"nums":[2.5,-0.5,1e+21,1e-7],"text":"\u003c\u0026\u003e é \u2028"}`
	if got := string(inspector.pattern.JSON); got != want {
		t.Fatalf("JSON pattern = %s, want %s", got, want)
	}
}

// A full expansion would produce 2^27 leaves; the size limit must stop it first.
func TestRTSInspectorRejectsLargePatterns(t *testing.T) {
	v := rts.List([]rts.Value{rts.Num(0)})
	for range 27 {
		v = rts.List([]rts.Value{v, v})
	}
	rt := rtshost.Runtime{Extensions: rts.NewExtensions(map[string]rts.Value{
		"mock": RTSValue(&recordingInspector{}),
		"v":    v,
	})}
	_, err := rtshost.NewEngine(nil).Eval(t.Context(), rt, "mock.count({json: v})", rts.Pos{})
	if err == nil || !strings.Contains(err.Error(), "mock.count(pattern): pattern too large") {
		t.Fatalf("mock.count error = %v, want pattern too large", err)
	}

	const text = `{"path":"/x"}`
	inspector := &recordingInspector{}
	o := &rtsInspector{inspector: inspector}
	args := []rts.Value{rts.Dict(map[string]rts.Value{"path": rts.Str("/x")})}
	ctx := rts.NewCtx(t.Context(), rts.Limits{MaxStr: len(text)})
	if _, err := o.count(ctx, rts.Pos{}, args); err != nil || inspector.pattern.Path != "/x" {
		t.Fatalf("pattern at the limit = %+v, %v, want path /x", inspector.pattern, err)
	}
	ctx = rts.NewCtx(t.Context(), rts.Limits{MaxStr: len(text) - 1})
	_, err = o.count(ctx, rts.Pos{}, args)
	if err == nil || !strings.Contains(err.Error(), "pattern too large") {
		t.Fatalf("pattern past the limit: error = %v, want pattern too large", err)
	}
}
