package rts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequestHostObject(t *testing.T) {
	e := NewEng()
	p := Pos{Path: "test", Line: 1, Col: 1}
	rt := RT{
		Env:  map[string]string{},
		Vars: map[string]string{},
		Req: &Req{
			Method: "GET",
			URL:    "https://example.test/path?p=1",
			H: map[string][]string{
				"x-test": {"ok"},
			},
		},
	}
	v, err := e.Eval(context.Background(), rt, "request.method", p)
	if err != nil {
		t.Fatalf("eval request.method: %v", err)
	}
	if v.K != VStr || v.S != "GET" {
		t.Fatalf("expected method GET")
	}
	v, err = e.Eval(
		context.Background(),
		rt,
		"request.header(\"x-test\")",
		p,
	)
	if err != nil {
		t.Fatalf("eval request.header: %v", err)
	}
	if v.K != VStr || v.S != "ok" {
		t.Fatalf("expected header ok")
	}
	v, err = e.Eval(context.Background(), rt, "request.query.p", p)
	if err != nil {
		t.Fatalf("eval request.query: %v", err)
	}
	if v.K != VStr || v.S != "1" {
		t.Fatalf("expected query value 1")
	}
	rt.Req.URL = "/path?p=2"
	v, err = e.Eval(context.Background(), rt, "request.query.p", p)
	if err != nil {
		t.Fatalf("eval request.query relative: %v", err)
	}
	if v.K != VStr || v.S != "2" {
		t.Fatalf("expected query value 2")
	}
	rt.Req.URL = "/path"
	v, err = e.Eval(context.Background(), rt, "rts.dict.keys(request.query)", p)
	if err != nil {
		t.Fatalf("eval request.query keys: %v", err)
	}
	if v.K != VList || len(v.L) != 0 {
		t.Fatalf("expected empty query keys")
	}
}

func TestRequestHostObjectInModule(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "mod.rts")
	if err := os.WriteFile(
		modPath,
		[]byte("export fn method() { return request.method }"),
		0o644,
	); err != nil {
		t.Fatalf("write module: %v", err)
	}
	e := NewEng()
	rt := RT{
		Env:     map[string]string{},
		Vars:    map[string]string{},
		BaseDir: dir,
		Uses:    []Use{{Path: "mod.rts", Alias: "m"}},
		Req:     &Req{Method: "GET"},
	}
	v, err := e.Eval(context.Background(), rt, "m.method()", Pos{Path: "test", Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("eval module method: %v", err)
	}
	if v.K != VStr || v.S != "GET" {
		t.Fatalf("expected method GET")
	}
	rt.Req = &Req{Method: "POST"}
	v, err = e.Eval(context.Background(), rt, "m.method()", Pos{Path: "test", Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("eval module method 2: %v", err)
	}
	if v.K != VStr || v.S != "POST" {
		t.Fatalf("expected method POST")
	}
}

func TestModuleAliasFromHeader(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mod.rts")
	src := []byte("module mod\nexport fn ok() { return 1 }\n")
	if err := os.WriteFile(p, src, 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	e := NewEng()
	rt := RT{
		Env:     map[string]string{},
		Vars:    map[string]string{},
		BaseDir: dir,
		Uses:    []Use{{Path: "mod.rts"}},
	}
	v, err := e.Eval(context.Background(), rt, "mod.ok()", Pos{Path: "test", Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("eval module: %v", err)
	}
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected ok() = 1")
	}
}

func TestModuleAliasOverride(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mod.rts")
	src := []byte("module mod\nexport fn ok() { return 1 }\n")
	if err := os.WriteFile(p, src, 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	e := NewEng()
	rt := RT{
		Env:     map[string]string{},
		Vars:    map[string]string{},
		BaseDir: dir,
		Uses:    []Use{{Path: "mod.rts", Alias: "alt"}},
	}
	v, err := e.Eval(context.Background(), rt, "alt.ok()", Pos{Path: "test", Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("eval module: %v", err)
	}
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected ok() = 1")
	}
}

func TestModuleAliasMissingName(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "mod.rts")
	src := []byte("export let x = 1\n")
	if err := os.WriteFile(p, src, 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	e := NewEng()
	rt := RT{
		Env:     map[string]string{},
		Vars:    map[string]string{},
		BaseDir: dir,
		Uses:    []Use{{Path: "mod.rts"}},
	}
	_, err := e.Eval(context.Background(), rt, "1", Pos{Path: "test", Line: 1, Col: 1})
	if err == nil || !strings.Contains(err.Error(), "missing module name") {
		t.Fatalf("expected missing module name error, got %v", err)
	}
}
