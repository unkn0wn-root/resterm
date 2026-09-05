package textarea

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestUnderlinedCursorDoesNotExposeEscapeSequences(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	for _, blink := range []bool{false, true} {
		m := newTextArea()
		m.Prompt = ""
		m.ShowLineNumbers = false
		m.SetHeight(1)
		m.SetWidth(30)
		m.SetValue("# @moc")
		m.SetCursor(3)
		m.Cursor.SetMode(cursor.CursorStatic)
		m.Cursor.Blink = blink
		m.SetRuneStyler(fixedLineRuneStyler{
			0: lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa00")).Underline(true),
		})
		view := m.View()
		if got := strings.TrimSpace(ansi.Strip(view)); got != "# @moc" {
			t.Fatalf("blink=%t: cursor exposed escapes: %q (raw %q)", blink, got, view)
		}
	}
}

func TestRuneDecorationsComposeWithSelectionAndSearch(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := newTextArea()
	m.Prompt = ""
	m.ShowLineNumbers = false
	m.SetHeight(1)
	m.SetWidth(12)
	m.SetValue("abcd")
	m.SetCursor(4)
	m.FocusedStyle.Text = lipgloss.NewStyle()
	m.FocusedStyle.CursorLine = lipgloss.NewStyle()
	m.style = &m.FocusedStyle
	decoration := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa00")).Underline(true)
	selection := lipgloss.NewStyle().Background(lipgloss.Color("#334455"))
	highlight := lipgloss.NewStyle().Background(lipgloss.Color("#556677"))
	m.SetRuneStyler(fixedLineRuneStyler{0: decoration})
	m.SetSelectionStyle(selection)
	m.SetSelectionRange(1, 2)
	m.SetHighlightStyle(highlight)
	m.SetHighlightRanges([]HighlightRange{{Start: 1, End: 3}})
	view := m.View()
	for _, want := range []string{
		decoration.Render("a"),
		selection.Inherit(decoration).Render("b"),
		highlight.Inherit(decoration).Render("c"),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing composed style %q in %q", want, view)
		}
	}
	if got := strings.TrimSpace(ansi.Strip(view)); got != "abcd" {
		t.Fatalf("decorations changed source layout: %q", got)
	}
}

type numberedRuneStyler struct{ lipgloss.Style }

func (s numberedRuneStyler) StylesForLine([]rune, int) []lipgloss.Style { return nil }

func (s numberedRuneStyler) LineNumberStyle(line int) (lipgloss.Style, bool) {
	return s.Style, line == 1
}

func TestEmptyLineNumberDecorationPreservesLayout(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	m := newTextArea()
	m.SetWidth(20)
	m.SetHeight(2)
	m.SetValue("abc\n")
	m.row = 1
	m.SetCursor(0)
	before := m.View()
	decoration := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000")).Underline(true)
	m.SetRuneStyler(numberedRuneStyler{decoration})
	after := m.View()
	if ansi.Strip(before) != ansi.Strip(after) {
		t.Fatalf("line marker changed layout: %q => %q", before, after)
	}
	want := decoration.Inherit(m.style.computedCursorLineNumber()).Render(m.formatLineNumber(2))
	if !strings.Contains(after, want) || before == after {
		t.Fatalf("empty line marker not rendered: %q", after)
	}
}
