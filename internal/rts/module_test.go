package rts

import (
	"context"
	"errors"
	"io/fs"
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

	c := NewCache(fs, testScope)
	ctx := NewCtx(context.Background(), Limits{})

	m1, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	v := m1.Exp["x"]
	if v.K != VNum || v.N != 1 {
		t.Fatalf("expected x=1")
	}

	m2, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load2: %v", err)
	}
	if m1 != m2 {
		t.Fatalf("expected cache hit")
	}

	fs.files[p].data = []byte("export let x = 2")
	fs.files[p].mod = time.Unix(20, 0)

	m3, err := c.Load(ctx, "", p)
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
	c := NewCache(fs, testScope)
	ctx := NewCtx(context.Background(), Limits{})

	m1, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	m2, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load2: %v", err)
	}
	if m1 != m2 {
		t.Fatalf("expected cache hit for unchanged content")
	}

	fs.files[p].data = []byte("export let x = 2")
	m3, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load3: %v", err)
	}
	if v := m3.Exp["x"]; v.K != VNum || v.N != 2 {
		t.Fatalf("x = %v after a same-size edit with the same mtime, want 2", v.N)
	}
}

func TestOldCodeKeepsTheTextItWasReadFrom(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mod.rts")
	v1 := "export fn boom() {\n  return null.a\n}\n" +
		"export fn maker() {\n  fn inner() {\n    return null.b\n  }\n  return inner\n}\n"
	v2 := "export fn boom() {\n  return null.c\n}\n"
	v3 := "export fn boom() {\n  return 1 +\n}\n"
	fs := &memFS{files: map[string]*memFile{p: {data: []byte(v1), mod: time.Unix(10, 0)}}}
	c := NewCache(fs, testScope)
	ctx := NewCtx(context.Background(), Limits{})

	m1, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	inner, err := CallValue(ctx, Pos{}, m1.Exp["maker"], nil)
	if err != nil {
		t.Fatalf("maker(): %v", err)
	}
	fs.files[p] = &memFile{data: []byte(v2), mod: time.Unix(20, 0)}
	m2, err := c.Load(ctx, "", p)
	if err != nil {
		t.Fatalf("load2: %v", err)
	}
	fs.files[p] = &memFile{data: []byte(v3), mod: time.Unix(30, 0)}
	_, broken := c.Load(ctx, "", p)

	call := func(fn Value) error {
		_, err := CallValue(ctx, Pos{}, fn, nil)
		return err
	}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"function from before the edits", call(m1.Exp["boom"]), v1},
		{"closure from before the edits", call(inner), v1},
		{"function from after an edit", call(m2.Exp["boom"]), v2},
		{"edit that does not parse", broken, v3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(errPos(t, tt.err).Source()); got != tt.want {
				t.Fatalf("source = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadErrorsCarryTheModuleText(t *testing.T) {
	dir := t.TempDir()
	srcs := map[string]string{
		"late.rts":  "let x = 1\nmodule late\n",
		"bare.rts":  "\nexport let x = 1\n",
		"empty.rts": "",
		"init.rts":  "module init\n\nlet cfg = null\nlet v = cfg.name\n",
	}
	fs := &memFS{files: map[string]*memFile{}}
	for name, src := range srcs {
		fs.files[filepath.Join(dir, name)] = &memFile{data: []byte(src), mod: time.Unix(10, 0)}
	}
	e := NewEng(testStdlib())
	e.C = NewCache(fs, e.moduleEnv)
	at := NewFile("test", nil).Pos(1, 1)
	use := func(u Use) error {
		_, err := e.Eval(t.Context(), EvalConfig{BaseDir: dir, Uses: []Use{u}}, "1", at)
		return err
	}

	// Before the alias load below, which leaves bare.rts in the cache.
	uncached := use(Use{Path: "bare.rts"})
	if err := use(Use{Path: "bare.rts", Alias: "bare"}); err != nil {
		t.Fatalf("load with an alias: %v", err)
	}
	reads := fs.reads
	cached := use(Use{Path: "bare.rts"})
	if fs.reads != reads {
		t.Fatalf("read bare.rts %d more times, want the cached module", fs.reads-reads)
	}
	tests := []struct {
		name string
		err  error
		line int
		col  int
		file string
	}{
		{"module after a statement", use(Use{Path: "late.rts"}), 2, 1, "late.rts"},
		{"missing name", uncached, 1, 1, "bare.rts"},
		{"missing name of a cached module", cached, 1, 1, "bare.rts"},
		{"missing name of an empty file", use(Use{Path: "empty.rts"}), 1, 1, "empty.rts"},
		{"module setup", use(Use{Path: "init.rts"}), 4, 12, "init.rts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := errPos(t, tt.err)
			if p.Line != tt.line || p.Col != tt.col || string(p.Source()) != srcs[tt.file] {
				t.Fatalf("error at %s with source %q, want %d:%d in %q", p, p.Source(), tt.line, tt.col, srcs[tt.file])
			}
		})
	}
}

func errPos(t *testing.T, err error) Pos {
	t.Helper()
	var re *RuntimeError
	if errors.As(err, &re) {
		return re.Pos
	}
	var pe *ParseError
	if errors.As(err, &pe) {
		return pe.Pos
	}
	t.Fatalf("error = %v, want a script error", err)
	return Pos{}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestModuleErrorsShowTheModuleLikeTheUsingFile(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	mod := "module v\nexport fn boom() {\n  return null.a\n}\n"
	writeFiles(t, root, map[string]string{
		"sub/v.rts":    mod,
		"sub/bare.rts": "export let x = 1\n",
		"shared/x.rts": mod,
	})
	sub := filepath.Join(root, "sub")
	at := NewFile("sub/api.http", nil).Pos(1, 1)
	tests := []struct {
		name string
		base string
		path string
		want string
	}{
		{"relative base", "sub", "./v.rts", "sub/v.rts"},
		{"path outside the base", "sub", "../shared/x.rts", "shared/x.rts"},
		{"no base", "", "sub/v.rts", "sub/v.rts"},
		{"missing module name", "sub", "bare.rts", "sub/bare.rts"},
		{"absolute base", sub, "v.rts", filepath.Join(sub, "v.rts")},
		{"absolute path", "other", sub + "/../sub/v.rts", filepath.Join(sub, "v.rts")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := EvalConfig{BaseDir: tt.base, Uses: []Use{{Path: tt.path}}}
			_, err := NewEng(testStdlib()).Eval(t.Context(), cfg, "v.boom()", at)
			if got := errPos(t, err).Path(); got != tt.want {
				t.Fatalf("error in %s, want it in %s", got, tt.want)
			}
		})
	}

	missing := []struct {
		name string
		use  Use
	}{
		{"missing file", Use{Path: "nope.rts"}},
		{"missing file with an alias", Use{Path: "nope.rts", Alias: "n"}},
	}
	for _, tt := range missing {
		t.Run(tt.name, func(t *testing.T) {
			cfg := EvalConfig{BaseDir: "sub", Uses: []Use{tt.use}}
			_, err := NewEng(testStdlib()).Eval(t.Context(), cfg, "1", at)
			var pe *fs.PathError
			if !errors.As(err, &pe) || pe.Path != "sub/nope.rts" {
				t.Fatalf("error = %v, want one for sub/nope.rts", err)
			}
		})
	}
}

func TestPathsToOneModuleShareItsEntry(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	writeFiles(t, root, map[string]string{
		"sub/v.rts": "module v\nlet n = 0\nexport fn bump() {\n  n = n + 1\n  return n\n}\n" +
			"export fn boom() {\n  return null.a\n}\n",
	})
	e := NewEng(testStdlib())
	at := NewFile("api.http", nil).Pos(1, 1)
	uses := []struct{ base, path string }{
		{"sub", "./v.rts"},
		{"", "sub/v.rts"},
		{"other", "../sub/v.rts"},
	}
	for i, u := range uses {
		cfg := EvalConfig{BaseDir: u.base, Uses: []Use{{Path: u.path}}}
		if v, err := e.Eval(t.Context(), cfg, "v.bump()", at); err != nil || v.N != float64(i+1) {
			t.Fatalf("v.bump() through %s = %v, %v, want %d", u.path, v.N, err, i+1)
		}
		_, err := e.Eval(t.Context(), cfg, "v.boom()", at)
		if got := errPos(t, err).Path(); got != "sub/v.rts" {
			t.Fatalf("v.boom() through %s: error in %s, want it in sub/v.rts", u.path, got)
		}
	}
	cfg := EvalConfig{BaseDir: filepath.Join(root, "sub"), Uses: []Use{{Path: "v.rts"}}}
	if v, err := e.Eval(t.Context(), cfg, "v.bump()", at); err != nil || v.N != 4 {
		t.Fatalf("v.bump() through an absolute path = %v, %v, want 4", v.N, err)
	}
}

func TestUseReadsARecentModuleNameFromTheFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.rts")
	fs := &memFS{files: map[string]*memFile{p: {data: []byte("module abc\nexport let x = 1"), mod: time.Now()}}}
	e := NewEng(testStdlib())
	e.C = NewCache(fs, e.moduleEnv)
	cfg := EvalConfig{BaseDir: dir, Uses: []Use{{Path: "a.rts"}}}
	pos := NewFile("test", nil).Pos(1, 1)

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
			e := NewEng(testStdlib())
			e.C = NewCache(fs, e.moduleEnv)
			cfg := EvalConfig{BaseDir: dir, Uses: []Use{{Path: "a.rts"}}}
			pos := NewFile("test", nil).Pos(1, 1)

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

	e := NewEng(testStdlib())
	e.C = NewCache(fs, e.moduleEnv)

	cfg := EvalConfig{
		BaseDir: dir,
		Uses: []Use{
			{Path: "a.rts"},
			{Path: "b.rts"},
		},
	}
	_, err := e.Eval(context.Background(), cfg, "1", NewFile("test", nil).Pos(1, 1))
	if err == nil || !strings.Contains(err.Error(), "alias already defined: mod") {
		t.Fatalf("expected alias collision error, got %v", err)
	}
}

func TestUseAliasErrorsPointAtTheUse(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.rts")
	fs := &memFS{files: map[string]*memFile{p: {data: []byte("module rts\nexport let x = 1"), mod: time.Unix(10, 0)}}}
	e := NewEng(testStdlib())
	e.C = NewCache(fs, e.moduleEnv)
	at := NewFile("api.http", nil).Pos(3, 1)
	tests := []struct {
		name string
		use  Use
		msg  string
	}{
		{"taken name", Use{Path: "a.rts", Pos: at}, "alias already defined: rts"},
		{"reserved word", Use{Path: "a.rts", Alias: "default", Pos: at}, "alias is a reserved word: default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := EvalConfig{BaseDir: dir, Uses: []Use{tt.use}}
			_, err := e.Eval(t.Context(), cfg, "1", NewFile("test", nil).Pos(9, 1))
			var re *RuntimeError
			if !errors.As(err, &re) || re.Msg != tt.msg || re.Pos != at {
				t.Fatalf("error = %v, want %q at %s", err, tt.msg, at)
			}
		})
	}
}

func TestUseAliasMayShadowAPredeclaredName(t *testing.T) {
	dir := t.TempDir()
	fs := &memFS{files: map[string]*memFile{
		filepath.Join(dir, "len.rts"): {data: []byte("module len\nexport let x = 1"), mod: time.Unix(10, 0)},
		filepath.Join(dir, "m.rts"):   {data: []byte("export let x = 2"), mod: time.Unix(10, 0)},
	}}
	e := NewEng(testStdlib())
	e.C = NewCache(fs, e.moduleEnv)
	tests := []struct {
		name string
		use  Use
		src  string
		want float64
	}{
		{"module name", Use{Path: "len.rts"}, "len.x", 1},
		{"alias", Use{Path: "m.rts", Alias: "str"}, "str.x", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := EvalConfig{BaseDir: dir, Uses: []Use{tt.use}}
			v, err := e.Eval(t.Context(), cfg, tt.src, NewFile("test", nil).Pos(1, 1))
			if err != nil || v.K != VNum || v.N != tt.want {
				t.Fatalf("%s = %+v, %v, want %v", tt.src, v, err, tt.want)
			}
		})
	}
}

func TestModuleMayShadowOnlyPredeclaredNames(t *testing.T) {
	dir := t.TempDir()
	fs := &memFS{files: map[string]*memFile{
		filepath.Join(dir, "len.rts"):     {data: []byte("module m\nexport let len = 1\n"), mod: time.Unix(10, 0)},
		filepath.Join(dir, "request.rts"): {data: []byte("module m\nexport let request = 1\n"), mod: time.Unix(10, 0)},
	}}
	e := NewEng(testStdlib(), WithModuleBindings(Extension("request", Str("host"))))
	e.C = NewCache(fs, e.moduleEnv)
	pos := NewFile("test", nil).Pos(1, 1)

	v, err := e.Eval(t.Context(), EvalConfig{BaseDir: dir, Uses: []Use{{Path: "len.rts"}}}, "m.len", pos)
	if err != nil || v.K != VNum || v.N != 1 {
		t.Fatalf("m.len = %+v, %v, want 1", v, err)
	}
	_, err = e.Eval(t.Context(), EvalConfig{BaseDir: dir, Uses: []Use{{Path: "request.rts"}}}, "m", pos)
	if want := `name already defined: "request"`; err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestLoadGivesTheErrorEvalWould(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.rts")
	src := "module bad\n\nexport fn ok(x) {\n  return x ==\n}\n"
	fs := &memFS{files: map[string]*memFile{p: {data: []byte(src), mod: time.Unix(10, 0)}}}
	e := NewEng(testStdlib())
	e.C = NewCache(fs, e.moduleEnv)
	cfg := EvalConfig{BaseDir: dir, Uses: []Use{{Path: "bad.rts"}}}
	pos := NewFile("test", nil).Pos(1, 1)

	err := e.Load(t.Context(), cfg, pos)
	_, want := e.Eval(t.Context(), cfg, "1", pos)
	var pe *ParseError
	if !errors.As(err, &pe) || want == nil || err.Error() != want.Error() {
		t.Fatalf("Load = %v, want the parse error Eval gives: %v", err, want)
	}
	if err := e.Load(t.Context(), EvalConfig{}, pos); err != nil {
		t.Fatalf("Load with no modules = %v, want nil", err)
	}
}

func TestLoadRunsModuleSetupOnce(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cnt.rts")
	src := `module cnt
let n = 0
let inits = 0
inits = inits + 1
export fn bump() {
  n = n + 1
  return n
}
export fn seen() {
  return inits
}
`
	fs := &memFS{files: map[string]*memFile{p: {data: []byte(src), mod: time.Unix(10, 0)}}}
	e := NewEng(testStdlib())
	e.C = NewCache(fs, e.moduleEnv)
	cfg := EvalConfig{BaseDir: dir, Uses: []Use{{Path: "cnt.rts"}}}
	pos := NewFile("test", nil).Pos(1, 1)

	if err := e.Load(t.Context(), cfg, pos); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, want := range []float64{1, 2} {
		if v, err := e.Eval(t.Context(), cfg, "cnt.bump()", pos); err != nil || v.N != want {
			t.Fatalf("cnt.bump() = %v, %v, want %v", v.N, err, want)
		}
	}
	if v, err := e.Eval(t.Context(), cfg, "cnt.seen()", pos); err != nil || v.N != 1 {
		t.Fatalf("setup ran %v times, %v, want once", v.N, err)
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
			e := NewEng(testStdlib())
			e.C = NewCache(fs, e.moduleEnv)
			_, err := e.Eval(context.Background(), tc.cfg, "1", NewFile("test", nil).Pos(1, 1))
			if err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("expected %q, got %v", tc.msg, err)
			}
		})
	}
}
