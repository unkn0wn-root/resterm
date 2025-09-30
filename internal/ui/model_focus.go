package ui

import (
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/list"
)

func (m *Model) cycleFocus(forward bool) {
	switch m.focus {
	case focusFile:
		if forward {
			m.setFocus(focusRequests)
		} else {
			m.setFocus(focusResponse)
		}
	case focusRequests:
		if forward {
			m.setFocus(focusEditor)
		} else {
			m.setFocus(focusFile)
		}
	case focusEditor:
		if forward {
			m.setFocus(focusResponse)
		} else {
			m.setFocus(focusRequests)
		}
	case focusResponse:
		if forward {
			m.setFocus(focusFile)
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
}

func (m *Model) allowPaneFocusShortcut() bool {
	switch m.focus {
	case focusEditor:
		return !m.editorInsertMode
	case focusFile:
		return m.fileList.FilterState() != list.Filtering
	case focusRequests:
		return m.requestList.FilterState() != list.Filtering
	case focusResponse:
		return true
	default:
		return true
	}
}

func (m *Model) setInsertMode(enabled bool, announce bool) {
	if enabled == m.editorInsertMode {
		return
	}
	m.editorInsertMode = enabled
	if enabled {
		m.editor.KeyMap = m.editorWriteKeyMap
		m.editor.Cursor.SetMode(cursor.CursorBlink)
		m.editor.Cursor.Blink = true
		if announce {
			m.statusMessage = statusMsg{text: "Insert mode", level: statusInfo}
		}
	} else {
		m.editor.KeyMap = m.editorViewKeyMap
		m.editor.Cursor.SetMode(cursor.CursorStatic)
		m.editor.Cursor.Blink = false
		if announce {
			m.statusMessage = statusMsg{text: "View mode", level: statusInfo}
		}
	}
}
