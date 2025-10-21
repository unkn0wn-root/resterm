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
