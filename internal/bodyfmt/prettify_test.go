package bodyfmt

import "testing"

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
	got := FormatRaw([]byte(`{"a":1}`), "application/json")
	want := "{\n  \"a\": 1\n}"
	if got != want {
		t.Fatalf("FormatRaw()=%q, want %q", got, want)
	}
}

func TestFormatRawLeavesUnknownTypesAlone(t *testing.T) {
	body := "line one\nline two\n"
	if got := FormatRaw([]byte(body), "text/plain"); got != "line one\nline two" {
		t.Fatalf("FormatRaw()=%q, want trailing newline trimmed only", got)
	}
}
