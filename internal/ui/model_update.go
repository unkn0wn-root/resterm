package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
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
	case editorEvent:
		if typed.dirty {
			m.dirty = true
		}
		if typed.status != nil {
			m.setStatusMessage(*typed.status)
		}
	case tea.KeyMsg:
		if !m.showSearchPrompt && !m.showEnvSelector {
			if cmd := m.handleKey(typed); cmd != nil {
				cmds = append(cmds, cmd)
			}
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

	if m.showSearchPrompt {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if m.searchJustOpened {
				m.searchJustOpened = false
				switch keyMsg.String() {
				case "shift+f", "F":
					return m, nil
				}
			}
			switch keyMsg.String() {
			case "esc":
				m.closeSearchPrompt()
				return m, nil
			case "ctrl+q", "ctrl+d":
				return m, tea.Quit
			case "ctrl+r":
				m.toggleSearchMode()
				return m, nil
			case "enter":
				cmd := m.submitSearchPrompt()
				return m, cmd
			}
		}
		var inputCmd tea.Cmd
		m.searchInput, inputCmd = m.searchInput.Update(msg)
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
			if m.suppressListKey {
				m.suppressListKey = false
			} else {
				var fileCmd tea.Cmd
				m.fileList, fileCmd = m.fileList.Update(msg)
				cmds = append(cmds, fileCmd)
			}
		case focusRequests:
			if m.suppressListKey {
				m.suppressListKey = false
			} else {
				var reqCmd tea.Cmd
				prevReqIndex := m.requestList.Index()
				m.requestList, reqCmd = m.requestList.Update(msg)
				m.syncEditorWithRequestSelection(prevReqIndex)
				cmds = append(cmds, reqCmd)
			}
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

	if _, ok := msg.(tea.WindowSizeMsg); ok || (m.focus == focusResponse && m.focusedPane() != nil && m.focusedPane().activeTab == responseTabHistory) {
		var histCmd tea.Cmd
		m.historyList, histCmd = m.historyList.Update(msg)
		if m.historyJumpToLatest {
			m.selectNewestHistoryEntry()
			m.historyJumpToLatest = false
		}
		m.captureHistorySelection()
		cmds = append(cmds, histCmd)
	}

	if winMsg, ok := msg.(tea.WindowSizeMsg); ok {
		for _, id := range m.visiblePaneIDs() {
			pane := m.pane(id)
			if pane == nil || pane.activeTab == responseTabHistory {
				continue
			}
			var paneCmd tea.Cmd
			pane.viewport, paneCmd = pane.viewport.Update(winMsg)
			if paneCmd != nil {
				cmds = append(cmds, paneCmd)
			}
		}
	} else if m.focus == focusResponse {
		pane := m.focusedPane()
		if pane != nil && pane.activeTab != responseTabHistory {
			skipViewport := false
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				switch keyMsg.String() {
				case "j", "k":
					skipViewport = true
				}
			}
			if !skipViewport {
				var paneCmd tea.Cmd
				pane.viewport, paneCmd = pane.viewport.Update(msg)
				if paneCmd != nil {
					cmds = append(cmds, paneCmd)
				}
			}
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

func shouldSendEditorRequest(msg tea.KeyMsg, insertMode bool) bool {
	keyStr := msg.String()
	switch keyStr {
	case "ctrl+enter", "cmd+enter", "alt+enter", "ctrl+j", "ctrl+m":
		return true
	case "enter":
		return !insertMode
	}
	switch msg.Type {
	case tea.KeyCtrlJ:
		return true
	case tea.KeyEnter:
		return !insertMode
	}
	return false
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	if m.showErrorModal || m.showOpenModal || m.showNewFileModal || m.showEnvSelector {
		return nil
	}
	return m.handleKeyWithChord(msg, true)
}

func (m *Model) handleKeyWithChord(msg tea.KeyMsg, allowChord bool) tea.Cmd {
	keyStr := msg.String()
	var prefixCmd tea.Cmd
	combine := func(c tea.Cmd) tea.Cmd {
		if prefixCmd == nil {
			return c
		}
		if c == nil {
			return prefixCmd
		}
		return tea.Batch(prefixCmd, c)
	}

	if m.operator.active {
		m.suppressEditorKey = true
		cmd := m.handleOperatorKey(msg)
		return combine(cmd)
	}

	if m.focus == focusEditor && m.editor.awaitingFindTarget() {
		if updated, cmd, ok := m.editor.HandleMotion(keyStr); ok {
			m.editor = updated
			m.suppressEditorKey = true
			return combine(cmd)
		}
	}

	if m.focus != focusFile && m.focus != focusRequests {
		m.suppressListKey = false
	}

	if allowChord {
		if !m.hasPendingChord && m.repeatChordActive {
			if handled, chordCmd := m.resolveChord(m.repeatChordPrefix, keyStr); handled {
				m.suppressListKey = true
				return combine(chordCmd)
			}
			m.repeatChordActive = false
			m.repeatChordPrefix = ""
		}
		if m.hasPendingChord {
			storedMsg := m.pendingChordMsg
			prefix := m.pendingChord
			m.pendingChord = ""
			m.hasPendingChord = false
			m.pendingChordMsg = tea.KeyMsg{}
			if handled, chordCmd := m.resolveChord(prefix, keyStr); handled {
				m.suppressListKey = true
				return combine(chordCmd)
			}
			prefixCmd = m.handleKeyWithChord(storedMsg, false)
			m.suppressListKey = true
			keyStr = msg.String()
		} else if m.canStartChord(msg, keyStr) {
			m.repeatChordActive = false
			m.repeatChordPrefix = ""
			m.pendingChord = keyStr
			m.pendingChordMsg = msg
			m.hasPendingChord = true
			m.suppressListKey = true
			return combine(nil)
		}
	}

	if m.showHelp && !m.helpJustOpened {
		switch keyStr {
		case "ctrl+q", "ctrl+d":
			return combine(tea.Quit)
		case "esc", "?", "shift+/":
			m.showHelp = false
			m.helpJustOpened = false
		}
		return combine(nil)
	}

	if isSpaceKey(msg) && m.canPreviewOnSpace() {
		if cmd := m.sendRequestFromList(false); cmd != nil {
			return combine(cmd)
		}
	}

	switch keyStr {
	case "tab":
		prev := m.focus
		m.cycleFocus(true)
		if prev == focusEditor || m.focus == focusEditor {
			m.suppressEditorKey = true
		}
		return combine(nil)
	case "shift+tab":
		prev := m.focus
		m.cycleFocus(false)
		if prev == focusEditor || m.focus == focusEditor {
			m.suppressEditorKey = true
		}
		return combine(nil)
	case "ctrl+e":
		if len(m.cfg.EnvironmentSet) == 0 {
			return combine(func() tea.Msg {
				return statusMsg{text: "No environments configured", level: statusWarn}
			})
		}
		m.openEnvironmentSelector()
		return combine(nil)
	case "ctrl+g":
		return combine(m.showGlobalSummary())
	case "ctrl+shift+g", "shift+ctrl+g":
		return combine(m.clearGlobalValues())
	case "ctrl+s":
		return combine(m.saveFile())
	case "ctrl+v":
		m.responsePaneChord = false
		return combine(m.toggleResponseSplitVertical())
	case "ctrl+u":
		m.responsePaneChord = false
		return combine(m.toggleResponseSplitHorizontal())
	case "ctrl+shift+v", "shift+ctrl+v":
		target := responsePanePrimary
		if m.focus == focusResponse {
			target = m.responsePaneFocus
		}
		return combine(m.togglePaneFollowLatest(target))
	case "?", "shift+/":
		m.toggleHelp()
		return combine(nil)
	case "ctrl+o":
		m.openOpenModal()
		return combine(nil)
	case "ctrl+shift+o", "shift+ctrl+o":
		return combine(m.reloadWorkspace())
	case "ctrl+n":
		m.openNewFileModal()
		return combine(nil)
	case "ctrl+shift+n", "shift+ctrl+n":
		return combine(m.openTemporaryDocument())
	case "ctrl+t":
		return combine(m.reparseDocument())
	case "ctrl+q", "ctrl+d":
		return combine(tea.Quit)
	case "j":
		if m.focus == focusFile && m.fileList.FilterState() != list.Filtering {
			items := m.fileList.Items()
			idx := m.fileList.Index()
			if len(items) == 0 || idx == -1 || idx >= len(items)-1 {
				m.setFocus(focusRequests)
				return combine(nil)
			}
		}
	case "k":
		if m.focus == focusRequests && m.requestList.FilterState() != list.Filtering {
			items := m.requestList.Items()
			idx := m.requestList.Index()
			if len(items) == 0 || idx <= 0 {
				m.setFocus(focusFile)
				return combine(nil)
			}
		}
	}

	if m.focus == focusEditor {
		if !m.editorInsertMode {
			switch keyStr {
			case "shift+f", "F":
				cmd := m.openSearchPrompt()
				m.suppressEditorKey = true
				return combine(cmd)
			case "n":
				var cmd tea.Cmd
				m.editor, cmd = m.editor.NextSearchMatch()
				m.suppressEditorKey = true
				return combine(cmd)
			case "p":
				if strings.TrimSpace(m.editor.search.query) != "" {
					var cmd tea.Cmd
					m.editor, cmd = m.editor.PrevSearchMatch()
					m.suppressEditorKey = true
					return combine(cmd)
				}
				cmd := m.runPasteClipboard(true)
				m.suppressEditorKey = true
				return combine(cmd)
			case "u":
				var cmd tea.Cmd
				m.editor, cmd = m.editor.UndoLastChange()
				m.suppressEditorKey = true
				return combine(cmd)
			case "ctrl+r":
				cmd := m.runRedoLastChange()
				m.suppressEditorKey = true
				return combine(cmd)
			case "d":
				if m.editor.hasSelection() {
					var cmd tea.Cmd
					m.editor, cmd = m.editor.DeleteSelection()
					m.suppressEditorKey = true
					return combine(cmd)
				}
				m.repeatChordActive = false
				m.repeatChordPrefix = ""
				m.startOperator("d")
				m.suppressEditorKey = true
				m.suppressListKey = true
				return combine(nil)
			case "D":
				cmd := m.runDeleteToLineEnd()
				m.suppressEditorKey = true
				return combine(cmd)
			case "x":
				cmd := m.runDeleteCharAtCursor()
				m.suppressEditorKey = true
				return combine(cmd)
			case "c":
				cmd := m.runChangeCurrentLine()
				m.suppressEditorKey = true
				m.setInsertMode(true, true)
				return combine(cmd)
			case "P":
				cmd := m.runPasteClipboard(false)
				m.suppressEditorKey = true
				return combine(cmd)
			case "i":
				m.setInsertMode(true, true)
				m.suppressEditorKey = true
				return combine(nil)
			case "esc":
				m.editor.ClearSelection()
				m.suppressEditorKey = true
				return combine(nil)
			case "v":
				var cmd tea.Cmd
				m.editor, cmd = m.editor.ToggleVisual()
				m.suppressEditorKey = true
				return combine(cmd)
			case "V":
				var cmd tea.Cmd
				m.editor, cmd = m.editor.ToggleVisualLine()
				m.suppressEditorKey = true
				return combine(cmd)
			case "y":
				var cmd tea.Cmd
				m.editor, cmd = m.editor.YankSelection()
				m.suppressEditorKey = true
				return combine(cmd)
			case "a":
				editorPtr := &m.editor
				editorPtr.ClearSelection()
				pos := editorPtr.caretPosition()
				lineLen := lineLength(editorPtr.Value(), pos.Line)
				targetCol := pos.Column
				if targetCol < lineLen {
					targetCol++
				} else {
					targetCol = lineLen
				}
				editorPtr.moveCursorTo(pos.Line, targetCol)
				m.setInsertMode(true, true)
				m.suppressEditorKey = true
				return combine(nil)
			}
			if updated, cmd, ok := m.editor.HandleMotion(keyStr); ok {
				m.editor = updated
				m.suppressEditorKey = true
				return combine(cmd)
			}
		} else {
			switch keyStr {
			case "esc":
				m.setInsertMode(false, true)
				m.suppressEditorKey = true
				return combine(nil)
			}
		}
		if shouldSendEditorRequest(msg, m.editorInsertMode) {
			if !m.sending {
				m.suppressEditorKey = true
				return combine(m.sendActiveRequest())
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
			return combine(m.openSelectedFile())
		}
	}

	if m.focus == focusRequests {
		switch {
		case keyStr == "enter":
			return combine(m.sendRequestFromList(true))
		case isSpaceKey(msg):
			return combine(m.sendRequestFromList(false))
		}
	}

	if m.focus == focusResponse {
		if m.responsePaneChord {
			switch keyStr {
			case "left", "h":
				m.responsePaneChord = false
				if m.responseSplit {
					m.focusResponsePane(responsePanePrimary)
				}
				return combine(nil)
			case "right", "l":
				m.responsePaneChord = false
				if m.responseSplit {
					m.focusResponsePane(responsePaneSecondary)
				}
				return combine(nil)
			case "ctrl+f":
				return combine(nil)
			default:
				m.responsePaneChord = false
			}
		}
		if keyStr == "ctrl+f" {
			if m.responseSplit {
				m.responsePaneChord = true
				return combine(nil)
			}
			m.setStatusMessage(statusMsg{text: "Enable split to switch panes", level: statusInfo})
			return combine(nil)
		}
		pane := m.focusedPane()
		switch keyStr {
		case "shift+f", "F":
			cmd := m.openSearchPrompt()
			return combine(cmd)
		case "n":
			cmd := m.advanceResponseSearch()
			return combine(cmd)
		case "p":
			cmd := m.retreatResponseSearch()
			return combine(cmd)
		case "down", "j":
			if pane == nil || pane.activeTab == responseTabHistory {
				return combine(nil)
			}
			pane.viewport.ScrollDown(1)
			pane.setCurrPosition()
			return combine(nil)
		case "up", "k":
			if pane == nil || pane.activeTab == responseTabHistory {
				return combine(nil)
			}
			pane.viewport.ScrollUp(1)
			pane.setCurrPosition()
			return combine(nil)
		case "pgdown":
			if pane == nil || pane.activeTab == responseTabHistory {
				return combine(nil)
			}
			pane.viewport.PageDown()
			pane.setCurrPosition()
			return combine(nil)
		case "pgup":
			if pane == nil || pane.activeTab == responseTabHistory {
				return combine(nil)
			}
			pane.viewport.PageUp()
			pane.setCurrPosition()
			return combine(nil)
		case "left", "ctrl+h", "h":
			return combine(m.activatePrevTabFor(m.responsePaneFocus))
		case "right", "ctrl+l", "l":
			return combine(m.activateNextTabFor(m.responsePaneFocus))
		case "enter":
			if pane != nil && pane.activeTab == responseTabHistory {
				return combine(m.replayHistorySelection())
			}
		}
	}

	if m.focus != focusFile && m.focus != focusRequests {
		m.suppressListKey = false
	}

	return combine(nil)
}

func (m *Model) canStartChord(msg tea.KeyMsg, keyStr string) bool {
	if msg.Type != tea.KeyRunes {
		return false
	}
	if m.editor.awaitingFindTarget() {
		return false
	}
	switch keyStr {
	case "g":
		if m.focus == focusEditor && m.editorInsertMode {
			return false
		}
		return true
	default:
		return false
	}
}

func (m *Model) resolveChord(prefix string, next string) (bool, tea.Cmd) {
	switch prefix {
	case "g":
		switch next {
		case "h":
			m.repeatChordPrefix = prefix
			m.repeatChordActive = true
			return true, m.runEditorResize(-editorSplitStep)
		case "l":
			m.repeatChordPrefix = prefix
			m.repeatChordActive = true
			return true, m.runEditorResize(editorSplitStep)
		case "j":
			m.repeatChordPrefix = prefix
			m.repeatChordActive = true
			return true, m.runSidebarResize(-sidebarSplitStep)
		case "k":
			m.repeatChordPrefix = prefix
			m.repeatChordActive = true
			return true, m.runSidebarResize(sidebarSplitStep)
		}
	}
	return false, nil
}

func (m *Model) startOperator(op string) {
	m.operator.active = true
	m.operator.operator = op
	m.operator.anchor = m.editor.caretPosition()
	if m.operator.motionKeys != nil {
		m.operator.motionKeys = m.operator.motionKeys[:0]
	}
}

func (m *Model) clearOperatorState() {
	m.operator.active = false
	m.operator.operator = ""
	m.operator.anchor = cursorPosition{}
	m.operator.motionKeys = nil
}

func (m *Model) handleOperatorKey(msg tea.KeyMsg) tea.Cmd {
	keyStr := msg.String()
	m.suppressListKey = true
	switch keyStr {
	case "esc", "ctrl+c", "ctrl+g":
		m.clearOperatorState()
		return nil
	}

	if m.operator.operator == "d" && keyStr == "d" {
		m.clearOperatorState()
		return m.runDeleteCurrentLine()
	}

	updated, motionCmd, handled := m.editor.HandleMotion(keyStr)
	if !handled {
		m.clearOperatorState()
		status := statusMsg{text: "Delete requires a motion", level: statusWarn}
		return toEditorEventCmd(editorEvent{status: &status})
	}

	m.operator.motionKeys = append(m.operator.motionKeys, keyStr)
	m.editor = updated

	if m.editor.pendingMotion != "" || m.editor.awaitingFindTarget() {
		return motionCmd
	}

	spec, err := classifyDeleteMotion(m.operator.motionKeys)
	if err != nil {
		anchor := m.operator.anchor
		editorPtr := &m.editor
		editorPtr.moveCursorTo(anchor.Line, anchor.Column)
		editorPtr.applySelectionHighlight()
		m.clearOperatorState()
		status := statusMsg{text: err.Error(), level: statusWarn}
		return batchCommands(motionCmd, toEditorEventCmd(editorEvent{status: &status}))
	}

	deleteCmd := m.applyEditorMutation(func(ed requestEditor) (requestEditor, tea.Cmd) {
		return ed.DeleteMotion(m.operator.anchor, spec)
	})
	m.clearOperatorState()
	return batchCommands(motionCmd, deleteCmd)
}

func batchCommands(cmds ...tea.Cmd) tea.Cmd {
	var nonNil []tea.Cmd
	for _, cmd := range cmds {
		if cmd != nil {
			nonNil = append(nonNil, cmd)
		}
	}
	switch len(nonNil) {
	case 0:
		return nil
	case 1:
		return nonNil[0]
	default:
		return tea.Batch(nonNil...)
	}
}

func (m *Model) runEditorResize(delta float64) tea.Cmd {
	changed, bounded, cmd := m.adjustEditorSplit(delta)
	if changed {
		return cmd
	}
	if bounded {
		if delta < 0 {
			m.setStatusMessage(statusMsg{text: "Editor already at minimum width", level: statusInfo})
		} else if delta > 0 {
			m.setStatusMessage(statusMsg{text: "Editor already at maximum width", level: statusInfo})
		}
	}
	return nil
}

func (m *Model) runSidebarResize(delta float64) tea.Cmd {
	changed, bounded, cmd := m.adjustSidebarSplit(delta)
	if changed {
		return cmd
	}
	if bounded {
		if delta > 0 {
			m.setStatusMessage(statusMsg{text: "Sidebar already at maximum height", level: statusInfo})
		} else if delta < 0 {
			m.setStatusMessage(statusMsg{text: "Sidebar already at minimum height", level: statusInfo})
		}
	}
	return nil
}

func (m *Model) applyEditorMutation(op func(requestEditor) (requestEditor, tea.Cmd)) tea.Cmd {
	before := m.editor.Value()
	editor, cmd := op(m.editor)
	if editor.Value() != before {
		m.dirty = true
	}
	m.editor = editor
	return cmd
}

func (m *Model) runDeleteCurrentLine() tea.Cmd {
	return m.applyEditorMutation(func(ed requestEditor) (requestEditor, tea.Cmd) {
		return ed.DeleteCurrentLine()
	})
}

func (m *Model) runDeleteToLineEnd() tea.Cmd {
	return m.applyEditorMutation(func(ed requestEditor) (requestEditor, tea.Cmd) {
		return ed.DeleteToLineEnd()
	})
}

func (m *Model) runDeleteCharAtCursor() tea.Cmd {
	return m.applyEditorMutation(func(ed requestEditor) (requestEditor, tea.Cmd) {
		return ed.DeleteCharAtCursor()
	})
}

func (m *Model) runChangeCurrentLine() tea.Cmd {
	return m.applyEditorMutation(func(ed requestEditor) (requestEditor, tea.Cmd) {
		return ed.ChangeCurrentLine()
	})
}

func (m *Model) runPasteClipboard(after bool) tea.Cmd {
	return m.applyEditorMutation(func(ed requestEditor) (requestEditor, tea.Cmd) {
		return ed.PasteClipboard(after)
	})
}

func (m *Model) runRedoLastChange() tea.Cmd {
	return m.applyEditorMutation(func(ed requestEditor) (requestEditor, tea.Cmd) {
		return ed.RedoLastChange()
	})
}
