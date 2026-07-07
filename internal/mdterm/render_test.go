package mdterm

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func TestRenderPlain(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{
			name: "headings",
			in:   "# Top\ntext\n## Section ##\n### Sub\n#### Deep",
			want: "Top\n===\ntext\n\nSection\n-------\n\nSub\n\nDeep",
		},
		{
			name: "no space no heading",
			in:   "#nope",
			want: "#nope",
		},
		{
			name: "nested list",
			in:   "- a\n  - b\n    - c\n- d",
			want: "• a\n  ◦ b\n    · c\n• d",
		},
		{
			name: "tab nested list",
			in:   "- a\n\t- b",
			want: "• a\n  ◦ b",
		},
		{
			name: "ordered list",
			in:   "1. one\n2) two\n12. twelve",
			want: "1. one\n2. two\n12. twelve",
		},
		{
			name:  "hanging indent",
			in:    "- aaa bbb ccc ddd eee fff",
			width: 20,
			want:  "• aaa bbb ccc ddd\n  eee fff",
		},
		{
			name: "list continuation",
			in:   "- item\n  more text",
			want: "• item\n  more text",
		},
		{
			name: "fence",
			in:   "```go\nfunc main() {}\n\n**raw**\n```\nafter",
			want: "    func main() {}\n\n    **raw**\nafter",
		},
		{
			name:  "fence never wraps",
			in:    "```\naaaa bbbb cccc dddd\n```",
			width: 10,
			want:  "    aaaa bbbb cccc dddd",
		},
		{
			name: "unclosed fence",
			in:   "```\ncode",
			want: "    code",
		},
		{
			name: "inline code span is not a fence",
			in:   "```resterm --update``` installs it\nnext",
			want: "resterm --update installs it\nnext",
		},
		{
			name: "control chars stripped",
			in:   "a\x1b[31mb\rc",
			want: "a[31mb\nc",
		},
		{
			name:  "tight width keeps wrapping",
			in:    "- a\n  - bb cc",
			width: 4,
			want:  "• a\n  ◦ bb\n    cc",
		},
		{
			name: "tilde fence",
			in:   "~~~\nx\n~~~",
			want: "    x",
		},
		{
			name:  "hr variants",
			in:    "---\n* * *\n___",
			width: 4,
			want:  "────\n────\n────",
		},
		{
			name: "blockquote",
			in:   "> quoted\n>> deeper",
			want: "│ quoted\n│ │ deeper",
		},
		{
			name:  "wrapped paragraph",
			in:    "one two three four five",
			width: 10,
			want:  "one two\nthree four\nfive",
		},
		{
			name: "blank collapse",
			in:   "\n\na\n\n\n\nb\n\n",
			want: "a\n\nb",
		},
		{
			name: "crlf",
			in:   "a\r\nb",
			want: "a\nb",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(tt.in, Options{Width: tt.width})
			if got != tt.want {
				t.Fatalf("Render(%q) =\n%q\nwant\n%q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRenderReleaseNotes(t *testing.T) {
	in := "## What's Changed\n" +
		"* fix multipart parsing by @dev in https://github.com/o/r/pull/311\n" +
		"\n" +
		"**Full Changelog**: https://github.com/o/r/compare/v1...v2"
	want := "What's Changed\n" +
		"--------------\n" +
		"• fix multipart parsing by @dev in\n" +
		"  https://github.com/o/r/pull/311\n" +
		"\n" +
		"Full Changelog: https://github.com/o/r/compare/v1...v2"
	got := Render(in, Options{Width: 60})
	if got != want {
		t.Fatalf("Render =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderColor(t *testing.T) {
	got := Render("## Section\n- item", Options{Color: ansi})
	if !strings.Contains(got, "\x1b[36;1mSection\x1b[0m") {
		t.Fatalf("h2 not accent+bold: %q", got)
	}
	if strings.Contains(got, "---") {
		t.Fatalf("plain underline in color mode: %q", got)
	}
	if !strings.Contains(got, "• item") {
		t.Fatalf("bullet missing: %q", got)
	}
}

func TestRenderWrappedStyleDoesNotBleed(t *testing.T) {
	got := Render("**aaaa bbbb cccc**", Options{Width: 6, Color: ansi})
	for ln := range strings.SplitSeq(got, "\n") {
		if strings.Contains(ln, "\x1b[") && !strings.HasSuffix(ln, "\x1b[0m") {
			t.Fatalf("open SGR state at line break: %q in %q", ln, got)
		}
	}
}

func TestRenderPipedIsANSIFree(t *testing.T) {
	in := "## H\n**b** `c` [t](https://u.io) https://x.io\n> q"
	got := Render(in, Options{Width: 80, Color: termcolor.Config{}})
	if strings.Contains(got, "\x1b") {
		t.Fatalf("plain render contains ANSI: %q", got)
	}
}
