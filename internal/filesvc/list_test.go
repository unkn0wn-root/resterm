package filesvc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileKindString(t *testing.T) {
	tests := []struct {
		name string
		kind FileKind
		want string
	}{
		{name: "request", kind: FileKindRequest, want: "request"},
		{name: "script", kind: FileKindScript, want: "script"},
		{name: "env", kind: FileKindEnv, want: "env"},
		{name: "unknown", kind: FileKind(99), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

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

func TestListWorkspaceFilesIncludesEnvJSON(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"a.http",
		"helpers.rts",
		"resterm.env.json",
		"notes.txt",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(""), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	entries, err := ListWorkspaceFiles(root, false, ListOptions{})
	if err != nil {
		t.Fatalf("ListWorkspaceFiles returned error: %v", err)
	}

	got := make(map[string]FileKind, len(entries))
	for _, entry := range entries {
		got[entry.Name] = entry.Kind
	}

	if got["a.http"] != FileKindRequest {
		t.Fatalf("expected a.http to be a request file, got %+v", entries)
	}
	if got["helpers.rts"] != FileKindScript {
		t.Fatalf("expected helpers.rts to be a script file, got %+v", entries)
	}
	if got["resterm.env.json"] != FileKindEnv {
		t.Fatalf("expected resterm.env.json to be an env file, got %+v", entries)
	}
	if _, ok := got["notes.txt"]; ok {
		t.Fatalf("did not expect notes.txt in workspace entries, got %+v", entries)
	}
}

func TestListWorkspaceFilesIncludesExplicitEnvFileInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "config")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	envPath := filepath.Join(nested, ".env.local")
	if err := os.WriteFile(envPath, []byte("workspace=dev\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	entries, err := ListWorkspaceFiles(root, false, ListOptions{ExplicitEnvFile: envPath})
	if err != nil {
		t.Fatalf("ListWorkspaceFiles returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected only explicit env file, got %+v", entries)
	}

	entry := entries[0]
	if entry.Name != filepath.Join("config", ".env.local") {
		t.Fatalf("unexpected entry name %q", entry.Name)
	}
	if entry.Kind != FileKindEnv {
		t.Fatalf("expected env kind, got %+v", entry)
	}
}

func TestListWorkspaceFilesSkipsExplicitEnvFileOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	envPath := filepath.Join(outsideDir, ".env.local")
	if err := os.WriteFile(envPath, []byte("workspace=dev\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	entries, err := ListWorkspaceFiles(root, true, ListOptions{ExplicitEnvFile: envPath})
	if err != nil {
		t.Fatalf("ListWorkspaceFiles returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries for outside env file, got %+v", entries)
	}
}
