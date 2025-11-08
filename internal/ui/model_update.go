package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textarea.Blink}
	if m.updateEnabled {
		cmds = append(cmds, newUpdateTickCmd(0))
	}
	if cmd := m.nextStreamMsgCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
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
	case profileNextIterationMsg:
		if cmd := m.executeProfileIteration(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case updateTickMsg:
		if cmd := m.enqueueUpdateCheck(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case updateCheckMsg:
		m.updateBusy = false
		m.updateLastCheck = time.Now()
		if m.updateEnabled {
			cmds = append(cmds, newUpdateTickCmd(updateInterval))
		}
		if typed.err != nil {
			errText := typed.err.Error()
			if errText != "" && errText != m.updateLastErr {
				m.updateLastErr = errText
				m.setStatusMessage(statusMsg{text: fmt.Sprintf("update check failed: %s", errText), level: statusWarn})
			}
		} else {
			m.updateLastErr = ""
			if typed.res != nil {
				ver := strings.TrimSpace(typed.res.Info.Version)
				if ver != "" && ver != m.updateAnnounce {
					res := *typed.res
					m.updateInfo = &res
					m.updateAnnounce = ver
					m.setStatusMessage(statusMsg{text: fmt.Sprintf("Update available: %s (run `resterm --update`)", ver), level: statusInfo})
				}
			}
		}
	case streamEventMsg:
		m.handleStreamEvents(typed)
		cmds = append(cmds, m.nextStreamMsgCmd())
	case streamStateMsg:
		m.handleStreamState(typed)
		cmds = append(cmds, m.nextStreamMsgCmd())
	case streamCompleteMsg:
		m.handleStreamComplete(typed)
		cmds = append(cmds, m.nextStreamMsgCmd())
	case streamReadyMsg:
		m.handleStreamReady(typed)
		cmds = append(cmds, m.nextStreamMsgCmd())
	case wsConsoleResultMsg:
		m.handleConsoleResult(typed)
		cmds = append(cmds, m.nextStreamMsgCmd())
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

	if m.showHistoryPreview {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			vp := m.historyPreviewViewport
			switch keyMsg.String() {
			case "esc", "enter":
				m.closeHistoryPreview()
				return m, nil
			case "ctrl+q", "ctrl+d":
				return m, tea.Quit
			case "down", "j":
				if vp != nil {
					vp.ScrollDown(1)
				}
				return m, nil
			case "up", "k":
				if vp != nil {
					vp.ScrollUp(1)
				}
				return m, nil
			case "pgdown", "ctrl+f":
				if vp != nil {
					vp.ScrollDown(vp.Height)
				}
				return m, nil
			case "pgup", "ctrl+b", "ctrl+u":
				if vp != nil {
					vp.ScrollUp(vp.Height)
				}
				return m, nil
			case "home":
				if vp != nil {
					vp.GotoTop()
				}
				return m, nil
			case "end":
				if vp != nil {
					vp.GotoBottom()
				}
				return m, nil
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

	if m.showThemeSelector {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "esc":
				m.showThemeSelector = false
				return m, nil
			case "ctrl+q", "ctrl+d":
				return m, tea.Quit
			case "enter":
				cmd := m.applyThemeSelection()
				return m, cmd
			case "?", "shift+/":
				m.toggleHelp()
				return m, nil
			}
		}
		var themeCmd tea.Cmd
		m.themeList, themeCmd = m.themeList.Update(msg)
		return m, themeCmd
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
		var scenCmd tea.Cmd
		prevReqIndex := m.requestList.Index()
		m.fileList, fileCmd = m.fileList.Update(msg)
		m.requestList, reqCmd = m.requestList.Update(msg)
		m.workflowList, scenCmd = m.workflowList.Update(msg)
		m.syncEditorWithRequestSelection(prevReqIndex)
		cmds = append(cmds, fileCmd, reqCmd, scenCmd)
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
		case focusWorkflows:
			if m.suppressListKey {
				m.suppressListKey = false
			} else {
				var scenCmd tea.Cmd
				prevIdx := m.workflowList.Index()
				m.workflowList, scenCmd = m.workflowList.Update(msg)
				if m.workflowList.Index() != prevIdx {
					m.updateWorkflowHistoryFilter()
				}
				cmds = append(cmds, scenCmd)
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

func isSendShortcut(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+enter", "cmd+enter", "alt+enter", "ctrl+j", "ctrl+m":
		return true
	}

	switch msg.Type {
	case tea.KeyCtrlJ:
		return true
	}

	return false
}

func shouldSendEditorRequest(msg tea.KeyMsg, insertMode bool) bool {
	if isSendShortcut(msg) {
		return true
	}

	keyStr := msg.String()
	switch keyStr {
	case "enter":
		return !insertMode
	}

	switch msg.Type {
	case tea.KeyEnter:
		return !insertMode
	}

	return false
}

func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	if m.showErrorModal || m.showOpenModal || m.showNewFileModal || m.showEnvSelector || m.showHistoryPreview {
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

	if m.focus != focusFile && m.focus != focusRequests && m.focus != focusWorkflows {
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
		vp := m.helpViewport
		switch keyStr {
		case "ctrl+q", "ctrl+d":
			return combine(tea.Quit)
		case "esc", "?", "shift+/":
			m.showHelp = false
			m.helpJustOpened = false
		case "down", "j":
			if vp != nil {
				vp.ScrollDown(1)
			}
		case "up", "k":
			if vp != nil {
				vp.ScrollUp(1)
			}
		case "pgdown", "ctrl+f":
			if vp != nil {
				vp.ScrollDown(maxInt(1, vp.Height))
			}
		case "pgup", "ctrl+b", "ctrl+u":
			if vp != nil {
				vp.ScrollUp(maxInt(1, vp.Height))
			}
		case "home":
			if vp != nil {
				vp.GotoTop()
			}
		case "end":
			if vp != nil {
				vp.GotoBottom()
			}
		}
		return combine(nil)
	}

	if cmd, handled := m.handleStreamKey(msg); handled {
		return combine(cmd)
	}
	if cmd, handled := m.handleWebSocketConsoleKey(msg); handled {
		return combine(cmd)
	}

	if isSpaceKey(msg) && m.canPreviewOnSpace() {
		if cmd := m.sendRequestFromList(false); cmd != nil {
			return combine(cmd)
		}
	}

	switch keyStr {
	case "tab":
		if m.focus == focusEditor && m.editorInsertMode {
			break
		}
		prev := m.focus
		m.cycleFocus(true)
		if prev == focusEditor || m.focus == focusEditor {
			m.suppressEditorKey = true
		}
		return combine(nil)
	case "shift+tab":
		if m.focus == focusEditor && m.editorInsertMode {
			break
		}
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
	case "ctrl+alt+t", "alt+ctrl+t":
		m.openThemeSelector()
		return combine(nil)
	case "ctrl+t":
		return combine(m.openTemporaryDocument())
	case "ctrl+p":
		m.suppressEditorKey = true
		return combine(m.reparseDocument())
	case "ctrl+alt+p", "alt+ctrl+p":
		m.suppressEditorKey = true
		return combine(m.reparseDocument())
	case "ctrl+alt+l", "alt+ctrl+l":
		return combine(m.selectTimelineTab())
	case "ctrl+shift+t", "shift+ctrl+t":
		return combine(m.reparseDocument())
	case "ctrl+q", "ctrl+d":
		return combine(tea.Quit)
	case "j":
	case "k":
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
				if m.editor.SearchActive() {
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
				exitCmd := m.editor.ExitSearchMode()
				m.editor.ClearSelection()
				m.suppressEditorKey = true
				return combine(exitCmd)
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

	if m.focus == focusWorkflows {
		switch {
		case keyStr == "enter":
			return combine(m.runSelectedWorkflow())
		case isSpaceKey(msg):
			return combine(m.runSelectedWorkflow())
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
			case "ctrl+f", "ctrl+b":
				return combine(nil)
			default:
				m.responsePaneChord = false
			}
		}
		if keyStr == "ctrl+f" || keyStr == "ctrl+b" {
			if m.responseSplit {
				m.responsePaneChord = true
				return combine(nil)
			}
			m.setStatusMessage(statusMsg{text: "Enable split to switch panes", level: statusInfo})
			return combine(nil)
		}
		pane := m.focusedPane()
		if pane != nil && pane.activeTab == responseTabCompare {
			if cmd := m.handleCompareTabKey(msg, pane); cmd != nil {
				return combine(cmd)
			}
		}
		switch keyStr {
		case "shift+f", "F":
			cmd := m.openSearchPrompt()
			return combine(cmd)
		case "n":
			cmd := m.advanceResponseSearch()
			return combine(cmd)
		case "p":
			if pane != nil && pane.activeTab == responseTabHistory {
				if entry, ok := m.selectedHistoryEntry(); ok {
					m.openHistoryPreview(entry)
				} else {
					m.setStatusMessage(statusMsg{text: "No history entry selected", level: statusWarn})
				}
				return combine(nil)
			}
			cmd := m.retreatResponseSearch()
			return combine(cmd)
		case "down", "j":
			if pane == nil {
				return combine(nil)
			}
			if pane.activeTab == responseTabStats {
				snapshot := pane.snapshot
				if snapshot != nil && snapshot.statsKind == statsReportKindWorkflow && snapshot.workflowStats != nil {
					return combine(m.moveWorkflowStatsSelection(1))
				}
			}
			if pane.activeTab == responseTabHistory {
				return combine(nil)
			}
			pane.viewport.ScrollDown(1)
			pane.setCurrPosition()
			return combine(nil)
		case "up", "k":
			if pane == nil {
				return combine(nil)
			}
			if pane.activeTab == responseTabStats {
				snapshot := pane.snapshot
				if snapshot != nil && snapshot.statsKind == statsReportKindWorkflow && snapshot.workflowStats != nil {
					return combine(m.moveWorkflowStatsSelection(-1))
				}
			}
			if pane.activeTab == responseTabHistory {
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
			if pane != nil {
				switch pane.activeTab {
				case responseTabHistory:
					return combine(m.loadHistorySelection(false))
				case responseTabStats:
					snapshot := pane.snapshot
					if snapshot != nil && snapshot.statsKind == statsReportKindWorkflow && snapshot.workflowStats != nil {
						return combine(m.toggleWorkflowStatsExpansion())
					}
				}
			}
		}
		if pane != nil && pane.activeTab == responseTabHistory {
			switch keyStr := msg.String(); keyStr {
			case "d":
				if entry, ok := m.selectedHistoryEntry(); ok {
					if deleted, err := m.deleteHistoryEntry(entry.ID); err != nil {
						m.setStatusMessage(statusMsg{text: fmt.Sprintf("history delete error: %v", err), level: statusError})
					} else if deleted {
						m.syncHistory()
						m.setStatusMessage(statusMsg{text: "History entry deleted", level: statusInfo})
					} else {
						m.setStatusMessage(statusMsg{text: "History entry not found", level: statusWarn})
					}
				} else {
					m.setStatusMessage(statusMsg{text: "No history entry selected", level: statusWarn})
				}
				return combine(nil)
			case "r", "R", "ctrl+r", "ctrl+R":
				return combine(m.replayHistorySelection())
			case "enter":
				// handled above
			default:
				if shouldSendEditorRequest(msg, false) {
					return combine(m.replayHistorySelection())
				}
			}
		}
	}

	if isSendShortcut(msg) {
		if m.sending {
			return combine(nil)
		}
		return combine(m.sendActiveRequest())
	}

	if m.focus != focusFile && m.focus != focusRequests && m.focus != focusWorkflows {
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
			if m.focus == focusFile || m.focus == focusRequests || m.focus == focusWorkflows {
				return true, m.runSidebarWidthResize(-sidebarWidthStep)
			}
			return true, m.runEditorResize(-editorSplitStep)
		case "l":
			m.repeatChordPrefix = prefix
			m.repeatChordActive = true
			if m.focus == focusFile || m.focus == focusRequests || m.focus == focusWorkflows {
				return true, m.runSidebarWidthResize(sidebarWidthStep)
			}
			return true, m.runEditorResize(editorSplitStep)
		case "j":
			m.repeatChordPrefix = prefix
			m.repeatChordActive = true
			if m.focus == focusWorkflows && len(m.workflowItems) > 0 {
				return true, m.runWorkflowResize(-workflowSplitStep)
			}
			return true, m.runSidebarResize(-sidebarSplitStep)
		case "k":
			m.repeatChordPrefix = prefix
			m.repeatChordActive = true
			if m.focus == focusWorkflows && len(m.workflowItems) > 0 {
				return true, m.runWorkflowResize(workflowSplitStep)
			}
			return true, m.runSidebarResize(sidebarSplitStep)
		case "J", "shift+j":
			m.repeatChordPrefix = prefix
			m.repeatChordActive = true
			return true, m.runWorkflowResize(workflowSplitStep)
		case "K", "shift+k":
			m.repeatChordPrefix = prefix
			m.repeatChordActive = true
			return true, m.runWorkflowResize(-workflowSplitStep)
		case "r":
			m.setFocus(focusRequests)
			return true, nil
		case "p":
			m.setFocus(focusResponse)
			return true, nil
		case "m":
			m.openThemeSelector()
			return true, nil
		case "i":
			m.setFocus(focusEditor)
			m.setInsertMode(false, true)
			return true, nil
		case "s":
			return true, m.setMainSplitOrientation(mainSplitHorizontal)
		case "v":
			return true, m.setMainSplitOrientation(mainSplitVertical)
		case "t":
			cmd := m.selectTimelineTab()
			return true, cmd
		case "T", "shift+t":
			m.openThemeSelector()
			return true, nil
		case "c":
			return true, m.startConfigCompareFromEditor()
		case "w":
			return true, m.toggleWebSocketConsole()
		case "1":
			return true, m.togglePaneCollapse(paneRegionSidebar)
		case "2":
			return true, m.togglePaneCollapse(paneRegionEditor)
		case "3":
			return true, m.togglePaneCollapse(paneRegionResponse)
		case "z":
			return true, m.toggleZoomForRegion(regionFromFocus(m.focus))
		case "Z", "shift+z":
			return true, m.clearZoomCmd()
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
	if m.zoomActive {
		m.setStatusMessage(statusMsg{text: "Disable zoom to resize panes", level: statusInfo})
		return nil
	}
	if m.collapseState(paneRegionEditor) || m.collapseState(paneRegionResponse) {
		m.setStatusMessage(statusMsg{text: "Expand panes before resizing", level: statusInfo})
		return nil
	}
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

func (m *Model) runSidebarWidthResize(delta float64) tea.Cmd {
	changed, bounded, cmd := m.adjustSidebarWidth(delta)
	if changed {
		return cmd
	}
	if bounded {
		if delta < 0 {
			m.setStatusMessage(statusMsg{text: "Sidebar already at minimum width", level: statusInfo})
		} else if delta > 0 {
			m.setStatusMessage(statusMsg{text: "Sidebar already at maximum width", level: statusInfo})
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

func (m *Model) runWorkflowResize(delta float64) tea.Cmd {
	changed, bounded, cmd := m.adjustWorkflowSplit(delta)
	if changed {
		return cmd
	}
	if bounded {
		if delta > 0 {
			m.setStatusMessage(statusMsg{text: "Workflows already at minimum height", level: statusInfo})
		} else if delta < 0 {
			m.setStatusMessage(statusMsg{text: "Workflows already at maximum height", level: statusInfo})
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
