package rts

import (
	"context"
	"testing"
	"time"
)

func evalRT(t *testing.T, rt RT, src string) Value {
	t.Helper()
	e := NewEng()
	v, err := e.Eval(context.Background(), rt, src, Pos{Path: "test", Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	return v
}

func TestStreamDisabled(t *testing.T) {
	rt := RT{}
	v := evalRT(t, rt, "stream.enabled()")
	if v.K != VBool || v.B != false {
		t.Fatalf("expected disabled stream, got %+v", v)
	}
	v = evalRT(t, rt, "len(stream.events())")
	if v.K != VNum || v.N != 0 {
		t.Fatalf("expected 0 events, got %+v", v)
	}
}

func TestStreamEnabled(t *testing.T) {
	st := &Stream{
		Kind: "sse",
		Summary: map[string]any{
			"eventCount": 2,
			"byteCount":  int64(12),
			"duration":   1500 * time.Millisecond,
		},
		Events: []map[string]any{
			{"event": "ping", "index": 0},
			{"event": "pong", "index": 1},
		},
	}
	rt := RT{Stream: st}
	v := evalRT(t, rt, "stream.enabled()")
	if v.K != VBool || v.B != true {
		t.Fatalf("expected enabled stream, got %+v", v)
	}
	v = evalRT(t, rt, "stream.kind()")
	if v.K != VStr || v.S != "sse" {
		t.Fatalf("expected kind sse, got %+v", v)
	}
	v = evalRT(t, rt, "stream.summary().eventCount")
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected eventCount 2, got %+v", v)
	}
	v = evalRT(t, rt, "stream.summary().duration")
	if v.K != VNum || v.N != 1500 {
		t.Fatalf("expected duration 1500ms, got %+v", v)
	}
	v = evalRT(t, rt, "stream.events()[0].event")
	if v.K != VStr || v.S != "ping" {
		t.Fatalf("expected ping event, got %+v", v)
	}
}
