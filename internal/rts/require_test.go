package rts

import (
	"context"
	"strings"
	"testing"
)

func TestRequireHelpers(t *testing.T) {
	e := NewEng()
	rt := RT{
		Env:     map[string]string{"mode": "dev"},
		Vars:    map[string]string{"token": "abc"},
		Globals: map[string]string{"g": "v"},
	}

	pos := Pos{Path: "test", Line: 1, Col: 1}

	v, err := e.Eval(context.Background(), rt, "env.require(\"mode\")", pos)
	if err != nil {
		t.Fatalf("env.require: %v", err)
	}
	if v.K != VStr || v.S != "dev" {
		t.Fatalf("expected env.require to return dev, got %+v", v)
	}

	v, err = e.Eval(context.Background(), rt, "vars.require(\"token\")", pos)
	if err != nil {
		t.Fatalf("vars.require: %v", err)
	}
	if v.K != VStr || v.S != "abc" {
		t.Fatalf("expected vars.require to return abc, got %+v", v)
	}

	v, err = e.Eval(context.Background(), rt, "vars.global.require(\"g\")", pos)
	if err != nil {
		t.Fatalf("vars.global.require: %v", err)
	}
	if v.K != VStr || v.S != "v" {
		t.Fatalf("expected vars.global.require to return v, got %+v", v)
	}

	_, err = e.Eval(context.Background(), rt, "env.require(\"missing\", \"no env\")", pos)
	if err == nil || !strings.Contains(err.Error(), "no env") {
		t.Fatalf("expected custom require error, got %v", err)
	}
}
