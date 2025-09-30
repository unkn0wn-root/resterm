package ui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.frameWidth = typed.Width
		m.frameHeight = typed.Height
		m.width = maxInt(typed.Width-2, 0)
		m.height = maxInt(typed.Height-2, 0)
		if !m.ready {
			m.ready = true
		}
		if cmd := m.applyLayout(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case tea.KeyMsg:
		if cmd := m.handleKey(typed); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case responseMsg:
		m.sending = false
		m.statusPulseBase = ""
		m.statusPulseFrame = 0
		if cmd := m.handleResponseMessage(typed); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case statusMsg:
		m.setStatusMessage(typed)
	case statusPulseMsg:
		if cmd := m.handleStatusPulse(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case responseRenderedMsg:
		if cmd := m.handleResponseRendered(typed); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case responseWrapMsg:
		if cmd := m.handleResponseWrap(typed); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case responseLoadingTickMsg:
		if cmd := m.handleResponseLoadingTick(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if m.showErrorModal {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc", "enter":
				m.closeErrorModal()
				return m, nil
			case "ctrl+q", "ctrl+d":
				return m, tea.Quit
			}
		}
		return m, nil
	}

	if m.showOpenModal {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc":
				m.closeOpenModal()
				return m, nil
			case "ctrl+q", "ctrl+d":
				return m, tea.Quit
			case "enter":
				cmd := m.submitOpenPath()
				return m, cmd
			}
		}
		var inputCmd tea.Cmd
		m.openPathInput, inputCmd = m.openPathInput.Update(msg)
		return m, inputCmd
	}

	if m.showNewFileModal {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc":
				m.closeNewFileModal()
				return m, nil
			case "ctrl+q", "ctrl+d":
				return m, tea.Quit
			case "enter":
				cmd := m.submitNewFile()
				return m, cmd
			case "tab", "shift+tab", "right", "left":
				if keyMsg.String() == "left" || keyMsg.String() == "shift+tab" {
					m.cycleNewFileExtension(-1)
				} else {
					m.cycleNewFileExtension(1)
				}
				return m, nil
			}
		}
		var inputCmd tea.Cmd
		m.newFileInput, inputCmd = m.newFileInput.Update(msg)
		return m, inputCmd
	}

	if m.showHelp {
		if m.helpJustOpened {
			m.helpJustOpened = false
		}
		return m, nil
	}

	if m.showEnvSelector {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc":
				m.showEnvSelector = false
				return m, nil
			case "ctrl+q", "ctrl+d":
				return m, tea.Quit
			case "enter":
				cmd := m.applyEnvironmentSelection()
				return m, cmd
			case "?", "shift+/":
				m.toggleHelp()
				return m, nil
			}
		}
		var envCmd tea.Cmd
		m.envList, envCmd = m.envList.Update(msg)
		return m, envCmd
	}

	if _, ok := msg.(tea.WindowSizeMsg); ok {
		var fileCmd tea.Cmd
		var reqCmd tea.Cmd
		prevReqIndex := m.requestList.Index()
		m.fileList, fileCmd = m.fileList.Update(msg)
		m.requestList, reqCmd = m.requestList.Update(msg)
		m.syncEditorWithRequestSelection(prevReqIndex)
		cmds = append(cmds, fileCmd, reqCmd)
	} else {
		switch m.focus {
		case focusFile:
			var fileCmd tea.Cmd
			m.fileList, fileCmd = m.fileList.Update(msg)
			cmds = append(cmds, fileCmd)
		case focusRequests:
			var reqCmd tea.Cmd
			prevReqIndex := m.requestList.Index()
			m.requestList, reqCmd = m.requestList.Update(msg)
			m.syncEditorWithRequestSelection(prevReqIndex)
			cmds = append(cmds, reqCmd)
		}
	}

	if _, ok := msg.(tea.WindowSizeMsg); ok || m.focus == focusEditor {
		if m.suppressEditorKey {
			m.suppressEditorKey = false
		} else {
			filtered := m.filterEditorMessage(msg)
			var editorCmd tea.Cmd
			m.editor, editorCmd = m.editor.Update(filtered)
			cmds = append(cmds, editorCmd)
		}
	}

	if _, ok := msg.(tea.WindowSizeMsg); ok || (m.focus == focusResponse && m.activeTab == responseTabHistory) {
		var histCmd tea.Cmd
		m.historyList, histCmd = m.historyList.Update(msg)
		if m.historyJumpToLatest {
			m.selectNewestHistoryEntry()
			m.historyJumpToLatest = false
		}
		m.captureHistorySelection()
		cmds = append(cmds, histCmd)
	}

	if _, ok := msg.(tea.WindowSizeMsg); ok || (m.focus == focusResponse && m.activeTab != responseTabHistory) {
		skipViewport := false
		if keyMsg, ok := msg.(tea.KeyMsg); ok && m.focus == focusResponse && m.activeTab != responseTabHistory {
			switch keyMsg.String() {
			case "j", "k":
				skipViewport = true
			}
		}
		if !skipViewport {
			var respCmd tea.Cmd
			m.responseViewport, respCmd = m.responseViewport.Update(msg)
			cmds = append(cmds, respCmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func isSpaceKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeySpace {
		return true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == ' ' {
		return true
	}
	switch msg.String() {
	case " ", "space":
		return true
	default:
		return false
	}
}

func (m *Model) canPreviewOnSpace() bool {
	if len(m.requestItems) == 0 {
		return false
	}
	if m.showHelp || m.showEnvSelector {
		return false
	}
	switch m.focus {
	case focusRequests:
		return true
	case focusEditor:
		return !m.editorInsertMode
	case focusFile:
		return true
	default:
		return false
	}
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	keyStr := msg.String()

	if m.showHelp && !m.helpJustOpened {
		switch keyStr {
		case "ctrl+q", "ctrl+d":
			return tea.Quit
		case "esc", "?", "shift+/":
			m.showHelp = false
			m.helpJustOpened = false
		}
		return nil
	}

	if isSpaceKey(msg) && m.canPreviewOnSpace() {
		if cmd := m.sendRequestFromList(false); cmd != nil {
			return cmd
		}
	}

	switch keyStr {
	case "tab":
		prev := m.focus
		m.cycleFocus(true)
		if prev == focusEditor || m.focus == focusEditor {
			m.suppressEditorKey = true
		}
		return nil
	case "shift+tab":
		prev := m.focus
		m.cycleFocus(false)
		if prev == focusEditor || m.focus == focusEditor {
			m.suppressEditorKey = true
		}
		return nil
	case "ctrl+e":
		if len(m.cfg.EnvironmentSet) == 0 {
			return func() tea.Msg {
				return statusMsg{text: "No environments configured", level: statusWarn}
			}
		}
		m.openEnvironmentSelector()
		return nil
	case "ctrl+s":
		return m.saveFile()
	case "?", "shift+/":
		m.toggleHelp()
		return nil
	case "ctrl+o":
		m.openOpenModal()
		return nil
	case "ctrl+shift+o", "shift+ctrl+o":
		return m.reloadWorkspace()
	case "ctrl+n":
		m.openNewFileModal()
		return nil
	case "ctrl+r":
		return m.reparseDocument()
	case "ctrl+up", "ctrl+shift+up", "shift+ctrl+up", "alt+up":
		if changed, cmd := m.adjustSidebarSplit(sidebarSplitStep); changed {
			return cmd
		}
	case "ctrl+down", "ctrl+shift+down", "shift+ctrl+down", "alt+down":
		if changed, cmd := m.adjustSidebarSplit(-sidebarSplitStep); changed {
			return cmd
		}
	case "ctrl+q", "ctrl+d":
		return tea.Quit
	case "h":
		if m.allowPaneFocusShortcut() {
			prev := m.focus
			m.cycleFocus(false)
			if prev == focusEditor || m.focus == focusEditor {
				m.suppressEditorKey = true
			}
			return nil
		}
	case "l":
		if m.allowPaneFocusShortcut() {
			prev := m.focus
			m.cycleFocus(true)
			if prev == focusEditor || m.focus == focusEditor {
				m.suppressEditorKey = true
			}
			return nil
		}
	case "j":
		if m.focus == focusFile && m.fileList.FilterState() != list.Filtering {
			items := m.fileList.Items()
			idx := m.fileList.Index()
			if len(items) == 0 || idx == -1 || idx >= len(items)-1 {
				m.setFocus(focusRequests)
				return nil
			}
		}
	case "k":
		if m.focus == focusRequests && m.requestList.FilterState() != list.Filtering {
			items := m.requestList.Items()
			idx := m.requestList.Index()
			if len(items) == 0 || idx <= 0 {
				m.setFocus(focusFile)
				return nil
			}
		}
	}

	if m.focus == focusEditor {
		if !m.editorInsertMode {
			switch keyStr {
			case "i":
				m.setInsertMode(true, true)
				m.suppressEditorKey = true
				return nil
			case "esc":
				m.suppressEditorKey = true
				return nil
			}
		} else {
			switch keyStr {
			case "esc":
				m.setInsertMode(false, true)
				m.suppressEditorKey = true
				return nil
			}
		}
		switch keyStr {
		case "ctrl+enter", "ctrl+j", "cmd+enter", "alt+enter":
			if !m.sending {
				m.suppressEditorKey = true
				return m.sendActiveRequest()
			}
		}
		if m.editorInsertMode {
			km := msg
			switch km.Type {
			case tea.KeyBackspace, tea.KeyDelete, tea.KeyRunes, tea.KeyEnter:
				if km.Type != tea.KeyRunes || len(km.Runes) > 0 {
					m.dirty = true
				}
			}
		}
	}

	if m.focus == focusFile {
		switch keyStr {
		case "enter":
			return m.openSelectedFile()
		}
	}

	if m.focus == focusRequests {
		switch {
		case keyStr == "enter":
			return m.sendRequestFromList(true)
		case isSpaceKey(msg):
			return m.sendRequestFromList(false)
		}
	}

	if m.focus == focusResponse {
		switch keyStr {
		case "left", "ctrl+h":
			return m.activatePrevTab()
		case "right", "ctrl+l":
			return m.activateNextTab()
		case "j":
			if m.activeTab != responseTabHistory {
				return m.activateNextTab()
			}
		case "k":
			if m.activeTab != responseTabHistory {
				return m.activatePrevTab()
			}
		case "enter":
			if m.activeTab == responseTabHistory {
				return m.replayHistorySelection()
			}
		}
	}

	return nil
}
