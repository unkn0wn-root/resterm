package request

import (
	"reflect"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
)

func TestForRunIsolatesInFlightRunFromLaterRuns(t *testing.T) {
	shared := New(engine.Config{EnvironmentFile: "/a/resterm.env.json"}, nil)
	inflight := shared.ForRun(
		engine.Config{EnvironmentFile: "/a/resterm.env.json"},
		&httpx.Response{Status: "200 OK"},
		nil,
	)

	shared.ForRun(engine.Config{EnvironmentFile: "/b/resterm.env.json"}, nil, nil)

	if got := inflight.cfg.EnvironmentFile; got != "/a/resterm.env.json" {
		t.Fatalf("in-flight config changed to %q", got)
	}
	if inflight.last.http == nil {
		t.Fatal("in-flight last response was cleared by a later run")
	}
	if got := shared.cfg.EnvironmentFile; got != "/a/resterm.env.json" {
		t.Fatalf("shared engine config changed to %q", got)
	}
	if inflight.rt != shared.rt || inflight.hc != shared.hc {
		t.Fatal("collaborators must stay shared across runs")
	}
}

// A field added to Engine but forgotten in ForRun would be silently zero on
// every run.
func TestForRunCarriesEveryEngineField(t *testing.T) {
	shared := New(engine.Config{EnvironmentFile: "/a/resterm.env.json"}, nil)
	run := shared.ForRun(engine.Config{EnvironmentFile: "/b/resterm.env.json"}, nil, nil)

	run.cfg = shared.cfg
	run.last = shared.last
	if !reflect.DeepEqual(run, shared) {
		t.Fatal("ForRun dropped an Engine field; decide whether it is shared or per-run")
	}
}
