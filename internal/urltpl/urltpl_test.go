package urltpl

import (
	"reflect"
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

func TestRawQueryIgnoresDelimitersInsideTemplates(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "query", in: "{{base?x=1}}/path?a=1&a=2#frag", want: "a=1&a=2", ok: true},
		{name: "fragment in value", in: "/path?q={{value#part}}&a=1#frag", want: "q={{value#part}}&a=1", ok: true},
		{name: "empty query", in: "/path?", ok: true},
		{name: "fragment first", in: "/path#frag?q=1"},
		{name: "no query", in: "{{base?x=1}}/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := RawQuery(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("RawQuery(%q) = %q, %t; want %q, %t", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseTargetQueryProtectsTemplateSeparators(t *testing.T) {
	raw := "{{base?fallback=true}}/path?q={{a&b=c#d}}&{{key}}=1&{{key}}=2#frag"
	got, err := ParseTargetQuery(raw)
	if err != nil {
		t.Fatalf("ParseTargetQuery: %v", err)
	}
	want := map[string][]string{
		"q":       {"{{a&b=c#d}}"},
		"{{key}}": {"1", "2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseTargetQuery() = %#v, want %#v", got, want)
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
