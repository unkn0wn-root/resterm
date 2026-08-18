package vars

import (
	"errors"
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

// A name that no helper claims stays an ordinary variable, so it is reported
// as undefined instead of as a broken helper call.
func TestUnknownDollarNameIsUndefined(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	for _, input := range []string{"{{$unknown}}", "{{$my-custom-var + 1h}}", "{{$timestamp + missing}}"} {
		out, err := resolver.ExpandTemplates(input)
		if !errors.Is(err, ErrUndefinedVariable) {
			t.Fatalf("%s: expected undefined variable, got %v", input, err)
		}
		if out != input {
			t.Fatalf("%s: expected placeholder to stay literal, got %q", input, out)
		}
	}
}

// A recognised helper used the wrong way is a file error, not a missing
// variable, so the message names the helper.
func TestMisusedHelperReportsHelperError(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	_, err := resolver.ExpandTemplates("{{$uuid + 1s}}")
	if err == nil || errors.Is(err, ErrUndefinedVariable) {
		t.Fatalf("expected a helper error, got %v", err)
	}
	if !strings.Contains(err.Error(), "$uuid") {
		t.Fatalf("expected error to name the helper, got %v", err)
	}
}

func TestDynamicHelperArguments(t *testing.T) {
	t.Parallel()

	resolver := NewResolver()
	out, err := resolver.ExpandTemplates(`{{$randomChoice("only")}}-{{$randomString(8)}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	choice, random, _ := strings.Cut(out, "-")
	if choice != "only" {
		t.Fatalf("expected the single option, got %q", choice)
	}
	if len(random) != 8 {
		t.Fatalf("expected 8 random characters, got %q", random)
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

func envRefResolver(values map[string]string) (*Resolver, *EnvRefs) {
	refs := NewEnvRefs(declaredNames(values))
	return NewResolver(NewValueMapTemplateProvider("envfile", authored(refs, values))), refs
}

func TestEnvRefResolves(t *testing.T) {
	key := "RESTERM_TEST_ENV_REF"
	t.Setenv(key, "super-secret")

	resolver, _ := envRefResolver(map[string]string{"auth.password": "env:" + key})

	out, err := resolver.ExpandTemplates("{{auth.password}}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "super-secret" {
		t.Fatalf("expected env ref to resolve, got %q", out)
	}
}

func TestEnvRefUppercaseFallback(t *testing.T) {
	key := "RESTERM_TEST_ENV_REF_UPPER"
	t.Setenv(key, "works")

	resolver, _ := envRefResolver(map[string]string{
		"auth.password": "env:" + strings.ToLower(key),
	})

	out, err := resolver.ExpandTemplates("{{auth.password}}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "works" {
		t.Fatalf("expected uppercase fallback to resolve, got %q", out)
	}
}

func TestEnvRefMissingReturnsUndefined(t *testing.T) {
	key := fmt.Sprintf("RESTERM_TEST_MISSING_ENV_REF_%d", time.Now().UnixNano())
	resolver, _ := envRefResolver(map[string]string{"auth.password": "env:" + key})

	out, err := resolver.ExpandTemplates("{{auth.password}}")
	if err == nil {
		t.Fatalf("expected undefined variable error")
	}
	if out != "{{auth.password}}" {
		t.Fatalf("expected unresolved template placeholder, got %q", out)
	}
}

func TestEnvRefInRuntimeValueStaysLiteral(t *testing.T) {
	key := "RESTERM_TEST_RUNTIME_LITERAL"
	t.Setenv(key, "leaked")

	var captured NameMap[Value]
	captured.Set("auth.password", Value{Text: "env:" + key})
	resolver := NewResolver(NewValueMapProvider("request", captured))

	out, err := resolver.ExpandTemplates("{{auth.password}}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "env:"+key {
		t.Fatalf("runtime value resolved to %q, want the text unchanged", out)
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

func TestLenientResolverPreservesPlaceholders(t *testing.T) {
	r := NewResolver(NewMapProvider("env", map[string]string{"host": "example.com"}))
	tr := NewTrace()
	r.SetTrace(tr)
	lr := r.Lenient()

	out, err := lr.ExpandTemplates("https://{{host}}/{{missing}}")
	if err != nil {
		t.Fatalf("lenient expansion returned error: %v", err)
	}
	if out != "https://example.com/{{missing}}" {
		t.Fatalf("unexpected output: %q", out)
	}

	if _, err := r.ExpandTemplates("{{missing}}"); err == nil {
		t.Fatalf("strict resolver should still fail")
	}

	found := false
	for _, it := range tr.Items() {
		if it.Name == "missing" && it.Missing {
			found = true
		}
	}
	if !found {
		t.Fatalf("trace did not record missing variable")
	}
}

func TestExpandTemplatesResultReportsUndefinedVariables(t *testing.T) {
	tests := []struct {
		name          string
		resolver      *Resolver
		input         string
		want          string
		wantUndefined bool
	}{
		{
			name: "resolved variable",
			resolver: NewResolver(NewMapProvider("env", map[string]string{
				"host": "example.com",
			})).Lenient(),
			input: "{{host}}",
			want:  "example.com",
		},
		{
			name:          "undefined variable",
			resolver:      NewResolver().Lenient(),
			input:         "{{missing}}",
			want:          "{{missing}}",
			wantUndefined: true,
		},
		{
			name: "nested undefined variable",
			resolver: NewResolver(NewTemplateProvider("file", map[string]string{
				"outer": "{{missing}}",
			})).Lenient(),
			input:         "{{outer}}",
			want:          "{{outer}}",
			wantUndefined: true,
		},
		{
			name:     "malformed opening marker",
			resolver: NewResolver().Lenient(),
			input:    "prefix{{",
			want:     "prefix{{",
		},
		{
			name:     "blank placeholder",
			resolver: NewResolver().Lenient(),
			input:    "{{ }}",
			want:     "{{ }}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.resolver.ExpandTemplatesResult(tt.input)
			if err != nil {
				t.Fatalf("ExpandTemplatesResult() error = %v", err)
			}
			if result.Value != tt.want {
				t.Fatalf("ExpandTemplatesResult() value = %q, want %q", result.Value, tt.want)
			}
			if result.HasUndefinedVariables != tt.wantUndefined {
				t.Fatalf(
					"ExpandTemplatesResult() HasUndefinedVariables = %v, want %v",
					result.HasUndefinedVariables,
					tt.wantUndefined,
				)
			}
		})
	}
}

func TestLenientResolverKeepsOuterPlaceholderForNestedFailure(t *testing.T) {
	r := NewResolver(NewTemplateProvider("file", map[string]string{"a": "{{missing}} x"}))
	lr := r.Lenient()

	out, err := lr.ExpandTemplates("{{a}}")
	if err != nil {
		t.Fatalf("lenient expansion returned error: %v", err)
	}
	if out != "{{a}}" {
		t.Fatalf("unexpected output: %q", out)
	}

	if _, err := r.ExpandTemplates("{{a}}"); err == nil {
		t.Fatalf("strict resolver should still fail after lenient render")
	}
}

func TestLenientResolverReportsCycles(t *testing.T) {
	r := NewResolver(NewTemplateProvider("file", map[string]string{
		"a": "{{b}}",
		"b": "{{a}}",
	}))

	out, err := r.Lenient().ExpandTemplates("{{a}}")
	if err == nil || !strings.Contains(err.Error(), "variable cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
	if out != "{{a}}" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestLenientResolverReportsExpressionErrors(t *testing.T) {
	out, err := NewResolver().Lenient().ExpandTemplates("{{= 1 + 1 }}")
	if err == nil || !strings.Contains(err.Error(), "expressions not enabled") {
		t.Fatalf("expected expression error, got %v", err)
	}
	if out != "{{= 1 + 1 }}" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestWithExprEvalLeavesTheOriginalAlone(t *testing.T) {
	base := NewResolver()
	base.SetExprEval(func(expr string, _ ExprPos) (string, error) {
		return "base:" + expr, nil
	})
	derived := base.WithExprEval(func(expr string, _ ExprPos) (string, error) {
		return "derived:" + expr, nil
	})

	got, err := derived.ExpandTemplates("{{= x }}")
	if err != nil || got != "derived:x" {
		t.Fatalf("derived = %q, %v; want %q", got, err, "derived:x")
	}
	got, err = base.ExpandTemplates("{{= x }}")
	if err != nil || got != "base:x" {
		t.Fatalf("original = %q, %v; want %q", got, err, "base:x")
	}
}

func TestWithExprEvalKeepsPinnedValues(t *testing.T) {
	base := NewResolver(NewTemplateProvider("file", map[string]string{"id": "{{$uuid}}"}))
	first, err := base.ExpandTemplates("{{id}}")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	derived := base.WithExprEval(func(string, ExprPos) (string, error) { return "", nil })
	second, err := derived.ExpandTemplates("{{id}}")
	if err != nil {
		t.Fatalf("expand derived: %v", err)
	}
	if first != second {
		t.Fatalf("derived resolver re-rolled the value: %q then %q", first, second)
	}
}

func TestWithProvidersKeepsPinnedValues(t *testing.T) {
	base := NewResolver(NewTemplateProvider("file", map[string]string{"id": "{{$uuid}}"}))
	first, err := base.ExpandTemplates("{{id}}")
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	derived := base.WithProviders(NewTemplateProvider("file", map[string]string{
		"id":    "{{$uuid}}",
		"fresh": "staged",
	}))
	second, err := derived.ExpandTemplates("{{id}}")
	if err != nil {
		t.Fatalf("expand derived: %v", err)
	}
	if first != second {
		t.Fatalf("re-planned resolver re-rolled the value: %q then %q", first, second)
	}
	if got, err := derived.ExpandTemplates("{{fresh}}"); err != nil || got != "staged" {
		t.Fatalf("fresh = %q, %v; want the new provider visible", got, err)
	}
}

func TestLenientResolverCycleNotMaskedByUndefined(t *testing.T) {
	r := NewResolver(NewTemplateProvider("file", map[string]string{
		"a": "{{b}}",
		"b": "{{a}}",
	}))

	out, err := r.Lenient().ExpandTemplates("{{missing}} {{a}}")
	if err == nil || !strings.Contains(err.Error(), "variable cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
	if out != "{{missing}} {{a}}" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestUndefinedVariableSentinel(t *testing.T) {
	r := NewResolver(NewTemplateProvider("file", map[string]string{"a": "{{missing}}"}))

	_, err := r.ExpandTemplates("{{missing}}")
	if !errors.Is(err, ErrUndefinedVariable) {
		t.Fatalf("expected sentinel on plain undefined, got %v", err)
	}

	_, err = r.ExpandTemplates("{{a}}")
	if !errors.Is(err, ErrUndefinedVariable) {
		t.Fatalf("expected sentinel through nested wrap, got %v", err)
	}

	_, err = r.Lenient().ExpandTemplates("{{= 1 }}")
	if errors.Is(err, ErrUndefinedVariable) {
		t.Fatalf("expression error must not match the sentinel")
	}
}

func TestNestedCycleOutranksUndefined(t *testing.T) {
	r := NewResolver(NewTemplateProvider("file", map[string]string{
		"a": "{{missing}} {{b}}",
		"b": "{{b}}",
	}))

	out, err := r.Lenient().ExpandTemplates("{{a}}")
	if err == nil || !strings.Contains(err.Error(), "variable cycle") {
		t.Fatalf("lenient render must report the nested cycle, got %v", err)
	}
	if out != "{{a}}" {
		t.Fatalf("unexpected output: %q", out)
	}

	if _, err := r.ExpandTemplates("{{a}}"); err == nil ||
		!strings.Contains(err.Error(), "variable cycle") {
		t.Fatalf("strict render must report the cycle over the missing name, got %v", err)
	}
}
