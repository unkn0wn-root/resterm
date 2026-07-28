package rts

import (
	"context"
	"strings"
	"testing"
)

func TestEnvironmentGroups(t *testing.T) {
	eng := NewEng(testStdlib)
	rt := RT{
		Env: map[string]string{
			"name": "api=dev, Auth=CI",
		},
		EnvGroups: map[string]string{
			"api":  "dev",
			"Auth": "CI",
		},
	}
	pos := Pos{Path: "test", Line: 1, Col: 1}

	tests := []struct {
		expr string
		kind VKind
		text string
		flag bool
	}{
		{expr: "env.name", kind: VStr, text: "api=dev, Auth=CI"},
		{expr: "env.groups.API", kind: VStr, text: "dev"},
		{expr: `env.groups.get("auth")`, kind: VStr, text: "CI"},
		{expr: `env.groups.require("API")`, kind: VStr, text: "dev"},
		{expr: `env.groups.has("AUTH")`, kind: VBool, flag: true},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			got, err := eng.Eval(context.Background(), rt, tt.expr, pos)
			if err != nil {
				t.Fatalf("eval: %v", err)
			}
			if got.K != tt.kind || got.S != tt.text || got.B != tt.flag {
				t.Fatalf("value = %+v, want kind=%v text=%q flag=%v", got, tt.kind, tt.text, tt.flag)
			}
		})
	}

	_, err := eng.Eval(
		context.Background(),
		rt,
		`env.groups.require("missing", "profile missing")`,
		pos,
	)
	if err == nil || !strings.Contains(err.Error(), "profile missing") {
		t.Fatalf("error = %v, want custom require error", err)
	}
}

func TestEnvironmentGroupsEmptyForFlatEnvironment(t *testing.T) {
	eng := NewEng(testStdlib)
	got, err := eng.Eval(
		context.Background(),
		RT{Env: map[string]string{"name": "dev"}},
		`env.name + ":" + env.groups.has("api")`,
		Pos{Path: "test", Line: 1, Col: 1},
	)
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	if got.K != VStr || got.S != "dev:false" {
		t.Fatalf("value = %+v, want dev:false", got)
	}
}
