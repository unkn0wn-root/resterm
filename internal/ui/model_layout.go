package ui

import (
	"math"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) applyLayout() tea.Cmd {
	if !m.ready {
		return nil
	}

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

	remaining := width - fileWidth
	if remaining < 2 {
		remaining = 2
	}

	m.editorContentHeight = paneHeight
	m.responseContentHeight = paneHeight

	var editorWidth, responseWidth int
	editorHeight := paneHeight
	responseHeight := paneHeight

	if m.mainSplitOrientation == mainSplitHorizontal {
		editorWidth = remaining
		responseWidth = remaining
		editorFrame := m.theme.EditorBorder.GetVerticalFrameSize()
		responseFrame := m.theme.ResponseBorder.GetVerticalFrameSize()
		frameAllowance := editorFrame + responseFrame
		availableHeight := paneHeight - frameAllowance
		if availableHeight < 2 {
			availableHeight = maxInt(paneHeight-frameAllowance, 1)
		}

		ratio := m.editorSplit
		if ratio <= 0 {
			ratio = editorSplitDefault
		}

		minEditor := minEditorPaneHeight
		if minEditor < 1 {
			minEditor = 1
		}
		minResponse := minResponsePaneHeight
		if minResponse < 1 {
			minResponse = 1
		}
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

		editorHeight = int(math.Round(float64(availableHeight) * ratio))
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
		responseHeight = availableHeight - editorHeight
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
		m.editorContentHeight = editorHeight
		m.responseContentHeight = responseHeight
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

		editorHeight = paneHeight
		responseHeight = paneHeight
		m.editorContentHeight = paneHeight
		m.responseContentHeight = paneHeight
	}

	if editorWidth < 1 {
		editorWidth = 1
	}
	if responseWidth < 1 {
		responseWidth = 1
	}

	if width > 0 {
		realSidebarRatio := float64(fileWidth) / float64(width)
		if realSidebarRatio < minSidebarWidthRatio {
			realSidebarRatio = minSidebarWidthRatio
		}
		if realSidebarRatio > maxSidebarWidthRatio {
			realSidebarRatio = maxSidebarWidthRatio
		}
		m.sidebarWidth = realSidebarRatio
	}

	hasWorkflow := len(m.workflowItems) > 0
	fileBorder := 0
	requestsBorder := 0
	workflowBorder := 0
	switch m.focus {
	case focusFile:
		fileBorder = sidebarFocusPad
	case focusRequests:
		requestsBorder = sidebarFocusPad
	case focusWorkflows:
		if hasWorkflow {
			workflowBorder = sidebarFocusPad
		}
	}

	padding := sidebarSplitPadding
	if hasWorkflow {
		padding++
	}

	borderExtra := fileBorder + requestsBorder + workflowBorder
	available := paneHeight - padding - borderExtra
	if available < 0 {
		available = 0
	}

	filesBase := int(math.Round(float64(available) * m.sidebarSplit))
	if filesBase < 0 {
		filesBase = 0
	}

	minRequestsBase := minSidebarRequests
	if hasWorkflow {
		minRequestsBase = minSidebarRequests * 2
	}
	if minRequestsBase > available {
		minRequestsBase = available
	}
	maxFilesBase := available - minRequestsBase
	if maxFilesBase < 0 {
		maxFilesBase = 0
	}

	minFilesBase := minSidebarFiles
	if minFilesBase > available {
		minFilesBase = available
	}
	if minFilesBase > maxFilesBase {
		minFilesBase = maxFilesBase
	}
	if filesBase < minFilesBase {
		filesBase = minFilesBase
	}
	if filesBase > maxFilesBase {
		filesBase = maxFilesBase
	}

	requestsBase := available - filesBase
	if requestsBase < minRequestsBase {
		desired := minRequestsBase
		if desired > available {
			desired = available
		}
		requestsBase = desired
		filesBase = available - requestsBase
		if filesBase < 0 {
			filesBase = 0
		}
	}

	restRequests := requestsBase
	workflowBase := 0
	requestBase := restRequests
	if hasWorkflow {
		if m.workflowSplit <= 0 {
			m.workflowSplit = workflowSplitDefault
		}
		if m.workflowSplit < minWorkflowSplit {
			m.workflowSplit = minWorkflowSplit
		}
		if m.workflowSplit > maxWorkflowSplit {
			m.workflowSplit = maxWorkflowSplit
		}

		available := restRequests
		minPrimary := minSidebarRequests
		minWorkflow := minSidebarRequests
		if available < minPrimary+minWorkflow {
			requestBase = minInt(available, minPrimary)
			workflowBase = available - requestBase
			if workflowBase < 0 {
				workflowBase = 0
			}
		} else {
			desiredWorkflow := int(math.Round(float64(available) * (1 - m.workflowSplit)))
			if desiredWorkflow < minWorkflow {
				desiredWorkflow = minWorkflow
			}
			if desiredWorkflow > available-minPrimary {
				desiredWorkflow = available - minPrimary
			}
			if desiredWorkflow < 0 {
				desiredWorkflow = 0
			}
			workflowBase = desiredWorkflow
			requestBase = available - workflowBase
		}

		total := requestBase + workflowBase
		if total > 0 {
			ratio := float64(requestBase) / float64(total)
			if ratio < minWorkflowSplit {
				ratio = minWorkflowSplit
			}
			if ratio > maxWorkflowSplit {
				ratio = maxWorkflowSplit
			}
			m.workflowSplit = ratio
		}
	} else {
		workflowBase = 0
	}
	requestsBase = requestBase + workflowBase

	filesHeight := filesBase + fileBorder
	requestHeight := requestBase + requestsBorder
	workflowHeight := workflowBase + workflowBorder
	combinedRequestsHeight := requestHeight + workflowHeight

	m.sidebarFilesHeight = filesHeight
	m.sidebarRequestsHeight = combinedRequestsHeight

	baseTotal := filesBase + requestsBase
	if baseTotal > 0 {
		ratio := float64(filesBase) / float64(baseTotal)
		if ratio < minSidebarSplit {
			ratio = minSidebarSplit
		}
		if ratio > maxSidebarSplit {
			ratio = maxSidebarSplit
		}
		m.sidebarSplit = ratio
	}

	if m.mainSplitOrientation == mainSplitVertical && remaining > 0 {
		realEditorRatio := float64(editorWidth) / float64(remaining)
		if realEditorRatio < minEditorSplit {
			realEditorRatio = minEditorSplit
		}
		if realEditorRatio > maxEditorSplit {
			realEditorRatio = maxEditorSplit
		}
		m.editorSplit = realEditorRatio
	}

	fileListHeight := filesBase - sidebarChrome
	if fileListHeight < 0 {
		fileListHeight = 0
	}
	m.fileList.SetSize(fileWidth-4, fileListHeight)

	requestListHeight := requestBase - sidebarChrome
	if requestListHeight < 0 {
		requestListHeight = 0
	}
	m.requestList.SetSize(fileWidth-4, requestListHeight)

	if hasWorkflow && workflowBase > 0 {
		workflowListHeight := workflowBase - sidebarChrome
		if workflowListHeight < 0 {
			workflowListHeight = 0
		}
		m.workflowList.SetSize(fileWidth-4, workflowListHeight)
	} else {
		m.workflowList.SetSize(fileWidth-4, 0)
	}
	m.editor.SetWidth(maxInt(editorWidth-4, 1))
	m.editor.SetHeight(maxInt(editorHeight, 1))

	primaryContentWidth := maxInt(responseWidth-4, 1)
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
			ratio := m.responseSplitRatio
			if ratio <= 0 {
				ratio = 0.5
			}
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
				ratio := m.responseSplitRatio
				if ratio <= 0 {
					ratio = 0.5
				}
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

func (m *Model) adjustSidebarSplit(delta float64) (bool, bool, tea.Cmd) {
	if !m.ready || m.height <= 0 {
		return false, false, nil
	}

	current := m.sidebarSplit
	if current <= 0 {
		current = sidebarSplitDefault
	}

	updated := current + delta
	bounded := false
	if updated < minSidebarSplit {
		updated = minSidebarSplit
		bounded = true
	}

	if updated > maxSidebarSplit {
		updated = maxSidebarSplit
		bounded = true
	}

	if math.Abs(updated-current) < 1e-6 {
		return false, bounded, nil
	}

	prevSplit := m.sidebarSplit
	prevFiles := m.sidebarFilesHeight
	prevRequests := m.sidebarRequestsHeight
	m.sidebarSplit = updated
	cmd := m.applyLayout()
	newSplit := m.sidebarSplit
	newFiles := m.sidebarFilesHeight
	newRequests := m.sidebarRequestsHeight
	changed := math.Abs(newSplit-prevSplit) > 1e-6 || newFiles != prevFiles || newRequests != prevRequests
	if !changed {
		return false, true, cmd
	}
	return true, bounded, cmd
}

func (m *Model) adjustWorkflowSplit(delta float64) (bool, bool, tea.Cmd) {
	if !m.ready || len(m.workflowItems) == 0 {
		return false, false, nil
	}

	current := m.workflowSplit
	if current <= 0 {
		current = workflowSplitDefault
	}

	updated := current + delta
	bounded := false
	if updated < minWorkflowSplit {
		updated = minWorkflowSplit
		bounded = true
	}
	if updated > maxWorkflowSplit {
		updated = maxWorkflowSplit
		bounded = true
	}

	if math.Abs(updated-current) < 1e-6 {
		return false, bounded, nil
	}

	prev := m.workflowSplit
	m.workflowSplit = updated
	cmd := m.applyLayout()
	changed := math.Abs(m.workflowSplit-prev) > 1e-6
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
