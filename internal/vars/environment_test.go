package vars

import (
	"os"
	"path/filepath"
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
