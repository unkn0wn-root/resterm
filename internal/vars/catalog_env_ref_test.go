package vars

import (
	"strings"
	"testing"
)

func TestCatalogRejectsEnvRefWithNoName(t *testing.T) {
	t.Run("environment", func(t *testing.T) {
		_, err := NewCatalog(EnvironmentSet{"dev": {"token": "env:"}})
		assertEnvRefError(t, err, `dev: "token"`)
	})

	t.Run("shared", func(t *testing.T) {
		_, err := NewCatalog(EnvironmentSet{
			"dev":        {"ok": "value"},
			SharedEnvKey: {"token": "env:   "},
		})
		assertEnvRefError(t, err, `$shared: "token"`)
	})

	t.Run("grouped profile", func(t *testing.T) {
		_, err := NewGroupedCatalog(nil, []Group{{
			Name:     "region",
			Default:  "eu",
			Profiles: EnvironmentSet{"eu": {"token": "env:"}},
		}})
		assertEnvRefError(t, err, `region.eu: "token"`)
	})

	t.Run("valid references load", func(t *testing.T) {
		if _, err := NewCatalog(EnvironmentSet{"dev": {
			"a": "env:NAME",
			"b": "env:{{picked}}",
			"c": "plain",
		}}); err != nil {
			t.Fatalf("NewCatalog() error = %v, want the catalog to load", err)
		}
	})
}

func assertEnvRefError(t *testing.T, err error, scope string) {
	t.Helper()
	if err == nil {
		t.Fatal("the catalog loaded a reference with no variable name")
	}
	if !strings.Contains(err.Error(), scope) {
		t.Fatalf("error = %q, want it to name %s", err, scope)
	}
	if !strings.Contains(err.Error(), "env: reference with no variable name") {
		t.Fatalf("error = %q, want it to name the problem", err)
	}
}
