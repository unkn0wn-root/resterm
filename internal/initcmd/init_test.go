package initcmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunStandardCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	op := Opt{Dir: dir, Template: "standard", Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}

	want := []string{
		fileRequests,
		fileEnv,
		fileEnvExample,
		fileHelp,
		fileRTSHelpers,
		gitignoreFile,
	}
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(data), "resterm.env.json") {
		t.Fatalf("expected resterm.env.json in .gitignore")
	}
}

func TestRunConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests.http")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	op := Opt{Dir: dir, Template: "minimal", Out: io.Discard}
	if err := Run(op); err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestRunConflictDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests.http")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	op := Opt{Dir: dir, Template: "minimal", Out: io.Discard}
	if err := Run(op); err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestRunForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "requests.http")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	op := Opt{Dir: dir, Template: "minimal", Force: true, Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.TrimSpace(string(data)) == "old" {
		t.Fatalf("expected overwrite")
	}
}

func TestRunDry(t *testing.T) {
	dir := t.TempDir()
	op := Opt{Dir: dir, Template: "minimal", DryRun: true, Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "requests.http")); !os.IsNotExist(err) {
		t.Fatalf("expected no files in dry-run")
	}
}

func TestRunNoGitignore(t *testing.T) {
	dir := t.TempDir()
	op := Opt{Dir: dir, Template: "minimal", NoGitignore: true, Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, gitignoreFile)); !os.IsNotExist(err) {
		t.Fatalf("expected no .gitignore when no-gitignore is set")
	}
}

func TestListTemplates(t *testing.T) {
	var buf bytes.Buffer
	if err := Run(Opt{List: true, Out: &buf}); err != nil {
		t.Fatalf("list: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "minimal") || !strings.Contains(out, "standard") {
		t.Fatalf("expected template names in output: %s", out)
	}
}

func TestHasGitignoreEntryWithComment(t *testing.T) {
	data := "resterm.env.json # local\n"
	if !hasGitignoreEntry(data, "resterm.env.json") {
		t.Fatalf("expected entry to match with trailing comment")
	}
}

func TestHasGitignoreEntryWithSlash(t *testing.T) {
	data := "/resterm.env.json\n"
	if !hasGitignoreEntry(data, "resterm.env.json") {
		t.Fatalf("expected entry to match with leading slash")
	}
}

func TestGitignoreAppendAddsNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte("node_modules"), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	op := Opt{Dir: dir, Template: "minimal", Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if string(data) != "node_modules\nresterm.env.json\n" {
		t.Fatalf("unexpected .gitignore content: %q", string(data))
	}
}

func TestGitignoreAppendPreservesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode checks are not reliable on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(path, []byte("node_modules\n"), 0o600); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}

	op := Opt{Dir: dir, Template: "minimal", Out: io.Discard}
	if err := Run(op); err != nil {
		t.Fatalf("run: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat .gitignore: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want %v", info.Mode().Perm(), 0o600)
	}
}
