package vars

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

type countingProvider struct {
	label string
	vals  map[string]string
	seen  []string
}

func newCountingProvider(label string, vals map[string]string) *countingProvider {
	normalized := make(map[string]string, len(vals))
	for key, value := range vals {
		normalized[strings.ToLower(strings.TrimSpace(key))] = value
	}
	return &countingProvider{label: label, vals: normalized}
}

func (p *countingProvider) Resolve(name string) (string, bool) {
	p.seen = append(p.seen, strings.TrimSpace(name))
	value, ok := p.vals[strings.ToLower(strings.TrimSpace(name))]
	return value, ok
}

func (p *countingProvider) Label() string {
	return p.label
}

func TestExpandTemplatesStatic(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(NewMapProvider("const", map[string]string{
		"svc.http": "http://localhost:8080",
		"token":    "abc123",
	}))

	input := "{{svc.http}}/api?token={{token}}"
	expanded, err := resolver.ExpandTemplatesStatic(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "http://localhost:8080/api?token=abc123"
	if expanded != expected {
		t.Fatalf("expected %q, got %q", expected, expanded)
	}

	missing := "{{svc.http}}/api/{{missing}}"
	expandedMissing, err := resolver.ExpandTemplatesStatic(missing)
	if err == nil {
		t.Fatalf("expected error for missing variable")
	}
	if expandedMissing != "http://localhost:8080/api/{{missing}}" {
		t.Fatalf("unexpected expansion result %q", expandedMissing)
	}

	dynamicInput := "{{svc.http}}/{{ $timestamp }}"
	dynamicExpanded, err := resolver.ExpandTemplatesStatic(dynamicInput)
	if err == nil {
		t.Fatalf("expected error for undefined dynamic variable")
	}
	if dynamicExpanded != "http://localhost:8080/{{ $timestamp }}" {
		t.Fatalf("unexpected dynamic expansion %q", dynamicExpanded)
	}
}

func TestExpandTemplatesWithProviderLabel(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(NewMapProvider("env", map[string]string{
		"id": "123",
	}))

	expanded, err := resolver.ExpandTemplates("{{env.id}}")
	if err != nil {
		t.Fatalf("unexpected error expanding namespaced variable: %v", err)
	}
	if expanded != "123" {
		t.Fatalf("expected value 123, got %q", expanded)
	}
}

func TestResolveStopsAtFirstDirectMatchWithoutTrace(t *testing.T) {
	t.Parallel()

	first := newCountingProvider("env", map[string]string{"token": "env-token"})
	second := newCountingProvider("file", map[string]string{"token": "file-token"})
	resolver := NewResolver(first, second)

	out, ok := resolver.Resolve("token")
	if !ok {
		t.Fatal("expected token to resolve")
	}
	if out != "env-token" {
		t.Fatalf("expected env-token, got %q", out)
	}
	if len(second.seen) != 0 {
		t.Fatalf("expected second provider to be skipped, got lookups %v", second.seen)
	}
}

func TestResolveStopsAtFirstPrefixedMatchWithoutTrace(t *testing.T) {
	t.Parallel()

	first := newCountingProvider("env", map[string]string{"token": "env-token"})
	second := newCountingProvider("env", map[string]string{"token": "file-token"})
	resolver := NewResolver(first, second)

	out, ok := resolver.Resolve("env.token")
	if !ok {
		t.Fatal("expected namespaced token to resolve")
	}
	if out != "env-token" {
		t.Fatalf("expected env-token, got %q", out)
	}
	if got := second.seen; len(got) != 1 || got[0] != "env.token" {
		t.Fatalf("expected second provider to skip subject lookup, got %v", got)
	}
}

func TestDynamicGuidAlias(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	expanded, err := resolver.ExpandTemplates("{{ $guid }}")
	if err != nil {
		t.Fatalf("unexpected error expanding $guid: %v", err)
	}

	if expanded == "{{ $guid }}" {
		t.Fatalf("expected $guid to be expanded")
	}
	if len(expanded) != 36 {
		t.Fatalf("expected uuid-style length 36, got %d (%q)", len(expanded), expanded)
	}
}

func TestDynamicCanBeShadowedByProviders(t *testing.T) {
	t.Parallel()

	resolver := NewResolver(NewMapProvider("const", map[string]string{
		"$timestamp": "shadowed",
	}))

	expanded, err := resolver.ExpandTemplates("{{ $timestamp }}")
	if err != nil {
		t.Fatalf("unexpected error expanding $timestamp: %v", err)
	}
	if expanded != "shadowed" {
		t.Fatalf("expected provider value, got %q", expanded)
	}
}

func TestDynamicHelpersCaseInsensitive(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	values := map[string]string{
		"{{$UUID}}":             "",
		"{{$Guid}}":             "",
		"{{$TIMESTAMPISO8601}}": "",
		"{{$timestampMS}}":      "",
		"{{$randomINT}}":        "",
	}

	for input := range values {
		out, err := resolver.ExpandTemplates(input)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", input, err)
		}
		if out == input {
			t.Fatalf("expected %s to expand, got %q", input, out)
		}
	}
}

func TestDynamicTimestampOffset(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	start := time.Now()
	out, err := resolver.ExpandTemplates("{{ $timestamp + 2s }}")
	end := time.Now()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		t.Fatalf("expected unix seconds, got %q", out)
	}
	min := start.Add(2 * time.Second).Unix()
	max := end.Add(2 * time.Second).Unix()
	if parsed < min || parsed > max {
		t.Fatalf("expected %d to be between %d and %d", parsed, min, max)
	}
}

func TestDynamicTimestampISOOffset(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	start := time.Now()
	out, err := resolver.ExpandTemplates("{{ $timestampISO8601 - 1h }}")
	end := time.Now()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed, err := time.Parse(time.RFC3339, out)
	if err != nil {
		t.Fatalf("expected rfc3339, got %q", out)
	}
	min := start.Add(-1 * time.Hour).Unix()
	max := end.Add(-1 * time.Hour).Unix()
	if parsed.Unix() < min || parsed.Unix() > max {
		t.Fatalf("expected %v to be between %d and %d", parsed, min, max)
	}
}

func TestSplitDynamicOffsetNoSpace(t *testing.T) {
	t.Parallel()

	base, offset, ok := splitDynamicOffset("$timestampISO8601-1h")
	if !ok {
		t.Fatalf("expected offset parse to succeed")
	}
	if base != "$timestampISO8601" {
		t.Fatalf("expected base to be $timestampISO8601, got %q", base)
	}
	if offset != -1*time.Hour {
		t.Fatalf("expected -1h offset, got %v", offset)
	}
}

func TestSplitDynamicOffsetHyphenatedBase(t *testing.T) {
	t.Parallel()

	base, offset, ok := splitDynamicOffset("$my-custom-var + 1h")
	if !ok {
		t.Fatalf("expected offset parse to succeed")
	}
	if base != "$my-custom-var" {
		t.Fatalf("expected base to be $my-custom-var, got %q", base)
	}
	if offset != time.Hour {
		t.Fatalf("expected 1h offset, got %v", offset)
	}
}

func TestDynamicTimestampMs(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	start := time.Now()
	out, err := resolver.ExpandTemplates("{{ $timestampMs }}")
	end := time.Now()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		t.Fatalf("expected unix milliseconds, got %q", out)
	}
	min := start.UnixNano() / int64(time.Millisecond)
	max := end.UnixNano() / int64(time.Millisecond)
	if parsed < min || parsed > max {
		t.Fatalf("expected %d to be between %d and %d", parsed, min, max)
	}
}

func TestExpandTemplatesExpr(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	resolver.SetExprEval(func(expr string, pos ExprPos) (string, error) {
		if expr != "1+1" {
			t.Fatalf("unexpected expr %q", expr)
		}
		return "2", nil
	})

	out, err := resolver.ExpandTemplates("{{= 1+1 }}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "2" {
		t.Fatalf("expected 2, got %q", out)
	}
}

func TestExpandTemplatesExprMissing(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	_, err := resolver.ExpandTemplates("{{= 1 }}")
	if err == nil {
		t.Fatalf("expected error for missing expr evaluator")
	}
}

func TestEnvRefResolver(t *testing.T) {
	key := "RESTERM_TEST_ENV_REF"
	t.Setenv(key, "super-secret")

	resolver := NewResolver(NewMapProvider("envfile", map[string]string{
		"auth.password": "env:" + key,
	}))
	resolver.AddRefResolver(EnvRefResolver)

	out, err := resolver.ExpandTemplates("{{auth.password}}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "super-secret" {
		t.Fatalf("expected env ref to resolve, got %q", out)
	}
}

func TestEnvRefResolverUppercaseFallback(t *testing.T) {
	key := "RESTERM_TEST_ENV_REF_UPPER"
	t.Setenv(key, "works")

	resolver := NewResolver(NewMapProvider("envfile", map[string]string{
		"auth.password": "env:" + strings.ToLower(key),
	}))
	resolver.AddRefResolver(EnvRefResolver)

	out, err := resolver.ExpandTemplates("{{auth.password}}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "works" {
		t.Fatalf("expected uppercase fallback to resolve, got %q", out)
	}
}

func TestEnvRefResolverMissingReturnsUndefined(t *testing.T) {
	key := fmt.Sprintf("RESTERM_TEST_MISSING_ENV_REF_%d", time.Now().UnixNano())
	resolver := NewResolver(NewMapProvider("envfile", map[string]string{
		"auth.password": "env:" + key,
	}))
	resolver.AddRefResolver(EnvRefResolver)

	out, err := resolver.ExpandTemplates("{{auth.password}}")
	if err == nil {
		t.Fatalf("expected undefined variable error")
	}
	if out != "{{auth.password}}" {
		t.Fatalf("expected unresolved template placeholder, got %q", out)
	}
}

func TestExpandTemplatesStaticExpr(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	called := false
	resolver.SetExprEval(func(expr string, pos ExprPos) (string, error) {
		called = true
		return "ok", nil
	})

	out, err := resolver.ExpandTemplatesStatic("{{= 1+1 }}")
	if err == nil {
		t.Fatalf("expected error for static expression")
	}
	if out != "{{= 1+1 }}" {
		t.Fatalf("unexpected expansion result %q", out)
	}
	if called {
		t.Fatalf("expected static expansion to skip expression eval")
	}
}
