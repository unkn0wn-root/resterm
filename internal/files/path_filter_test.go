package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathFilter(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "active.custom")
	if err := os.WriteFile(env, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		filter PathFilter
		path   string
		want   bool
	}{
		{"zero rejects files", PathFilter{}, filepath.Join(dir, "request.http"), false},
		{"any accepts files", AnyPathFilter(), filepath.Join(dir, "notes.txt"), true},
		{"request accepts HTTP", RequestPathFilter(), filepath.Join(dir, "request.http"), true},
		{"request rejects JSON", RequestPathFilter(), filepath.Join(dir, "data.json"), false},
		{"workspace accepts request", WorkspacePathFilter(env), filepath.Join(dir, "request.rest"), true},
		{"workspace accepts data", WorkspacePathFilter(env), filepath.Join(dir, "query.graphql"), true},
		{"workspace accepts dotenv", WorkspacePathFilter(env), filepath.Join(dir, ".env.local"), true},
		{"workspace accepts active env", WorkspacePathFilter(env), env, true},
		{"workspace rejects unrelated", WorkspacePathFilter(env), filepath.Join(dir, "notes.txt"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.Accept(tt.path); got != tt.want {
				t.Fatalf("Accept(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestWorkspacePathFilterAcceptsActiveEnvAlias(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "active.custom")
	alias := filepath.Join(dir, "alias.custom")
	if err := os.WriteFile(env, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(env, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if !WorkspacePathFilter(env).Accept(alias) {
		t.Fatal("workspace filter rejected a symlink to the active environment file")
	}
}
