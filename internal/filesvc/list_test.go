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
		{name: "graphql", kind: FileKindGraphQL, want: "graphql"},
		{name: "json", kind: FileKindJSON, want: "json"},
		{name: "javascript", kind: FileKindJavaScript, want: "javascript"},
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

func TestFileKindBadgeLabel(t *testing.T) {
	tests := []struct {
		kind FileKind
		want string
	}{
		{kind: FileKindRequest, want: ""},
		{kind: FileKindScript, want: "RTS"},
		{kind: FileKindEnv, want: "ENV"},
		{kind: FileKindGraphQL, want: "GQL"},
		{kind: FileKindJSON, want: "JSON"},
		{kind: FileKindJavaScript, want: "JS"},
		{kind: FileKind(99), want: ""},
	}

	for _, tt := range tests {
		if got := tt.kind.BadgeLabel(); got != tt.want {
			t.Fatalf("BadgeLabel(%v) = %q, want %q", tt.kind, got, tt.want)
		}
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
	if got["query.graphql"] != FileKindGraphQL {
		t.Fatalf("expected query.graphql to be a graphql file, got %+v", entries)
	}
	if got["short.gql"] != FileKindGraphQL {
		t.Fatalf("expected short.gql to be a graphql file, got %+v", entries)
	}
	if got["payload.json"] != FileKindJSON {
		t.Fatalf("expected payload.json to be a json file, got %+v", entries)
	}
	if got["pre.js"] != FileKindJavaScript {
		t.Fatalf("expected pre.js to be a javascript file, got %+v", entries)
	}
	if got["module.mjs"] != FileKindJavaScript {
		t.Fatalf("expected module.mjs to be a javascript file, got %+v", entries)
	}
	if got["common.cjs"] != FileKindJavaScript {
		t.Fatalf("expected common.cjs to be a javascript file, got %+v", entries)
	}
	if _, ok := got["notes.txt"]; ok {
		t.Fatalf("did not expect notes.txt in workspace entries, got %+v", entries)
	}
}

func TestClassifyWorkspacePathPrecedence(t *testing.T) {
	tests := []struct {
		path string
		want FileKind
		ok   bool
	}{
		{path: "requests.http", want: FileKindRequest, ok: true},
		{path: "requests.rest", want: FileKindRequest, ok: true},
		{path: "helpers.rts", want: FileKindScript, ok: true},
		{path: "resterm.env.json", want: FileKindEnv, ok: true},
		{path: "rest-client.env.json", want: FileKindEnv, ok: true},
		{path: "payload.json", want: FileKindJSON, ok: true},
		{path: "query.graphql", want: FileKindGraphQL, ok: true},
		{path: "query.gql", want: FileKindGraphQL, ok: true},
		{path: "pre.js", want: FileKindJavaScript, ok: true},
		{path: "pre.mjs", want: FileKindJavaScript, ok: true},
		{path: "pre.cjs", want: FileKindJavaScript, ok: true},
		{path: "notes.txt", ok: false},
	}

	for _, tt := range tests {
		got, ok := ClassifyWorkspacePath(tt.path)
		if ok != tt.ok {
			t.Fatalf("ClassifyWorkspacePath(%q) ok = %v, want %v", tt.path, ok, tt.ok)
		}
		if got != tt.want {
			t.Fatalf("ClassifyWorkspacePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
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
