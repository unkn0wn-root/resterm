package rts_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/stdlib"
)

func TestHaltEndsEvaluation(t *testing.T) {
	dir := t.TempDir()
	mod := "module m\nexport fn call(f) {\n  f()\n}\n"
	if err := os.WriteFile(filepath.Join(dir, "m.rts"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		src  string
	}{
		{"direct", "stop()\nmark()"},
		{"try", "let r = try stop()\nmark()"},
		{"user fn", "fn f() {\n  stop()\n  mark()\n}\nlet r = try f()\nmark()"},
		{"module fn", "let r = try m.call(stop)\nmark()"},
		{"list callback", "fn f(x) {\n  stop()\n}\nlet r = try rts.list.map([1, 2], f)\nmark()"},
		{"member call", "let o = {s: stop}\nlet r = try o.s()\nmark()"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marks := 0
			stop := rts.NativeNamed("stop", func(*rts.Ctx, rts.Pos, []rts.Value) (rts.Value, error) {
				return rts.Null(), rts.ErrHalt
			})
			mark := rts.NativeNamed("mark", func(*rts.Ctx, rts.Pos, []rts.Value) (rts.Value, error) {
				marks++
				return rts.Null(), nil
			})
			cfg := rts.EvalConfig{
				Bindings: []rts.Extensions{rts.NewExtensions(map[string]rts.Value{"stop": stop, "mark": mark})},
				Uses:     []rts.Use{{Path: "m.rts"}},
				BaseDir:  dir,
			}
			_, err := rts.NewEng(stdlib.New).ExecModule(t.Context(), cfg, tt.src, rts.NewFile("t.rts", nil).Pos(1, 1))
			if !errors.Is(err, rts.ErrHalt) {
				t.Fatalf("err = %v, want the halt", err)
			}
			if !rts.IsAbort(err) {
				t.Fatalf("IsAbort misses the halt: %v", err)
			}
			if marks != 0 {
				t.Fatalf("%d statements ran after the halt", marks)
			}
		})
	}
}

func TestAbortIsNotHalt(t *testing.T) {
	eng := rts.NewEng(stdlib.New)
	eng.Lim.MaxSteps = 20
	_, err := eng.ExecModule(t.Context(), rts.EvalConfig{}, "for {\n}", rts.NewFile("t.rts", nil).Pos(1, 1))
	if !rts.IsAbort(err) || errors.Is(err, rts.ErrHalt) {
		t.Fatalf("err = %v, want an abort that is not a halt", err)
	}
}
