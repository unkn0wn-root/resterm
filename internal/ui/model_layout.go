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

	fileWidth := m.width / 5
	if fileWidth < 20 {
		fileWidth = 20
	}

	remaining := m.width - fileWidth
	minimumRemaining := minEditorPaneWidth + minResponsePaneWidth
	if remaining < minimumRemaining {
		remaining = minimumRemaining
	}

	desiredEditor := int(math.Round(float64(remaining) * m.editorSplit))
	if desiredEditor < minEditorPaneWidth {
		desiredEditor = minEditorPaneWidth
	}

	maxEditor := remaining - minResponsePaneWidth
	if desiredEditor > maxEditor {
		desiredEditor = maxEditor
	}

	editorWidth := desiredEditor
	if editorWidth < minEditorPaneWidth {
		editorWidth = minEditorPaneWidth
	}

	responseWidth := remaining - editorWidth
	if responseWidth < minResponsePaneWidth {
		responseWidth = minResponsePaneWidth
		editorWidth = remaining - responseWidth
		if editorWidth < minEditorPaneWidth {
			editorWidth = minEditorPaneWidth
		}
	}

	if editorWidth < 1 {
		editorWidth = 1
	}

	if responseWidth < 1 {
		responseWidth = 1
	}

	available := paneHeight - sidebarSplitPadding
	if available < 0 {
		available = 0
	}

	filesHeight := int(math.Round(float64(paneHeight) * m.sidebarSplit))
	if filesHeight < 0 {
		filesHeight = 0
	}

	maxFiles := available - minSidebarRequests
	if maxFiles < 0 {
		maxFiles = 0
	}

	minFilesAllowed := minSidebarFiles
	if minFilesAllowed > available {
		minFilesAllowed = available
	}
	if minFilesAllowed > maxFiles {
		minFilesAllowed = maxFiles
	}
	if filesHeight < minFilesAllowed {
		filesHeight = minFilesAllowed
	}
	if filesHeight > maxFiles {
		filesHeight = maxFiles
	}

	requestsHeight := available - filesHeight
	if requestsHeight < minSidebarRequests {
		desired := minSidebarRequests
		if desired > available {
			desired = available
		}
		requestsHeight = desired
		filesHeight = available - requestsHeight
	}

	if filesHeight < 0 {
		filesHeight = 0
	}

	if requestsHeight < 0 {
		requestsHeight = 0
	}

	m.sidebarFilesHeight = filesHeight
	m.sidebarRequestsHeight = requestsHeight
	if paneHeight > 0 {
		ratio := float64(filesHeight) / float64(paneHeight)
		if ratio < minSidebarSplit {
			ratio = minSidebarSplit
		}
		if ratio > maxSidebarSplit {
			ratio = maxSidebarSplit
		}
		m.sidebarSplit = ratio
	}

	if remaining > 0 {
		realEditorRatio := float64(editorWidth) / float64(remaining)
		if realEditorRatio < minEditorSplit {
			realEditorRatio = minEditorSplit
		}
		if realEditorRatio > maxEditorSplit {
			realEditorRatio = maxEditorSplit
		}
		m.editorSplit = realEditorRatio
	}

	m.fileList.SetSize(fileWidth-4, maxInt(filesHeight-2, 1))
	m.requestList.SetSize(fileWidth-4, maxInt(requestsHeight-2, 1))
	m.editor.SetWidth(maxInt(editorWidth-4, 1))
	m.editor.SetHeight(paneHeight - 2)

	primaryContentWidth := maxInt(responseWidth-4, 1)
	responseContentHeight := paneHeight - 4
	primaryPane := &m.responsePanes[0]
	secondaryPane := &m.responsePanes[1]

	if m.responseSplit {
		switch m.responseSplitOrientation {
		case responseSplitHorizontal:
			width := primaryContentWidth
			available := responseContentHeight - responseSplitSeparatorHeight
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
			primaryPane.viewport.Height = responseContentHeight
			secondaryPane.viewport.Width = maxInt(secondaryWidth, 1)
			secondaryPane.viewport.Height = responseContentHeight
		}
	} else {
		primaryPane.viewport.Width = maxInt(primaryContentWidth, 1)
		primaryPane.viewport.Height = responseContentHeight
		secondaryPane.viewport.Width = maxInt(primaryContentWidth, 1)
		secondaryPane.viewport.Height = responseContentHeight
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
	return m.syncResponsePanes()
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

	m.sidebarSplit = updated
	return true, bounded, m.applyLayout()
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
