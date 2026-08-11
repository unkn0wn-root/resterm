package rtshost

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func TestMutatorNormalizesTokenMutations(t *testing.T) {
	var out prerequest.Output
	req := &rts.Req{}
	mut := NewMutator(&out, req, nil, nil)

	mut.SetMethod(" post ")
	mut.SetURL(" https://api.example.com/users ")

	if out.Method == nil || *out.Method != "POST" || req.Method != "POST" {
		t.Fatalf("expected normalized method, out=%#v req=%q", out.Method, req.Method)
	}
	if out.URL == nil || *out.URL != "https://api.example.com/users" ||
		req.URL != "https://api.example.com/users" {
		t.Fatalf("expected normalized url, out=%#v req=%q", out.URL, req.URL)
	}
}

func TestMutatorMirrorsHeadersOntoRequestView(t *testing.T) {
	var out prerequest.Output
	req := &rts.Req{H: map[string][]string{"x-drop": {"gone"}}}
	mut := NewMutator(&out, req, nil, nil)

	mut.SetHeader("X-Test", "1")
	mut.AddHeader("X-Test", "2")
	mut.DelHeader("X-Drop")

	if got := out.Headers.Values("X-Test"); len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("expected recorded header values [1 2], got %#v", got)
	}
	if got := req.H["x-test"]; len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("expected request view header values [1 2], got %#v", got)
	}
	if _, ok := req.H["x-drop"]; ok {
		t.Fatalf("expected deleted header to leave the request view: %#v", req.H)
	}
}

func TestMutatorPatchesRequestURLOnQuery(t *testing.T) {
	var out prerequest.Output
	req := &rts.Req{URL: "https://example.com/path?seed=1"}
	mut := NewMutator(&out, req, nil, nil)

	mut.SetQuery("user", "alice")

	if out.Query["user"] != "alice" {
		t.Fatalf("expected recorded query user=alice, got %#v", out.Query)
	}
	if got := req.Q["user"]; len(got) != 1 || got[0] != "alice" {
		t.Fatalf("expected request view query user=alice, got %#v", req.Q)
	}
	if !strings.Contains(req.URL, "seed=1") || !strings.Contains(req.URL, "user=alice") {
		t.Fatalf("expected patched request url, got %q", req.URL)
	}
}

func TestMutatorKeepsRuntimeVarsAndGlobalsInSync(t *testing.T) {
	var out prerequest.Output
	vv := map[string]string{}
	gv := map[string]string{"old": "gone"}
	mut := NewMutator(&out, nil, vv, gv)

	mut.SetVar("token", "abc")
	mut.SetGlobal("NewGlobal", "ng", true)
	mut.DelGlobal("Old")

	recorded, _ := out.Variables.Get("token")
	if vv["token"] != "abc" || recorded != "abc" {
		t.Fatalf("expected token variable, vars=%#v out=%#v", vv, out.Variables.Map())
	}
	if gv["newglobal"] != "ng" {
		t.Fatalf("expected runtime global newglobal=ng, got %#v", gv)
	}
	if got := out.Globals["NewGlobal"]; got.Value != "ng" || !got.Secret {
		t.Fatalf("expected recorded secret global, got %#v", got)
	}
	if _, ok := gv["old"]; ok {
		t.Fatalf("expected deleted global to leave the runtime view: %#v", gv)
	}
	if got := out.Globals["Old"]; !got.Delete {
		t.Fatalf("expected recorded global deletion, got %#v", got)
	}
}

func TestMutatorWithoutRuntimeViewsOnlyRecords(t *testing.T) {
	var out prerequest.Output
	mut := NewMutator(&out, nil, nil, nil)

	mut.SetVar("token", "abc")
	mut.SetGlobal("token", "abc", false)
	mut.DelGlobal("token")
	mut.SetQuery("user", "alice")
	mut.SetBody("payload")

	if mut.Request() == nil {
		t.Fatalf("expected an empty request view instead of nil")
	}
	if recorded, _ := out.Variables.Get("token"); recorded != "abc" || out.Query["user"] != "alice" {
		t.Fatalf("expected recorded mutations, out=%#v", out)
	}
	if out.Body == nil || *out.Body != "payload" {
		t.Fatalf("expected recorded body, got %#v", out.Body)
	}
}
