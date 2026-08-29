package rts

import "testing"

func TestOpenGroup(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want rune
	}{
		{name: "empty"},
		{name: "no groups", src: "status == 200"},
		{name: "balanced call", src: "sum(1, 2)"},
		{name: "open call", src: "licensing.check(", want: ')'},
		{name: "open index", src: "items[", want: ']'},
		{name: "open block", src: "fn(x) {", want: '}'},
		{name: "innermost wins", src: "sum(items[", want: ']'},
		{name: "closed innermost", src: "sum(items[0]", want: ')'},
		{name: "bracket in a string", src: `contains("(")`},
		{name: "quote in a string", src: `contains("\"(")`},
		{name: "bracket in a comment", src: "status == 200 # note ("},
		{name: "single quoted", src: "name == 'a (b'"},
		{name: "stray closer", src: "1 + 2)"},
		{name: "closer of another kind", src: "sum(1]", want: ')'},
		{name: "unterminated string alone", src: `"abc`},
		{name: "unterminated string in a call", src: `contains("abc`, want: ')'},
		{name: "string ends with its line", src: "contains(\"abc\n) == true"},
		{name: "spans lines", src: "sum(\n  1,\n  2\n)"},
		{name: "open across lines", src: "sum(\n  1,", want: ')'},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := OpenGroup(tt.src); got != tt.want {
				t.Fatalf("OpenGroup(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

func TestMask(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "empty"},
		{name: "plain text is kept", src: "status == 200", want: "status == 200"},
		{name: "string is hidden", src: `a == "b c"`, want: "a ==      "},
		{name: "comment is hidden", src: "a == 1 # note", want: "a == 1       "},
		{name: "group keeps its delimiters", src: "sum(1, 2) == 3", want: "sum(    ) == 3"},
		{name: "nested group is hidden", src: "f(g(x)) as y", want: "f(    ) as y"},
		{name: "separator survives a group", src: "f(a) => b", want: "f( ) => b"},
		{name: "separator inside a group is hidden", src: "f(a => b)", want: "f(      )"},
		{name: "separator inside a string is hidden", src: `f("=>") => c`, want: "f(    ) => c"},
		{name: "open group hides the rest", src: "f(a, b", want: "f(    "},
		{name: "stray closer is kept", src: "a ) b", want: "a ) b"},
		{name: "line breaks go too", src: "f(\n  a\n) as x", want: "f(     ) as x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Mask(tt.src)
			if got != tt.want {
				t.Fatalf("Mask(%q) = %q, want %q", tt.src, got, tt.want)
			}
			if len(got) != len(tt.src) {
				t.Fatalf("Mask(%q) length = %d, want %d", tt.src, len(got), len(tt.src))
			}
			for i := range got {
				if got[i] != ' ' && got[i] != tt.src[i] {
					t.Fatalf("Mask(%q)[%d] = %q, want %q", tt.src, i, got[i], tt.src[i])
				}
			}
		})
	}
}

func TestGroupScannerMatchesOpenGroup(t *testing.T) {
	sources := []string{
		"sum(1, 2)",
		"contains(\"x\\\n)\")",
		"contains(\"x\\\r\n)\")",
		"contains(\"x\n)\")",
		"call(\n # ) ignored\n)",
		"[a mismatched) still open",
		"call(\x00) ignored",
		"(\"\\\x00\")",
	}

	for _, src := range sources {
		for cut := range len(src) + 1 {
			var scan GroupScanner
			scan.Feed(src[:cut])
			scan.Feed(src[cut:])
			if got, want := scan.Closer(), OpenGroup(src); got != want {
				t.Fatalf("chunks [%q, %q] closed with %q, whole read gives %q", src[:cut], src[cut:], got, want)
			}
		}

		var scan GroupScanner
		for i := range len(src) {
			scan.Feed(src[i : i+1])
		}
		if got, want := scan.Closer(), OpenGroup(src); got != want {
			t.Fatalf("byte chunks over %q closed with %q, whole read gives %q", src, got, want)
		}
	}
}
