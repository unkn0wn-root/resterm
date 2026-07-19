package mock

import (
	"context"
	"net/http"
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
  query: {kind: "payment"},
  headers: {
    Authorization: {prefix: "Bearer "},
    "X-Trace": {present: true},
    "X-Debug": {absent: true}
  },
  json: {status: "completed"}
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
	if got := inspector.pattern.Query["kind"]; len(got) != 1 || got[0] != "payment" {
		t.Fatalf("query rule = %+v", got)
	}
	if got := inspector.pattern.Headers["Authorization"]; got.Op != restfile.MockHeaderOpPrefix ||
		got.Values[0] != "Bearer " {
		t.Fatalf("Authorization rule = %+v", got)
	}
	if got := inspector.pattern.Headers["X-Debug"]; got.Op != restfile.MockHeaderOpAbsent {
		t.Fatalf("X-Debug rule = %+v", got)
	}
	if string(inspector.pattern.JSON) != `{"status":"completed"}` {
		t.Fatalf("JSON pattern = %s", inspector.pattern.JSON)
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
