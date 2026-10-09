package runner

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestWriteStateFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix file modes")
	}
	dir := filepath.Join(t.TempDir(), "state")
	p := filepath.Join(dir, "auth.json")
	mode := func(path string) os.FileMode {
		t.Helper()
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		return st.Mode().Perm()
	}
	if err := writeStateFile(p, authStateFile{Version: stateFileVersion}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := mode(dir); got != 0o700 {
		t.Fatalf("dir mode = %v, want 0700", got)
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := writeStateFile(p, authStateFile{Version: stateFileVersion}); err != nil {
		t.Fatalf("write again: %v", err)
	}
	if got := mode(p); got != 0o600 {
		t.Fatalf("file mode = %v, want 0600", got)
	}
	es, err := os.ReadDir(dir)
	if err != nil || len(es) != 1 {
		t.Fatalf("state dir holds %d entries, %v, want only auth.json", len(es), err)
	}
}
