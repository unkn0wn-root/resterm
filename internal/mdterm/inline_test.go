package mdterm

import (
	"strings"
	"testing"

	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

var ansi = termcolor.Config{Enabled: true, Profile: termenv.ANSI}

func TestInlinePlain(t *testing.T) {
	st := newStyler(termcolor.Config{})
	tests := []struct {
		name, in, want string
	}{
		{"bold", "a **b** c", "a b c"},
		{"underscore bold", "__x__", "x"},
		{"italic", "*x*", "x"},
		{"bold italic", "***x***", "x"},
		{"nested", "**a *b* c**", "a b c"},
		{"unclosed bold", "**a", "**a"},
		{"literal asterisk", "2 * 3", "2 * 3"},
		{"snake case", "snake_case_name", "snake_case_name"},
		{"multibyte snake case", "café_style_ naming", "café_style_ naming"},
		{"code", "run `go test` now", "run go test now"},
		{"code keeps markers", "`**not bold**`", "**not bold**"},
		{"unclosed backtick", "a `b", "a `b"},
		{"link", "[docs](https://example.com)", "docs (https://example.com)"},
		{"link text equals url", "[https://x.io](https://x.io)", "https://x.io"},
		{
			"link balanced parens",
			"[w](https://en.wikipedia.org/wiki/Go_(game))",
			"w (https://en.wikipedia.org/wiki/Go_(game))",
		},
		{"image", "![alt](https://x/img.png)", "alt (https://x/img.png)"},
		{"escaped paren in url", `[t](a\)b)`, "t (a)b)"},
		{"broken link", "[text](no url", "[text](no url"},
		{"bare url", "in https://github.com/a/b/pull/311 today", "in https://github.com/a/b/pull/311 today"},
		{"bare url trailing paren", "(see https://x.io/a)", "(see https://x.io/a)"},
		{"escape", `\*not\*`, "*not*"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderInline(tt.in, 0, st); got != tt.want {
				t.Fatalf("renderInline(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestInlineColor(t *testing.T) {
	st := newStyler(ansi)
	if got, want := renderInline("**b**", 0, st), "\x1b[1mb\x1b[0m"; got != want {
		t.Fatalf("bold = %q, want %q", got, want)
	}
	if got, want := renderInline("`x`", 0, st), "\x1b[35mx\x1b[0m"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if got := renderInline("`**x**`", 0, st); strings.Contains(got, "\x1b[1m") {
		t.Fatalf("emphasis styled inside code span: %q", got)
	}
	got := renderInline("**a *b* c**", 0, st)
	if !strings.Contains(got, "\x1b[1;3mb\x1b[0m") {
		t.Fatalf("nested italic not bold+italic: %q", got)
	}
	if got := renderInline("https://x.io", 0, st); !strings.Contains(got, "\x1b[2m") {
		t.Fatalf("bare url not faint: %q", got)
	}
}
