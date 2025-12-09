package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
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
		m.sendCancel = nil
		m.statusPulseBase = ""
		m.statusPulseFrame = 0
		if cmd := m.handleResponseMessage(typed); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case statusMsg:
		m.setStatusMessage(typed)
	case statusPulseMsg:
		if cmd := m.handleStatusPulse(typed); cmd != nil {
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
		return m, tea.Batch(cmds...)
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
		m.navigatorFilter, _ = m.navigatorFilter.Update(msg)
	} else {
		switch m.focus {
		case focusFile, focusRequests, focusWorkflows:
			if m.suppressListKey {
				m.suppressListKey = false
			} else {
				if navCmd := m.updateNavigator(msg); navCmd != nil {
					cmds = append(cmds, navCmd)
				}
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

func canonicalShortcutKey(msg tea.KeyMsg) string {
	key := msg.String()
	if key == "" {
		switch msg.Type {
		case tea.KeyCtrlJ:
			key = "ctrl+j"
		default:
			return ""
		}
	}
	return bindings.NormalizeKeyString(key)
}

func isPlainRuneKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1
}

func (m *Model) isNavigatorFiltering() bool {
	if !m.navigatorFilter.Focused() {
		return false
	}
	switch m.focus {
	case focusFile, focusRequests, focusWorkflows:
		return true
	default:
		return false
	}
}

func (m *Model) resetChordState() {
	m.hasPendingChord = false
	m.pendingChord = ""
	m.pendingChordMsg = tea.KeyMsg{}
	m.repeatChordActive = false
	m.repeatChordPrefix = ""
}

func (m *Model) navGate(kind navigator.Kind, warn string) bool {
	if m.navigator == nil {
		return true
	}
	sel := m.navigator.Selected()
	if sel == nil {
		return true
	}
	if sel.Kind != kind {
		return false
	}
	if !samePath(sel.Payload.FilePath, m.currentFile) {
		if warn != "" {
			m.setStatusMessage(statusMsg{text: warn, level: statusInfo})
		}
		return false
	}
	return true
}

func (m *Model) handleShortcutKey(key string, msg tea.KeyMsg) (tea.Cmd, bool) {
	if key == "" || m.bindingsMap == nil {
		return nil, false
	}
	binding, ok := m.bindingsMap.MatchSingle(key)
	if !ok {
		return nil, false
	}
	if binding.Action == bindings.ActionSendRequest {
		return nil, false
	}
	cmd, handled := m.runShortcutBinding(binding, msg)
	if !handled {
		return nil, false
	}
	return cmd, true
}

func (m *Model) runShortcutBinding(binding bindings.Binding, msg tea.KeyMsg) (tea.Cmd, bool) {
	switch binding.Action {
	case bindings.ActionCycleFocusNext:
		if m.focus == focusEditor && m.editorInsertMode {
			return nil, true
		}
		prev := m.focus
		m.cycleFocus(true)
		if prev == focusEditor || m.focus == focusEditor {
			m.suppressEditorKey = true
		}
		return nil, true
	case bindings.ActionCycleFocusPrev:
		if m.focus == focusEditor && m.editorInsertMode {
			return nil, true
		}
		prev := m.focus
		m.cycleFocus(false)
		if prev == focusEditor || m.focus == focusEditor {
			m.suppressEditorKey = true
		}
		return nil, true
	case bindings.ActionOpenEnvSelector:
		if len(m.cfg.EnvironmentSet) == 0 {
			return func() tea.Msg {
				return statusMsg{text: "No environments configured", level: statusWarn}
			}, true
		}
		m.openEnvironmentSelector()
		return nil, true
	case bindings.ActionShowGlobals:
		return m.showGlobalSummary(), true
	case bindings.ActionClearGlobals:
		return m.clearGlobalValues(), true
	case bindings.ActionSaveFile:
		return m.saveFile(), true
	case bindings.ActionToggleResponseSplitVert:
		m.responsePaneChord = false
		return m.toggleResponseSplitVertical(), true
	case bindings.ActionToggleResponseSplitHorz:
		m.responsePaneChord = false
		return m.toggleResponseSplitHorizontal(), true
	case bindings.ActionTogglePaneFollowLatest:
		m.responsePaneChord = false
		target := responsePanePrimary
		if m.focus == focusResponse {
			target = m.responsePaneFocus
		}
		return m.togglePaneFollowLatest(target), true
	case bindings.ActionToggleHelp:
		m.toggleHelp()
		return nil, true
	case bindings.ActionOpenPathModal:
		m.openOpenModal()
		return nil, true
	case bindings.ActionReloadWorkspace:
		return m.reloadWorkspace(), true
	case bindings.ActionOpenNewFileModal:
		m.openNewFileModal()
		return nil, true
	case bindings.ActionOpenThemeSelector:
		m.openThemeSelector()
		return nil, true
	case bindings.ActionOpenTempDocument:
		return m.openTemporaryDocument(), true
	case bindings.ActionReparseDocument:
		m.suppressEditorKey = true
		return m.reparseDocument(), true
	case bindings.ActionSelectTimelineTab:
		return m.selectTimelineTab(), true
	case bindings.ActionQuitApp:
		return tea.Quit, true
	case bindings.ActionCancelRun:
		return m.cancelActiveRuns(), true
	case bindings.ActionSidebarWidthDecrease:
		if m.focus == focusFile || m.focus == focusRequests || m.focus == focusWorkflows {
			return m.runSidebarWidthResize(-sidebarWidthStep), true
		}
		return m.runEditorResize(-editorSplitStep), true
	case bindings.ActionSidebarWidthIncrease:
		if m.focus == focusFile || m.focus == focusRequests || m.focus == focusWorkflows {
			return m.runSidebarWidthResize(sidebarWidthStep), true
		}
		return m.runEditorResize(editorSplitStep), true
	case bindings.ActionSidebarHeightDecrease:
		if m.focus == focusWorkflows && len(m.workflowItems) > 0 {
			return m.runWorkflowResize(-workflowSplitStep), true
		}
		return m.runSidebarResize(-sidebarSplitStep), true
	case bindings.ActionSidebarHeightIncrease:
		if m.focus == focusWorkflows && len(m.workflowItems) > 0 {
			return m.runWorkflowResize(workflowSplitStep), true
		}
		return m.runSidebarResize(sidebarSplitStep), true
	case bindings.ActionWorkflowHeightIncrease:
		return m.runWorkflowResize(workflowSplitStep), true
	case bindings.ActionWorkflowHeightDecrease:
		return m.runWorkflowResize(-workflowSplitStep), true
	case bindings.ActionFocusRequests:
		m.setFocus(focusRequests)
		return nil, true
	case bindings.ActionFocusResponse:
		m.setFocus(focusResponse)
		return nil, true
	case bindings.ActionFocusEditorNormal:
		m.setFocus(focusEditor)
		m.setInsertMode(false, true)
		return nil, true
	case bindings.ActionSetMainSplitHorizontal:
		return m.setMainSplitOrientation(mainSplitHorizontal), true
	case bindings.ActionSetMainSplitVertical:
		return m.setMainSplitOrientation(mainSplitVertical), true
	case bindings.ActionStartCompareRun:
		return m.startConfigCompareFromEditor(), true
	case bindings.ActionToggleWebsocketConsole:
		return m.toggleWebSocketConsole(), true
	case bindings.ActionToggleSidebarCollapse:
		return m.togglePaneCollapse(paneRegionSidebar), true
	case bindings.ActionToggleEditorCollapse:
		return m.togglePaneCollapse(paneRegionEditor), true
	case bindings.ActionToggleResponseCollapse:
		return m.togglePaneCollapse(paneRegionResponse), true
	case bindings.ActionToggleZoom:
		return m.toggleZoomForRegion(regionFromFocus(m.focus)), true
	case bindings.ActionClearZoom:
		return m.clearZoomCmd(), true
	case bindings.ActionCopyResponseTab:
		return m.copyResponseTab(), true
	case bindings.ActionToggleHeaderPreview:
		return m.toggleHeaderPreview(), true
	default:
		return nil, false
	}
}

func (m *Model) isSendShortcut(msg tea.KeyMsg) bool {
	if m.bindingsMap == nil {
		return false
	}
	key := canonicalShortcutKey(msg)
	if key == "" {
		return false
	}
	for _, binding := range m.bindingsMap.Bindings(bindings.ActionSendRequest) {
		if len(binding.Steps) == 1 && binding.Steps[0] == key {
			return true
		}
	}
	return false
}

func (m *Model) shouldSendEditorRequest(msg tea.KeyMsg, insertMode bool) bool {
	if m.isSendShortcut(msg) {
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
	shortcutKey := canonicalShortcutKey(msg)
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

	if m.isNavigatorFiltering() {
		m.resetChordState()
		switch keyStr {
		case "ctrl+q", "ctrl+d":
			return combine(tea.Quit)
		default:
			return combine(nil)
		}
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
		if !m.hasPendingChord && m.repeatChordActive && shortcutKey != "" {
			if handled, chordCmd := m.resolveChord(m.repeatChordPrefix, shortcutKey, msg); handled {
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
			if shortcutKey != "" {
				if handled, chordCmd := m.resolveChord(prefix, shortcutKey, msg); handled {
					m.suppressListKey = true
					return combine(chordCmd)
				}
			}
			prefixCmd = m.handleKeyWithChord(storedMsg, false)
			m.suppressListKey = true
			keyStr = msg.String()
		} else if m.canStartChord(msg, shortcutKey) {
			m.repeatChordActive = false
			m.repeatChordPrefix = ""
			m.pendingChord = shortcutKey
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
		if !m.navGate(navigator.KindRequest, "Open file to preview this request") {
			return combine(nil)
		}
		if cmd := m.sendRequestFromList(false); cmd != nil {
			return combine(cmd)
		}
	}

	if shortcutKey != "" {
		// Let plain characters through in insert mode so they become text, not shortcuts
		if m.focus != focusEditor || !m.editorInsertMode || !isPlainRuneKey(msg) {
			if cmd, handled := m.handleShortcutKey(shortcutKey, msg); handled {
				return combine(cmd)
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
		if m.shouldSendEditorRequest(msg, m.editorInsertMode) {
			m.suppressEditorKey = true
			return combine(m.sendActiveRequest())
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
			if m.navigator != nil {
				return combine(nil)
			}
			return combine(m.openSelectedFile())
		}
	}

	if m.focus == focusRequests {
		switch {
		case keyStr == "enter":
			if !m.navGate(navigator.KindRequest, "Open file to send this request") {
				return combine(nil)
			}
			return combine(m.sendRequestFromList(true))
		case isSpaceKey(msg):
			if !m.navGate(navigator.KindRequest, "Open file to preview this request") {
				return combine(nil)
			}
			return combine(m.sendRequestFromList(false))
		}
	}

	if m.focus == focusWorkflows {
		switch {
		case keyStr == "enter":
			if !m.navGate(navigator.KindWorkflow, "Open file to run this workflow") {
				return combine(nil)
			}
			return combine(m.runSelectedWorkflow())
		case isSpaceKey(msg):
			if !m.navGate(navigator.KindWorkflow, "Open file to run this workflow") {
				return combine(nil)
			}
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
		case "esc":
			return combine(m.clearResponseSearch())
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
		case "down", "j", "shift+j", "J":
			if pane == nil {
				return combine(nil)
			}
			if pane.activeTab == responseTabStats {
				snapshot := pane.snapshot
				if snapshot != nil && snapshot.statsKind == statsReportKindWorkflow && snapshot.workflowStats != nil {
					if keyStr == "shift+j" || keyStr == "J" {
						return combine(m.jumpWorkflowStatsSelection(1))
					}
					if snapshot.workflowStats.scrollExpanded(pane, 1) {
						pane.setCurrPosition()
						return combine(m.selectWorkflowStatsByViewport(pane, snapshot, 1))
					}
					pane.viewport.ScrollDown(1)
					pane.setCurrPosition()
					return combine(m.selectWorkflowStatsByViewport(pane, snapshot, 1))
				}
			}
			if pane.activeTab == responseTabHistory {
				return combine(nil)
			}
			pane.viewport.ScrollDown(1)
			pane.setCurrPosition()
			return combine(nil)
		case "up", "k", "shift+k", "K":
			if pane == nil {
				return combine(nil)
			}
			if pane.activeTab == responseTabStats {
				snapshot := pane.snapshot
				if snapshot != nil && snapshot.statsKind == statsReportKindWorkflow && snapshot.workflowStats != nil {
					if keyStr == "shift+k" || keyStr == "K" {
						return combine(m.jumpWorkflowStatsSelection(-1))
					}
					if snapshot.workflowStats.scrollExpanded(pane, -1) {
						pane.setCurrPosition()
						return combine(m.selectWorkflowStatsByViewport(pane, snapshot, -1))
					}
					pane.viewport.ScrollUp(1)
					pane.setCurrPosition()
					return combine(m.selectWorkflowStatsByViewport(pane, snapshot, -1))
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
				if m.shouldSendEditorRequest(msg, false) {
					if cmd := m.cancelActiveRuns(); cmd != nil {
						return combine(cmd)
					}
					return combine(m.replayHistorySelection())
				}
			}
		}
	}

	if m.isSendShortcut(msg) {
		return combine(m.sendActiveRequest())
	}

	if m.focus != focusFile && m.focus != focusRequests && m.focus != focusWorkflows {
		m.suppressListKey = false
	}

	return combine(nil)
}

func (m *Model) canStartChord(msg tea.KeyMsg, key string) bool {
	if key == "" || m.bindingsMap == nil {
		return false
	}
	if !m.bindingsMap.HasChordPrefix(key) {
		return false
	}
	if m.editor.awaitingFindTarget() {
		return false
	}
	if m.focus == focusEditor && m.editorInsertMode && isPlainRuneKey(msg) {
		return false
	}
	return true
}

func (m *Model) resolveChord(prefix string, next string, msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.bindingsMap == nil || prefix == "" || next == "" {
		return false, nil
	}
	binding, ok := m.bindingsMap.ResolveChord(prefix, next)
	if !ok {
		return false, nil
	}
	if binding.Repeatable {
		m.repeatChordPrefix = prefix
		m.repeatChordActive = true
	} else {
		m.repeatChordActive = false
		m.repeatChordPrefix = ""
	}
	cmd, handled := m.runShortcutBinding(binding, msg)
	if !handled {
		return true, nil
	}
	return true, cmd
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
	if m.navigator == nil {
		return nil
	}
	if delta > 0 {
		if n := m.navigator.Selected(); n != nil {
			if n.Kind == navigator.KindFile && len(n.Children) == 0 {
				m.expandNavigatorFile(n.Payload.FilePath)
			}
			n.Expanded = true
			m.navigator.Refresh()
		}
	} else if delta < 0 {
		if n := m.navigator.Selected(); n != nil && n.Expanded {
			n.Expanded = false
			m.navigator.Refresh()
		}
	}
	return nil
}

func (m *Model) runWorkflowResize(delta float64) tea.Cmd {
	if m.navigator == nil {
		return nil
	}
	if delta > 0 {
		m.navigator.CollapseAll()
	} else if delta < 0 {
		m.navigator.ExpandAll()
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
