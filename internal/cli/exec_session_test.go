package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const brokenEnvFile = `{"dev":{"token":"env:"},"prod":{"token":"env:PROD"}}`

func TestResolveSessionOpensOnABrokenEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "resterm.env.json")
	if err := os.WriteFile(envFile, []byte(brokenEnvFile), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	flags := NewExecFlags()
	flags.Workspace = dir
	cfg, err := flags.ResolveSession(filepath.Join(dir, "api.http"))
	if err != nil {
		t.Fatalf("ResolveSession() error = %v, want the session to open", err)
	}
	if cfg.Env.FileErr == nil {
		t.Fatal("FileErr is nil, want the reason the file did not load")
	}
	if got := cfg.Env.FileErr.Error(); !strings.Contains(got, "env: reference with no variable name") {
		t.Fatalf("FileErr = %q, want it to name the problem", got)
	}
	if cfg.Env.File != envFile {
		t.Fatalf("File = %q, want %q so the session can name the file to fix", cfg.Env.File, envFile)
	}
	if !cfg.Env.Catalog.Empty() {
		t.Fatalf("catalog = %v, want nothing loaded", cfg.Env.Catalog.Names())
	}
}

func TestResolveStopsOnABrokenEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "resterm.env.json")
	if err := os.WriteFile(envFile, []byte(brokenEnvFile), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	flags := NewExecFlags()
	flags.Workspace = dir
	if _, err := flags.Resolve(filepath.Join(dir, "api.http")); err == nil {
		t.Fatal("Resolve() ran on an environment file that did not load")
	}
}

func TestResolveSessionKeepsSelectionFlagsOnABrokenEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "resterm.env.json")
	if err := os.WriteFile(envFile, []byte(`{"$groups":{"api":{"dev":{"a":"env:"}}}}`), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	flags := NewExecFlags()
	flags.EnvFile = envFile
	flags.EnvGroups = GroupFlags{"api": "dev"}
	flags.CompareTargetsRaw = "dev,prod"
	flags.CompareGroup = "api"
	cfg, err := flags.ResolveSession(filepath.Join(dir, "api.http"))
	if err != nil {
		t.Fatalf("ResolveSession() error = %v, want the session to open", err)
	}
	if got := cfg.Env.Intent.Groups["api"]; got != "dev" {
		t.Fatalf("intent group = %q, want the flag kept for the reload", got)
	}
	if cfg.Env.File != envFile {
		t.Fatalf("File = %q, want %q", cfg.Env.File, envFile)
	}
	if len(cfg.Compare.Targets) != 2 {
		t.Fatalf("compare targets = %v, want both kept", cfg.Compare.Targets)
	}
}

func TestResolveSessionStopsOnAnUnreadableEnvFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "resterm.env.json"), 0o755); err != nil {
		t.Fatalf("create directory: %v", err)
	}

	for _, tc := range []struct {
		name    string
		envFile string
	}{
		{name: "discovered", envFile: ""},
		{name: "passed with --env-file", envFile: filepath.Join(dir, "resterm.env.json")},
		{name: "missing", envFile: filepath.Join(dir, "typo.env.json")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := NewExecFlags()
			flags.Workspace = dir
			flags.EnvFile = tc.envFile
			if _, err := flags.ResolveSession(filepath.Join(dir, "api.http")); err == nil {
				t.Fatal("ResolveSession() opened on an environment file it could not read")
			}
		})
	}
}

func TestResolveSessionStopsOnFlagErrors(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "resterm.env.json")
	if err := os.WriteFile(envFile, []byte(brokenEnvFile), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	flags := NewExecFlags()
	flags.Workspace = dir
	flags.CompareGroup = "api"
	if _, err := flags.ResolveSession(filepath.Join(dir, "api.http")); err == nil {
		t.Fatal("ResolveSession() accepted --compare-group without --compare")
	}
}
