package bodyfmt

import (
	"bytes"
	"testing"

	"github.com/alecthomas/chroma/quick"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func TestDetectSyntax(t *testing.T) {
	tests := []struct {
		ct   string
		want syntax
	}{
		{ct: "application/json; charset=utf-8", want: syntaxJSON},
		{ct: "APPLICATION/JSON", want: syntaxJSON},
		{ct: "text/xml", want: syntaxXML},
		{ct: "application/xhtml+xml", want: syntaxXML},
		{ct: "text/html", want: syntaxHTML},
		{ct: "application/yaml", want: syntaxYAML},
		{ct: "text/ecmascript", want: syntaxJS},
		{ct: "text/plain", want: syntaxPlain},
		{ct: "", want: syntaxPlain},
	}

	for _, tt := range tests {
		if got := detect(tt.ct); got != tt.want {
			t.Fatalf("detect(%q)=%v, want %v", tt.ct, got, tt.want)
		}
	}
}

func TestFormatRawIndentsJSON(t *testing.T) {
	got := FormatRaw([]byte(`{"a":1}`), "application/json", Original)
	want := "{\n  \"a\": 1\n}"
	if got != want {
		t.Fatalf("FormatRaw()=%q, want %q", got, want)
	}
}

func TestFormatRawLeavesUnknownTypesAlone(t *testing.T) {
	body := "line one\nline two\n"
	if got := FormatRaw([]byte(body), "text/plain", Original); got != "line one\nline two" {
		t.Fatalf("FormatRaw()=%q, want trailing newline trimmed only", got)
	}
}

func TestHighlightMatchesQuick(t *testing.T) {
	// Keep the previous highlighting path as a reference in tests only, so
	// its full lexer registry is not linked into the application.
	inputs := []struct {
		name   string
		lang   syntax
		lexer  string
		source string
	}{
		{"json", syntaxJSON, "json", `{"name":"café","count":42,"ok":true}`},
		{"malformed json", syntaxJSON, "json", `{"name":"unfinished`},
		{"xml", syntaxXML, "xml", `<root enabled="true"><name>café &amp; tea</name></root>`},
		{"malformed xml", syntaxXML, "xml", `<root><name value="unfinished`},
		{"html", syntaxHTML, "html", `<style>body { color: red; }</style><script>const n = 42;</script><p>café</p>`},
		{"yaml", syntaxYAML, "yaml", "name: café\nitems:\n  - true\n  - 42\n"},
		{"javascript", syntaxJS, "javascript", "// café\nconst value = {name: 'tea', count: 42};\n"},
	}
	colors := []struct {
		name      string
		profile   termenv.Profile
		formatter string
	}{
		{"ansi", termenv.ANSI, "terminal16"},
		{"ansi256", termenv.ANSI256, "terminal256"},
		{"truecolor", termenv.TrueColor, "terminal16m"},
	}
	styles := []struct {
		name string
		want string
	}{
		{"", "monokai"},
		{" monokai ", "monokai"},
		{"github", "github"},
		{"unknown-style", "unknown-style"},
	}
	for _, input := range inputs {
		for _, color := range colors {
			for _, style := range styles {
				t.Run(input.name+"/"+color.name+"/"+style.name, func(t *testing.T) {
					var want bytes.Buffer
					if err := quick.Highlight(
						&want,
						input.source,
						input.lexer,
						color.formatter,
						style.want,
					); err != nil {
						t.Fatal(err)
					}
					got, ok := highlight(input.source, input.lang.lexer(), termcolor.Enabled(color.profile), style.name)
					if !ok || got != want.String() {
						t.Fatalf("highlight() = %q, %v; want %q, true", got, ok, want.String())
					}
				})
			}
		}
	}
}
