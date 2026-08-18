package vars

import (
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"
)

func authored(refs *EnvRefs, src map[string]string) NameMap[Value] {
	var out NameMap[Value]
	for _, name := range slices.Sorted(maps.Keys(src)) {
		out.Set(name, refs.ResolveDeclared(src[name]))
	}
	return out
}

func declaredNames(src map[string]string) *Resolver {
	var out NameMap[Value]
	for _, name := range slices.Sorted(maps.Keys(src)) {
		out.Set(name, Value{Text: src[name]})
	}
	return NewResolver(NewValueMapTemplateProvider("file", out))
}

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
	if got := env.Values()["auth.token"]; got != "env:"+second {
		t.Fatalf("resolved auth.token = %q, want one reference resolution", got)
	}
	if _, ok := env.Values()["auth.missing"]; ok {
		t.Fatal("missing reference remained in the resolved view")
	}
	if got := env.Values()["plain"]; got != "visible" {
		t.Fatalf("resolved plain = %q, want %q", got, "visible")
	}
	if secrets := env.Secrets(); len(secrets) != 1 || secrets[0] != "env:"+second {
		t.Fatalf("Secrets() = %#v, want the resolved reference value", secrets)
	}
	if env.Label() != src.Label() || env.Scope() != src.Scope() {
		t.Fatalf("snapshot changed environment identity: label=%q scope=%q", env.Label(), env.Scope())
	}

	t.Setenv(missing, "ambient-now")
	res := NewResolver(NewValueMapProvider("environment", env.ProviderValues()), EnvProvider{})
	if got, err := res.ExpandTemplates("{{auth.missing}}"); err == nil {
		t.Fatalf("{{auth.missing}} resolved to %q, want it undefined", got)
	}
	if got, err := res.ExpandTemplates("{{auth.token}}"); err != nil || got != "env:"+second {
		t.Fatalf("{{auth.token}} = %q, %v, want the value already in Values()", got, err)
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
	// Withheld values still need redaction in case response data echoes them.
	if got := public.Secrets(); len(got) != 1 || got[0] != "private" {
		t.Fatalf("withheld Secrets() = %#v, want the value it hides", got)
	}

	res := NewResolver(NewValueMapProvider("environment", public.ProviderValues()), EnvProvider{})
	if got, err := res.ExpandTemplates("{{auth.token}}"); err == nil {
		t.Fatalf("withheld auth.token resolved to %q, want it undefined", got)
	}
	if got, err := res.ExpandTemplates("{{plain}}"); err != nil || got != "visible" {
		t.Fatalf("withheld plain = %q, %v, want %q", got, err, "visible")
	}
	if got := env.Values()["auth.token"]; got != "private" {
		t.Fatalf("WithoutRefValues() mutated the snapshot it came from: %q", got)
	}

	if got := public.Refs().ResolveDeclared("env:" + key); !got.Missing {
		t.Fatalf("withheld snapshot resolved an authored reference to %#v", got)
	}
}

func TestZeroResolvedEnvHasNoReferences(t *testing.T) {
	var r ResolvedEnv
	if got := r.Secrets(); got != nil {
		t.Fatalf("Secrets() = %#v, want none", got)
	}
	if got := r.WithoutRefValues().Secrets(); got != nil {
		t.Fatalf("withheld Secrets() = %#v, want none", got)
	}
	if got := r.Refs().ResolveDeclared("env:ANY"); !got.Missing {
		t.Fatalf("ResolveDeclared() = %#v, want missing", got)
	}
	if got := r.Refs().ResolveDeclared("plain"); got.Missing || got.Text != "plain" {
		t.Fatalf("ResolveDeclared() = %#v, want the text unchanged", got)
	}
}

func TestEnvRefsReadEachVariableOnce(t *testing.T) {
	key := "RESTERM_ENV_REFS_ONCE"
	t.Setenv(key, "first")

	var refs EnvRefs
	if got := refs.resolve(key); got.Text != "first" || !got.Final {
		t.Fatalf("first read = %#v, want the process value marked final", got)
	}
	t.Setenv(key, "second")
	if got := refs.resolve(key); got.Text != "first" {
		t.Fatalf("second read = %q, want the captured value", got.Text)
	}
	if secrets := refs.Secrets(); len(secrets) != 1 || secrets[0] != "first" {
		t.Fatalf("Secrets() = %#v, want one entry", secrets)
	}
}

func TestEnvRefKeyReadsTheEnvForm(t *testing.T) {
	for _, raw := range []string{"plain", "environment:X", "envs:X", ""} {
		if key, ok := EnvRefKey(raw); ok {
			t.Fatalf("EnvRefKey(%q) = %q, true, want it treated as text", raw, key)
		}
	}
	for raw, want := range map[string]string{
		"env:NAME":    "NAME",
		"ENV: NAME ":  "NAME",
		"  env:name ": "name",
		"env:":        "",
		"env:   ":     "",
	} {
		if key, ok := EnvRefKey(raw); !ok || key != want {
			t.Fatalf("EnvRefKey(%q) = %q, %t, want %q", raw, key, ok, want)
		}
	}
}

func TestDeclaredEnvRefWithNoNameIsUndefined(t *testing.T) {
	t.Setenv("TOKEN", "ambient-value")

	var refs EnvRefs
	for _, raw := range []string{"env:", "env:   "} {
		if got := refs.ResolveDeclared(raw); !got.Missing {
			t.Fatalf("ResolveDeclared(%q) = %#v, want it undefined", raw, got)
		}
	}
	if len(refs.Secrets()) != 0 {
		t.Fatalf("Secrets() = %#v, want none", refs.Secrets())
	}

	var vals NameMap[Value]
	vals.Set("token", refs.ResolveDeclared("env:"))
	res := NewResolver(NewValueMapTemplateProvider("file", vals), EnvProvider{})
	if got, err := res.ExpandTemplates("{{token}}"); err == nil {
		t.Fatalf("{{token}} resolved to %q, want it undefined", got)
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
