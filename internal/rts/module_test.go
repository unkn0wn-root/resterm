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

	c := NewCache(fs)
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

func TestUseAliasCollisionBeforeParse(t *testing.T) {
	dir := t.TempDir()
	fs := &memFS{files: map[string]*memFile{}}
	p1 := filepath.Join(dir, "a.rts")
	p2 := filepath.Join(dir, "b.rts")
	fs.files[p1] = &memFile{data: []byte("module mod\nexport let x ="), mod: time.Unix(10, 0)}
	fs.files[p2] = &memFile{data: []byte("module mod\nexport let y ="), mod: time.Unix(10, 0)}

	e := NewEng()
	e.C = NewCache(fs)
	e.C.SetStdlib(e.Stdlib)

	rt := RT{
		BaseDir: dir,
		Uses: []Use{
			{Path: "a.rts"},
			{Path: "b.rts"},
		},
	}
	_, err := e.Eval(context.Background(), rt, "1", Pos{Path: "test", Line: 1, Col: 1})
	if err == nil || !strings.Contains(err.Error(), "alias already defined: mod") {
		t.Fatalf("expected alias collision error, got %v", err)
	}
}
