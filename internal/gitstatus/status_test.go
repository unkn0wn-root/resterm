package gitstatus

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePorcelainV2(t *testing.T) {
	repo := canonicalPath(t.TempDir())
	out := joinRecords(
		"# branch.head main",
		"# branch.ab +2 -1",
		"1 .M N... 100644 100644 100644 abc abc api.http",
		"1 A. N... 000000 100644 100644 abc abc created.http",
		"1 .D N... 100644 100644 000000 abc abc removed.http",
		"2 R. N... 100644 100644 100644 abc abc R100 renamed.http",
		"old.http",
		"u UU N... 100644 100644 100644 100644 abc def ghi conflict.http",
		"? payload.json",
	)

	snap := parsePorcelainV2(repo, out)
	if snap.Branch != "main" {
		t.Fatalf("expected branch main, got %q", snap.Branch)
	}
	if snap.Ahead != 2 || snap.Behind != 1 {
		t.Fatalf("expected ahead/behind 2/1, got %d/%d", snap.Ahead, snap.Behind)
	}

	tests := map[string]Status{
		"api.http":      StatusModified,
		"created.http":  StatusAdded,
		"removed.http":  StatusDeleted,
		"renamed.http":  StatusRenamed,
		"conflict.http": StatusConflict,
		"payload.json":  StatusUntracked,
	}
	for name, want := range tests {
		got, ok := snap.File(filepath.Join(repo, name))
		if !ok {
			t.Fatalf("expected status for %s", name)
		}
		if got.Status != want {
			t.Fatalf("expected %s status %v, got %v", name, want, got.Status)
		}
	}

	counts := snap.Counts()
	if counts.Modified != 1 ||
		counts.Added != 1 ||
		counts.Deleted != 1 ||
		counts.Renamed != 1 ||
		counts.Conflict != 1 ||
		counts.Untracked != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
}

func TestParsePorcelainV2KeepsHighestPriorityForPath(t *testing.T) {
	repo := canonicalPath(t.TempDir())
	out := joinRecords(
		"? api.http",
		"1 .D N... 100644 100644 000000 abc abc api.http",
	)

	snap := parsePorcelainV2(repo, out)
	got, ok := snap.File(filepath.Join(repo, "api.http"))
	if !ok {
		t.Fatalf("expected status for api.http")
	}
	if got.Status != StatusDeleted {
		t.Fatalf("expected deleted to win, got %v", got.Status)
	}
}

func TestLoadLimitsStatusToProvidedPaths(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.name", "Resterm Test")
	runGit(t, root, "config", "user.email", "resterm@example.test")

	api := filepath.Join(root, "api.http")
	env := filepath.Join(root, "resterm.env.json")
	readme := filepath.Join(root, "README.md")
	writeFile(t, api, "GET https://example.com\n")
	writeFile(t, env, "{}\n")
	writeFile(t, readme, "hello\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")

	payload := filepath.Join(root, "payload.json")
	notes := filepath.Join(root, "notes.txt")
	writeFile(t, api, "GET https://changed.example\n")
	writeFile(t, readme, "changed\n")
	writeFile(t, payload, "{}\n")
	writeFile(t, notes, "ignored by resterm\n")

	snap, err := Load(context.Background(), root, []string{api, env, payload})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if snap.RepoRoot == "" {
		t.Fatalf("expected repository snapshot")
	}

	if got, ok := snap.File(api); !ok || got.Status != StatusModified {
		t.Fatalf("expected modified api.http, got %+v present=%v", got, ok)
	}
	if got, ok := snap.File(payload); !ok || got.Status != StatusUntracked {
		t.Fatalf("expected untracked payload.json, got %+v present=%v", got, ok)
	}
	if _, ok := snap.File(readme); ok {
		t.Fatalf("did not expect unsupported README.md status")
	}
	if _, ok := snap.File(notes); ok {
		t.Fatalf("did not expect unsupported notes.txt status")
	}
	if _, ok := snap.File(env); ok {
		t.Fatalf("did not expect clean env file status")
	}
}

func TestLoadOutsideRepositoryReturnsEmptySnapshot(t *testing.T) {
	requireGit(t)
	root := t.TempDir()

	snap, err := Load(context.Background(), root, []string{filepath.Join(root, "api.http")})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if snap.RepoRoot != "" || len(snap.Files) != 0 {
		t.Fatalf("expected empty snapshot outside repository, got %+v", snap)
	}
}

func joinRecords(records ...string) string {
	var out strings.Builder
	for _, record := range records {
		out.WriteString(record + "\x00")
	}
	return out.String()
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
