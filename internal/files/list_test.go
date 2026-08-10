package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListRequestsNonRecursive(t *testing.T) {
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

	entries, err := ListRequests(root, ListOptions{})
	if err != nil {
		t.Fatalf("ListRequests returned error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "a.http" {
		t.Fatalf("expected only top-level file, got %+v", entries)
	}
}

func TestListRequestsRecursive(t *testing.T) {
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

	entries, err := ListRequests(root, ListOptions{Recursive: true})
	if err != nil {
		t.Fatalf("ListRequests returned error: %v", err)
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

func TestListWorkspaceIncludesEnvJSON(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"a.http",
		"helpers.rts",
		"resterm.env.json",
		"query.graphql",
		"short.gql",
		"payload.json",
		"pre.js",
		"module.mjs",
		"common.cjs",
		"notes.txt",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(""), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	entries, err := ListWorkspace(root, ListOptions{})
	if err != nil {
		t.Fatalf("ListWorkspace returned error: %v", err)
	}

	got := make(map[string]Kind, len(entries))
	for _, entry := range entries {
		got[entry.Name] = entry.Kind
	}

	if got["a.http"] != KindRequest {
		t.Fatalf("expected a.http to be a request file, got %+v", entries)
	}
	if got["helpers.rts"] != KindScript {
		t.Fatalf("expected helpers.rts to be a script file, got %+v", entries)
	}
	if got["resterm.env.json"] != KindEnv {
		t.Fatalf("expected resterm.env.json to be an env file, got %+v", entries)
	}
	if got["query.graphql"] != KindGraphQL {
		t.Fatalf("expected query.graphql to be a graphql file, got %+v", entries)
	}
	if got["short.gql"] != KindGraphQL {
		t.Fatalf("expected short.gql to be a graphql file, got %+v", entries)
	}
	if got["payload.json"] != KindJSON {
		t.Fatalf("expected payload.json to be a json file, got %+v", entries)
	}
	if got["pre.js"] != KindJavaScript {
		t.Fatalf("expected pre.js to be a javascript file, got %+v", entries)
	}
	if got["module.mjs"] != KindJavaScript {
		t.Fatalf("expected module.mjs to be a javascript file, got %+v", entries)
	}
	if got["common.cjs"] != KindJavaScript {
		t.Fatalf("expected common.cjs to be a javascript file, got %+v", entries)
	}
	if _, ok := got["notes.txt"]; ok {
		t.Fatalf("did not expect notes.txt in workspace entries, got %+v", entries)
	}
}

func TestListWorkspaceIncludesExplicitEnvFileInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "config")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	envPath := filepath.Join(nested, ".env.local")
	if err := os.WriteFile(envPath, []byte("workspace=dev\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	entries, err := ListWorkspace(root, ListOptions{ExplicitEnvFile: envPath})
	if err != nil {
		t.Fatalf("ListWorkspace returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected only explicit env file, got %+v", entries)
	}

	entry := entries[0]
	if entry.Name != filepath.Join("config", ".env.local") {
		t.Fatalf("unexpected entry name %q", entry.Name)
	}
	if entry.Kind != KindEnv {
		t.Fatalf("expected env kind, got %+v", entry)
	}
}

func TestListWorkspaceSkipsExplicitEnvFileOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	envPath := filepath.Join(outsideDir, ".env.local")
	if err := os.WriteFile(envPath, []byte("workspace=dev\n"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	entries, err := ListWorkspace(root, ListOptions{Recursive: true, ExplicitEnvFile: envPath})
	if err != nil {
		t.Fatalf("ListWorkspace returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no entries for outside env file, got %+v", entries)
	}
}

// A workspace with one unreadable subdirectory must still list what it can.
// Root cannot read its own contents only when the root itself is broken, which
// stays fatal.
func TestListWorkspaceSkipsUnreadableSubdir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.http"), []byte(""), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	blocked := filepath.Join(root, "blocked")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "hidden.http"), []byte(""), 0o644); err != nil {
		t.Fatalf("write nested file: %v", err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	entries, err := ListWorkspace(root, ListOptions{Recursive: true})
	if err != nil {
		t.Fatalf("ListWorkspace returned error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "a.http" {
		t.Fatalf("expected the readable file only, got %+v", entries)
	}
}

func TestListWorkspaceFailsOnMissingRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := ListWorkspace(missing, ListOptions{Recursive: true}); err == nil {
		t.Fatal("expected an error for a missing root")
	}
	if _, err := ListWorkspace(missing, ListOptions{}); err == nil {
		t.Fatal("expected an error for a missing root without recursion")
	}
}
