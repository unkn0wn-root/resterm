package rtshost

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/stdlib"
)

func TestStreamDisabled(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	rt.Stream = nil

	v := evalHost(t, eng, rt, "stream.enabled()")
	if v.K != rts.VBool || v.B {
		t.Fatalf("stream.enabled() = %+v, want false", v)
	}
	v = evalHost(t, eng, rt, "len(stream.events())")
	if v.K != rts.VNum || v.N != 0 {
		t.Fatalf("len(stream.events()) = %+v, want 0", v)
	}
}

func TestStreamEnabled(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	rt.Stream = &Stream{
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
	tests := map[string]string{
		`str(stream.enabled())`:            "true",
		`stream.kind()`:                    "sse",
		`str(stream.summary().eventCount)`: "2",
		`str(stream.summary().duration)`:   "1500",
		`stream.events()[0].event`:         "ping",
		`str(stream.events()[1].index)`:    "1",
		`str(stream.summary().byteCount)`:  "12",
	}
	for src, want := range tests {
		v := evalHost(t, eng, rt, src)
		if v.K != rts.VStr || v.S != want {
			t.Errorf("%s = %+v, want %q", src, v, want)
		}
	}
}
