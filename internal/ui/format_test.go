package ui

import "testing"

func TestOneLineFoldsWhitespaceThenEscapes(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "line break folds to a space", value: "first\n  second", want: "first second"},
		{name: "CRLF folds to one space", value: "first\r\n  second", want: "first second"},
		{name: "escape is quoted", value: "a\x1b[2Jb", want: `a\x1b[2Jb`},
		{name: "both", value: "first\n\x1b[2Jsecond", want: `first \x1b[2Jsecond`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := oneLine(tt.value); got != tt.want {
				t.Fatalf("oneLine(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestDisplayLinesKeepsBreaks(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "breaks survive", value: "first\nsecond", want: "first\nsecond"},
		{name: "CRLF is one break", value: "first\r\nsecond", want: "first\nsecond"},
		{name: "lone CR is escaped", value: "first\rsecond", want: `first\rsecond`},
		{name: "escape is quoted", value: "first\n\x1b[2Jsecond", want: "first\n" + `\x1b[2Jsecond`},
		{name: "single line", value: "a\x1b[2Jb", want: `a\x1b[2Jb`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayLines(tt.value); got != tt.want {
				t.Fatalf("displayLines(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}
