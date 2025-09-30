package filesvc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListRequestFilesNonRecursive(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.http"), []byte(""), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.rest"), []byte(""), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	entries, err := ListRequestFiles(root, false)
	if err != nil {
		t.Fatalf("ListRequestFiles returned error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "a.http" {
		t.Fatalf("expected only top-level file, got %+v", entries)
	}
}

func TestListRequestFilesRecursive(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.http"), []byte(""), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.rest"), []byte(""), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	entries, err := ListRequestFiles(root, true)
	if err != nil {
		t.Fatalf("ListRequestFiles returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected both files, got %+v", entries)
	}
	paths := map[string]bool{}
	for _, entry := range entries {
		paths[entry.Name] = true
	}
	if !paths["a.http"] || !paths[filepath.Join("sub", "nested.rest")] {
		t.Fatalf("expected recursive entries, got %+v", entries)
	}
}
