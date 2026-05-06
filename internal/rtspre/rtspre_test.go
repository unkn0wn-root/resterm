package rtspre

import (
	"context"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestRunSkipsNonRTSPreRequestBlocks(t *testing.T) {
	eng := rts.NewEng(nil)
	calls := 0
	scripts := []restfile.ScriptBlock{
		{Kind: "pre-request", Lang: "js", Body: "not rts"},
		{Kind: "test", Lang: "rts", Body: "not pre"},
		{Kind: "pre-request", Lang: "rts", Body: "let x = 1"},
	}

	err := Run(context.Background(), eng, ExecInput{
		Scripts: scripts,
		BuildRT: func() rts.RT {
			calls++
			return rts.RT{}
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one RTS pre-request execution, got %d", calls)
	}
}

func TestRuntimeGlobals(t *testing.T) {
	globals := map[string]vars.GlobalMutation{
		"fallback": {Value: "a"},
		"named":    {Name: "Token", Value: "b"},
		"secret":   {Name: "Secret", Value: "c", Secret: true},
		"deleted":  {Name: "Deleted", Value: "d", Delete: true},
	}

	got := RuntimeGlobals(globals, true)
	if got["fallback"] != "a" || got["token"] != "b" {
		t.Fatalf("unexpected runtime globals: %#v", got)
	}
	if _, ok := got["secret"]; ok {
		t.Fatalf("expected secret global to be omitted: %#v", got)
	}
	if _, ok := got["deleted"]; ok {
		t.Fatalf("expected deleted global to be omitted: %#v", got)
	}
}

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
