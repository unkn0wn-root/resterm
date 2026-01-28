package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) cycleFocus(forward bool) tea.Cmd {
	next := m.nextVisibleFocus(forward)
	if next == m.focus {
		return nil
	}
	return m.setFocus(next)
}

func (m *Model) focusVisible(target paneFocus) bool {
	return !m.effectiveRegionCollapsed(regionFromFocus(target))
}

func (m *Model) nextVisibleFocus(forward bool) paneFocus {
	sequence := []paneFocus{focusRequests, focusEditor, focusResponse}
	currentRegion := regionFromFocus(m.focus)
	idx := 0
	for i, f := range sequence {
		if regionFromFocus(f) == currentRegion {
			idx = i
			break
		}
	}
	step := 1
	if !forward {
		step = -1
	}
	for i := 0; i < len(sequence); i++ {
		idx = (idx + step + len(sequence)) % len(sequence)
		candidate := sequence[idx]
		if m.focusVisible(candidate) {
			return candidate
		}
	}
	return m.focus
}

func (m *Model) ensureVisibleFocus() tea.Cmd {
	if m.focusVisible(m.focus) {
		return nil
	}
	next := m.nextVisibleFocus(true)
	if next == m.focus {
		next = m.nextVisibleFocus(false)
	}
	if next == m.focus {
		return nil
	}
	return m.setFocus(next)
}

func (m *Model) setFocus(target paneFocus) tea.Cmd {
	var cmds []tea.Cmd
	region := regionFromFocus(target)
	if !m.focusVisible(target) {
		label := m.collapsedStatusLabel(region)
		msg := statusMsg{
			text:  fmt.Sprintf("%s minimized", label),
			level: statusInfo,
		}
		if m.zoomActive && m.zoomRegion != region && !m.collapseState(region) {
			msg.text = fmt.Sprintf("%s hidden while zoomed", label)
		}
		m.setStatusMessage(msg)
		return nil
	}
	if m.focus == target {
		return nil
	}
	prev := m.focus
	m.focus = target
	clearedSel := false
	if prev == focusResponse && target != focusResponse {
		for _, id := range m.visiblePaneIDs() {
			if pane := m.pane(id); pane != nil && pane.sel.on {
				pane.sel.clear()
				clearedSel = true
			}
		}
	}
	if target != focusResponse {
		m.responsePaneChord = false
	}
	if target == focusEditor {
		if m.editorInsertMode {
			if cmd := m.editor.Cursor.SetMode(cursor.CursorBlink); cmd != nil {
				cmds = append(cmds, cmd)
			}
			if cmd := m.editor.Cursor.Focus(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		} else {
			if cmd := m.editor.Cursor.SetMode(cursor.CursorStatic); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if cmd := m.editor.Focus(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	} else {
		if prev == focusEditor && m.editorInsertMode {
			if cmd := m.setInsertMode(false, false); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		m.editor.Blur()
	}
	if target == focusResponse {
		m.ensurePaneFocusValid()
		m.setLivePane(m.responsePaneFocus)
	}
	if clearedSel {
		if cmd := m.syncResponsePanes(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return batchCommands(cmds...)
}

func (m *Model) setInsertMode(enabled bool, announce bool) tea.Cmd {
	var cmds []tea.Cmd
	if enabled == m.editorInsertMode {
		return nil
	}
	m.editorInsertMode = enabled
	if enabled {
		m.editor.SetMotionsEnabled(false)
		m.editor.KeyMap = m.editorWriteKeyMap
		if cmd := m.editor.Cursor.SetMode(cursor.CursorBlink); cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.editor.Cursor.Blink = true
		if cmd := m.editor.Cursor.Focus(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.editor.SetMetadataHintsEnabled(true)
	} else {
		m.editor.ClearSelection()
		m.editor.SetMotionsEnabled(true)
		m.editor.KeyMap = m.editorViewKeyMap
		if cmd := m.editor.Cursor.SetMode(cursor.CursorStatic); cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.editor.Cursor.Blink = false
		m.editor.SetMetadataHintsEnabled(false)
	}
	m.editor.undoCoalescing = false
	return batchCommands(cmds...)
}
