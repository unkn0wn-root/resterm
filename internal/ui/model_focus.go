package ui

import (
	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) cycleFocus(forward bool) tea.Cmd {
	switch m.focus {
	case focusFile, focusRequests, focusWorkflows:
		if forward {
			return m.setFocus(focusEditor)
		} else {
			return m.setFocus(focusResponse)
		}
	case focusEditor:
		if forward {
			return m.setFocus(focusResponse)
		} else {
			return m.setFocus(focusRequests)
		}
	case focusResponse:
		if forward {
			return m.setFocus(focusRequests)
		} else {
			return m.setFocus(focusEditor)
		}
	}
	return nil
}

func (m *Model) setFocus(target paneFocus) tea.Cmd {
	var cmds []tea.Cmd
	if m.focus == target {
		return nil
	}
	prev := m.focus
	m.focus = target
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
