package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

func TestStyleLinesRendersChainWithLocationColor(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prevProfile)

	model := New(Config{})
	chain := `╰─> Get "https://api.local"`
	note := "help: No response payload was received."
	st := model.errSty()

	gotChain := st.line(diag.Line{Kind: diag.LineChain, Text: chain})
	wantChain := st.loc.Render(chain)
	if gotChain != wantChain {
		t.Fatalf("chain line style = %q, want location style %q", gotChain, wantChain)
	}
	if strings.Contains(gotChain, "\x1b[2m") {
		t.Fatalf("chain line should not use faint style, got %q", gotChain)
	}
	if gotChain == model.themeRuntime.fallbackFg(
		model.theme.Error,
		diagErrorLightColor,
		diagErrorDarkColor,
	).Render(chain) {
		t.Fatalf("chain line should not use error color, got %q", gotChain)
	}
	if gotChain == model.themeRuntime.subtleTextStyle(model.theme).Render(chain) {
		t.Fatalf("chain line should not use info/subtle color, got %q", gotChain)
	}

	gotNote := st.line(diag.Line{Kind: diag.LineHelp, Text: note})
	if gotNote == gotChain || !strings.Contains(gotNote, "\x1b[2m") {
		t.Fatalf("note line should stay subtle; note=%q chain=%q", gotNote, gotChain)
	}
}

func TestStyleLinesKeepsTheCaretUnderTheExcerpt(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prevProfile)

	const line = "\t\"名前\": \"{{missing}}\""
	model := New(Config{})
	styled := model.styleLines(diag.Lines(diag.Report{
		Path:   "sample.http",
		Source: []byte("{\n" + line + "\n}\n"),
		Items: []diag.Diagnostic{{
			Class:    diag.ClassParse,
			Severity: diag.SeverityError,
			Message:  "undefined variable: missing",
			Span: diag.Span{Start: diag.Pos{
				Line: 2,
				Col:  strings.Index(line, "{{missing}}") + 1,
			}},
		}},
	}))

	var excerpt, caret string
	for _, styledLine := range strings.Split(styled, "\n") {
		plain := ansi.Strip(styledLine)
		switch {
		case strings.Contains(plain, "{{missing}}"):
			excerpt = plain
		case strings.Contains(plain, "^"):
			caret = plain
		}
	}
	before, _, ok := strings.Cut(excerpt, "{{missing}}")
	if !ok {
		t.Fatalf("styled report has no excerpt:\n%s", styled)
	}
	pad, _, _ := strings.Cut(caret, "^")
	if ansi.StringWidth(pad) != ansi.StringWidth(before) ||
		strings.Count(pad, "\t") != strings.Count(before, "\t") {
		t.Errorf("caret is not under the placeholder:\n%s\n%s", excerpt, caret)
	}
}
