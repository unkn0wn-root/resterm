package ui

import (
	"math"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/config"
)

func (m *Model) applyLayout() tea.Cmd {
	if !m.ready {
		return nil
	}

	sidebarCollapsed := m.effectiveRegionCollapsed(paneRegionSidebar)
	editorCollapsed := m.effectiveRegionCollapsed(paneRegionEditor)
	responseCollapsed := m.effectiveRegionCollapsed(paneRegionResponse)

	chromeHeight := lipgloss.Height(m.renderHeader()) +
		lipgloss.Height(m.renderCommandBar()) +
		lipgloss.Height(m.renderStatusBar())
	sidebarFrameHeight := m.paneVerticalFrameHeight(paneRegionSidebar)
	editorFrameHeight := m.paneVerticalFrameHeight(paneRegionEditor)
	responseFrameHeight := m.paneVerticalFrameHeight(paneRegionResponse)
	heightBudget := newPaneHeightBudget(
		m.height,
		chromeHeight,
		m.visiblePaneFrameHeight(),
		paneBottomPadding,
	)
	paneHeight := heightBudget.contentHeight

	m.paneContentHeight = paneHeight
	sidebarContentHeight := heightBudget.contentHeightForFrame(sidebarFrameHeight)
	editorFullHeight := heightBudget.contentHeightForFrame(editorFrameHeight)
	responseFullHeight := heightBudget.contentHeightForFrame(responseFrameHeight)

	if m.sidebarSplit <= 0 {
		m.sidebarSplit = sidebarSplitDefault
	}

	if m.editorSplit <= 0 {
		m.editorSplit = editorSplitDefault
	}

	if m.editorSplit < minEditorSplit {
		m.editorSplit = minEditorSplit
	}

	if m.editorSplit > maxEditorSplit {
		m.editorSplit = maxEditorSplit
	}

	if m.sidebarWidth <= 0 {
		m.sidebarWidth = sidebarWidthDefault
	}

	width := m.width
	desiredSidebar := 0
	if width > 0 {
		desiredSidebar = int(math.Round(float64(width) * m.sidebarWidth))
	}

	minSidebar := minSidebarWidthPixels
	if width > 0 {
		minRatioWidth := int(math.Round(float64(width) * minSidebarWidthRatio))
		if minRatioWidth > minSidebar {
			minSidebar = minRatioWidth
		}
		if minSidebar > width {
			minSidebar = width
		}
	}
	if minSidebar < 1 {
		minSidebar = 1
	}

	maxSidebar := max(minSidebarWidthPixels, 1)
	if width > 0 {
		ratioCap := max(int(math.Round(float64(width)*maxSidebarWidthRatio)), 1)
		maxSidebar = ratioCap
		contentCap := max(width-1, 1)
		if maxSidebar > contentCap {
			maxSidebar = contentCap
		}
	}
	if maxSidebar < 1 {
		maxSidebar = 1
	}
	if minSidebar > maxSidebar {
		minSidebar = maxSidebar
	}

	if desiredSidebar < minSidebar {
		desiredSidebar = minSidebar
	}
	if desiredSidebar > maxSidebar {
		desiredSidebar = maxSidebar
	}
	if desiredSidebar < 1 {
		desiredSidebar = 1
	}

	fileWidth := desiredSidebar
	m.sidebarWidthPx = fileWidth
	if sidebarCollapsed {
		fileWidth = 0
		m.sidebarWidthPx = 0
	}
	if fileWidth > width {
		fileWidth = width
	}
	remaining := max(width-fileWidth, 0)
	if remaining < 2 && (!editorCollapsed || !responseCollapsed) {
		remaining = 2
	}

	sidebarFrameWidth := m.theme.BrowserBorder.GetHorizontalFrameSize()
	editorFrameWidth := m.theme.EditorBorder.GetHorizontalFrameSize()
	responseFrameWidth := m.theme.ResponseBorder.GetHorizontalFrameSize()

	m.editorContentHeight = editorFullHeight
	m.responseContentHeight = responseFullHeight

	var editorWidth, responseWidth int
	var editorHeight, responseHeight int

	if m.mainSplitOrientation == mainSplitHorizontal {
		editorWidth = remaining
		responseWidth = remaining
		ratio := m.editorSplit
		if ratio <= 0 {
			ratio = editorSplitDefault
		}
		switch {
		case editorCollapsed && responseCollapsed:
			editorHeight, responseHeight = 0, 0
		case editorCollapsed:
			editorHeight = 0
			responseHeight = max(heightBudget.contentHeightForFrame(responseFrameHeight), 1)
		case responseCollapsed:
			editorHeight = max(heightBudget.contentHeightForFrame(editorFrameHeight), 1)
			responseHeight = 0
		default:
			editorHeight, responseHeight = splitStackedContentHeight(
				heightBudget.stackedContentHeight(editorFrameHeight, responseFrameHeight),
				ratio,
			)
		}
		m.editorContentHeight = editorHeight
		m.responseContentHeight = responseHeight
	} else {
		if remaining <= 0 {
			editorWidth = 0
			responseWidth = 0
		} else {
			editorMin := minEditorPaneWidth
			responseMin := minResponsePaneWidth
			ratio := m.editorSplit
			if ratio <= 0 {
				ratio = editorSplitDefault
			}

			if remaining < editorMin+responseMin {
				scaledEditor := max(int(math.Round(float64(remaining)*ratio)), 1)
				if scaledEditor > remaining-1 {
					scaledEditor = remaining - 1
				}
				editorMin = scaledEditor
				responseMin = remaining - editorMin
				if responseMin < 1 {
					responseMin = 1
					editorMin = max(remaining-responseMin, 1)
				}
			}

			desiredEditor := max(int(math.Round(float64(remaining)*ratio)), editorMin)

			maxEditor := max(remaining-responseMin, editorMin)
			if desiredEditor > maxEditor {
				desiredEditor = maxEditor
			}

			editorWidth = desiredEditor
			responseWidth = remaining - editorWidth
			if responseWidth < responseMin {
				responseWidth = responseMin
				editorWidth = remaining - responseWidth
			}
			if editorWidth < editorMin {
				editorWidth = editorMin
				responseWidth = remaining - editorWidth
			}

			if responseWidth < 1 {
				responseWidth = 1
				if remaining > 1 {
					editorWidth = remaining - responseWidth
				}
			}

			if editorWidth < 1 {
				editorWidth = 1
				if remaining > 1 {
					responseWidth = max(remaining-editorWidth, 1)
				}
			}
		}

		editorHeight = editorFullHeight
		m.editorContentHeight = editorFullHeight
		m.responseContentHeight = responseFullHeight
	}

	if m.mainSplitOrientation == mainSplitVertical {
		editorWidth, responseWidth = redistributeCollapsedWidths(
			editorWidth,
			responseWidth,
			editorCollapsed,
			responseCollapsed,
		)
	} else {
		m.editorContentHeight, m.responseContentHeight = redistributeCollapsedHeights(
			m.editorContentHeight,
			m.responseContentHeight,
			editorCollapsed,
			responseCollapsed,
		)
		editorHeight = m.editorContentHeight
	}

	if editorWidth < 1 && !editorCollapsed {
		editorWidth = 1
	}
	if responseWidth < 1 && !responseCollapsed {
		responseWidth = 1
	}
	m.responseWidthPx = responseWidth

	if width > 0 && !sidebarCollapsed && (!editorCollapsed || !responseCollapsed) {
		realSidebarRatio := float64(fileWidth) / float64(width)
		if realSidebarRatio < minSidebarWidthRatio {
			realSidebarRatio = minSidebarWidthRatio
		}
		if realSidebarRatio > maxSidebarWidthRatio {
			realSidebarRatio = maxSidebarWidthRatio
		}
		m.sidebarWidth = realSidebarRatio
	}

	m.sidebarFilesHeight = sidebarContentHeight
	m.sidebarRequestsHeight = sidebarContentHeight
	m.navigatorCompact = sidebarContentHeight < requestCompactSwitch
	if m.navigator != nil {
		m.navigator.SetCompact(m.navigatorCompact)
	}
	listWidth := paneContentWidth(fileWidth, sidebarFrameWidth)
	m.navigatorFilter.Width = listWidth
	m.fileList.SetSize(listWidth, sidebarContentHeight)
	m.requestList.SetSize(listWidth, 0)
	m.workflowList.SetSize(listWidth, 0)

	if m.mainSplitOrientation == mainSplitVertical && remaining > 0 &&
		!m.collapseState(paneRegionEditor) &&
		!m.collapseState(paneRegionResponse) && !m.zoomActive {
		realEditorRatio := float64(editorWidth) / float64(remaining)
		if realEditorRatio < minEditorSplit {
			realEditorRatio = minEditorSplit
		}
		if realEditorRatio > maxEditorSplit {
			realEditorRatio = maxEditorSplit
		}
		m.editorSplit = realEditorRatio
	}
	m.editor.SetWidth(max(paneContentWidth(editorWidth, editorFrameWidth), 1))
	m.editor.SetHeight(max(editorHeight, 1))

	primaryContentWidth := max(paneContentWidth(responseWidth, responseFrameWidth), 1)
	primaryPane := &m.responsePanes[0]
	secondaryPane := &m.responsePanes[1]

	responseViewportHeight := max(m.responseContentHeight-responseTabsHeight, 1)
	baseViewportHeight := max(responseViewportHeight, 1)

	if m.responseSplit {
		switch m.responseSplitOrientation {
		case responseSplitHorizontal:
			width := primaryContentWidth
			available := max(m.responseContentHeight-(responseTabsHeight*2+responseSplitSeparatorHeight), 0)
			ratio := clampResponseSplitRatio(m.responseSplitRatio)
			m.responseSplitRatio = ratio
			primaryHeight := int(math.Round(float64(available) * ratio))
			minHeight := minResponseSplitHeight
			if available < minHeight*2 {
				minHeight = max(available/2, 1)
			}
			if primaryHeight < minHeight {
				primaryHeight = minHeight
			}
			maxPrimary := available - minHeight
			if maxPrimary < primaryHeight {
				primaryHeight = maxPrimary
			}
			if primaryHeight < 1 {
				primaryHeight = max(available, 1)
			}
			secondaryHeight := available - primaryHeight
			if secondaryHeight < 1 && available > 0 {
				secondaryHeight = 1
				tmp := max(available-secondaryHeight, 1)
				primaryHeight = tmp
			}
			primaryPane.viewport.Width = max(width, 1)
			primaryPane.viewport.Height = max(primaryHeight, 1)
			secondaryPane.viewport.Width = max(width, 1)
			secondaryPane.viewport.Height = max(secondaryHeight, 1)
		default:
			available := max(primaryContentWidth-responseSplitSeparatorWidth, 0)
			var primaryWidth, secondaryWidth int
			if available <= 0 {
				primaryWidth, secondaryWidth = 1, 1
			} else if available < minResponseSplitWidth*2 {
				primaryWidth = max(available/2, 1)
				secondaryWidth = max(available-primaryWidth, 1)
			} else {
				ratio := clampResponseSplitRatio(m.responseSplitRatio)
				m.responseSplitRatio = ratio
				primaryWidth = max(int(math.Round(float64(available)*ratio)), minResponseSplitWidth)
				maxPrimary := available - minResponseSplitWidth
				if maxPrimary < minResponseSplitWidth {
					maxPrimary = available - minResponseSplitWidth
				}
				if maxPrimary < 1 {
					maxPrimary = 1
				}
				if primaryWidth > maxPrimary {
					primaryWidth = maxPrimary
				}
				if primaryWidth < 1 {
					primaryWidth = 1
				}
				secondaryWidth = max(available-primaryWidth, 1)
			}
			primaryPane.viewport.Width = max(primaryWidth, 1)
			primaryPane.viewport.Height = max(baseViewportHeight, 1)
			secondaryPane.viewport.Width = max(secondaryWidth, 1)
			secondaryPane.viewport.Height = max(baseViewportHeight, 1)
		}
	} else {
		primaryPane.viewport.Width = max(primaryContentWidth, 1)
		primaryPane.viewport.Height = max(baseViewportHeight, 1)
		secondaryPane.viewport.Width = max(primaryContentWidth, 1)
		secondaryPane.viewport.Height = max(baseViewportHeight, 1)
	}

	historyPane := primaryPane
	if m.responseSplit {
		if m.responsePanes[1].activeTab == responseTabHistory {
			historyPane = secondaryPane
		}
	}
	historyWidth := max(historyPane.viewport.Width, 1)
	historyHeight := max(historyPane.viewport.Height, 1)
	listHeight := max(historyHeight-m.historyHeaderHeight(), 1)
	m.historyList.SetSize(historyWidth, listHeight)
	if len(m.envList.Items()) > 0 {
		envWidth := max(min(40, m.width-6), 20)
		envHeight := max(min(paneHeight-4, 12), 5)
		m.envList.SetSize(envWidth, envHeight)
	}
	if len(m.themeList.Items()) > 0 {
		themeWidth := max(min(48, m.width-6), 24)
		themeHeight := max(min(paneHeight-4, 14), 5)
		m.themeList.SetSize(themeWidth, themeHeight)
	}
	return m.syncResponsePanes()
}

func clampResponseSplitRatio(ratio float64) float64 {
	if ratio == 0 {
		return responseSplitRatioDefault
	}
	if ratio < config.LayoutResponseRatioMin {
		return config.LayoutResponseRatioMin
	}
	if ratio > config.LayoutResponseRatioMax {
		return config.LayoutResponseRatioMax
	}
	return ratio
}

func (m *Model) adjustSidebarWidth(delta float64) (bool, bool, tea.Cmd) {
	if !m.ready || m.width <= 0 {
		return false, false, nil
	}

	current := m.sidebarWidth
	if current <= 0 {
		current = sidebarWidthDefault
	}

	updated := current + delta
	bounded := false
	if updated < minSidebarWidthRatio {
		updated = minSidebarWidthRatio
		bounded = true
	}
	if updated > maxSidebarWidthRatio {
		updated = maxSidebarWidthRatio
		bounded = true
	}

	if math.Abs(updated-current) < 1e-6 {
		return false, bounded, nil
	}

	prevRatio := m.sidebarWidth
	prevWidth := m.sidebarWidthPx
	m.sidebarWidth = updated
	cmd := m.applyLayout()
	newRatio := m.sidebarWidth
	newWidth := m.sidebarWidthPx
	changed := math.Abs(newRatio-prevRatio) > 1e-6 || newWidth != prevWidth
	if !changed {
		return false, true, cmd
	}
	return true, bounded, cmd
}

func (m *Model) setMainSplitOrientation(orientation mainSplitOrientation) tea.Cmd {
	if orientation != mainSplitVertical && orientation != mainSplitHorizontal {
		return nil
	}
	if m.mainSplitOrientation == orientation {
		return nil
	}

	previous := m.mainSplitOrientation
	m.mainSplitOrientation = orientation
	cmd := m.applyLayout()

	var note string
	switch orientation {
	case mainSplitHorizontal:
		note = "Response pane moved below editor"
	default:
		note = "Response pane moved beside editor"
	}
	if previous == orientation {
		return cmd
	}
	status := func() tea.Msg {
		return statusMsg{text: note, level: statusInfo}
	}
	if cmd != nil {
		return tea.Batch(cmd, status)
	}
	return status
}

func (m *Model) adjustEditorSplit(delta float64) (bool, bool, tea.Cmd) {
	if !m.ready || m.width <= 0 {
		return false, false, nil
	}

	current := m.editorSplit
	if current <= 0 {
		current = editorSplitDefault
	}

	prevSplit := current
	updated := current + delta
	bounded := false
	if updated < minEditorSplit {
		updated = minEditorSplit
		bounded = true
	}
	if updated > maxEditorSplit {
		updated = maxEditorSplit
		bounded = true
	}

	if math.Abs(updated-current) < 1e-6 {
		return false, bounded, nil
	}

	prevEditorWidth := m.editor.Width()
	prevResponseWidth := m.responseContentWidth()
	m.editorSplit = updated
	cmd := m.applyLayout()

	newSplit := m.editorSplit
	newEditorWidth := m.editor.Width()
	newResponseWidth := m.responseContentWidth()
	changed := math.Abs(newSplit-prevSplit) > 1e-6 || newEditorWidth != prevEditorWidth ||
		newResponseWidth != prevResponseWidth
	if !changed {
		return false, true, cmd
	}

	return true, bounded, cmd
}

func splitStackedContentHeight(availableHeight int, ratio float64) (int, int) {
	availableHeight = max(availableHeight, 0)
	minEditor := max(minEditorPaneHeight, 1)
	minResponse := max(minResponsePaneHeight, 1)
	if availableHeight < minEditor+minResponse {
		distributed := max(availableHeight/2, 1)
		minEditor = distributed
		minResponse = availableHeight - distributed
		if minResponse < 1 {
			minResponse = 1
			if availableHeight > 1 {
				minEditor = availableHeight - minResponse
			}
		}
	}

	editorHeight := max(int(math.Round(float64(availableHeight)*ratio)), minEditor)

	maxEditor := max(availableHeight-minResponse, minEditor)
	if editorHeight > maxEditor {
		editorHeight = maxEditor
	}

	responseHeight := availableHeight - editorHeight
	if responseHeight < minResponse {
		responseHeight = minResponse
		editorHeight = max(availableHeight-responseHeight, 1)
	}
	if responseHeight < 1 {
		responseHeight = 1
	}
	return editorHeight, responseHeight
}

type paneHeightBudget struct {
	contentHeight  int
	rowFrameHeight int
	bottomPadding  int
}

func newPaneHeightBudget(
	totalHeight int,
	chromeHeight int,
	rowFrameHeight int,
	bottomPadding int,
) paneHeightBudget {
	return paneHeightBudget{
		contentHeight: max(
			totalHeight-chromeHeight-rowFrameHeight-bottomPadding,
			minPaneContentHeight,
		),
		rowFrameHeight: rowFrameHeight,
		bottomPadding:  bottomPadding,
	}
}

func (b paneHeightBudget) contentHeightForFrame(frameHeight int) int {
	return max(b.contentHeight+b.rowFrameHeight-frameHeight, 0)
}

func (b paneHeightBudget) outerHeight() int {
	return b.contentHeight + b.rowFrameHeight + b.bottomPadding
}

func (b paneHeightBudget) stackedContentHeight(firstFrameHeight, secondFrameHeight int) int {
	return max(
		b.outerHeight()-firstFrameHeight-secondFrameHeight-(b.bottomPadding*2),
		0,
	)
}

func (m *Model) visiblePaneFrameHeight() int {
	frameHeight := 0
	for _, region := range [...]paneRegion{
		paneRegionSidebar,
		paneRegionEditor,
		paneRegionResponse,
	} {
		if m.effectiveRegionCollapsed(region) {
			continue
		}
		frameHeight = max(frameHeight, m.paneVerticalFrameHeight(region))
	}
	return frameHeight
}

func (m *Model) paneVerticalFrameHeight(region paneRegion) int {
	switch region {
	case paneRegionSidebar:
		return m.theme.BrowserBorder.GetVerticalFrameSize()
	case paneRegionResponse:
		return m.theme.ResponseBorder.GetVerticalFrameSize()
	default:
		return m.theme.EditorBorder.GetVerticalFrameSize()
	}
}

func redistributeCollapsedWidths(
	editorWidth, responseWidth int,
	editorCollapsed, responseCollapsed bool,
) (int, int) {
	if editorWidth < 0 {
		editorWidth = 0
	}
	if responseWidth < 0 {
		responseWidth = 0
	}

	freed := 0
	if editorCollapsed {
		freed += editorWidth
		editorWidth = 0
	}
	if responseCollapsed {
		freed += responseWidth
		responseWidth = 0
	}
	if freed > 0 {
		switch {
		case !editorCollapsed:
			editorWidth += freed
		case !responseCollapsed:
			responseWidth += freed
		}
	}
	if editorWidth < 1 && !editorCollapsed {
		editorWidth = 1
	}
	if responseWidth < 1 && !responseCollapsed {
		responseWidth = 1
	}
	return editorWidth, responseWidth
}

func redistributeCollapsedHeights(
	editorHeight, responseHeight int,
	editorCollapsed, responseCollapsed bool,
) (int, int) {
	if editorHeight < 0 {
		editorHeight = 0
	}
	if responseHeight < 0 {
		responseHeight = 0
	}
	freed := 0
	if editorCollapsed {
		freed += editorHeight
		editorHeight = 0
	}
	if responseCollapsed {
		freed += responseHeight
		responseHeight = 0
	}
	if freed > 0 {
		switch {
		case !editorCollapsed:
			editorHeight += freed
		case !responseCollapsed:
			responseHeight += freed
		}
	}
	if editorHeight < 1 && !editorCollapsed {
		editorHeight = 1
	}
	if responseHeight < 1 && !responseCollapsed {
		responseHeight = 1
	}
	return editorHeight, responseHeight
}

func paneInnerWidth(outerWidth, frameWidth int) int {
	if outerWidth <= 0 {
		return 0
	}
	inner := outerWidth - frameWidth
	if inner < 0 {
		return 0
	}
	return inner
}

func paneContentWidth(outerWidth, frameWidth int) int {
	if outerWidth <= 0 {
		return 0
	}
	inner := max(outerWidth-frameWidth, 0)
	content := max(inner-(paneHorizontalPadding*2), 0)
	return content
}

func paneOuterWidthFromContent(contentWidth, frameWidth int) int {
	if contentWidth <= 0 {
		return 0
	}
	outer := contentWidth + frameWidth + (paneHorizontalPadding * 2)
	if outer < 0 {
		return 0
	}
	return outer
}
