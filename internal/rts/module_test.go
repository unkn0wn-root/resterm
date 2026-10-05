package rts

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type memFS struct {
	files map[string]*memFile
	reads int
}

type memFile struct {
	data []byte
	mod  time.Time
}

type memInfo struct {
	name string
	mod  time.Time
	sz   int64
}

func (m memInfo) Name() string       { return m.name }
func (m memInfo) Size() int64        { return m.sz }
func (m memInfo) Mode() os.FileMode  { return 0 }
func (m memInfo) ModTime() time.Time { return m.mod }
func (m memInfo) IsDir() bool        { return false }
func (m memInfo) Sys() any           { return nil }

func (fs *memFS) ReadFile(path string) ([]byte, error) {
	fs.reads++
	f, ok := fs.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), f.data...), nil
}

func (fs *memFS) Stat(path string) (os.FileInfo, error) {
	f, ok := fs.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return memInfo{name: filepath.Base(path), mod: f.mod, sz: int64(len(f.data))}, nil
}

func TestModCacheReload(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mod.rts")
	fs := &memFS{files: map[string]*memFile{}}
	fs.files[p] = &memFile{data: []byte("export let x = 1"), mod: time.Unix(10, 0)}

	c := NewCache(fs, testStdlib)
	ctx := NewCtx(context.Background(), Limits{})

	m1, p1, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p1 != p {
		t.Fatalf("unexpected path")
	}
	v := m1.Exp["x"]
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected x=1")
	}

	m2, _, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load2: %v", err)
	}
	if m1 != m2 {
		t.Fatalf("expected cache hit")
	}

	fs.files[p].data = []byte("export let x = 2")
	fs.files[p].mod = time.Unix(20, 0)

	m3, _, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load3: %v", err)
	}
	if m3 == m2 {
		t.Fatalf("expected reload")
	}
	v = m3.Exp["x"]
	if v.K != VNum || v.N != 2 {
		t.Fatalf("expected x=2")
	}
}

func TestModCacheChecksARecentModuleByContent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mod.rts")
	fs := &memFS{files: map[string]*memFile{p: {data: []byte("export let x = 1"), mod: time.Now()}}}
	c := NewCache(fs, testStdlib)
	ctx := NewCtx(context.Background(), Limits{})

	m1, _, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	m2, _, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load2: %v", err)
	}
	if m1 != m2 {
		t.Fatalf("expected cache hit for unchanged content")
	}

	fs.files[p].data = []byte("export let x = 2")
	m3, _, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load3: %v", err)
	}
	if v := m3.Exp["x"]; v.K != VNum || v.N != 2 {
		t.Fatalf("x = %v after a same-size edit with the same mtime, want 2", v.N)
	}
}

func TestUseReadsARecentModuleNameFromTheFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.rts")
	fs := &memFS{files: map[string]*memFile{p: {data: []byte("module abc\nexport let x = 1"), mod: time.Now()}}}
	e := NewEng(testStdlib)
	e.C = NewCache(fs, e.modulePre)
	cfg := EvalConfig{BaseDir: dir, Uses: []Use{{Path: "a.rts"}}}
	pos := Pos{Path: "test", Line: 1, Col: 1}

	if v, err := e.Eval(context.Background(), cfg, "abc.x", pos); err != nil || v.N != 1 {
		t.Fatalf("abc.x = %v, %v, want 1", v.N, err)
	}
	fs.files[p].data = []byte("module xyz\nexport let x = 2")
	if v, err := e.Eval(context.Background(), cfg, "xyz.x", pos); err != nil || v.N != 2 {
		t.Fatalf("xyz.x = %v, %v after a same-size rename with the same mtime, want 2", v.N, err)
	}
}

func TestUseChecksAModuleOnceMoreAfterItSettles(t *testing.T) {
	tests := []struct {
		name string
		edit string
		expr string
		want float64
	}{
		{name: "edited before it settled", edit: "module xyz\nexport let x = 2", expr: "xyz.x", want: 2},
		{name: "unchanged", expr: "abc.x", want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			p := filepath.Join(dir, "a.rts")
			mod := time.Now().Add(-time.Second)
			fs := &memFS{files: map[string]*memFile{p: {data: []byte("module abc\nexport let x = 1"), mod: mod}}}
			e := NewEng(testStdlib)
			e.C = NewCache(fs, e.modulePre)
			cfg := EvalConfig{BaseDir: dir, Uses: []Use{{Path: "a.rts"}}}
			pos := Pos{Path: "test", Line: 1, Col: 1}

			if v, err := e.Eval(context.Background(), cfg, "abc.x", pos); err != nil || v.N != 1 {
				t.Fatalf("abc.x = %v, %v, want 1", v.N, err)
			}
			if tt.edit != "" {
				fs.files[p].data = []byte(tt.edit)
			}
			time.Sleep(1100 * time.Millisecond)
			if v, err := e.Eval(context.Background(), cfg, tt.expr, pos); err != nil || v.N != tt.want {
				t.Fatalf("%s = %v, %v after the module settled, want %v", tt.expr, v.N, err, tt.want)
			}
			reads := fs.reads
			if _, err := e.Eval(context.Background(), cfg, tt.expr, pos); err != nil || fs.reads != reads {
				t.Fatalf("read the module %d more times after it settled, want 0 (err %v)", fs.reads-reads, err)
			}
		})
	}
}

func TestUseAliasCollisionBeforeParse(t *testing.T) {
	dir := t.TempDir()
	fs := &memFS{files: map[string]*memFile{}}
	p1 := filepath.Join(dir, "a.rts")
	p2 := filepath.Join(dir, "b.rts")
	fs.files[p1] = &memFile{data: []byte("module mod\nexport let x ="), mod: time.Unix(10, 0)}
	fs.files[p2] = &memFile{data: []byte("module mod\nexport let y ="), mod: time.Unix(10, 0)}

	e := NewEng(testStdlib)
	e.C = NewCache(fs, e.modulePre)

	cfg := EvalConfig{
		BaseDir: dir,
		Uses: []Use{
			{Path: "a.rts"},
			{Path: "b.rts"},
		},
	}
	_, err := e.Eval(context.Background(), cfg, "1", Pos{Path: "test", Line: 1, Col: 1})
	if err == nil || !strings.Contains(err.Error(), "alias already defined: mod") {
		t.Fatalf("expected alias collision error, got %v", err)
	}
}

// The evaluator refuses a reserved binding rather than creating one nothing
// can reference.
func TestEngineRejectsReservedBindings(t *testing.T) {
	dir := t.TempDir()
	fs := &memFS{files: map[string]*memFile{}}
	p := filepath.Join(dir, "a.rts")
	fs.files[p] = &memFile{data: []byte("module mod\nexport let x = 1\n"), mod: time.Unix(10, 0)}

	cases := []struct {
		name string
		cfg  EvalConfig
		msg  string
	}{
		{
			"use alias",
			EvalConfig{BaseDir: dir, Uses: []Use{{Path: "a.rts", Alias: "default"}}},
			"alias is a reserved word: default",
		},
		{
			"extension",
			EvalConfig{BaseDir: dir, Bindings: []Extensions{Extension("default", Num(1))}},
			"name is a reserved word: default",
		},
		{
			"extension other keyword",
			EvalConfig{BaseDir: dir, Bindings: []Extensions{Extension("range", Num(1))}},
			"name is a reserved word: range",
		},
		{
			"local",
			EvalConfig{BaseDir: dir, Locals: Local("default", Num(1))},
			"name is a reserved word: default",
		},
		{
			"local other keyword",
			EvalConfig{BaseDir: dir, Locals: Local("range", Num(1))},
			"name is a reserved word: range",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := NewEng(testStdlib)
			e.C = NewCache(fs, e.modulePre)
			_, err := e.Eval(context.Background(), tc.cfg, "1", Pos{Path: "test", Line: 1, Col: 1})
			if err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("expected %q, got %v", tc.msg, err)
			}
		})
	}
}
