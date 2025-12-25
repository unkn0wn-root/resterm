package vars

import "testing"

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
