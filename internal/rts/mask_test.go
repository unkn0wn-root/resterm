package rts

import "testing"

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

func TestMaskText(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "empty"},
		{name: "plain text is kept", src: "status == 200", want: "status == 200"},
		{name: "string is hidden", src: `contains("{{", "{")`, want: "contains(    ,    )"},
		{name: "comment is hidden", src: "x == 1 # {{", want: "x == 1     "},
		{name: "comment marker inside a string", src: `x == "# {{"`, want: "x ==       "},
		{name: "quote inside a comment", src: `x == 1 # "{{`, want: "x == 1      "},
		{name: "unterminated string is hidden", src: `x == "{{`, want: "x ==    "},
		{name: "nested group is kept", src: "f(g(x)) as y", want: "f(g(x)) as y"},
		{name: "braces outside a string are kept", src: `f("{{") && {{x}`, want: `f(    ) && {{x}`},
		// RTS comments start with #; the parser removes directive comment prefixes.
		{name: "no // comment", src: "x == 1 // {{", want: "x == 1 // {{"},
		{name: "line breaks go too", src: "f(\n  a\n) as x", want: "f(   a ) as x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MaskText(tt.src)
			if got != tt.want {
				t.Fatalf("MaskText(%q) = %q, want %q", tt.src, got, tt.want)
			}
			if len(got) != len(tt.src) {
				t.Fatalf("MaskText(%q) length = %d, want %d", tt.src, len(got), len(tt.src))
			}
			for i := range got {
				if got[i] != ' ' && got[i] != tt.src[i] {
					t.Fatalf("MaskText(%q)[%d] = %q, want %q", tt.src, i, got[i], tt.src[i])
				}
			}
		})
	}
}
