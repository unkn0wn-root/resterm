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

	paneHeight := m.height - chromeHeight - 4
	if paneHeight < 4 {
		paneHeight = 4
	}

	m.paneContentHeight = paneHeight

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

	maxSidebar := minSidebarWidthPixels
	if maxSidebar < 1 {
		maxSidebar = 1
	}
	if width > 0 {
		ratioCap := int(math.Round(float64(width) * maxSidebarWidthRatio))
		if ratioCap < 1 {
			ratioCap = 1
		}
		maxSidebar = ratioCap
		contentCap := width - 1
		if contentCap < 1 {
			contentCap = 1
		}
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
	remaining := width - fileWidth
	if remaining < 0 {
		remaining = 0
	}
	if remaining < 2 && (!editorCollapsed || !responseCollapsed) {
		remaining = 2
	}

	sidebarFrameWidth := m.theme.BrowserBorder.GetHorizontalFrameSize()
	editorFrameWidth := m.theme.EditorBorder.GetHorizontalFrameSize()
	responseFrameWidth := m.theme.ResponseBorder.GetHorizontalFrameSize()

	m.editorContentHeight = paneHeight
	m.responseContentHeight = paneHeight

	var editorWidth, responseWidth int
	var editorHeight, responseHeight int

	if m.mainSplitOrientation == mainSplitHorizontal {
		editorWidth = remaining
		responseWidth = remaining
		editorFrameHeight := m.theme.EditorBorder.GetVerticalFrameSize()
		responseFrameHeight := m.theme.ResponseBorder.GetVerticalFrameSize()
		ratio := m.editorSplit
		if ratio <= 0 {
			ratio = editorSplitDefault
		}
		editorHeight, responseHeight = m.horizontalContentHeights(paneHeight, editorCollapsed, responseCollapsed, ratio, editorFrameHeight, responseFrameHeight)
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
				scaledEditor := int(math.Round(float64(remaining) * ratio))
				if scaledEditor < 1 {
					scaledEditor = 1
				}
				if scaledEditor > remaining-1 {
					scaledEditor = remaining - 1
				}
				editorMin = scaledEditor
				responseMin = remaining - editorMin
				if responseMin < 1 {
					responseMin = 1
					editorMin = remaining - responseMin
					if editorMin < 1 {
						editorMin = 1
					}
				}
			}

			desiredEditor := int(math.Round(float64(remaining) * ratio))
			if desiredEditor < editorMin {
				desiredEditor = editorMin
			}

			maxEditor := remaining - responseMin
			if maxEditor < editorMin {
				maxEditor = editorMin
			}
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
					responseWidth = remaining - editorWidth
					if responseWidth < 1 {
						responseWidth = 1
					}
				}
			}
		}

		editorHeight = paneHeight
		m.editorContentHeight = paneHeight
		m.responseContentHeight = paneHeight
	}

	if m.mainSplitOrientation == mainSplitVertical {
		editorWidth, responseWidth = redistributeCollapsedWidths(editorWidth, responseWidth, editorCollapsed, responseCollapsed)
	} else {
		m.editorContentHeight, m.responseContentHeight = redistributeCollapsedHeights(m.editorContentHeight, m.responseContentHeight, editorCollapsed, responseCollapsed)
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

	m.sidebarFilesHeight = paneHeight
	m.sidebarRequestsHeight = paneHeight
	m.navigatorCompact = paneHeight < requestCompactSwitch
	if m.navigator != nil {
		m.navigator.SetCompact(m.navigatorCompact)
	}
	listWidth := paneContentWidth(fileWidth, sidebarFrameWidth)
	m.navigatorFilter.Width = listWidth
	m.fileList.SetSize(listWidth, paneHeight)
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
	m.editor.SetWidth(maxInt(paneContentWidth(editorWidth, editorFrameWidth), 1))
	m.editor.SetHeight(maxInt(editorHeight, 1))

	primaryContentWidth := maxInt(paneContentWidth(responseWidth, responseFrameWidth), 1)
	primaryPane := &m.responsePanes[0]
	secondaryPane := &m.responsePanes[1]

	const responseTabsHeight = 1
	responseViewportHeight := m.responseContentHeight - responseTabsHeight
	if responseViewportHeight < 1 {
		responseViewportHeight = 1
	}
	baseViewportHeight := responseViewportHeight
	if baseViewportHeight < 1 {
		baseViewportHeight = 1
	}

	if m.responseSplit {
		switch m.responseSplitOrientation {
		case responseSplitHorizontal:
			width := primaryContentWidth
			available := m.responseContentHeight - (responseTabsHeight*2 + responseSplitSeparatorHeight)
			if available < 0 {
				available = 0
			}
			ratio := clampResponseSplitRatio(m.responseSplitRatio)
			m.responseSplitRatio = ratio
			primaryHeight := int(math.Round(float64(available) * ratio))
			minHeight := minResponseSplitHeight
			if available < minHeight*2 {
				minHeight = maxInt(available/2, 1)
			}
			if primaryHeight < minHeight {
				primaryHeight = minHeight
			}
			maxPrimary := available - minHeight
			if maxPrimary < primaryHeight {
				primaryHeight = maxPrimary
			}
			if primaryHeight < 1 {
				primaryHeight = maxInt(available, 1)
			}
			secondaryHeight := available - primaryHeight
			if secondaryHeight < 1 && available > 0 {
				secondaryHeight = 1
				tmp := available - secondaryHeight
				if tmp < 1 {
					tmp = 1
				}
				primaryHeight = tmp
			}
			primaryPane.viewport.Width = maxInt(width, 1)
			primaryPane.viewport.Height = maxInt(primaryHeight, 1)
			secondaryPane.viewport.Width = maxInt(width, 1)
			secondaryPane.viewport.Height = maxInt(secondaryHeight, 1)
		default:
			available := primaryContentWidth - responseSplitSeparatorWidth
			if available < 0 {
				available = 0
			}
			var primaryWidth, secondaryWidth int
			if available <= 0 {
				primaryWidth, secondaryWidth = 1, 1
			} else if available < minResponseSplitWidth*2 {
				primaryWidth = maxInt(available/2, 1)
				secondaryWidth = available - primaryWidth
				if secondaryWidth < 1 {
					secondaryWidth = 1
				}
			} else {
				ratio := clampResponseSplitRatio(m.responseSplitRatio)
				m.responseSplitRatio = ratio
				primaryWidth = int(math.Round(float64(available) * ratio))
				if primaryWidth < minResponseSplitWidth {
					primaryWidth = minResponseSplitWidth
				}
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
				secondaryWidth = available - primaryWidth
				if secondaryWidth < 1 {
					secondaryWidth = 1
				}
			}
			primaryPane.viewport.Width = maxInt(primaryWidth, 1)
			primaryPane.viewport.Height = maxInt(baseViewportHeight, 1)
			secondaryPane.viewport.Width = maxInt(secondaryWidth, 1)
			secondaryPane.viewport.Height = maxInt(baseViewportHeight, 1)
		}
	} else {
		primaryPane.viewport.Width = maxInt(primaryContentWidth, 1)
		primaryPane.viewport.Height = maxInt(baseViewportHeight, 1)
		secondaryPane.viewport.Width = maxInt(primaryContentWidth, 1)
		secondaryPane.viewport.Height = maxInt(baseViewportHeight, 1)
	}

	historyPane := primaryPane
	if m.responseSplit {
		if m.responsePanes[1].activeTab == responseTabHistory {
			historyPane = secondaryPane
		}
	}
	historyWidth := maxInt(historyPane.viewport.Width, 1)
	historyHeight := maxInt(historyPane.viewport.Height, 1)
	m.historyList.SetSize(historyWidth, historyHeight)
	if len(m.envList.Items()) > 0 {
		envWidth := minInt(40, m.width-6)
		if envWidth < 20 {
			envWidth = 20
		}
		envHeight := minInt(paneHeight-4, 12)
		if envHeight < 5 {
			envHeight = 5
		}
		m.envList.SetSize(envWidth, envHeight)
	}
	if len(m.themeList.Items()) > 0 {
		themeWidth := minInt(48, m.width-6)
		if themeWidth < 24 {
			themeWidth = 24
		}
		themeHeight := minInt(paneHeight-4, 14)
		if themeHeight < 5 {
			themeHeight = 5
		}
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
	changed := math.Abs(newSplit-prevSplit) > 1e-6 || newEditorWidth != prevEditorWidth || newResponseWidth != prevResponseWidth
	if !changed {
		return false, true, cmd
	}

	return true, bounded, cmd
}

func (m *Model) horizontalContentHeights(
	paneHeight int,
	editorCollapsed, responseCollapsed bool,
	ratio float64,
	editorFrame, responseFrame int,
) (int, int) {
	if paneHeight < 0 {
		paneHeight = 0
	}
	if editorCollapsed && responseCollapsed {
		return 0, maxInt(paneHeight, 1)
	}
	if editorCollapsed {
		return 0, maxInt(paneHeight, 1)
	}
	if responseCollapsed {
		return maxInt(paneHeight, 1), 0
	}

	frameAllowance := 0
	if !editorCollapsed {
		frameAllowance += editorFrame
	}
	if !responseCollapsed {
		frameAllowance += responseFrame
	}

	availableHeight := paneHeight - frameAllowance
	if availableHeight < 0 {
		availableHeight = 0
	}

	minEditor := maxInt(minEditorPaneHeight, 1)
	minResponse := maxInt(minResponsePaneHeight, 1)
	if availableHeight < minEditor+minResponse {
		distributed := maxInt(availableHeight/2, 1)
		minEditor = distributed
		minResponse = availableHeight - distributed
		if minResponse < 1 {
			minResponse = 1
			if availableHeight > 1 {
				minEditor = availableHeight - minResponse
			}
		}
	}

	editorHeight := int(math.Round(float64(availableHeight) * ratio))
	if editorHeight < minEditor {
		editorHeight = minEditor
	}

	maxEditor := availableHeight - minResponse
	if maxEditor < minEditor {
		maxEditor = minEditor
	}
	if editorHeight > maxEditor {
		editorHeight = maxEditor
	}

	responseHeight := availableHeight - editorHeight
	if responseHeight < minResponse {
		responseHeight = minResponse
		editorHeight = availableHeight - responseHeight
		if editorHeight < 1 {
			editorHeight = 1
		}
	}
	if responseHeight < 1 {
		responseHeight = 1
	}
	return editorHeight, responseHeight
}

func redistributeCollapsedWidths(editorWidth, responseWidth int, editorCollapsed, responseCollapsed bool) (int, int) {
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

func redistributeCollapsedHeights(editorHeight, responseHeight int, editorCollapsed, responseCollapsed bool) (int, int) {
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
	inner := outerWidth - frameWidth
	if inner < 0 {
		inner = 0
	}
	content := inner - (paneHorizontalPadding * 2)
	if content < 0 {
		content = 0
	}
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
