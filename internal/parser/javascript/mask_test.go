package js

import "testing"

func TestMaskText(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "empty"},
		{name: "code is kept", src: "vars.get(a) + 1", want: "vars.get(a) + 1"},
		{name: "strings are hidden", src: `f("a b", 'c')`, want: "f(     ,    )"},
		{name: "escaped quote", src: `f("a\"b") + x`, want: "f(      ) + x"},
		{name: "other quote inside a string", src: `f("it's") + x`, want: "f(      ) + x"},
		{name: "unterminated string ends at the line", src: "f('a\nx", want: "f(  \nx"},
		{name: "line continuation", src: "f('a\\\nb') + x", want: "f(   \n  ) + x"},
		{name: "line comment", src: "x // a 'b\ny", want: "x        \ny"},
		{name: "block comment", src: "x /* a\n'b */ y", want: "x     \n      y"},
		{name: "unterminated block comment", src: "x /* a\ny", want: "x     \n "},
		{name: "template text", src: "`a 'b` + x", want: "       + x"},
		{name: "template substitution", src: "`a ${b + c} d` + x", want: "     b + c     + x"},
		{name: "template across lines", src: "`a\nb` + x", want: "  \n   + x"},
		{name: "braces in a substitution", src: "`${ {a: 1}.a } x`", want: "    {a: 1}.a     "},
		{name: "nested template", src: "`a ${`b ${c}`} d`", want: "          c      "},
		{name: "string in a substitution", src: "`${'}'} x`", want: "          "},
		{name: "unterminated substitution", src: "`a ${b", want: "     b"},
		{name: "escaped substitution", src: "`\\${a}` + x", want: "        + x"},
		{name: "regex", src: `x = /a"b/g.test(y)`, want: "x =      g.test(y)"},
		{name: "regex with escaped slashes", src: `f(/^https?:\/\//, y)`, want: "f(              , y)"},
		{name: "regex with a slash in a class", src: `f(/[/'"]/, y)`, want: "f(       , y)"},
		{name: "regex with a star", src: "f(/\\/*$/)\ny", want: "f(      )\ny"},
		{name: "regex after a keyword", src: `return /'/.test(y)`, want: "return    .test(y)"},
		{name: "regex ends at the line", src: "x = (/a\ny", want: "x = (  \ny"},
		{name: "division after a name", src: "a / b / c", want: "a / b / c"},
		{name: "division after a number", src: "1 / 2 / 3", want: "1 / 2 / 3"},
		{name: "division after a call", src: "f(a) / b / c", want: "f(a) / b / c"},
		{name: "division after an index", src: "a[0] / b / c", want: "a[0] / b / c"},
		{name: "division after unicode", src: "é / b / c", want: "é / b / c"},
		{name: "name that starts with a keyword", src: "returned / b / c", want: "returned / b / c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := MaskText(tt.src); got != tt.want {
				t.Fatalf("MaskText(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

func FuzzMaskTextKeepsOffsets(f *testing.F) {
	for _, seed := range []string{
		"`a ${b} c`",
		"`${`${'`'}`}`",
		"x = /[/]/g / 2 // c\n/* d */",
		"'a\\\nb' + \"c",
		"`${",
		"/\\",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		got := MaskText(src)
		if len(got) != len(src) {
			t.Fatalf("MaskText(%q) length = %d, want %d", src, len(got), len(src))
		}
		for i := range got {
			if got[i] != src[i] && (got[i] != ' ' || src[i] == '\n') {
				t.Fatalf("MaskText(%q)[%d] = %q, want %q or a space", src, i, got[i], src[i])
			}
		}
	})
}
