package rts

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var shadowPos = Pos{Path: "test", Line: 1, Col: 1}

// shadowRT binds every host object so the prelude under test is the full one an
// expression really sees.
func shadowRT() RT {
	return RT{
		Env:    map[string]string{"host": "example.test"},
		Vars:   map[string]string{"token": "abc"},
		Resp:   &Resp{Status: "200 OK", Code: 200},
		Res:    &Resp{Status: "201 Created", Code: 201},
		Trace:  &Trace{},
		Stream: &Stream{Kind: "sse"},
		Req:    &Req{Method: "GET", URL: "https://example.test"},
	}
}

func preludeNames(t *testing.T, e *Eng, rt RT) []string {
	t.Helper()
	cx := NewCtx(context.Background(), e.Lim)
	pre, err := e.buildPre(cx, rt, shadowPos, evalBase)
	if err != nil {
		t.Fatalf("build prelude: %v", err)
	}
	if len(pre) == 0 {
		t.Fatal("prelude is empty")
	}
	names := make([]string, 0, len(pre))
	for name := range pre {
		names = append(names, name)
	}
	return names
}

// The prelude is the only list of what a local may shadow. Walking the
// generated one keeps a newly bound host object from quietly staying immune.
func TestLocalsShadowEveryPreludeName(t *testing.T) {
	e := NewEng(testStdlib)
	rt := shadowRT()

	for _, name := range preludeNames(t, e, rt) {
		t.Run(name, func(t *testing.T) {
			rt := rt
			rt.Locals = Local(name, Str("loop item"))
			v, err := e.Eval(context.Background(), rt, name, shadowPos)
			if err != nil {
				t.Fatalf("eval %s: %v", name, err)
			}
			if v.K != VStr || v.S != "loop item" {
				t.Fatalf("%s = %+v, want the local to win", name, v)
			}
		})
	}
}

func TestExtensionsRejectEveryPreludeName(t *testing.T) {
	e := NewEng(testStdlib)
	rt := shadowRT()

	for _, name := range preludeNames(t, e, rt) {
		t.Run(name, func(t *testing.T) {
			rt := rt
			rt.Extensions = Extension(name, Str("host object"))
			_, err := e.Eval(context.Background(), rt, "1", shadowPos)
			want := "name already defined: " + name
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("eval with extension %s: error = %v, want %q", name, err, want)
			}
		})
	}
}

// A loop variable named status is the item outside assertions and the response
// status inside them. Both readings have to keep working.
func TestAssertionShorthandsShadowLocals(t *testing.T) {
	e := NewEng(testStdlib)
	rt := shadowRT()
	rt.Locals = Local("status", Num(11))

	v, err := e.Eval(context.Background(), rt, "status", shadowPos)
	if err != nil {
		t.Fatalf("eval status: %v", err)
	}
	if v.K != VNum || v.N != 11 {
		t.Fatalf("status outside an assertion = %+v, want the loop item", v)
	}

	v, err = e.EvalAssertion(context.Background(), rt, "status", shadowPos)
	if err != nil {
		t.Fatalf("assert status: %v", err)
	}
	if v.K != VNum || v.N != 201 {
		t.Fatalf("status inside an assertion = %+v, want the response status", v)
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
	rt := RT{
		BaseDir: dir,
		Uses:    []Use{{Path: "users.rts"}},
		Locals:  Local("users", Str("loop item")),
	}

	v, err := e.Eval(context.Background(), rt, "users", shadowPos)
	if err != nil {
		t.Fatalf("eval users: %v", err)
	}
	if v.K != VStr || v.S != "loop item" {
		t.Fatalf("users = %+v, want the local to win over the alias", v)
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
			t.Fatalf("%s item = %q, want %q", name, got.S, "original")
		}
		if _, ok := layer.values["added"]; ok {
			t.Fatalf("%s picked up a name added after construction", name)
		}
	}
}
