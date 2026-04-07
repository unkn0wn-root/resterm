package vars

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEnvironmentFileFlattensNestedObjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	data := []byte(`{
  "dev": {
    "base": {
      "url": "https://api.dev",
      "headers": {
        "auth": "token"
      }
    },
    "timeout": 30,
    "enabled": true,
    "tags": ["alpha", "beta"],
    "empty": null
  }
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	envs, err := LoadEnvironmentFile(path)
	if err != nil {
		t.Fatalf("load env: %v", err)
	}

	dev := envs["dev"]
	if dev["base.url"] != "https://api.dev" {
		t.Fatalf("expected base.url to be flattened, got %q", dev["base.url"])
	}
	if dev["base.headers.auth"] != "token" {
		t.Fatalf("expected nested headers to flatten, got %q", dev["base.headers.auth"])
	}
	if dev["timeout"] != "30" {
		t.Fatalf("expected timeout to stringify, got %q", dev["timeout"])
	}
	if dev["enabled"] != "true" {
		t.Fatalf("expected enabled to stringify, got %q", dev["enabled"])
	}
	if dev["tags[0]"] != "alpha" || dev["tags[1]"] != "beta" {
		t.Fatalf("expected array elements to flatten, got %q %q", dev["tags[0]"], dev["tags[1]"])
	}
	if dev["empty"] != "" {
		t.Fatalf("expected null to become empty string, got %q", dev["empty"])
	}
}

func TestSharedMergesIntoAllEnvironments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	data := []byte(`{
  "$shared": {
    "api": { "version": "v2" },
    "auth": { "clientId": "demo-client" }
  },
  "dev": {
    "base": { "url": "https://dev.example.com" }
  },
  "prod": {
    "base": { "url": "https://prod.example.com" },
    "auth": { "clientId": "prod-client" }
  }
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	envs, err := LoadEnvironmentFile(path)
	if err != nil {
		t.Fatalf("load env: %v", err)
	}

	// $shared must be removed from the set.
	if _, ok := envs[SharedEnvKey]; ok {
		t.Fatal("$shared should not appear in the returned EnvironmentSet")
	}

	// dev inherits shared values.
	dev := envs["dev"]
	if dev["api.version"] != "v2" {
		t.Fatalf("dev should inherit api.version from $shared, got %q", dev["api.version"])
	}
	if dev["auth.clientId"] != "demo-client" {
		t.Fatalf("dev should inherit auth.clientId from $shared, got %q", dev["auth.clientId"])
	}
	if dev["base.url"] != "https://dev.example.com" {
		t.Fatalf("dev base.url wrong, got %q", dev["base.url"])
	}

	// prod overrides auth.clientId but inherits api.version.
	prod := envs["prod"]
	if prod["api.version"] != "v2" {
		t.Fatalf("prod should inherit api.version from $shared, got %q", prod["api.version"])
	}
	if prod["auth.clientId"] != "prod-client" {
		t.Fatalf("prod should override auth.clientId, got %q", prod["auth.clientId"])
	}
}

func TestLoadEnvironmentFileOnlySharedReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env.json")
	data := []byte(`{
  "$shared": {
    "base": { "url": "https://api.example.com" }
  }
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	_, err := LoadEnvironmentFile(path)
	if err == nil {
		t.Fatalf("expected parse error for env file containing only $shared")
	}
	if !strings.Contains(err.Error(), `defines only "$shared"`) {
		t.Fatalf("expected only-shared parse error, got %v", err)
	}
}

func TestIsReservedEnvironmentTrimsWhitespace(t *testing.T) {
	if !IsReservedEnvironment("  $shared\t") {
		t.Fatal("expected trimmed reserved environment name to be recognized")
	}
}

func TestSelectEnvTrimsInputs(t *testing.T) {
	set := EnvironmentSet{"dev": {"base.url": "https://api.dev"}}

	if got := SelectEnv(set, "  stage  ", "dev"); got != "stage" {
		t.Fatalf("SelectEnv override = %q, want %q", got, "stage")
	}
	if got := SelectEnv(set, "", "  dev  "); got != "dev" {
		t.Fatalf("SelectEnv current = %q, want %q", got, "dev")
	}
}

func TestEnvValuesTrimsName(t *testing.T) {
	set := EnvironmentSet{"dev": {"base.url": "https://api.dev"}}

	env := EnvValues(set, "  dev  ")
	if env == nil {
		t.Fatal("expected trimmed environment lookup to succeed")
	}
	if env["base.url"] != "https://api.dev" {
		t.Fatalf("unexpected env values: %#v", env)
	}
}
