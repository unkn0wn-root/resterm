package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
)

type modalRenderKey struct {
	x, y          int
	width, height int
	boxWidth      int
	boxHeight     int
}

type modalScreen struct {
	under *cellbuf.Buffer
	rows  []string
}

func newModalScreen(under *cellbuf.Buffer) *modalScreen {
	rows := make([]string, under.Height())
	for y := range rows {
		_, rows[y] = cellbuf.RenderLine(under, y)
	}
	return &modalScreen{under: under, rows: rows}
}

func (s *modalScreen) with(box string, x, y, boxWidth, boxHeight int) string {
	band := &cellbuf.Buffer{Lines: make([]cellbuf.Line, boxHeight)}
	for i := range band.Lines {
		band.Lines[i] = slices.Clone(s.under.Lines[y+i])
	}
	drawModalBox(band, box, x, boxWidth, boxHeight)

	rows := slices.Clone(s.rows)
	for i := range boxHeight {
		_, rows[y+i] = cellbuf.RenderLine(band, i)
	}
	return strings.Join(rows, "\n")
}

// The pointer lets Model.View populate the same cache across model copies.
// Bubble Tea calls Update and View serially, so shared access is safe.
type modalRenderCache struct {
	key    modalRenderKey
	screen *modalScreen
}

func (c *modalRenderCache) get(key modalRenderKey) *modalScreen {
	if c == nil || c.key != key {
		return nil
	}
	return c.screen
}

func (c *modalRenderCache) keep(key modalRenderKey, screen *modalScreen) {
	if c == nil {
		return
	}
	c.key = key
	c.screen = screen
}

func (c *modalRenderCache) invalidate() {
	if c == nil {
		return
	}
	*c = modalRenderCache{}
}

func (m *Model) invalidateModalRender() {
	m.modalRender.invalidate()
}

func (m *Model) renderCenteredModal(box string) string {
	x, y, width, height := m.modalPlacement(box)
	if width <= 0 || height <= 0 {
		return box
	}

	boxWidth := min(lipgloss.Width(box), width-x)
	boxHeight := min(lipgloss.Height(box), height-y)
	key := modalRenderKey{
		x: x, y: y,
		width: width, height: height,
		boxWidth: boxWidth, boxHeight: boxHeight,
	}
	screen := m.modalRender.get(key)
	if screen == nil {
		under := screenBuffer(width, height, m.renderModalUnderlay(width, height))
		m.applyModalBackdrop(under, x, y, boxWidth, boxHeight)
		screen = newModalScreen(under)
		m.modalRender.keep(key, screen)
	}
	return screen.with(box, x, y, boxWidth, boxHeight)
}

func (m *Model) modalPlacement(box string) (x, y, width, height int) {
	boxWidth := lipgloss.Width(box)
	boxHeight := lipgloss.Height(box)
	width = max(m.width, boxWidth)
	height = max(m.height, boxHeight)
	return max((width-boxWidth)/2, 0), max((height-boxHeight)/2, 0), width, height
}

func (m *Model) renderModalUnderlay(width, height int) string {
	return lipgloss.Place(
		width,
		height,
		lipgloss.Top,
		lipgloss.Left,
		m.renderAppContent(renderContext{modalUnderlay: true}),
		lipgloss.WithWhitespaceChars(" "),
	)
}

func (m *Model) applyModalBackdrop(
	buf *cellbuf.Buffer,
	boxX, boxY, boxWidth, boxHeight int,
) {
	backdrop, ok := m.themeRuntime.modalBackdropColor(m.theme).(ansi.Color)
	if !ok || backdrop == nil {
		return
	}

	for row := 0; row < buf.Height(); row++ {
		for col := 0; col < buf.Width(); col++ {
			if row >= boxY && row < boxY+boxHeight &&
				col >= boxX && col < boxX+boxWidth {
				continue
			}

			cell := buf.Cell(col, row)
			if cell == nil {
				cell = cellbuf.NewCell(' ')
			} else {
				if cell.Rune != ' ' || len(cell.Comb) != 0 || cell.Width != 1 {
					continue
				}
				cell = cell.Clone()
			}
			cell.Style.Fg = backdrop
			buf.SetCell(col, row, cell)
		}
	}
}

func drawModalBox(band *cellbuf.Buffer, box string, x, boxWidth, boxHeight int) {
	modal := screenBuffer(boxWidth, boxHeight, box)
	for row := range boxHeight {
		for col := range boxWidth {
			cell := modal.Cell(col, row)
			// Skip the trailing cell of a wide character.
			if cell == nil || cell.Width == 0 {
				continue
			}

			band.SetCell(x+col, row, cell)
		}
	}
}

func screenBuffer(width, height int, content string) *cellbuf.Buffer {
	buf := cellbuf.NewBuffer(width, height)
	cellbuf.SetContent(buf, content)
	return buf
}
