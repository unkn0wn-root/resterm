package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvironmentExplicitDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env.local")
	content := "workspace=local\nAPI_URL=http://localhost\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	envs, resolved := loadEnvironment(envPath, "", dir)
	if resolved != envPath {
		t.Fatalf("resolved path = %q, want %q", resolved, envPath)
	}
	env := envs["local"]
	if env == nil {
		t.Fatalf("expected local environment, got %v", envs)
	}
	if env["API_URL"] != "http://localhost" {
		t.Fatalf("API_URL = %q, want %q", env["API_URL"], "http://localhost")
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

	envs, resolved := loadEnvironment("", "", dir)
	if envs != nil {
		t.Fatalf("expected no auto-discovered envs, got %v", envs)
	}
	if resolved != "" {
		t.Fatalf("resolved path = %q, want empty", resolved)
	}
}
