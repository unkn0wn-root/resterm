package vars

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeEnvFile(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "resterm.env.json")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func scopeOfFile(t *testing.T, path string) (string, Environment) {
	t.Helper()
	cat, err := LoadEnvironmentFile(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	env, err := cat.Resolve(cat.DefaultSelection())
	if err != nil {
		t.Fatalf("resolve %s: %v", path, err)
	}
	return env.Scope(), env
}

// Two projects both calling an environment "dev" must not share the runtime
// values keyed by scope. This is the collision the scope source exists to break.
func TestFlatScopeSeparatesEnvironmentFiles(t *testing.T) {
	base := t.TempDir()
	a := writeEnvFile(t, filepath.Join(base, "A"), `{"dev":{"who":"A"}}`)
	b := writeEnvFile(t, filepath.Join(base, "B"), `{"dev":{"who":"B"}}`)

	scopeA, envA := scopeOfFile(t, a)
	scopeB, envB := scopeOfFile(t, b)

	if envA.Label() != "dev" || envB.Label() != "dev" {
		t.Fatalf("labels = %q and %q, want both dev", envA.Label(), envB.Label())
	}
	if scopeA == scopeB {
		t.Fatalf("both files resolved to scope %q", scopeA)
	}
	if !strings.HasPrefix(scopeA, scopeFlatSourceVersion+"dev@") {
		t.Fatalf("scope = %q, want the environment name to stay readable", scopeA)
	}

	// One file used by two workspaces is deliberate sharing, so the scope has to
	// match. This is why the identity is the file rather than the workspace.
	again, _ := scopeOfFile(t, filepath.Join(filepath.Dir(a), ".", "resterm.env.json"))
	if again != scopeA {
		t.Fatalf("scopes %q and %q differ for one file", scopeA, again)
	}
}

func TestGroupedScopeSeparatesEnvironmentFiles(t *testing.T) {
	body := `{"$groups":{"api":{"$default":"dev","dev":{"api.url":"x"},"prod":{"api.url":"y"}}}}`
	base := t.TempDir()
	scopeA, _ := scopeOfFile(t, writeEnvFile(t, filepath.Join(base, "A"), body))
	scopeB, _ := scopeOfFile(t, writeEnvFile(t, filepath.Join(base, "B"), body))

	if scopeA == scopeB {
		t.Fatalf("identical group definitions in two files shared scope %q", scopeA)
	}
	for _, scope := range []string{scopeA, scopeB} {
		if !strings.HasPrefix(scope, scopeSourceVersion) {
			t.Fatalf("scope = %q, want the %s prefix", scope, scopeSourceVersion)
		}
	}
}

// A scope decides whether runtime state may cross a workspace boundary, so
// every scope carries a version prefix even without a file to name. A bare
// name could pose as any other scope kind.
func TestUnsourcedScopesAreVersioned(t *testing.T) {
	cat, err := NewCatalog(EnvironmentSet{"dev": {"who": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	env, err := cat.Resolve(cat.DefaultSelection())
	if err != nil {
		t.Fatal(err)
	}
	if got := env.Scope(); got != scopeFlatVersion+"dev" {
		t.Fatalf("scope = %q, want the prefixed environment name", got)
	}

	grouped, err := NewGroupedCatalog(nil, []Group{{
		Name:     "api",
		Default:  "dev",
		Profiles: EnvironmentSet{"dev": {"api.url": "x"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	genv, err := grouped.Resolve(grouped.DefaultSelection())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(genv.Scope(), scopeVersion) {
		t.Fatalf("scope = %q, want the %s prefix", genv.Scope(), scopeVersion)
	}
}

// Provenance is read from the version prefix alone. An environment name sits
// behind the prefix, so a name shaped like another scope kind changes nothing,
// and scopes from before the flat prefixes read as not sourced.
func TestSourcedScopeClassification(t *testing.T) {
	base := t.TempDir()
	flat, _ := scopeOfFile(t, writeEnvFile(t, filepath.Join(base, "flat"), `{"dev":{"who":"x"}}`))
	grouped, _ := scopeOfFile(t, writeEnvFile(
		t,
		filepath.Join(base, "grouped"),
		`{"$groups":{"api":{"$default":"dev","dev":{"api.url":"x"}}}}`,
	))

	posing, err := NewCatalog(EnvironmentSet{"g2:anything": {"who": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	posingEnv, err := posing.Resolve(posing.DefaultSelection())
	if err != nil {
		t.Fatal(err)
	}

	for scope, want := range map[string]bool{
		flat:    true,
		grouped: true,
		flatScope("g1:dev", "/a/resterm.env.json"): true,
		posingEnv.Scope():                          false,
		"":                                         false,
		"dev":                                      false,
		"dev@0123456789abcdef":                     false,
	} {
		if got := SourcedScope(scope); got != want {
			t.Fatalf("SourcedScope(%q) = %v, want %v", scope, got, want)
		}
	}
}

// The digest is a fixed width suffix, so no two (name, source) pairs can render
// the same scope even when a name itself contains the separator.
func TestFlatScopeIsUnambiguous(t *testing.T) {
	seen := map[string]string{}
	for _, tc := range []struct{ name, source string }{
		{"dev", "/a/resterm.env.json"},
		{"dev", "/b/resterm.env.json"},
		{"dev@0123456789abcdef", "/a/resterm.env.json"},
		{"", "/a/resterm.env.json"},
	} {
		got := flatScope(tc.name, tc.source)
		if prev, ok := seen[got]; ok {
			t.Fatalf("scope %q produced by both %q and %q", got, prev, tc.name+"|"+tc.source)
		}
		seen[got] = tc.name + "|" + tc.source
		if suffix := got[len(got)-scopeDigestLen:]; suffix != sourceDigest(tc.source) {
			t.Fatalf("scope %q does not end in its source digest", got)
		}
	}
}
