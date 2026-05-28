package ui

import (
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

type mouseRect struct {
	x int
	y int
	w int
	h int
}

func (r mouseRect) Contains(x, y int) bool {
	return x >= r.x && y >= r.y && x < r.x+r.w && y < r.y+r.h
}

func (r mouseRect) Empty() bool {
	return r.w <= 0 || r.h <= 0
}

type mouseLayout struct {
	file     mouseRect
	editor   mouseRect
	response mouseRect
}

type responseMouseHit struct {
	id   responsePaneID
	rect mouseRect
	ok   bool
}

func (m *Model) currentMouseLayout() mouseLayout {
	originX, originY := m.appFrameContentOrigin()
	headerH := lipgloss.Height(m.renderHeader())
	commandH := lipgloss.Height(m.renderCommandBar())
	panesY := originY + headerH + commandH

	fileW, fileH := m.sidebarMouseSize()
	editorW, editorH := m.editorMouseSize()

	ly := mouseLayout{
		file: mouseRect{x: originX, y: panesY, w: fileW, h: fileH},
	}

	if m.mainSplitOrientation == mainSplitHorizontal {
		rightX := originX + fileW
		ly.editor = mouseRect{x: rightX, y: panesY, w: editorW, h: editorH}

		responseW := max(max(editorW, m.width-fileW), 0)
		responseW, responseH := m.responseMouseSize(responseW)
		ly.response = mouseRect{
			x: rightX,
			y: panesY + editorH,
			w: responseW,
			h: responseH,
		}
		return ly
	}

	ly.editor = mouseRect{x: originX + fileW, y: panesY, w: editorW, h: editorH}
	responseW := m.responseTargetWidth(fileW, editorW)
	responseW, responseH := m.verticalResponseMouseSize(fileW, editorW, responseW)
	ly.response = mouseRect{
		x: originX + fileW + editorW,
		y: panesY,
		w: responseW,
		h: responseH,
	}
	return ly
}

func (m *Model) appFrameContentOrigin() (int, int) {
	frame := m.theme.AppFrame
	x := frame.GetMarginLeft() + frame.GetBorderLeftSize() + frame.GetPaddingLeft()
	y := frame.GetMarginTop() + frame.GetBorderTopSize() + frame.GetPaddingTop()
	return x, y
}

func (m *Model) sidebarMouseSize() (int, int) {
	if m.effectiveRegionCollapsed(paneRegionSidebar) {
		return 0, 0
	}

	style := m.sidebarFrameStyle(m.navigatorPaneFocused())
	frameWidth := style.GetHorizontalFrameSize()
	width := m.sidebarWidthPx
	if width <= 0 {
		width = paneOuterWidthFromContent(m.fileList.Width(), frameWidth)
		if width <= 0 {
			width = paneOuterWidthFromContent(1, frameWidth)
		}
	}

	contentHeight := m.sidebarFilesHeight
	if contentHeight <= 0 {
		contentHeight = m.paneContentHeight
	}
	return max(width, 0), paneMouseHeight(contentHeight, style)
}

func (m *Model) editorMouseSize() (int, int) {
	if m.effectiveRegionCollapsed(paneRegionEditor) {
		return 0, 0
	}

	style := m.editorFrameStyle(m.focus == focusEditor)
	contentWidth := m.editor.ViewWidth()
	width := paneOuterWidthFromContent(contentWidth, style.GetHorizontalFrameSize())
	if width < 1 {
		width = contentWidth + style.GetHorizontalFrameSize()
	}

	contentHeight := m.editorContentHeight
	if contentHeight <= 0 {
		contentHeight = m.paneContentHeight
	}
	height := max(m.editor.Height(), contentHeight)
	return width, paneMouseHeight(height, style)
}

func (m *Model) verticalResponseMouseSize(fileW, editorW, responseW int) (int, int) {
	if responseW <= 0 {
		return 0, 0
	}

	width, height := m.responseMouseSize(responseW)
	excess := fileW + editorW + width - m.width
	if excess <= 0 {
		return width, height
	}

	adjusted := responseW - excess
	if adjusted <= 0 {
		return 0, 0
	}
	width, height = m.responseMouseSize(adjusted)
	if fileW+editorW+width > m.width {
		return 0, 0
	}
	return width, height
}

func (m *Model) responseMouseSize(availableWidth int) (int, int) {
	if m.effectiveRegionCollapsed(paneRegionResponse) {
		return 0, 0
	}

	style := m.respFrameStyle(m.focus == focusResponse)
	frameWidth := style.GetHorizontalFrameSize()
	width := max(max(availableWidth, 0), frameWidth)
	if width < 1 && frameWidth == 0 {
		width = 1
	}

	contentHeight := m.responseContentHeight
	if contentHeight <= 0 {
		contentHeight = m.paneContentHeight
	}
	height := max(contentHeight+paneBottomPadding, 1)
	return width, height + style.GetVerticalFrameSize()
}

func paneMouseHeight(contentHeight int, style lipgloss.Style) int {
	if contentHeight < 0 {
		contentHeight = 0
	}
	return contentHeight + paneBottomPadding + style.GetVerticalFrameSize()
}

func (m *Model) paneBodyRect(outer mouseRect, style lipgloss.Style) mouseRect {
	if outer.Empty() {
		return mouseRect{}
	}
	left := style.GetBorderLeftSize() + style.GetPaddingLeft() + paneHorizontalPadding
	right := style.GetBorderRightSize() + style.GetPaddingRight() + paneHorizontalPadding
	top := style.GetBorderTopSize() + style.GetPaddingTop()
	bottom := style.GetBorderBottomSize() + style.GetPaddingBottom()
	return mouseRect{
		x: outer.x + left,
		y: outer.y + top,
		w: max(outer.w-left-right, 0),
		h: max(outer.h-top-bottom, 0),
	}
}

func (m *Model) responseMouseHit(outer mouseRect, x, y int) responseMouseHit {
	body := m.paneBodyRect(outer, m.respFrameStyle(m.focus == focusResponse))
	if !body.Contains(x, y) {
		return responseMouseHit{}
	}
	if !m.responseSplit {
		if pane := m.pane(responsePanePrimary); pane != nil {
			rect := responsePaneOuter(body.x, body.y, pane.viewport)
			if rect.Contains(x, y) {
				return responseMouseHit{id: responsePanePrimary, rect: rect, ok: true}
			}
		}
		return responseMouseHit{}
	}

	primary := m.pane(responsePanePrimary)
	secondary := m.pane(responsePaneSecondary)
	if primary == nil || secondary == nil {
		return responseMouseHit{}
	}

	if m.responseSplitOrientation == responseSplitHorizontal {
		primaryRect := responsePaneOuter(body.x, body.y, primary.viewport)
		if primaryRect.Contains(x, y) {
			return responseMouseHit{id: responsePanePrimary, rect: primaryRect, ok: true}
		}
		secondaryRect := responsePaneOuter(
			body.x,
			primaryRect.y+primaryRect.h+responseSplitSeparatorHeight,
			secondary.viewport,
		)
		if secondaryRect.Contains(x, y) {
			return responseMouseHit{id: responsePaneSecondary, rect: secondaryRect, ok: true}
		}
		return responseMouseHit{}
	}

	primaryRect := responsePaneOuter(body.x, body.y, primary.viewport)
	if primaryRect.Contains(x, y) {
		return responseMouseHit{id: responsePanePrimary, rect: primaryRect, ok: true}
	}
	secondaryRect := responsePaneOuter(
		primaryRect.x+primaryRect.w+responseSplitSeparatorWidth,
		body.y,
		secondary.viewport,
	)
	if secondaryRect.Contains(x, y) {
		return responseMouseHit{id: responsePaneSecondary, rect: secondaryRect, ok: true}
	}
	return responseMouseHit{}
}

func responsePaneOuter(x, y int, vp viewport.Model) mouseRect {
	return mouseRect{
		x: x,
		y: y,
		w: max(vp.Width, 1),
		h: max(vp.Height, 1) + responseTabsHeight,
	}
}
