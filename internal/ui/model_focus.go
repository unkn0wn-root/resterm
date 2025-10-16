package ui

import (
	"github.com/charmbracelet/bubbles/cursor"
)

func (m *Model) cycleFocus(forward bool) {
	switch m.focus {
	case focusFile:
		if forward {
			if len(m.requestItems) > 0 {
				m.setFocus(focusRequests)
			} else if len(m.workflowItems) > 0 {
				m.setFocus(focusWorkflows)
			} else {
				m.setFocus(focusEditor)
			}
		} else {
			m.setFocus(focusResponse)
		}
	case focusRequests:
		if forward {
			if len(m.workflowItems) > 0 {
				m.setFocus(focusWorkflows)
			} else {
				m.setFocus(focusEditor)
			}
		} else {
			m.setFocus(focusFile)
		}
	case focusWorkflows:
		if forward {
			m.setFocus(focusEditor)
		} else {
			if len(m.requestItems) > 0 {
				m.setFocus(focusRequests)
			} else {
				m.setFocus(focusFile)
			}
		}
	case focusEditor:
		if forward {
			m.setFocus(focusResponse)
		} else {
			if len(m.workflowItems) > 0 {
				m.setFocus(focusWorkflows)
			} else {
				m.setFocus(focusRequests)
			}
		}
	case focusResponse:
		if forward {
			m.setFocus(focusFile)
		} else {
			if len(m.workflowItems) > 0 {
				m.setFocus(focusWorkflows)
			} else {
				m.setFocus(focusEditor)
			}
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

func (m *Model) setInsertMode(enabled bool, announce bool) {
	if enabled == m.editorInsertMode {
		return
	}
	m.editorInsertMode = enabled
	if enabled {
		m.editor.SetMotionsEnabled(false)
		m.editor.KeyMap = m.editorWriteKeyMap
		m.editor.Cursor.SetMode(cursor.CursorBlink)
		m.editor.Cursor.Blink = true
		m.editor.SetMetadataHintsEnabled(true)
		if announce {
			m.setStatusMessage(statusMsg{text: "Insert mode", level: statusInfo})
		}
	} else {
		m.editor.ClearSelection()
		m.editor.SetMotionsEnabled(true)
		m.editor.KeyMap = m.editorViewKeyMap
		m.editor.Cursor.SetMode(cursor.CursorStatic)
		m.editor.Cursor.Blink = false
		m.editor.SetMetadataHintsEnabled(false)
		if announce {
			m.setStatusMessage(statusMsg{text: "View mode", level: statusInfo})
		}
	}
	m.editor.undoCoalescing = false
}
