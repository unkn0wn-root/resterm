package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestViewUnicodeCommentsKeepTerminalWidth(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	for _, symbol := range []string{"⚙️", "👩‍💻", "🇳🇴", "é", "你好"} {
		for _, width := range []int{80, 120, 280} {
			t.Run(fmt.Sprintf("%s/%d", symbol, width), func(t *testing.T) {
				m := newTestModelWithDoc("# " + symbol + " Resterm Workspace Configuration\n" + sampleRequestDoc)
				updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
				*m = updated.(Model)
				for col := range len([]rune(symbol)) + 3 {
					m.editor.SetCursor(col)
					for row, line := range strings.Split(m.View(), "\n") {
						styled := ansi.StringWidth(line)
						visible := ansi.StringWidth(ansi.Strip(line))
						if styled != visible || visible > width {
							t.Fatalf(
								"cursor %d, row %d: styled width %d, visible width %d, terminal width %d",
								col,
								row,
								styled,
								visible,
								width,
							)
						}
					}
				}
			})
		}
	}
}

func TestViewInvisibleRunesKeepTerminalWidth(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })

	runes := []struct {
		name string
		r    rune
	}{
		{"soft hyphen", 0x00ad},
		{"zero width space", 0x200b},
		{"left to right mark", 0x200e},
		{"word joiner", 0x2060},
		{"byte order mark", 0xfeff},
		{"arabic letter mark", 0x061c},
		{"interlinear anchor", 0xfff9},
		{"mongolian vowel separator", 0x180e},
		{"language tag", 0xe0001},
		{"right to left override", 0x202e},
	}
	const lead = "# lorem "
	for _, rr := range runes {
		for _, width := range []int{80, 120, 280} {
			t.Run(fmt.Sprintf("%s/%d", rr.name, width), func(t *testing.T) {
				doc := lead + strings.Repeat(string(rr.r), 4) + " ipsum\n" + sampleRequestDoc
				m := newTestModelWithDoc(doc)
				updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
				*m = updated.(Model)

				for col := len(lead) - 1; col <= len(lead)+4; col++ {
					m.editor.SetCursor(col)
					view := m.View()
					if strings.ContainsRune(view, rr.r) {
						t.Fatalf("invisible rune reached the terminal with the cursor at %d", col)
					}
					if got := lipgloss.Height(view); got != 40 {
						t.Fatalf("view is %d rows, want 40", got)
					}
					for i, row := range strings.Split(view, "\n") {
						if got := ansi.StringWidth(row); got > width {
							t.Fatalf("row %d is %d cells, limit %d: %q",
								i, got, width, ansi.Strip(row))
						}
					}
				}
				if !strings.ContainsRune(m.editor.Value(), rr.r) {
					t.Fatal("display escaping reached the editor buffer")
				}
			})
		}
	}
}
