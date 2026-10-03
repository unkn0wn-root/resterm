package vars

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func mapLookup(values map[string]string) Lookup {
	return func(name string) (string, bool, error) {
		v, ok := values[NameKey(name)]
		return v, ok, nil
	}
}

func TestInterpolate(t *testing.T) {
	look := mapLookup(map[string]string{
		"base":   "https://api.test",
		"id":     "42",
		"empty":  "",
		"nested": "{{id}}",
		"expr":   "{{= 1 + 1}}",
		"$uuid":  "declared",
	})
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "empty"},
		{name: "plain text", text: "no placeholders", want: "no placeholders"},
		{name: "variables", text: "{{base}}/users/{{id}}", want: "https://api.test/users/42"},
		{name: "names are normalized", text: "{{ BASE }}", want: "https://api.test"},
		{name: "empty value", text: "[{{empty}}]", want: "[]"},
		{name: "values are not expanded again", text: "{{nested}}", want: "{{id}}"},
		{name: "expression values are not run", text: "{{expr}}", want: "{{= 1 + 1}}"},
		{name: "variable named like a helper wins", text: "{{$uuid}}", want: "declared"},
		{name: "unmatched braces stay", text: "a } b {c}", want: "a } b {c}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Interpolate(tt.text, look)
			if err != nil {
				t.Fatalf("Interpolate(%q) error: %v", tt.text, err)
			}
			if got != tt.want {
				t.Fatalf("Interpolate(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestInterpolateRendersHelpers(t *testing.T) {
	got, err := Interpolate("{{$timestamp}}|{{$randomInt(5, 5)}}|{{$uuid}}|{{$uuid}}", mapLookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(got, "|")
	if !regexp.MustCompile(`^\d+$`).MatchString(parts[0]) || parts[1] != "5" {
		t.Fatalf("Interpolate = %q, want a timestamp and 5", got)
	}
	if parts[2] == parts[3] {
		t.Fatalf("each {{$uuid}} rendered %q, want a new value per placeholder", parts[2])
	}
}

func TestInterpolateErrors(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		want  string
		undef bool
	}{
		{name: "undefined", text: "/u/{{missing}}", want: "undefined variable: missing", undef: true},
		{name: "unknown helper", text: "{{$nosuchhelper}}", want: "undefined variable: $nosuchhelper", undef: true},
		{name: "bad helper arguments", text: "{{$randomInt(soon)}}", want: "not a whole number"},
		{name: "expression", text: "{{= 1 + 1}}", want: "expression {{= 1 + 1}} is not allowed"},
		{name: "blank", text: "a {{ }} b", want: "placeholder {{ }} has no name"},
		{name: "unclosed", text: "{{base}/x", want: "placeholder {{base} is not closed with }}"},
		{name: "unclosed at the end", text: "/u/{{id", want: "placeholder {{id is not closed with }}"},
		{name: "structural error wins", text: "{{missing}} {{= x}}", want: "expression {{= x}} is not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Interpolate(tt.text, mapLookup(map[string]string{"base": "b", "id": "1"}))
			if err == nil {
				t.Fatalf("Interpolate(%q) = %q, want error %q", tt.text, got, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Interpolate(%q) error = %q, want %q", tt.text, err, tt.want)
			}
			if errors.Is(err, ErrUndefinedVariable) != tt.undef {
				t.Fatalf("errors.Is(%v, ErrUndefinedVariable) = %t, want %t", err, !tt.undef, tt.undef)
			}
		})
	}
}

func TestInterpolateKeepsLookupErrors(t *testing.T) {
	cycle := errors.New("variable cycle: x -> x")
	look := func(name string) (string, bool, error) {
		if name == "x" {
			return "", false, cycle
		}
		return "", false, nil
	}
	_, err := Interpolate("{{missing}} {{x}}", look)
	if !errors.Is(err, cycle) {
		t.Fatalf("error = %v, want the lookup error", err)
	}
}

func TestInterpolateDoesNotLookUpRejectedPlaceholders(t *testing.T) {
	var names []string
	look := func(name string) (string, bool, error) {
		names = append(names, name)
		return "v", true, nil
	}
	if _, err := Interpolate("{{= secret}} {{ }}", look); err == nil {
		t.Fatal("want an error")
	}
	if _, err := Interpolate("{{a}} {{b", look); err == nil {
		t.Fatal("want an error")
	}
	if len(names) != 0 {
		t.Fatalf("looked up %q, want no lookups", names)
	}
}

func TestInterpolateDoesNotReadOSEnvironment(t *testing.T) {
	t.Setenv("RESTERM_INTERPOLATE_LEAK", "leaked")
	got, err := Interpolate("{{RESTERM_INTERPOLATE_LEAK}}", mapLookup(nil))
	if !errors.Is(err, ErrUndefinedVariable) {
		t.Fatalf("Interpolate = %q, %v, want an undefined variable", got, err)
	}
}
