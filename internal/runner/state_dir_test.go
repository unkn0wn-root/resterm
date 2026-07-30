package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStatePathsSeparatesWorkspacesByDefault(t *testing.T) {
	alpha, err := resolveStatePaths(Options{PersistAuth: true}, "/projects/alpha")
	if err != nil {
		t.Fatal(err)
	}
	beta, err := resolveStatePaths(Options{PersistAuth: true}, "/projects/beta")
	if err != nil {
		t.Fatal(err)
	}
	if alpha.Auth == beta.Auth {
		t.Fatalf("both workspaces share the auth file %q", alpha.Auth)
	}
	if !strings.Contains(alpha.Root, "alpha") {
		t.Fatalf("state dir %q should be recognisable", alpha.Root)
	}
	// A workspace keeps one directory across runs, however its path is spelled.
	if defaultStateDir("testdata") != defaultStateDir(filepath.Join("testdata", ".")) {
		t.Fatal("equivalent spellings must resolve to the same directory")
	}
}

func TestResolveStatePathsHonoursExplicitStateDir(t *testing.T) {
	dir := t.TempDir()
	paths, err := resolveStatePaths(Options{PersistAuth: true, StateDir: dir}, "/projects/alpha")
	if err != nil {
		t.Fatal(err)
	}
	if paths.Root != dir {
		t.Fatalf("root = %q, want the explicit directory %q", paths.Root, dir)
	}
}

// The state directory has to follow the workspace root Build derives from the
// request file.
func TestBuildDerivesStateDirFromFile(t *testing.T) {
	base := t.TempDir()
	auth := make([]string, 0, 2)
	for _, ws := range []string{"alpha", "beta"} {
		dir := filepath.Join(base, ws)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "req.http")
		body := "### r\n# @name r\nGET http://example.test/x\n"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}

		plan, err := Build(Options{
			FilePath:    path,
			PersistAuth: true,
			Select:      Select{Request: "r"},
		})
		if err != nil {
			t.Fatalf("build %s: %v", ws, err)
		}
		if plan.state.Auth == "" {
			t.Fatalf("%s: no auth path", ws)
		}
		auth = append(auth, plan.state.Auth)
	}
	if auth[0] == auth[1] {
		t.Fatalf("both workspaces share the auth file %q", auth[0])
	}
}
