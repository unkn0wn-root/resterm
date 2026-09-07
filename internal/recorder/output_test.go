package recorder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOutputRefusesExistingAndSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.http")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateOutput(path); err == nil {
		t.Fatal("existing output overwritten")
	}
	link := filepath.Join(dir, "link.http")
	if err := os.Symlink(path, link); err != nil {
		t.Skip(err)
	}
	if _, err := CreateOutput(link); err == nil {
		t.Fatal("symlink output accepted")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "keep" {
		t.Fatal("existing file changed")
	}
}
