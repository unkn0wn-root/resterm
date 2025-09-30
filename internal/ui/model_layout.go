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

	fileWidth := m.width / 5
	if fileWidth < 20 {
		fileWidth = 20
	}

	remaining := m.width - fileWidth
	if remaining < 40 {
		remaining = 40
	}

	editorWidth := remaining * 3 / 5
	if editorWidth < 30 {
		editorWidth = 30
	}

	responseWidth := remaining - editorWidth
	if responseWidth < 30 {
		responseWidth = 30
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

	m.fileList.SetSize(fileWidth-4, maxInt(filesHeight-2, 1))
	m.requestList.SetSize(fileWidth-4, maxInt(requestsHeight-2, 1))
	m.editor.SetWidth(editorWidth - 4)
	m.editor.SetHeight(paneHeight - 2)
	m.responseViewport.Width = responseWidth - 4
	m.responseViewport.Height = paneHeight - 4
	m.historyList.SetSize(responseWidth-4, paneHeight-4)
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
	return m.syncResponseContent()
}

func (m *Model) adjustSidebarSplit(delta float64) (bool, tea.Cmd) {
	if m.focus != focusFile && m.focus != focusRequests {
		return false, nil
	}

	if !m.ready || m.height <= 0 {
		return false, nil
	}

	current := m.sidebarSplit
	if current <= 0 {
		current = sidebarSplitDefault
	}

	updated := current + delta
	if updated < minSidebarSplit {
		updated = minSidebarSplit
	}

	if updated > maxSidebarSplit {
		updated = maxSidebarSplit
	}

	if math.Abs(updated-current) < 1e-6 {
		return false, nil
	}

	m.sidebarSplit = updated
	return true, m.applyLayout()
}
