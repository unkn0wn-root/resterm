package rtshost

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestRuntimeGlobals(t *testing.T) {
	globals := vars.CollectNames(map[string]vars.GlobalMutation{
		"fallback": {Value: "a"},
		"Token":    {Name: "Token", Value: "b"},
		"Secret":   {Name: "Secret", Value: "c", Secret: true},
		"Deleted":  {Name: "Deleted", Value: "d", Delete: true},
	})

	got := RuntimeGlobals(globals, OmitSecrets)
	if got["fallback"] != "a" || got["token"] != "b" {
		t.Fatalf("unexpected runtime globals: %#v", got)
	}
	if _, ok := got["secret"]; ok {
		t.Fatalf("expected secret global to be omitted: %#v", got)
	}
	if _, ok := got["deleted"]; ok {
		t.Fatalf("expected deleted global to be omitted: %#v", got)
	}

	got = RuntimeGlobals(globals, IncludeSecrets)
	if got["secret"] != "c" {
		t.Fatalf("expected secret global to be included: %#v", got)
	}
	if _, ok := got["deleted"]; ok {
		t.Fatalf("expected deleted global to stay omitted: %#v", got)
	}
}
