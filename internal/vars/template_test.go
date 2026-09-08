package vars

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
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
// unresolved placeholders stay literal and which single error is reported:
// the first structural error if any occurred, otherwise the first undefined
// variable.
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

type reportErr struct {
	msg string
	rep diag.Report
}

func (e *reportErr) Error() string           { return e.msg }
func (e *reportErr) Diagnostic() diag.Report { return e.rep }

func placeholderErr(t *testing.T, err error) *PlaceholderError {
	t.Helper()
	var pe *PlaceholderError
	if !errors.As(err, &pe) {
		t.Fatalf("error = %T %v, want *PlaceholderError", err, err)
	}
	return pe
}

// An expression failure already carries a report. Wrapping it in a
// placeholder must keep that report's class, position, and stack.
func TestPlaceholderErrorKeepsExpressionReport(t *testing.T) {
	inner := &reportErr{msg: "x is not defined", rep: diag.Report{Items: []diag.Diagnostic{{
		Class:    diag.ClassScript,
		Severity: diag.SeverityError,
		Message:  "x is not defined",
		Span:     diag.Span{Start: diag.Pos{Path: "lib.rts", Line: 3, Col: 5}},
		Frames:   []diag.StackFrame{{Name: "helper", Pos: diag.Pos{Path: "lib.rts", Line: 3, Col: 5}}},
	}}}}
	r := NewResolver()
	r.SetExprEval(func(string, ExprPos) (string, error) { return "", inner })

	_, err := r.ExpandTemplatesAt("{{= helper() }}", diag.Pos{Path: "requests.http", Line: 6, Col: 1})
	err = diag.WrapAs(diag.ClassProtocol, err, "expand body template")

	got := diag.Render(err)
	want := "error[script]: x is not defined\n" +
		"--> lib.rts:3:5\n" +
		"expand body template\n" +
		"Stack:\n" +
		"  at lib.rts:3:5 in helper"
	if got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}

func TestPlaceholderErrorFillsMissingPosition(t *testing.T) {
	inner := &reportErr{msg: "boom", rep: diag.Report{Items: []diag.Diagnostic{{
		Class: diag.ClassScript, Severity: diag.SeverityError, Message: "boom",
	}}}}
	r := NewResolver()
	r.SetExprEval(func(string, ExprPos) (string, error) { return "", inner })

	_, err := r.ExpandTemplatesAt("id={{= fail() }}", diag.Pos{Path: "requests.http", Line: 6, Col: 1})

	rep := placeholderErr(t, err).Diagnostic()
	if rep.Class() != diag.ClassScript ||
		rep.Items[0].Span.Start != (diag.Pos{Path: "requests.http", Line: 6, Col: 4}) {
		t.Fatalf("report = %+v, want class script at requests.http:6:4", rep.Items[0])
	}
}

func TestExpressionEvaluatesAtItsPlaceholder(t *testing.T) {
	var got []ExprPos
	r := NewResolver()
	r.SetExprEval(func(_ string, pos ExprPos) (string, error) {
		got = append(got, pos)
		return "1", nil
	})

	if _, err := r.ExpandTemplatesAt("id={{= 1+1 }}\n{{=2}}", diag.Pos{Path: "a.http", Line: 4, Col: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ExpandTemplatesAt("{{= 1 }}", diag.Pos{Path: "auth.http", Line: 3}); err != nil {
		t.Fatal(err)
	}

	want := []ExprPos{
		{Path: "a.http", Line: 4, Col: 17},
		{Path: "a.http", Line: 5, Col: 4},
		{Path: "auth.http", Line: 3},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("expression positions = %+v, want %+v", got, want)
	}
}

func TestExpandTemplatesLocatedMapsLines(t *testing.T) {
	lines := []int{3, 5, 6}
	locate := func(line, col int) diag.Pos {
		if line < 1 || line > len(lines) {
			return diag.Pos{}
		}
		return diag.Pos{Path: "requests.http", Line: lines[line-1], Col: col}
	}

	_, err := NewResolver().ExpandTemplatesLocated("{\n  \"token\": \"{{auth.token}}\"\n}", locate)

	pe := placeholderErr(t, err)
	want := diag.Span{
		Start: diag.Pos{Path: "requests.http", Line: 5, Col: 13},
		End:   diag.Pos{Path: "requests.http", Line: 5, Col: 27},
	}
	if pe.Match != "{{auth.token}}" || pe.Span != want || !strings.Contains(err.Error(), "auth.token") {
		t.Fatalf("placeholder = %q at %+v, want %+v", pe.Match, pe.Span, want)
	}
}

func TestExpandTemplatesAtWithoutColumnLocatesLine(t *testing.T) {
	start := diag.Pos{Path: "auth.http", Line: 3}

	_, err := NewResolver().ExpandTemplatesAt("Bearer {{token}}", start)

	if pe := placeholderErr(t, err); pe.Span != (diag.Span{Start: start, End: start}) {
		t.Fatalf("span = %+v, want the directive line", pe.Span)
	}
}

// The hand scan must agree with templateVarPattern on what opens a
// placeholder, so the reference rebuilds the finding from the pattern.
func TestUnclosedPlaceholdersMatchesPattern(t *testing.T) {
	inputs := []string{
		"{{ok}} {{auth.token} and {{host} {{ name } {{{x}}} {{",
		"{{}}", "{{a}b}}", "{{a{{b}}", "a\n{{x}\n{{y}}", "plain", "{{", "}}{{", "{{ }}", "{{a}}}",
	}
	for _, input := range inputs {
		got := UnclosedPlaceholders(input, diag.Pos{Line: 1, Col: 1})
		want := referenceUnclosed(input)
		if len(got) != len(want) {
			t.Fatalf("%q: got %+v, want %+v", input, got, want)
		}
		for i := range want {
			if got[i].Text != want[i].Text || got[i].Off != want[i].Off {
				t.Fatalf("%q: finding %d = %+v, want %+v", input, i, got[i], want[i])
			}
		}
	}
	got := UnclosedPlaceholders("a\n{{x}\n{{y}}", diag.Pos{Path: "b.http", Line: 4, Col: 3})
	want := diag.Span{Start: diag.Pos{Path: "b.http", Line: 5, Col: 1}, End: diag.Pos{Path: "b.http", Line: 5, Col: 5}}
	if len(got) != 1 || got[0].Span != want {
		t.Fatalf("span = %+v, want %+v", got, want)
	}
}

func referenceUnclosed(input string) []Unclosed {
	closed := templateVarPattern.FindAllStringIndex(input, -1)
	var out []Unclosed
	for off, c := 0, 0; ; {
		i := strings.Index(input[off:], "{{")
		if i < 0 {
			return out
		}
		i += off
		for c < len(closed) && closed[c][1] <= i {
			c++
		}
		if c < len(closed) && closed[c][0] <= i {
			off = closed[c][1]
			continue
		}
		end := i + 2 + nameLen(input[i+2:])
		if end < len(input) && input[end] == '}' {
			end++
		}
		out = append(out, Unclosed{Text: input[i:end], Off: i})
		off = end
	}
}
