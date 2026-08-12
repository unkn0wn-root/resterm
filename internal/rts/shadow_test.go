package rts

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var shadowPos = Pos{Path: "test", Line: 1, Col: 1}

func TestLocalsShadowEveryEarlierBindingLayer(t *testing.T) {
	e := NewEng(testStdlib)
	cfg := EvalConfig{
		Bindings: []Extensions{
			NewExtensions(map[string]Value{"host": Str("extension")}),
		},
	}
	names := []string{"host", "len", "str"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			cfg := cfg
			cfg.Locals = Local(name, Str("local"))
			v, err := e.Eval(context.Background(), cfg, name, shadowPos)
			if err != nil {
				t.Fatalf("Eval: %v", err)
			}
			if v.K != VStr || v.S != "local" {
				t.Fatalf("%s = %+v, want local", name, v)
			}
		})
	}
}

func TestOverridesShadowLocals(t *testing.T) {
	e := NewEng(testStdlib)
	cfg := EvalConfig{
		Locals:    Local("status", Num(11)),
		Overrides: Local("status", Num(201)),
	}
	v, err := e.Eval(context.Background(), cfg, "status", shadowPos)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if v.K != VNum || v.N != 201 {
		t.Fatalf("status = %+v, want override", v)
	}
}

func TestBindingLayersCannotReplaceExistingNames(t *testing.T) {
	e := NewEng(testStdlib)
	cases := []string{"len", "str"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := EvalConfig{Bindings: []Extensions{Extension(name, Str("host"))}}
			_, err := e.Eval(context.Background(), cfg, "1", shadowPos)
			want := "name already defined: " + name
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %v, want %q", err, want)
			}
		})
	}
}

func TestLocalsShadowUseAliases(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "users.rts")
	fs := &memFS{files: map[string]*memFile{
		p: {data: []byte("module users\nexport let x = 1\n"), mod: time.Unix(10, 0)},
	}}

	e := NewEng(testStdlib)
	e.C = NewCache(fs, e.modulePre)
	cfg := EvalConfig{
		BaseDir: dir,
		Uses:    []Use{{Path: "users.rts"}},
		Locals:  Local("users", Str("local")),
	}
	v, err := e.Eval(context.Background(), cfg, "users", shadowPos)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if v.K != VStr || v.S != "local" {
		t.Fatalf("users = %+v, want local", v)
	}
}

func TestBindingConstructorsCopyTheirInput(t *testing.T) {
	src := map[string]Value{"item": Str("original")}
	ext := NewExtensions(src)
	loc := NewLocals(src)

	src["item"] = Str("changed")
	src["added"] = Str("late")
	for name, layer := range map[string]binds{"extensions": ext.binds, "locals": loc.binds} {
		if got := layer.values["item"]; got.S != "original" {
			t.Fatalf("%s item = %q, want original", name, got.S)
		}
		if _, ok := layer.values["added"]; ok {
			t.Fatalf("%s retained caller map", name)
		}
	}
}
