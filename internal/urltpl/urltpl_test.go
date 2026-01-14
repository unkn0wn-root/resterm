package urltpl

import (
	"strings"
	"testing"
)

func strPtr(s string) *string {
	return &s
}

func TestPatchQueryTemplateKeepsTemplateSeparators(t *testing.T) {
	raw := "{{base}}/path?q={{a&b}}&keep=1"
	patch := map[string]*string{"x": strPtr("1")}

	got, err := PatchQuery(raw, patch)
	if err != nil {
		t.Fatalf("PatchQuery: %v", err)
	}
	if !strings.Contains(got, "q={{a&b}}") {
		t.Fatalf("expected template value preserved, got %q", got)
	}
	if !strings.Contains(got, "keep=1") {
		t.Fatalf("expected keep query preserved, got %q", got)
	}
	if !strings.Contains(got, "x=1") {
		t.Fatalf("expected added query param, got %q", got)
	}
}

func TestPatchQueryTemplateMatchesNetURLOrder(t *testing.T) {
	raw := "{{base}}/path?b=2&a=1"
	patch := map[string]*string{
		"a": strPtr("x"),
		"c": strPtr("3"),
	}

	got, err := PatchQuery(raw, patch)
	if err != nil {
		t.Fatalf("PatchQuery: %v", err)
	}
	want := "{{base}}/path?a=x&b=2&c=3"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPatchQueryTemplateEncodedKeyMatch(t *testing.T) {
	raw := "{{base}}/path?q%5B%5D=1&keep=1"
	patch := map[string]*string{"q[]": nil}

	got, err := PatchQuery(raw, patch)
	if err != nil {
		t.Fatalf("PatchQuery: %v", err)
	}
	want := "{{base}}/path?keep=1"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPatchQueryTemplateQuestionMarkInTemplate(t *testing.T) {
	raw := "{{base?x=1}}/path?keep=1#frag"
	patch := map[string]*string{"q": strPtr("1")}

	got, err := PatchQuery(raw, patch)
	if err != nil {
		t.Fatalf("PatchQuery: %v", err)
	}
	want := "{{base?x=1}}/path?keep=1&q=1#frag"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPatchQueryTemplateValueInPatch(t *testing.T) {
	raw := "https://example.com/path?keep=1"
	patch := map[string]*string{"q": strPtr("{{token}}")}

	got, err := PatchQuery(raw, patch)
	if err != nil {
		t.Fatalf("PatchQuery: %v", err)
	}
	if strings.Contains(got, "%7B%7B") || strings.Contains(got, "%7D%7D") {
		t.Fatalf("expected template braces unescaped, got %q", got)
	}
	if !strings.Contains(got, "q={{token}}") {
		t.Fatalf("expected template value, got %q", got)
	}
}

func TestPatchQueryTemplateEncodesNonTemplateValues(t *testing.T) {
	raw := "{{base}}/path?keep=1"
	patch := map[string]*string{"q": strPtr("hello world {{token}}")}

	got, err := PatchQuery(raw, patch)
	if err != nil {
		t.Fatalf("PatchQuery: %v", err)
	}
	if !strings.Contains(got, "q=hello+world+{{token}}") {
		t.Fatalf("expected encoded spaces with template preserved, got %q", got)
	}
}

func TestPatchQueryUnbalancedTemplateUsesNetURLParsing(t *testing.T) {
	raw := "https://example.com/path?q={{a&b"
	patch := map[string]*string{"x": strPtr("1")}

	got, err := PatchQuery(raw, patch)
	if err != nil {
		t.Fatalf("PatchQuery: %v", err)
	}
	if !strings.Contains(got, "b=") {
		t.Fatalf("expected raw query split at &, got %q", got)
	}
	if !strings.Contains(got, "x=1") {
		t.Fatalf("expected added query param, got %q", got)
	}
	if !strings.Contains(got, "q=%7B%7Ba") {
		t.Fatalf("expected net/url encoding, got %q", got)
	}
}

func TestPatchQueryTemplatePreservesEmptyKey(t *testing.T) {
	raw := "{{base}}/path?=1&keep=1"
	patch := map[string]*string{"x": strPtr("1")}

	got, err := PatchQuery(raw, patch)
	if err != nil {
		t.Fatalf("PatchQuery: %v", err)
	}
	if !strings.Contains(got, "?=1") {
		t.Fatalf("expected empty key to remain, got %q", got)
	}
	if !strings.Contains(got, "x=1") {
		t.Fatalf("expected added query param, got %q", got)
	}
}
