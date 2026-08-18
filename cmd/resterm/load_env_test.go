package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestLoadEnvironmentExplicitDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env.local")
	content := "workspace=local\nAPI_URL=http://localhost\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	cat, err := vars.LoadEnvironmentFile(envPath)
	if err != nil {
		t.Fatalf("load environment: %v", err)
	}
	env, err := cat.Resolve(cat.DefaultSelection())
	if err != nil {
		t.Fatalf("resolve environment: %v", err)
	}
	if env.Label() != "local" {
		t.Fatalf("expected local environment, got %v", cat.Names())
	}
	if env.Values()["API_URL"] != "http://localhost" {
		t.Fatalf("API_URL = %q, want %q", env.Values()["API_URL"], "http://localhost")
	}
}

func TestLoadEnvironmentIgnoresDotEnvDiscovery(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(
		envPath,
		[]byte("workspace=dev\nAPI_URL=https://api\n"),
		0o644,
	); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	cat, resolved, err := vars.Discover(dir)
	if err != nil {
		t.Fatalf("load environment: %v", err)
	}
	if !cat.Empty() {
		t.Fatalf("expected no auto-discovered envs, got %v", cat.Names())
	}
	if resolved != "" {
		t.Fatalf("resolved path = %q, want empty", resolved)
	}
}

func TestLoadEnvironmentDiscoveryReturnsInvalidFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resterm.env.json")
	if err := os.WriteFile(
		path,
		[]byte(`{"$groups":{"api":{"dev":{"token":"a"}},"auth":{"dev":{"TOKEN":"b"}}}}`),
		0o600,
	); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	_, resolved, err := vars.Discover(filepath.Dir(filepath.Join(dir, "api.http")), dir)
	if err == nil {
		t.Fatal("expected invalid discovered environment file to fail")
	}
	if resolved != path {
		t.Fatalf("resolved path = %q, want %q", resolved, path)
	}
	if !strings.Contains(err.Error(), "collides") {
		t.Fatalf("error = %v, want collision detail", err)
	}
}

func TestHandleInitSubcommandAmbiguousFile(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "init"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	handled, err := handleInitSubcommand([]string{"init"})
	if !handled {
		t.Fatalf("expected init to be handled")
	}
	if err == nil {
		t.Fatalf("expected ambiguity error")
	}
}

func TestRunInitListArgs(t *testing.T) {
	if err := runInit([]string{"--list", "extra"}); err == nil {
		t.Fatalf("expected error for extra args")
	}
}

func TestLoadEnvironmentDiscoverHTTPClientMergesPrivate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "http-client.env.json"),
		[]byte(`{"dev":{"host":"localhost","username":"public"}}`), 0o644); err != nil {
		t.Fatalf("write public env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "http-client.private.env.json"),
		[]byte(`{"dev":{"username":"private","password":"secret"}}`), 0o644); err != nil {
		t.Fatalf("write private env: %v", err)
	}

	cat, resolved, err := vars.Discover(dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if filepath.Base(resolved) != "http-client.env.json" {
		t.Fatalf("resolved = %q, want http-client.env.json", resolved)
	}
	env, err := cat.Resolve(cat.DefaultSelection())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	vals := env.Values()
	if vals["username"] != "private" {
		t.Fatalf("username = %q, want private", vals["username"])
	}
	if vals["password"] != "secret" {
		t.Fatalf("password = %q, want secret", vals["password"])
	}
}
