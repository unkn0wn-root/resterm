package launch

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSpoolWrite(t *testing.T) {
	s := NewSpool(t.TempDir())
	t.Cleanup(func() { _ = s.Close() })
	data := []byte("%PDF-1.7")

	path, err := s.Write("invoice.pdf", data)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if filepath.Base(path) != "invoice.pdf" {
		t.Fatalf("Write() path = %q, want the exact name", path)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("file = %q, %v, want %q", got, err, data)
	}
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("file mode = %o, want 600", mode)
	}
}

func TestSpoolKeepsSameNamesApart(t *testing.T) {
	s := NewSpool(t.TempDir())
	t.Cleanup(func() { _ = s.Close() })

	first, err := s.Write("response.json", []byte(`{"n":1}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Write("response.json", []byte(`{"n":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("Write() reused %q", first)
	}
	if got, _ := os.ReadFile(first); string(got) != `{"n":1}` {
		t.Fatalf("first file = %q, overwritten", got)
	}
}

func TestSpoolCloseRemovesFiles(t *testing.T) {
	parent := t.TempDir()
	s := NewSpool(parent)
	path, err := s.Write("image.png", []byte("png"))
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat() after Close error = %v, want not exist", err)
	}
	if _, err := s.Write("late.png", []byte("png")); err == nil {
		t.Fatal("Write() after Close succeeded")
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Fatalf("parent keeps %d entries after Close", len(entries))
	}
}

func TestSpoolCreatesNothingUntilWrite(t *testing.T) {
	parent := t.TempDir()
	s := NewSpool(parent)
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Fatalf("NewSpool() created %d entries", len(entries))
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
