package fsx

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.json")
	if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := WriteAtomic(p, []byte("new"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(p)
	if err != nil || string(got) != "new" {
		t.Fatalf("content = %q, %v, want new", got, err)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", st.Mode().Perm())
		}
	}
	es, err := os.ReadDir(dir)
	if err != nil || len(es) != 1 {
		t.Fatalf("dir holds %d entries, %v, want only the target", len(es), err)
	}
}

func TestWriteAtomicMissingDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing", "out.json")
	if err := WriteAtomic(p, []byte("x"), 0o600); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestWriteAtomicLongName(t *testing.T) {
	p := filepath.Join(t.TempDir(), strings.Repeat("a", 245)+".json")
	if err := WriteAtomic(p, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write %d-char name: %v", len(filepath.Base(p)), err)
	}
}

func TestWriteInPlace(t *testing.T) {
	tests := []struct {
		name      string
		old, data string
	}{
		{"grow", "GET /a\n", "GET /a/longer\n"},
		{"shrink", "GET /a/longer\n", "GET /b\n"},
		{"same size", "GET /a\n", "GET /b\n"},
		{"to empty", "GET /a\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "req.http")
			if err := os.WriteFile(p, []byte(tt.old), 0o600); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "link.http")
			if err := os.Link(p, link); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}

			if err := WriteInPlace(p, []byte(tt.data), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := os.ReadFile(link)
			if err != nil || string(got) != tt.data {
				t.Fatalf("hard link reads %q, %v, want %q", got, err, tt.data)
			}
			after, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(before, after) {
				t.Error("file was replaced")
			}
			if runtime.GOOS != "windows" && after.Mode().Perm() != 0o600 {
				t.Errorf("mode = %v, want 0600", after.Mode().Perm())
			}
		})
	}
}

func TestWriteInPlaceNewFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "new.http")
	if err := WriteInPlace(p, []byte("GET /a\n"), 0o640); err != nil {
		t.Fatalf("write: %v", err)
	}
	ref := filepath.Join(dir, "ref.http")
	if err := os.WriteFile(ref, nil, 0o640); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(p)
	if err != nil || string(got) != "GET /a\n" {
		t.Fatalf("content = %q, %v, want GET /a", got, err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(ref)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode() != want.Mode() {
		t.Errorf("mode = %v, want %v like os.WriteFile", st.Mode(), want.Mode())
	}
}

func TestWriteInPlaceSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.http")
	if err := os.WriteFile(target, []byte("GET /old/path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.http")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := WriteInPlace(link, []byte("GET /new\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	st, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link mode = %v, want a symlink", st.Mode())
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "GET /new\n" {
		t.Fatalf("target = %q, %v, want GET /new", got, err)
	}
}

func TestWriteInPlaceDirectory(t *testing.T) {
	if err := WriteInPlace(t.TempDir(), []byte("x"), 0o644); err == nil {
		t.Fatal("expected an error for a directory")
	}
}

func TestWriteInPlaceReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write a read-only file")
	}
	p := filepath.Join(t.TempDir(), "req.http")
	if err := os.WriteFile(p, []byte("GET /a\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := WriteInPlace(p, []byte("GET /b\n"), 0o644); err == nil {
		t.Fatal("wrote a read-only file")
	}
	got, err := os.ReadFile(p)
	if err != nil || string(got) != "GET /a\n" {
		t.Fatalf("content = %q, %v, want it unchanged", got, err)
	}
}
