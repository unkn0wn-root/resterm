package rts

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var shadowPos = NewFile("test", nil).Pos(1, 1)

func TestLocalsShadowEveryEarlierBindingLayer(t *testing.T) {
	e := NewEng(testStdlib())
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
	e := NewEng(testStdlib())
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
	e := NewEng(testStdlib())
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

func TestTopLevelDeclarationsShadowPredeclaredNames(t *testing.T) {
	e := NewEng(testStdlib())
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"let", `let len = "mine"` + "\nexport let v = len", "mine"},
		{"fn", `fn str(x) { return "mine" }` + "\nexport let v = str(1)", "mine"},
		{"rts reaches the shadowed name", "let len = 3\nexport let v = rts.str(rts.len(\"ab\"))", "2"},
		{"stdlib reaches the shadowed name", "fn str(x) { return x }\nexport let v = stdlib.str(1)", "1"},
		{"inner block shadows rts", "fn f() {\n  let rts = \"mine\"\n  return rts\n}\nexport let v = f()", "mine"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, err := e.ExecModule(t.Context(), EvalConfig{}, tt.src, shadowPos)
			if err != nil {
				t.Fatalf("ExecModule: %v", err)
			}
			if v := comp.Exp["v"]; v.K != VStr || v.S != tt.want {
				t.Fatalf("v = %+v, want %q", v, tt.want)
			}
		})
	}
}

func TestTopLevelNamesCannotBeDeclaredAgain(t *testing.T) {
	e := NewEng(testStdlib())
	tests := []struct {
		name string
		cfg  EvalConfig
	}{
		{"rts", EvalConfig{}},
		{"stdlib", EvalConfig{}},
		{"item", EvalConfig{Locals: Local("item", Num(1))}},
		{"t", EvalConfig{Overrides: Local("t", Num(1))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.ExecModule(t.Context(), tt.cfg, "let "+tt.name+" = 1", shadowPos)
			want := fmt.Sprintf("name already defined: %q", tt.name)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("error = %v, want %q", err, want)
			}
		})
	}
}

// Every layer and declaration binds into the top-level scope, so the universe
// the engine shares keeps exactly the predeclared names.
func TestPredeclaredNamesAreNeverWritten(t *testing.T) {
	e := NewEng(testStdlib())
	_, err := e.ExecModule(t.Context(), EvalConfig{}, "len = 1", shadowPos)
	if want := `assign to const name "len"`; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
	cfg := EvalConfig{
		Bindings:  []Extensions{Extension("host", Str("extension"))},
		Locals:    Local("item", Num(1)),
		Overrides: Local("t", Num(2)),
	}
	src := "let len = 1\nfn str(x) { return x }\nlen = 2"
	if _, err := e.ExecModule(t.Context(), cfg, src, shadowPos); err != nil {
		t.Fatalf("ExecModule: %v", err)
	}
	want := slices.Sorted(maps.Keys(testStdlib()))
	if got := slices.Sorted(maps.Keys(e.uni.Vars)); !slices.Equal(got, want) {
		t.Fatalf("universe names = %v, want %v", got, want)
	}
	for name, b := range e.uni.Vars {
		if !b.Const || b.V.K != VNative {
			t.Fatalf("universe %s = %+v, want the const builtin", name, b)
		}
	}
}

func TestLocalsShadowUseAliases(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "users.rts")
	fs := &memFS{files: map[string]*memFile{
		p: {data: []byte("module users\nexport let x = 1\n"), mod: time.Unix(10, 0)},
	}}

	e := NewEng(testStdlib())
	e.C = NewCache(fs, e.moduleEnv)
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
