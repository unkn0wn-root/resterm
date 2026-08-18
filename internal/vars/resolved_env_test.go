package vars

import (
	"fmt"
	"testing"
	"time"
)

func TestResolveCapturesEachEnvRefOnce(t *testing.T) {
	first := "RESTERM_ENV_REF_FIRST"
	second := "RESTERM_ENV_REF_SECOND"
	missing := fmt.Sprintf("RESTERM_ENV_REF_MISSING_%d", time.Now().UnixNano())
	t.Setenv(first, "env:"+second)
	t.Setenv(second, "resolved-twice")

	src := testEnvironment(t, "dev", map[string]string{
		"auth.token":   "env:" + first,
		"auth.missing": "env:" + missing,
		"plain":        "visible",
	})
	env := src.Resolve()

	if got := src.Values()["auth.token"]; got != "env:"+first {
		t.Fatalf("catalog auth.token = %q, want the authored reference unchanged", got)
	}
	if got := env.AuthoredValues()["auth.token"]; got != "env:"+first {
		t.Fatalf("authored auth.token = %q, want the authored reference", got)
	}
	if got := env.Values()["auth.token"]; got != "env:"+second {
		t.Fatalf("resolved auth.token = %q, want one reference resolution", got)
	}
	if _, ok := env.Values()["auth.missing"]; ok {
		t.Fatal("missing reference remained in the resolved view")
	}
	if got := env.Values()["plain"]; got != "visible" {
		t.Fatalf("resolved plain = %q, want %q", got, "visible")
	}
	if !env.HasRefs() {
		t.Fatal("HasRefs() = false, want the declared references reported")
	}
	if secrets := env.Secrets(); len(secrets) != 1 || secrets[0] != "env:"+second {
		t.Fatalf("Secrets() = %#v, want the resolved reference value", secrets)
	}
	if env.Label() != src.Label() || env.Scope() != src.Scope() {
		t.Fatalf("snapshot changed environment identity: label=%q scope=%q", env.Label(), env.Scope())
	}

	// Template resolution must use the value captured above.
	t.Setenv(first, "changed-after-snapshot")
	res := NewResolver(NewMapProvider("environment", env.AuthoredValues()))
	res.AddRefResolver(env.RefResolver())
	got, err := res.ExpandTemplates("{{auth.token}}")
	if err != nil {
		t.Fatalf("expand auth.token: %v", err)
	}
	if got != "env:"+second {
		t.Fatalf("template auth.token = %q, want the value already in Values()", got)
	}
	if got := env.Values()["auth.token"]; got != "env:"+second {
		t.Fatalf("snapshot changed to %q after the process environment moved", got)
	}
}

func TestWithoutRefValuesWithholdsMappedValues(t *testing.T) {
	key := "RESTERM_ENV_REF_PUBLIC"
	t.Setenv(key, "private")
	t.Setenv("AUTH.TOKEN", "ambient-alias")

	env := testEnvironment(t, "dev", map[string]string{
		"auth.token": "env:" + key,
		"plain":      "visible",
	}).Resolve()
	public := env.WithoutRefValues()

	if _, ok := public.Values()["auth.token"]; ok {
		t.Fatal("withheld view still exposes the mapped value")
	}
	if got := public.Values()["plain"]; got != "visible" {
		t.Fatalf("withheld plain = %q, want %q", got, "visible")
	}
	if len(public.Secrets()) != 0 {
		t.Fatalf("withheld Secrets() = %#v, want none", public.Secrets())
	}

	// Include EnvProvider to verify that AUTH.TOKEN cannot replace the hidden alias.
	res := NewResolver(
		NewMapProvider("environment", public.AuthoredValues()),
		EnvProvider{},
	)
	res.AddRefResolver(public.RefResolver())
	if got, err := res.ExpandTemplates("{{auth.token}}"); err == nil {
		t.Fatalf("withheld auth.token resolved to %q, want it undefined", got)
	}
	if got, err := res.ExpandTemplates("{{plain}}"); err != nil || got != "visible" {
		t.Fatalf("withheld plain = %q, %v, want %q", got, err, "visible")
	}
	if got := env.Values()["auth.token"]; got != "private" {
		t.Fatalf("WithoutRefValues() mutated the snapshot it came from: %q", got)
	}
}

func TestResolveLeavesCatalogsWithoutRefsAlone(t *testing.T) {
	key := "RESTERM_ENV_REF_UNDECLARED"
	t.Setenv(key, "ambient")

	env := testEnvironment(t, "dev", map[string]string{"plain": "visible"}).Resolve()
	if env.HasRefs() {
		t.Fatal("HasRefs() = true for a catalog with no env: reference")
	}
	if len(env.Secrets()) != 0 {
		t.Fatalf("Secrets() = %#v, want none", env.Secrets())
	}

	value, handled, found := env.RefResolver()("env:" + key)
	if !handled || !found || value != "ambient" {
		t.Fatalf("undeclared reference = %q, handled = %t, found = %t", value, handled, found)
	}
}

func testEnvironment(t *testing.T, name string, values map[string]string) Environment {
	t.Helper()
	cat, err := NewCatalog(EnvironmentSet{name: values})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	env, err := cat.Resolve(Selection{name: name})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	return env
}
