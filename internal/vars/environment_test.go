package vars

import (
	"errors"
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

	cat, err := LoadEnvironmentFile(path)
	if err != nil {
		t.Fatalf("load env: %v", err)
	}

	dev := resolveValues(t, cat, "dev")
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

	cat, err := LoadEnvironmentFile(path)
	if err != nil {
		t.Fatalf("load env: %v", err)
	}

	dev := resolveValues(t, cat, "dev")
	if dev["api.version"] != "v2" {
		t.Fatalf("dev should inherit api.version from $shared, got %q", dev["api.version"])
	}
	if dev["auth.clientId"] != "demo-client" {
		t.Fatalf("dev should inherit auth.clientId from $shared, got %q", dev["auth.clientId"])
	}
	if dev["base.url"] != "https://dev.example.com" {
		t.Fatalf("dev base.url wrong, got %q", dev["base.url"])
	}

	prod := resolveValues(t, cat, "prod")
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

func TestCatalogDefaultSelection(t *testing.T) {
	tests := []struct {
		name string
		set  EnvironmentSet
		want string
	}{
		{
			name: "empty",
			set:  nil,
			want: "",
		},
		{
			name: "prefer dev",
			set: EnvironmentSet{
				"stage": {},
				"dev":   {},
			},
			want: "dev",
		},
		{
			name: "prefer default",
			set: EnvironmentSet{
				"prod":    {},
				"default": {},
			},
			want: "default",
		},
		{
			name: "prefer local",
			set: EnvironmentSet{
				"prod":  {},
				"local": {},
			},
			want: "local",
		},
		{
			name: "sorted fallback",
			set: EnvironmentSet{
				"stage": {},
				"alpha": {},
			},
			want: "alpha",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cat, err := NewCatalog(tc.set)
			if err != nil {
				t.Fatalf("new catalog: %v", err)
			}
			if got := cat.DefaultSelection().Name(); got != tc.want {
				t.Fatalf("DefaultSelection().Name() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCatalogSelect(t *testing.T) {
	cat, err := NewCatalog(EnvironmentSet{"dev": {"base.url": "https://api.dev"}})
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	sel, err := cat.Select("  dev  ", nil)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if got := sel.Name(); got != "dev" {
		t.Fatalf("selection name = %q, want dev", got)
	}

	if _, err := cat.Select("stage", nil); err == nil {
		t.Fatal("expected unknown environment to be rejected")
	}
	if _, err := cat.Select("dev", map[string]string{"api": "dev"}); err == nil {
		t.Fatal("expected group selection against flat catalog to be rejected")
	}

	empty := Catalog{}
	sel, err = empty.Select("adhoc", nil)
	if err != nil {
		t.Fatalf("select on empty catalog: %v", err)
	}
	if got := sel.Name(); got != "adhoc" {
		t.Fatalf("empty catalog selection name = %q, want adhoc", got)
	}
}

func TestGroupedCatalogSelect(t *testing.T) {
	cat, err := NewGroupedCatalog(nil, []Group{{
		Name:     "api",
		Profiles: EnvironmentSet{"dev": {"url": "d"}, "prod": {"url": "p"}},
		Default:  "dev",
	}})
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}

	if _, err := cat.Select("", map[string]string{"missing": "dev"}); err == nil {
		t.Fatal("expected unknown group to be rejected")
	}
	if _, err := cat.Select("", map[string]string{"api": "dev", "API": "prod"}); err == nil {
		t.Fatal("expected duplicate group keys to be rejected")
	}
	if _, err := cat.Select("dev", nil); err == nil {
		t.Fatal("expected environment name against grouped catalog to be rejected")
	}
}

// History replay tells a stale saved selection from any other failure with
// errors.Is, so every name that no longer resolves has to carry the sentinel.
func TestUnknownSelectionIsSentinel(t *testing.T) {
	flat, err := NewCatalog(EnvironmentSet{"dev": {"url": "d"}, "prod": {"url": "p"}})
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}
	grouped, err := NewGroupedCatalog(nil, []Group{{
		Name:     "api",
		Profiles: EnvironmentSet{"dev": {"url": "d"}},
		Default:  "dev",
	}})
	if err != nil {
		t.Fatalf("new grouped catalog: %v", err)
	}

	cases := []struct {
		name string
		err  error
	}{
		{"unknown environment", firstErr(flat.Select("stage", nil))},
		{"unknown group", firstErr(grouped.Select("", map[string]string{"missing": "dev"}))},
		{"unknown profile", firstErr(grouped.Select("", map[string]string{"api": "gone"}))},
		{"unknown compare group", compareErr(grouped, "missing")},
	}
	for _, tc := range cases {
		if tc.err == nil {
			t.Fatalf("%s: expected an error", tc.name)
		}
		if !errors.Is(tc.err, ErrUnknownSelection) {
			t.Fatalf("%s: %v does not match ErrUnknownSelection", tc.name, tc.err)
		}
		if strings.Contains(tc.err.Error(), ErrUnknownSelection.Error()) {
			t.Fatalf("%s: sentinel text leaked into message %q", tc.name, tc.err)
		}
	}

	// A malformed request is not a stale selection.
	_, err = flat.Select("dev", map[string]string{"api": "dev"})
	if err == nil || errors.Is(err, ErrUnknownSelection) {
		t.Fatalf("group selection against a flat catalog = %v, want a non-sentinel error", err)
	}
}

func firstErr(_ Selection, err error) error {
	return err
}

func compareErr(cat Catalog, group string) error {
	_, err := cat.CompareTargets(cat.DefaultSelection(), group, "", []string{"dev", "prod"})
	return err
}

func resolveValues(t *testing.T, cat Catalog, name string) map[string]string {
	t.Helper()
	sel, err := cat.Select(name, nil)
	if err != nil {
		t.Fatalf("select %q: %v", name, err)
	}
	env, err := cat.Resolve(sel)
	if err != nil {
		t.Fatalf("resolve %q: %v", name, err)
	}
	return env.Values()
}
