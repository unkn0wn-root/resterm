package str

import "testing"

func TestFoldLines(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"one line":            {in: `status == 200`, want: `status == 200`},
		"spacing inside kept": {in: `text() == "a  b"`, want: `text() == "a  b"`},
		"tabs inside kept":    {in: "a\t== b", want: "a\t== b"},
		"lines joined":        {in: "sum(\n  1,\n  2\n)", want: "sum( 1, 2 )"},
		"blank lines dropped": {in: "sum(\n\n  1)", want: "sum( 1)"},
		"indent dropped":      {in: "(\n    a == b\n  )", want: "( a == b )"},
		"empty":               {},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := FoldLines(tt.in); got != tt.want {
				t.Fatalf("FoldLines(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIndent(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"each line":            {in: "a\nb", want: "  a\n  b"},
		"blank line stays":     {in: "a\n\nb", want: "  a\n\n  b"},
		"trailing space kept":  {in: "a \nb", want: "  a \n  b"},
		"space-only line kept": {in: "a\n \nb", want: "  a\n   \n  b"},
		"trailing newline":     {in: "a\n", want: "  a\n"},
		"empty":                {},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Indent(tt.in, "  "); got != tt.want {
				t.Fatalf("Indent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
