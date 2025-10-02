package ui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
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
	selection      selectionState
	mode           selectionMode
	pendingMotion  string
	search         editorSearch
	motionsEnabled bool
}

type searchMatch struct {
	start int
	end   int
}

type editorSearch struct {
	query   string
	isRegex bool
	matches []searchMatch
	index   int
	active  bool
}

func newRequestEditor() requestEditor {
	ta := textarea.New()
	return requestEditor{Model: ta, motionsEnabled: true}
}

func (e *requestEditor) SetMotionsEnabled(enabled bool) {
	e.motionsEnabled = enabled
	if !enabled {
		e.pendingMotion = ""
	}
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
	e.applySelectionHighlight()
}

func (e *requestEditor) applySelectionHighlight() {
	if e.hasSelection() && e.selection.IsActive() {
		start, end := e.selection.Range()
		if start.Offset != end.Offset {
			e.Model.SetSelectionRange(start.Offset, end.Offset)
			return
		}
	}
	if match, ok := e.currentSearchMatch(); ok {
		start := e.clampOffset(match.start)
		end := e.clampOffset(match.end)
		if end > start {
			e.Model.SetSelectionRange(start, end)
			return
		}
	}
	e.Model.ClearSelectionRange()
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
	case "gg":
		if !e.motionsEnabled {
			break
		}
		handled = true
		if e.mode != selectionVisual {
			e.clearSelection()
		}
		if cmd := e.executeMotion(func() { e.moveToBufferTop() }); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case "G":
		if !e.motionsEnabled {
			break
		}
		handled = true
		if e.mode != selectionVisual {
			e.clearSelection()
		}
		if cmd := e.executeMotion(func() { e.moveToBufferBottom() }); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case "^":
		if !e.motionsEnabled {
			break
		}
		handled = true
		if e.mode != selectionVisual {
			e.clearSelection()
		}
		if cmd := e.executeMotion(func() { e.moveToLineStartNonBlank() }); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case "e":
		if !e.motionsEnabled {
			break
		}
		handled = true
		if e.mode != selectionVisual {
			e.clearSelection()
		}
		if cmd := e.executeMotion(func() { e.moveToWordEnd() }); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case "ctrl+f":
		if !e.motionsEnabled {
			break
		}
		handled = true
		if e.mode != selectionVisual {
			e.clearSelection()
		}
		if cmd := e.executeMotion(func() { e.pageDown(true) }); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case "ctrl+b":
		if !e.motionsEnabled {
			break
		}
		handled = true
		if e.mode != selectionVisual {
			e.clearSelection()
		}
		if cmd := e.executeMotion(func() { e.pageUp(true) }); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case "ctrl+d":
		if !e.motionsEnabled {
			break
		}
		handled = true
		if e.mode != selectionVisual {
			e.clearSelection()
		}
		if cmd := e.executeMotion(func() { e.pageDown(false) }); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case "ctrl+u":
		if !e.motionsEnabled {
			break
		}
		handled = true
		if e.mode != selectionVisual {
			e.clearSelection()
		}
		if cmd := e.executeMotion(func() { e.pageUp(false) }); cmd != nil {
			cmds = append(cmds, cmd)
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

func (e requestEditor) DeleteSelection() (requestEditor, tea.Cmd) {
	text := e.selectedText()
	if text == "" {
		return e, toEditorEventCmd(editorEvent{status: &statusMsg{text: "No selection to delete", level: statusWarn}})
	}
	if e.removeSelection() {
		status := statusMsg{text: "Selection deleted", level: statusInfo}
		return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
	}
	return e, nil
}

func (e requestEditor) HandleMotion(command string) (requestEditor, tea.Cmd, bool) {
	if !e.motionsEnabled {
		return e, nil, false
	}
	if e.pendingMotion == "g" {
		if command == "g" {
			e.pendingMotion = ""
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g', 'g'}}
			updated, cmd := e.Update(msg)
			updated.pendingMotion = ""
			return updated, cmd, true
		}
		e.pendingMotion = ""
	}

	switch command {
	case "g":
		e.pendingMotion = "g"
		return e, nil, true
	case "G", "^", "e", "ctrl+f", "ctrl+b", "ctrl+d", "ctrl+u":
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(command)}
		updated, cmd := e.Update(msg)
		updated.pendingMotion = ""
		return updated, cmd, true
	}

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
	updated.pendingMotion = ""
	return updated, cmd, true
}

func (e requestEditor) ApplySearch(query string, isRegex bool) (requestEditor, tea.Cmd) {
	trimmed := strings.TrimSpace(query)
	pe := &e
	pe.search.query = trimmed
	pe.search.isRegex = isRegex
	pe.search.matches = nil
	pe.search.index = -1
	pe.search.active = false

	if trimmed == "" {
		pe.applySelectionHighlight()
		status := statusMsg{text: "Enter a search pattern", level: statusWarn}
		return e, toEditorEventCmd(editorEvent{status: &status})
	}

	matches, err := pe.buildSearchMatches(trimmed, isRegex)
	if err != nil {
		pe.applySelectionHighlight()
		status := statusMsg{text: fmt.Sprintf("Invalid regex: %v", err), level: statusError}
		return e, toEditorEventCmd(editorEvent{status: &status})
	}

	pe.search.matches = matches
	if len(matches) == 0 {
		pe.applySelectionHighlight()
		status := statusMsg{text: fmt.Sprintf("No matches for %q", trimmed), level: statusWarn}
		return e, toEditorEventCmd(editorEvent{status: &status})
	}

	pe.clearSelection()
	offset := pe.caretPosition().Offset
	index, wrapped := firstMatchIndex(matches, offset)
	if index < 0 {
		index = 0
	}
	moveCmd := pe.jumpToSearchIndex(index)
	statusText := fmt.Sprintf("Match %d/%d for %q", index+1, len(matches), trimmed)
	if wrapped {
		statusText += " (wrapped)"
	}
	status := statusMsg{text: statusText, level: statusInfo}
	if moveCmd != nil {
		return e, tea.Batch(moveCmd, toEditorEventCmd(editorEvent{status: &status}))
	}
	return e, toEditorEventCmd(editorEvent{status: &status})
}

func (e requestEditor) NextSearchMatch() (requestEditor, tea.Cmd) {
	pe := &e
	trimmed := strings.TrimSpace(pe.search.query)
	if trimmed == "" {
		status := statusMsg{text: "No active search", level: statusWarn}
		return e, toEditorEventCmd(editorEvent{status: &status})
	}

	if len(pe.search.matches) == 0 {
		matches, err := pe.buildSearchMatches(trimmed, pe.search.isRegex)
		if err != nil {
			status := statusMsg{text: fmt.Sprintf("Invalid regex: %v", err), level: statusError}
			return e, toEditorEventCmd(editorEvent{status: &status})
		}
		pe.search.matches = matches
		pe.search.index = -1
		if len(matches) == 0 {
			status := statusMsg{text: fmt.Sprintf("No matches for %q", trimmed), level: statusWarn}
			pe.applySelectionHighlight()
			return e, toEditorEventCmd(editorEvent{status: &status})
		}
	}

	if pe.search.index < 0 || pe.search.index >= len(pe.search.matches) {
		offset := pe.caretPosition().Offset
		index, wrapped := firstMatchIndex(pe.search.matches, offset)
		moveCmd := pe.jumpToSearchIndex(index)
		statusText := fmt.Sprintf("Match %d/%d for %q", index+1, len(pe.search.matches), trimmed)
		if wrapped {
			statusText += " (wrapped)"
		}
		status := statusMsg{text: statusText, level: statusInfo}
		if moveCmd != nil {
			return e, tea.Batch(moveCmd, toEditorEventCmd(editorEvent{status: &status}))
		}
		return e, toEditorEventCmd(editorEvent{status: &status})
	}

	nextIndex := pe.search.index + 1
	wrapped := false
	if nextIndex >= len(pe.search.matches) {
		nextIndex = 0
		wrapped = true
	}
	moveCmd := pe.jumpToSearchIndex(nextIndex)
	statusText := fmt.Sprintf("Match %d/%d for %q", nextIndex+1, len(pe.search.matches), trimmed)
	if wrapped {
		statusText += " (wrapped)"
	}
	status := statusMsg{text: statusText, level: statusInfo}
	if moveCmd != nil {
		return e, tea.Batch(moveCmd, toEditorEventCmd(editorEvent{status: &status}))
	}
	return e, toEditorEventCmd(editorEvent{status: &status})
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
	e.applySelectionHighlight()
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

func (e *requestEditor) moveToBufferTop() {
	line := 0
	col := 0
	lines := strings.Split(e.Value(), "\n")
	if len(lines) > 0 {
		col = firstNonWhitespaceColumn(lines[0])
	}
	e.moveCursorTo(line, col)
}

func (e *requestEditor) moveToBufferBottom() {
	lines := strings.Split(e.Value(), "\n")
	if len(lines) == 0 {
		e.moveCursorTo(0, 0)
		return
	}
	line := len(lines) - 1
	col := firstNonWhitespaceColumn(lines[line])
	e.moveCursorTo(line, col)
}

func (e *requestEditor) moveToLineStartNonBlank() {
	lines := strings.Split(e.Value(), "\n")
	if len(lines) == 0 {
		e.moveCursorTo(0, 0)
		return
	}
	line := e.Line()
	if line < 0 {
		line = 0
	}
	if line >= len(lines) {
		line = len(lines) - 1
	}
	col := firstNonWhitespaceColumn(lines[line])
	e.moveCursorTo(line, col)
}

func (e *requestEditor) moveToWordEnd() {
	value := e.Value()
	runes := []rune(value)
	if len(runes) == 0 {
		return
	}
	pos := e.caretPosition()
	idx := pos.Offset
	if idx >= len(runes) {
		idx = len(runes) - 1
	}
	if idx < 0 {
		idx = 0
	}

	if unicode.IsSpace(runes[idx]) {
		for idx < len(runes) && unicode.IsSpace(runes[idx]) {
			idx++
		}
		if idx >= len(runes) {
			return
		}
	}

	for idx+1 < len(runes) && !unicode.IsSpace(runes[idx+1]) {
		idx++
	}

	line, col := positionForOffset(value, idx)
	e.moveCursorTo(line, col)
}

func (e *requestEditor) executeMotion(fn func()) tea.Cmd {
	fn()
	var cmd tea.Cmd
	e.Model, cmd = e.Model.Update(nil)
	return cmd
}

func (e requestEditor) pageStep(full bool) int {
	height := e.Height()
	if height <= 0 {
		height = 1
	}
	if full {
		if height > 1 {
			return height - 1
		}
		return 1
	}
	step := height / 2
	if step < 1 {
		step = 1
	}
	return step
}

func (e *requestEditor) pageDown(full bool) {
	e.moveLines(e.pageStep(full))
}

func (e *requestEditor) pageUp(full bool) {
	e.moveLines(-e.pageStep(full))
}

func (e *requestEditor) moveLines(delta int) {
	switch {
	case delta > 0:
		for i := 0; i < delta; i++ {
			e.CursorDown()
		}
	case delta < 0:
		for i := 0; i < -delta; i++ {
			e.CursorUp()
		}
	}
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

func firstNonWhitespaceColumn(line string) int {
	runes := []rune(line)
	for i, r := range runes {
		if !unicode.IsSpace(r) {
			return i
		}
	}
	return 0
}

func positionForOffset(value string, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		return 0, 0
	}
	remaining := offset
	for i, line := range lines {
		lineLen := utf8.RuneCountInString(line)
		if remaining <= lineLen {
			return i, remaining
		}
		remaining -= lineLen + 1
		if remaining < 0 {
			return i, lineLen
		}
	}
	last := len(lines) - 1
	return last, utf8.RuneCountInString(lines[last])
}

func (e requestEditor) clampOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	total := utf8.RuneCountInString(e.Value())
	if offset > total {
		return total
	}
	return offset
}

func (e requestEditor) currentSearchMatch() (searchMatch, bool) {
	if !e.search.active {
		return searchMatch{}, false
	}
	if e.search.index < 0 || e.search.index >= len(e.search.matches) {
		return searchMatch{}, false
	}
	return e.search.matches[e.search.index], true
}

func firstMatchIndex(matches []searchMatch, offset int) (int, bool) {
	if len(matches) == 0 {
		return -1, false
	}
	for i, match := range matches {
		if offset < match.end {
			return i, false
		}
	}
	return 0, true
}

func literalMatches(content, pattern string) []searchMatch {
	patternRunes := []rune(pattern)
	contentRunes := []rune(content)
	plen := len(patternRunes)
	if plen == 0 || len(contentRunes) < plen {
		return nil
	}
	matches := make([]searchMatch, 0)
	for i := 0; i <= len(contentRunes)-plen; i++ {
		match := true
		for j := 0; j < plen; j++ {
			if contentRunes[i+j] != patternRunes[j] {
				match = false
				break
			}
		}
		if match {
			matches = append(matches, searchMatch{start: i, end: i + plen})
		}
	}
	return matches
}

func regexMatches(content string, rx *regexp.Regexp) []searchMatch {
	indices := rx.FindAllStringIndex(content, -1)
	if len(indices) == 0 {
		return nil
	}
	matches := make([]searchMatch, 0, len(indices))
	for _, idx := range indices {
		if len(idx) != 2 {
			continue
		}
		startByte, endByte := idx[0], idx[1]
		if endByte <= startByte {
			continue
		}
		start := utf8.RuneCountInString(content[:startByte])
		end := utf8.RuneCountInString(content[:endByte])
		matches = append(matches, searchMatch{start: start, end: end})
	}
	return matches
}

func (e requestEditor) buildSearchMatches(query string, isRegex bool) ([]searchMatch, error) {
	value := e.Value()
	if isRegex {
		rx, err := regexp.Compile(query)
		if err != nil {
			return nil, err
		}
		return regexMatches(value, rx), nil
	}
	return literalMatches(value, query), nil
}

func (e *requestEditor) jumpToSearchIndex(index int) tea.Cmd {
	if index < 0 || index >= len(e.search.matches) {
		return nil
	}
	match := e.search.matches[index]
	start := e.clampOffset(match.start)
	line, col := positionForOffset(e.Value(), start)
	e.search.index = index
	e.search.active = true
	return e.executeMotion(func() {
		e.moveCursorTo(line, col)
		e.applySelectionHighlight()
	})
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
