package rtshost

import (
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/httpheader"
	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/queryparams"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestMutatorNormalizesTokenMutations(t *testing.T) {
	var out prerequest.Output
	req := &Request{}
	mut := NewMutator(&out, req, nil, nil, nil)

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
	req := &Request{Headers: httpheader.Values{"x-drop": {"gone"}}}
	mut := NewMutator(&out, req, nil, nil, nil)

	mut.SetHeader(mustHeaderName(t, "X-Test"), "1")
	mut.AddHeader(mustHeaderName(t, "X-Test"), "2")
	mut.DelHeader(mustHeaderName(t, "X-Drop"))

	if got := out.Headers.Values("X-Test"); len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("expected recorded header values [1 2], got %#v", got)
	}
	if got := req.Headers["x-test"]; len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("expected request view header values [1 2], got %#v", got)
	}
	if _, ok := req.Headers["x-drop"]; ok {
		t.Fatalf("expected deleted header to leave the request view: %#v", req.Headers)
	}
}

func mustHeaderName(t *testing.T, s string) httpheader.Name {
	t.Helper()
	n, err := httpheader.Parse(s)
	if err != nil {
		t.Fatalf("httpheader.Parse(%q): %v", s, err)
	}
	return n
}

func TestMutatorPatchesRequestURLOnQuery(t *testing.T) {
	var out prerequest.Output
	req := &Request{URL: "https://example.com/path?seed=1", Query: queryparams.Values{"seed": {"1"}}}
	mut := NewMutator(&out, req, nil, nil, nil)

	mut.SetQuery("user", "alice")

	if out.Query["user"] != "alice" {
		t.Fatalf("expected recorded query user=alice, got %#v", out.Query)
	}
	if got := req.Query["user"]; len(got) != 1 || got[0] != "alice" {
		t.Fatalf("expected request view query user=alice, got %#v", req.Query)
	}
	if !strings.Contains(req.URL, "seed=1") || !strings.Contains(req.URL, "user=alice") {
		t.Fatalf("expected patched request url, got %q", req.URL)
	}
}

func TestMutatorDerivesQueryAfterSetURL(t *testing.T) {
	var out prerequest.Output
	req := &Request{URL: "https://example.com/old?stale=1"}
	mut := NewMutator(&out, req, nil, nil, nil)

	mut.SetURL("https://example.com/new?keep=1")
	mut.SetQuery("added", "2")

	if _, ok := req.Query["stale"]; ok {
		t.Fatalf("request query retained the old URL: %#v", req.Query)
	}
	if got := req.Query["keep"]; !slices.Equal(got, []string{"1"}) {
		t.Fatalf("request query keep = %q, want [1]", got)
	}
	if got := req.Query["added"]; !slices.Equal(got, []string{"2"}) {
		t.Fatalf("request query added = %q, want [2]", got)
	}
}

func TestMutatorPatchesTheEmptyQueryName(t *testing.T) {
	var out prerequest.Output
	req := &Request{URL: "https://example.com/path"}
	mut := NewMutator(&out, req, nil, nil, nil)

	mut.SetQuery("", "1")

	if out.Query[""] != "1" {
		t.Fatalf("recorded query = %#v, want the empty name", out.Query)
	}
	if req.URL != "https://example.com/path?=1" {
		t.Fatalf("request url = %q, want the empty name in the query", req.URL)
	}
}

func TestMutatorKeepsRuntimeVarsAndGlobalsInSync(t *testing.T) {
	var out prerequest.Output
	vv := map[string]string{}
	gv := map[string]string{"old": "gone"}
	mut := NewMutator(&out, nil, vv, gv, nil)

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
	if got, _ := out.Globals.Get("NewGlobal"); got.Value != "ng" || !got.Secret {
		t.Fatalf("expected recorded secret global, got %#v", got)
	}
	if _, ok := gv["old"]; ok {
		t.Fatalf("expected deleted global to leave the runtime view: %#v", gv)
	}
	if got, _ := out.Globals.Get("Old"); !got.Delete {
		t.Fatalf("expected recorded global deletion, got %#v", got)
	}
}

func TestMutatorDropsBlankVariableNames(t *testing.T) {
	var out prerequest.Output
	vv := map[string]string{}
	mut := NewMutator(&out, nil, vv, nil, nil)

	mut.SetVar("  ", "ghost")

	if out.Variables.Len() != 0 {
		t.Fatalf("expected no recorded variables, got %#v", out.Variables.Map())
	}
	if len(vv) != 0 {
		t.Fatalf("expected the runtime view to stay empty, got %#v", vv)
	}
}

func TestMutatorKeepsSecretValuesThroughDeleteAndOverwrite(t *testing.T) {
	var out prerequest.Output
	var sec vars.Secrets
	mut := NewMutator(&out, nil, nil, map[string]string{}, &sec)

	mut.SetGlobal("token", "hidden", true)
	mut.DelGlobal("token")
	mut.SetGlobal("other", "also-hidden", true)
	mut.SetGlobal("other", "public", false)

	if entry, _ := out.Globals.Get("token"); !entry.Delete {
		t.Fatalf("expected the delete to win, got %#v", entry)
	}
	if entry, _ := out.Globals.Get("other"); entry.Secret || entry.Value != "public" {
		t.Fatalf("expected the public overwrite to win, got %#v", entry)
	}
	want := []string{"hidden", "also-hidden"}
	if got := sec.Values(); !slices.Equal(got, want) {
		t.Fatalf("secrets = %#v, want %#v", got, want)
	}
}

func TestMutatorWithoutRuntimeViewsOnlyRecords(t *testing.T) {
	var out prerequest.Output
	mut := NewMutator(&out, nil, nil, nil, nil)

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
