package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/cursor"
)

func (m *Model) cycleFocus(forward bool) {
	switch m.focus {
	case focusFile, focusRequests, focusWorkflows:
		if forward {
			m.setFocus(focusEditor)
		} else {
			m.setFocus(focusResponse)
		}
	case focusEditor:
		if forward {
			m.setFocus(focusResponse)
		} else {
			m.setFocus(focusRequests)
		}
	case focusResponse:
		if forward {
			m.setFocus(focusRequests)
		} else {
			m.setFocus(focusEditor)
		}
	}
}

func (m *Model) setFocus(target paneFocus) {
	if m.focus == target {
		return
	}
	prev := m.focus
	m.focus = target
	if target != focusResponse {
		m.responsePaneChord = false
	}
	if target == focusEditor {
		if m.editorInsertMode {
			m.editor.Cursor.SetMode(cursor.CursorBlink)
			m.editor.Cursor.Focus()
		} else {
			m.editor.Cursor.SetMode(cursor.CursorStatic)
		}
		m.editor.Focus()
	} else {
		if prev == focusEditor && m.editorInsertMode {
			m.setInsertMode(false, false)
		}
		m.editor.Blur()
	}
	if target == focusResponse {
		m.ensurePaneFocusValid()
		m.setLivePane(m.responsePaneFocus)
	}
}

func (m *Model) setInsertMode(enabled bool, announce bool) tea.Cmd {
	if enabled == m.editorInsertMode {
		return nil
	}
	m.editorInsertMode = enabled
	if enabled {
		m.editor.SetMotionsEnabled(false)
		m.editor.KeyMap = m.editorWriteKeyMap
		m.editor.Cursor.SetMode(cursor.CursorBlink)
		m.editor.Cursor.Blink = true
		cmd := m.editor.Cursor.Focus()
		m.editor.SetMetadataHintsEnabled(true)
		// Mode is shown in status bar, no need for temporary message
		return cmd
	} else {
		m.editor.ClearSelection()
		m.editor.SetMotionsEnabled(true)
		m.editor.KeyMap = m.editorViewKeyMap
		m.editor.Cursor.SetMode(cursor.CursorStatic)
		m.editor.Cursor.Blink = false
		m.editor.SetMetadataHintsEnabled(false)
		// Mode is shown in status bar, no need for temporary message
	}
	m.editor.undoCoalescing = false
	return nil
}
