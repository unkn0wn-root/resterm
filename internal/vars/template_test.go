package vars

import (
	"slices"
	"strconv"
	"testing"
)

// Callers rely on fn seeing every regex match, including a blank {{ }} with
// an empty trimmed name. A bare {{}} and an unterminated {{ never match.
func TestReplaceTemplateVarsCallbackContract(t *testing.T) {
	var calls []string
	out := ReplaceTemplateVars("a {{x}} b {{ }} c {{}} {{oops", func(match, name string) string {
		calls = append(calls, match+"|"+name)
		return "<" + name + ">"
	})
	if want := "a <x> b <> c {{}} {{oops"; out != want {
		t.Fatalf("out = %q, want %q", out, want)
	}
	if want := []string{"{{x}}|x", "{{ }}|"}; !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

// Render must stay byte-for-byte equivalent to ExpandTemplates, including how
// unresolved placeholders stay literal while only the first error is reported.
func TestCompileTemplateRenderMatchesExpandTemplates(t *testing.T) {
	r := NewResolver(NewMapProvider("test", map[string]string{"name": "resterm", "empty": ""}))
	inputs := []string{
		"",
		"plain text",
		"{{name}}",
		"pre {{name}} post",
		"{{ name }} padded",
		"{{name}}{{name}}",
		"{{empty}}[]",
		"{{missing}} tail",
		"{{missing}} then {{other}}",
		"{{}} blank stays literal",
		"{{ }} blank stays literal",
		"unterminated {{oops",
		"{{= }} empty expression",
		"{{=1+1}} expressions not enabled",
		"{{$nosuchdynamic}}",
		"mixed {{name}} {{missing}} {{ }} {{name}}",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			want, wantErr := r.ExpandTemplates(input)
			got, gotErr := CompileTemplate(input).Render(r)
			if got != want {
				t.Fatalf("Render = %q, ExpandTemplates = %q", got, want)
			}
			if (gotErr == nil) != (wantErr == nil) ||
				(gotErr != nil && gotErr.Error() != wantErr.Error()) {
				t.Fatalf("Render error = %v, ExpandTemplates error = %v", gotErr, wantErr)
			}
		})
	}
}

func TestCompileTemplateRenderResolvesDynamics(t *testing.T) {
	out, err := CompileTemplate("{{$timestamp}}").Render(NewResolver())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := strconv.ParseInt(out, 10, 64); err != nil {
		t.Fatalf("Render($timestamp) = %q, want a unix timestamp", out)
	}
}
