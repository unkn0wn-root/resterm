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
