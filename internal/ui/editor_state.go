package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

type cursorPosition struct {
	Line   int
	Column int
	Offset int
}

type selectionState struct {
	active bool
	anchor cursorPosition
	caret  cursorPosition
}

func (s *selectionState) Activate(pos cursorPosition) {
	s.active = true
	s.anchor = pos
	s.caret = pos
}

func (s *selectionState) Update(pos cursorPosition) {
	if !s.active {
		return
	}
	s.caret = pos
	if s.anchor.Offset == s.caret.Offset {
		s.active = false
	}
}

func (s *selectionState) Clear() {
	s.active = false
	s.anchor = cursorPosition{}
	s.caret = cursorPosition{}
}

func (s selectionState) IsActive() bool {
	return s.active
}

func (s selectionState) Range() (cursorPosition, cursorPosition) {
	if !s.active {
		return s.caret, s.caret
	}
	if s.anchor.Offset <= s.caret.Offset {
		return s.anchor, s.caret
	}
	return s.caret, s.anchor
}

func (s selectionState) Caret() cursorPosition {
	return s.caret
}

// editorEvent is emitted from the editor so the root model can react (marking
// the document dirty, surfacing status messages, etc.).
type editorEvent struct {
	dirty  bool
	status *statusMsg
}

func toEditorEventCmd(evt editorEvent) tea.Cmd {
	return func() tea.Msg {
		return evt
	}
}

type requestEditor struct {
	textarea.Model
	selection selectionState
	mode      selectionMode
}

func newRequestEditor() requestEditor {
	ta := textarea.New()
	return requestEditor{Model: ta}
}

type selectionMode int

const (
	selectionNone selectionMode = iota
	selectionManual
	selectionVisual
)

func (e requestEditor) hasSelection() bool {
	return e.mode != selectionNone
}

func (e *requestEditor) startSelection(pos cursorPosition, mode selectionMode) {
	e.selection.Activate(pos)
	e.mode = mode
	e.applySelectionHighlight()
}

func (e *requestEditor) clearSelection() {
	e.selection.Clear()
	e.mode = selectionNone
	e.Model.ClearSelectionRange()
}

func (e *requestEditor) applySelectionHighlight() {
	if !e.hasSelection() || !e.selection.IsActive() {
		e.Model.ClearSelectionRange()
		return
	}
	start, end := e.selection.Range()
	if start.Offset == end.Offset {
		e.Model.ClearSelectionRange()
		return
	}
	e.Model.SetSelectionRange(start.Offset, end.Offset)
}

func (e requestEditor) Update(msg tea.Msg) (requestEditor, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		var innerCmd tea.Cmd
		e.Model, innerCmd = e.Model.Update(msg)
		return e, innerCmd
	}

	before := e.caretPosition()
	prevSelection := e.selection
	prevMode := e.mode

	transformed := keyMsg
	handled := false
	var cmds []tea.Cmd

	switch keyMsg.String() {
	case "ctrl+space":
		if e.hasSelection() {
			e.clearSelection()
		} else {
			e.startSelection(before, selectionManual)
		}
		handled = true
	case "esc":
		if e.hasSelection() {
			e.clearSelection()
			handled = true
		}
	case "ctrl+c":
		if text := e.selectedText(); text != "" {
			cmds = append(cmds, e.copyToClipboard(text))
		}
		handled = true
	case "ctrl+x":
		if text := e.selectedText(); text != "" {
			cmds = append(cmds, e.copyToClipboard(text))
			if e.removeSelection() {
				cmds = append(cmds, toEditorEventCmd(editorEvent{dirty: true}))
			}
		}
		handled = true
	case "ctrl+v":
		if e.hasSelection() {
			if e.removeSelection() {
				cmds = append(cmds, toEditorEventCmd(editorEvent{dirty: true}))
			}
		}
	case "backspace", "ctrl+h", "delete":
		if e.hasSelection() {
			if e.removeSelection() {
				cmds = append(cmds, toEditorEventCmd(editorEvent{dirty: true}))
			}
			handled = true
		}
	}

	if !handled {
		if stripped, ok := stripSelectionMovement(keyMsg); ok {
			if !e.hasSelection() {
				e.startSelection(before, selectionManual)
			}
			transformed = stripped
		} else if isMovementKey(keyMsg) {
			if e.mode != selectionVisual {
				e.clearSelection()
			}
		} else if insertsText(keyMsg) && e.hasSelection() {
			if e.removeSelection() {
				cmds = append(cmds, toEditorEventCmd(editorEvent{dirty: true}))
			}
		}
	}

	if !handled {
		var innerCmd tea.Cmd
		e.Model, innerCmd = e.Model.Update(transformed)
		if innerCmd != nil {
			cmds = append(cmds, innerCmd)
		}
	}

	after := e.caretPosition()
	if transformed.String() != keyMsg.String() && !e.hasSelection() {
		e.clearSelection()
	}

	e.selection.Update(after)
	if e.mode == selectionVisual {
		e.selection.active = true
		e.selection.caret = after
	} else if !e.selection.IsActive() {
		e.mode = selectionNone
	}
	e.applySelectionHighlight()

	selectionActive := e.mode == selectionVisual || (e.mode != selectionNone && e.selection.IsActive())
	prevActive := prevMode == selectionVisual || (prevMode != selectionNone && prevSelection.IsActive())

	if selectionActive {
		if prevSelection.Caret() != e.selection.Caret() || !prevActive {
			start, end := e.selection.Range()
			summary := fmt.Sprintf("Selection L%d:%d–L%d:%d", start.Line+1, start.Column+1, end.Line+1, end.Column+1)
			cmds = append(cmds, toEditorEventCmd(editorEvent{status: &statusMsg{text: summary, level: statusInfo}}))
		}
	} else if prevActive {
		cmds = append(cmds, toEditorEventCmd(editorEvent{status: &statusMsg{text: "Selection cleared", level: statusInfo}}))
	}

	return e, tea.Batch(cmds...)
}

func (e *requestEditor) ClearSelection() {
	e.clearSelection()
}

func (e requestEditor) ToggleVisual() (requestEditor, tea.Cmd) {
	if e.mode == selectionVisual {
		e.clearSelection()
		return e, toEditorEventCmd(editorEvent{status: &statusMsg{text: "Visual mode off", level: statusInfo}})
	}
	e.startSelection(e.caretPosition(), selectionVisual)
	return e, toEditorEventCmd(editorEvent{status: &statusMsg{text: "Visual mode", level: statusInfo}})
}

func (e requestEditor) YankSelection() (requestEditor, tea.Cmd) {
	text := e.selectedText()
	if text == "" {
		return e, toEditorEventCmd(editorEvent{status: &statusMsg{text: "No selection to yank", level: statusWarn}})
	}
	cmd := e.copyToClipboard(text)
	e.clearSelection()
	return e, cmd
}

func (e requestEditor) HandleMotion(command string) (requestEditor, tea.Cmd, bool) {
	var msg tea.KeyMsg
	switch command {
	case "h":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "l":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "j":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "k":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	case "w":
		msg = tea.KeyMsg{Type: tea.KeyRight, Alt: true}
	case "b":
		msg = tea.KeyMsg{Type: tea.KeyLeft, Alt: true}
	case "0":
		msg = tea.KeyMsg{Type: tea.KeyHome}
	case "$":
		msg = tea.KeyMsg{Type: tea.KeyEnd}
	default:
		return e, nil, false
	}
	updated, cmd := e.Update(msg)
	return updated, cmd, true
}

func (e requestEditor) caretPosition() cursorPosition {
	line := e.Line()
	info := e.LineInfo()
	column := info.StartColumn + info.ColumnOffset
	offset := e.offsetForPosition(line, column)
	return cursorPosition{Line: line, Column: column, Offset: offset}
}

func (e requestEditor) selectedText() string {
	if !e.hasSelection() {
		return ""
	}
	start, end := e.selection.Range()
	if start.Offset == end.Offset {
		return ""
	}
	content := []rune(e.Value())
	if start.Offset < 0 {
		start.Offset = 0
	}
	if end.Offset > len(content) {
		end.Offset = len(content)
	}
	if start.Offset >= end.Offset {
		return ""
	}
	return string(content[start.Offset:end.Offset])
}

func (e *requestEditor) removeSelection() bool {
	if !e.hasSelection() {
		return false
	}
	start, end := e.selection.Range()
	if start.Offset == end.Offset {
		e.clearSelection()
		return false
	}

	runes := []rune(e.Value())
	if start.Offset < 0 {
		start.Offset = 0
	}
	if end.Offset > len(runes) {
		end.Offset = len(runes)
	}
	if start.Offset >= end.Offset {
		e.clearSelection()
		return false
	}

	updated := append([]rune{}, runes[:start.Offset]...)
	updated = append(updated, runes[end.Offset:]...)
	e.SetValue(string(updated))
	e.clearSelection()
	e.moveCursorTo(start.Line, start.Column)
	return true
}

func (e *requestEditor) moveCursorTo(line, column int) {
	if line < 0 {
		line = 0
	}
	lc := e.LineCount()
	if line >= lc {
		line = lc - 1
		if line < 0 {
			line = 0
		}
	}

	for e.Line() > line {
		e.CursorUp()
	}
	for e.Line() < line {
		e.CursorDown()
	}

	if column < 0 {
		column = 0
	}
	lineRunes := lineLength(e.Value(), line)
	if column > lineRunes {
		column = lineRunes
	}
	e.SetCursor(column)
}

func (e requestEditor) offsetForPosition(line, column int) int {
	if line < 0 {
		return 0
	}
	lines := strings.Split(e.Value(), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	if line >= len(lines) {
		line = len(lines) - 1
	}
	offset := 0
	for i := 0; i < line; i++ {
		offset += utf8.RuneCountInString(lines[i]) + 1
	}
	col := column
	if col < 0 {
		col = 0
	}
	lineLen := utf8.RuneCountInString(lines[line])
	if col > lineLen {
		col = lineLen
	}
	offset += col
	return offset
}

func lineLength(value string, line int) int {
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		return 0
	}
	if line < 0 {
		line = 0
	}
	if line >= len(lines) {
		line = len(lines) - 1
	}
	return utf8.RuneCountInString(lines[line])
}

func stripSelectionMovement(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	switch msg.Type {
	case tea.KeyShiftLeft:
		msg.Type = tea.KeyLeft
		return msg, true
	case tea.KeyShiftRight:
		msg.Type = tea.KeyRight
		return msg, true
	case tea.KeyShiftUp:
		msg.Type = tea.KeyUp
		return msg, true
	case tea.KeyShiftDown:
		msg.Type = tea.KeyDown
		return msg, true
	case tea.KeyShiftHome:
		msg.Type = tea.KeyHome
		return msg, true
	case tea.KeyShiftEnd:
		msg.Type = tea.KeyEnd
		return msg, true
	case tea.KeyCtrlShiftLeft:
		msg.Type = tea.KeyCtrlLeft
		return msg, true
	case tea.KeyCtrlShiftRight:
		msg.Type = tea.KeyCtrlRight
		return msg, true
	case tea.KeyCtrlShiftUp:
		msg.Type = tea.KeyCtrlUp
		return msg, true
	case tea.KeyCtrlShiftDown:
		msg.Type = tea.KeyCtrlDown
		return msg, true
	default:
		return msg, false
	}
}

func isMovementKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown,
		tea.KeyHome, tea.KeyEnd, tea.KeyCtrlLeft, tea.KeyCtrlRight,
		tea.KeyCtrlUp, tea.KeyCtrlDown, tea.KeyPgUp, tea.KeyPgDown,
		tea.KeyCtrlPgUp, tea.KeyCtrlPgDown:
		return true
	default:
		return false
	}
}

func insertsText(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		return true
	}
	switch msg.String() {
	case "enter", "ctrl+m", "ctrl+j", "tab":
		return true
	default:
		return false
	}
}

func (e requestEditor) copyToClipboard(text string) tea.Cmd {
	trimmed := text
	return func() tea.Msg {
		if trimmed == "" {
			return editorEvent{}
		}
		if err := clipboard.WriteAll(trimmed); err != nil {
			return editorEvent{status: &statusMsg{text: "Clipboard unavailable", level: statusWarn}}
		}
		return editorEvent{status: &statusMsg{text: "Copied selection", level: statusInfo}}
	}
}
