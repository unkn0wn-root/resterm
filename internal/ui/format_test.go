package ui

import "testing"

func TestDisplayText(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "ordinary text is unchanged",
			value: `fixtures/a b\"quote/users.http`,
			want:  `fixtures/a b\"quote/users.http`,
		},
		{
			name:  "zero width joiner keeps emoji sequences whole",
			value: "\U0001F468\u200d\U0001F469\u200d\U0001F467.http",
			want:  "\U0001F468\u200d\U0001F469\u200d\U0001F467.http",
		},
		{
			name:  "zero width non joiner keeps persian words readable",
			value: "mi\u200cravad.http",
			want:  "mi\u200cravad.http",
		},
		{
			name:  "private use glyph from a patched font survives",
			value: "\ue0b0icons.http",
			want:  "\ue0b0icons.http",
		},
		{
			name:  "soft hyphen is invisible but harmless",
			value: "a\u00adb",
			want:  "a\u00adb",
		},
		{
			name:  "line controls preserve surrounding graphics",
			value: "a\\\"\r\n\tb/\U0001F468\u200d\U0001F469",
			want:  `a\"\r\n\tb/` + "\U0001F468\u200d\U0001F469",
		},
		{name: "terminal escape", value: "a\x1b[2Jb", want: `a\x1b[2Jb`},
		{name: "C1 control", value: "a\u009bJb", want: `a\u009bJb`},
		{name: "unicode line separator", value: "a\u2028b", want: `a\u2028b`},
		{name: "bidi override", value: "a\u202eb", want: `a\u202eb`},
		{name: "bidi isolate", value: "a\u2066b", want: `a\u2066b`},
		{name: "arabic letter mark", value: "a\u061cb", want: `a\u061cb`},
		{name: "left to right mark", value: "a\u200eb", want: `a\u200eb`},
		{name: "right to left mark", value: "a\u200fb", want: `a\u200fb`},
		{name: "invalid UTF-8", value: string([]byte{'a', 0xff, 'b'}), want: `a\xffb`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayText(tt.value); got != tt.want {
				t.Fatalf("displayText(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

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
