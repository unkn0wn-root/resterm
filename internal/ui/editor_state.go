package ui

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/ui/hint"
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

type editorEvent struct {
	dirty  bool
	status *statusMsg
}

func toEditorEventCmd(evt editorEvent) tea.Cmd {
	return func() tea.Msg {
		return evt
	}
}

func statusCmd(level statusLevel, text string) tea.Cmd {
	status := statusMsg{
		level: level,
		text:  text,
	}

	return toEditorEventCmd(editorEvent{status: &status})
}

type requestEditor struct {
	textarea.Model
	selection            selectionState
	mode                 selectionMode
	pendingMotion        string
	search               editorSearch
	motionsEnabled       bool
	undoStack            []editorSnapshot
	redoStack            []editorSnapshot
	undoCoalescing       bool
	registerText         string
	metadataHints        metadataHintState
	metadataHintsEnabled bool
	hintManager          hint.Manager
}

const editorUndoLimit = 64

type editorSnapshot struct {
	value     string
	cursor    cursorPosition
	selection selectionState
	mode      selectionMode
	viewStart int
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

const metadataHintDisplayLimit = 6

type metadataHintState struct {
	active       bool
	anchorOffset int
	selection    int
	filtered     []hint.Hint
	ctx          hint.Context
}

func (s *metadataHintState) deactivate() {
	s.active = false
	s.filtered = nil
	s.selection = 0
	s.anchorOffset = 0
	s.ctx = hint.Context{}
}

func (s *metadataHintState) update(anchor int, filtered []hint.Hint, ctx hint.Context) {
	if len(filtered) == 0 {
		s.deactivate()
		return
	}
	if !s.active || s.anchorOffset != anchor || s.selection >= len(filtered) {
		s.selection = 0
	}
	s.active = true
	s.anchorOffset = anchor
	s.filtered = filtered
	s.ctx = ctx
}

func (s *metadataHintState) move(delta int) {
	if !s.active || len(s.filtered) == 0 {
		return
	}
	count := len(s.filtered)
	idx := (s.selection + delta) % count
	if idx < 0 {
		idx += count
	}
	s.selection = idx
}

func (s metadataHintState) display(limit int) (items []hint.Hint, selected int, ok bool) {
	if !s.active || len(s.filtered) == 0 || limit <= 0 {
		return nil, 0, false
	}
	if s.selection >= len(s.filtered) {
		return nil, 0, false
	}
	start := s.selection - limit/2
	if start < 0 {
		start = 0
	}
	maxStart := len(s.filtered) - limit
	if maxStart < 0 {
		maxStart = 0
	}
	if start > maxStart {
		start = maxStart
	}
	end := start + limit
	if end > len(s.filtered) {
		end = len(s.filtered)
	}
	window := make([]hint.Hint, end-start)
	copy(window, s.filtered[start:end])
	return window, s.selection - start, true
}

func newRequestEditor() requestEditor {
	ta := textarea.New()
	return requestEditor{
		Model:          ta,
		motionsEnabled: true,
		hintManager:    hint.NewManager(hint.MetaSource()),
	}
}

func (e *requestEditor) SetMotionsEnabled(enabled bool) {
	e.motionsEnabled = enabled
	if !enabled {
		e.pendingMotion = ""
	}
}

func (e requestEditor) ViewStart() int {
	return e.Model.ViewStart()
}

func (e *requestEditor) SetViewStart(offset int) {
	e.Model.SetViewStart(offset)
}

func (e requestEditor) captureSnapshot() editorSnapshot {
	return editorSnapshot{
		value:     e.Value(),
		cursor:    e.caretPosition(),
		selection: e.selection,
		mode:      e.mode,
		viewStart: e.ViewStart(),
	}
}

func appendSnapshot(
	stack []editorSnapshot,
	snapshot editorSnapshot,
) []editorSnapshot {
	stack = append(stack, snapshot)
	if len(stack) > editorUndoLimit {
		stack = stack[1:]
	}
	return stack
}

func (e *requestEditor) storeUndoSnapshot() {
	e.undoStack = appendSnapshot(e.undoStack, e.captureSnapshot())
	e.redoStack = nil
}

func (e *requestEditor) pushUndoSnapshot() {
	e.storeUndoSnapshot()
	e.undoCoalescing = false
}

func (e *requestEditor) pushUndoSnapshotAuto() {
	if e.undoCoalescing {
		return
	}
	e.storeUndoSnapshot()
	e.undoCoalescing = true
}

func (e *requestEditor) restoreSnapshot(snapshot editorSnapshot) {
	e.SetValue(snapshot.value)
	e.selection = snapshot.selection
	e.mode = snapshot.mode
	e.pendingMotion = ""
	e.Model.SetViewStart(snapshot.viewStart)
	e.moveCursorTo(snapshot.cursor.Line, snapshot.cursor.Column)
	e.applySelectionHighlight()
	e.undoCoalescing = false
}

type selectionMode int

const (
	selectionNone selectionMode = iota
	selectionManual
	selectionVisual
	selectionVisualLine
)

type deleteMotionSpec struct {
	command             string
	includeFinalForward bool
	linewise            bool
}

type motionSpan struct {
	startLine int
	endLine   int
	start     int
	end       int
	linewise  bool
}

func (e requestEditor) hasSelection() bool {
	return e.mode != selectionNone
}

func (e requestEditor) isVisualMode() bool {
	return e.mode == selectionVisual || e.mode == selectionVisualLine
}

func (e requestEditor) isVisualLineMode() bool {
	return e.mode == selectionVisualLine
}

func (e requestEditor) awaitingFindTarget() bool {
	switch e.pendingMotion {
	case "f", "t", "T":
		return true
	default:
		return false
	}
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
		if startOffset, endOffset, ok := e.selectionOffsets(); ok &&
			endOffset > startOffset {
			e.SetSelectionRange(startOffset, endOffset)
			return
		}
	}
	if match, ok := e.currentSearchMatch(); ok {
		start := e.clampOffset(match.start)
		end := e.clampOffset(match.end)
		if end > start {
			e.SetSelectionRange(start, end)
			return
		}
	}
	e.ClearSelectionRange()
}

func (e requestEditor) selectionOffsets() (int, int, bool) {
	if !e.selection.IsActive() {
		return 0, 0, false
	}
	start, end := e.selection.Range()
	if e.isVisualLineMode() {
		lineStart, _, _ := e.lineBounds(start.Line)
		_, lineEnd, _ := e.lineBounds(end.Line)
		return lineStart, lineEnd, true
	}
	startOffset := start.Offset
	endOffset := end.Offset
	if e.isVisualMode() && startOffset == endOffset {
		// Visual mode should include the rune under the cursor.
		runes := []rune(e.Value())
		if startOffset >= 0 && startOffset < len(runes) {
			endOffset = nextRuneOffset(runes, startOffset)
		}
	}
	return startOffset, endOffset, true
}

func (e requestEditor) selectionSummaryRange() (
	cursorPosition,
	cursorPosition,
) {
	start, end := e.selection.Range()
	if !e.selection.IsActive() {
		return start, end
	}
	if e.isVisualLineMode() {
		lineStart, _, startIdx := e.lineBounds(start.Line)
		_, lineEnd, endIdx := e.lineBounds(end.Line)
		value := e.Value()
		start = cursorPosition{Line: startIdx, Column: 0, Offset: lineStart}
		endLen := lineLength(value, endIdx)
		endCol := 0
		if endLen > 0 {
			endCol = endLen - 1
		}
		end = cursorPosition{Line: endIdx, Column: endCol, Offset: lineEnd}
		return start, end
	}
	return start, end
}

func (e *requestEditor) SetMetadataHintsEnabled(enabled bool) {
	e.metadataHintsEnabled = enabled
	if !enabled {
		e.metadataHints.deactivate()
	}
}

func (e *requestEditor) metadataHintsDisplay(
	limit int,
) (items []hint.Hint, selection int, ok bool) {
	return e.metadataHints.display(limit)
}

func (e *requestEditor) handleMetadataHintNavigation(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !e.metadataHints.active || len(e.metadataHints.filtered) == 0 {
		return false, nil
	}
	switch msg.String() {
	case "down", "ctrl+n":
		e.metadataHints.move(1)
		return true, nil
	case "up", "ctrl+p", "shift+tab":
		e.metadataHints.move(-1)
		return true, nil
	case "tab", "enter", "ctrl+m":
		cmd := e.applyMetadataHintSelection()
		return true, cmd
	default:
		return false, nil
	}
}

func (e *requestEditor) refreshMetadataHints() {
	if !e.metadataHintsEnabled {
		e.metadataHints.deactivate()
		return
	}
	value := e.Value()
	runes := []rune(value)
	caret := e.caretPosition()
	if caret.Offset <= 0 || caret.Offset > len(runes) {
		e.metadataHints.deactivate()
		return
	}
	anchor := hint.Anchor(runes, caret.Offset)
	if anchor == -1 {
		e.metadataHints.deactivate()
		return
	}
	if !hint.InDirectiveContext(runes, anchor) {
		e.metadataHints.deactivate()
		return
	}
	queryRunes := runes[anchor+1 : caret.Offset]
	if caret.Offset < len(runes) && hint.IsQueryRune(runes[caret.Offset]) {
		// Caret sits before additional directive characters; keep existing hints.
		return
	}
	ctx, ok := hint.AnalyzeContext(queryRunes)
	if !ok {
		e.metadataHints.deactivate()
		return
	}
	filtered := e.hintManager.Options(ctx)
	if len(filtered) == 0 {
		e.metadataHints.deactivate()
		return
	}
	insertAnchor := anchor
	if ctx.Mode == hint.ModeSubcommand {
		insertAnchor = anchor + 1 + ctx.TokenStart
	}
	e.metadataHints.update(insertAnchor, filtered, ctx)
}

func (e *requestEditor) applyMetadataHintSelection() tea.Cmd {
	if !e.metadataHints.active || len(e.metadataHints.filtered) == 0 {
		return nil
	}
	if e.metadataHints.selection < 0 || e.metadataHints.selection >= len(e.metadataHints.filtered) {
		return nil
	}
	selected := e.metadataHints.filtered[e.metadataHints.selection]
	insert := selected.Label
	if selected.Insert != "" {
		insert = selected.Insert
	}
	start := e.metadataHints.anchorOffset
	caret := e.caretPosition()
	if start < 0 || caret.Offset < start {
		e.metadataHints.deactivate()
		return nil
	}
	runes := []rune(e.Value())
	end := caret.Offset
	if end > len(runes) {
		end = len(runes)
	}
	before := runes[:start]
	after := runes[end:]
	bodyRunes := []rune(insert)
	replacementRunes := append([]rune{}, bodyRunes...)
	needsSpace := len(after) == 0 || !unicode.IsSpace(after[0])
	e.pushUndoSnapshot()
	updated := append([]rune{}, before...)
	insertStart := len(updated)
	updated = append(updated, replacementRunes...)
	insertEnd := len(updated)
	newOffset := insertEnd
	placeholderStart := -1
	placeholderEnd := -1
	if back := selected.CursorBack; back > 0 {
		bodyLen := len(bodyRunes)
		if bodyLen > 0 {
			if back > bodyLen {
				back = bodyLen
			}
			cursorMin := insertStart
			target := insertEnd - back
			if target < cursorMin {
				target = cursorMin
			}
			newOffset = target
			placeholderStart = target
			placeholderEnd = insertEnd
		}
	}
	if needsSpace {
		updated = append(updated, ' ')
	}
	updated = append(updated, after...)
	newValue := string(updated)
	prevView := e.ViewStart()
	e.SetValue(newValue)
	e.SetViewStart(prevView)
	if placeholderStart >= 0 && placeholderEnd > placeholderStart {
		startLine, startCol := positionForOffset(newValue, placeholderStart)
		endLine, endCol := positionForOffset(newValue, placeholderEnd)
		startPos := cursorPosition{Line: startLine, Column: startCol, Offset: placeholderStart}
		endPos := cursorPosition{Line: endLine, Column: endCol, Offset: placeholderEnd}
		e.startSelection(endPos, selectionManual)
		e.selection.Update(startPos)
		newOffset = placeholderStart
	} else {
		e.clearSelection()
	}
	line, col := positionForOffset(newValue, newOffset)
	e.moveCursorTo(line, col)
	e.applySelectionHighlight()
	e.metadataHints.deactivate()
	return toEditorEventCmd(editorEvent{dirty: true})
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

	if consumed, hintCmd := e.handleMetadataHintNavigation(keyMsg); consumed {
		if hintCmd != nil {
			cmds = append(cmds, hintCmd)
		}
		return e, tea.Batch(cmds...)
	}

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
		e.metadataHints.deactivate()
	case "ctrl+c":
		if text := e.selectedText(); text != "" {
			cmds = append(cmds, (&e).copyToClipboard(text, ""))
		}
		handled = true
	case "ctrl+x":
		if text := e.selectedText(); text != "" {
			cmds = append(cmds, (&e).copyToClipboard(text, ""))
			if _, removed := (&e).removeSelection(); removed {
				cmds = append(cmds, toEditorEventCmd(editorEvent{dirty: true}))
			}
		}
		handled = true
	case " ":
		if e.motionsEnabled && !e.metadataHints.active {
			handled = true
		}
	case "ctrl+v":
		if e.hasSelection() {
			if _, removed := (&e).removeSelection(); removed {
				cmds = append(cmds, toEditorEventCmd(editorEvent{dirty: true}))
			}
		}
	case "backspace", "ctrl+h", "delete":
		if e.hasSelection() {
			if _, removed := (&e).removeSelection(); removed {
				cmds = append(cmds, toEditorEventCmd(editorEvent{dirty: true}))
			}
			handled = true
		}
	case "gg":
		if !e.motionsEnabled {
			break
		}
		handled = true
		if !e.isVisualMode() {
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
		if !e.isVisualMode() {
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
		if !e.isVisualMode() {
			e.clearSelection()
		}
		move := func() {
			e.moveToLineStartNonBlank()
		}

		if cmd := e.executeMotion(move); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case "e", "E", "w", "W", "b", "B":
		if e.metadataHints.active {
			break
		}
		if !e.motionsEnabled {
			break
		}
		handled = true
		if !e.isVisualMode() {
			e.clearSelection()
		}
		key := keyMsg.String()
		big := key == "E" || key == "W" || key == "B"
		var move func()
		switch key {
		case "e", "E":
			move = func() { e.moveToWordEnd(big) }
		case "w", "W":
			move = func() { e.moveToWordNext(big) }
		case "b", "B":
			move = func() { e.moveToWordStart(big) }
		}
		if cmd := e.executeMotion(move); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case "ctrl+f":
		if !e.motionsEnabled {
			break
		}
		handled = true
		if !e.isVisualMode() {
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
		if !e.isVisualMode() {
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
		if !e.isVisualMode() {
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
		if !e.isVisualMode() {
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
			if !e.isVisualMode() {
				e.clearSelection()
			}
		} else if insertsText(keyMsg) && e.hasSelection() {
			if removedText, removed := (&e).removeSelection(); removed {
				if removedText != "" {
					status := (&e).writeClipboardWithFallback(removedText, "")

					if status.level == statusWarn {
						warnCmd := toEditorEventCmd(editorEvent{status: &status})
						cmds = append(cmds, warnCmd)
					}
				}

				dirtyCmd := toEditorEventCmd(editorEvent{dirty: true})
				cmds = append(cmds, dirtyCmd)
			}
		}
	}

	autoChange := shouldRecordUndo(transformed)

	if !handled {
		if autoChange {
			e.pushUndoSnapshotAuto()
		}
		var innerCmd tea.Cmd
		e.Model, innerCmd = e.Model.Update(transformed)
		if innerCmd != nil {
			cmds = append(cmds, innerCmd)
		}
	}

	after := e.caretPosition()
	e.refreshMetadataHints()
	if transformed.String() != keyMsg.String() && !e.hasSelection() {
		e.clearSelection()
	}

	e.selection.Update(after)
	if e.isVisualMode() {
		e.selection.active = true
		e.selection.caret = after
	} else if !e.selection.IsActive() {
		e.mode = selectionNone
	}
	e.applySelectionHighlight()

	if !autoChange {
		selectionChanged := prevSelection.active != e.selection.active ||
			prevSelection.anchor != e.selection.anchor ||
			prevSelection.caret != e.selection.caret

		modeChanged := prevMode != e.mode
		cursorMoved := before != after
		if handled || selectionChanged || modeChanged || cursorMoved {
			e.undoCoalescing = false
		}
	}

	selectionActive := e.isVisualMode() ||
		(e.mode != selectionNone && e.selection.IsActive())
	prevActive := prevMode == selectionVisual ||
		prevMode == selectionVisualLine ||
		(prevMode != selectionNone && prevSelection.IsActive())

	if selectionActive {
		if prevSelection.Caret() != e.selection.Caret() ||
			!prevActive || prevMode != e.mode {
			start, end := e.selectionSummaryRange()
			summary := fmt.Sprintf(
				"Selection L%d:%d–L%d:%d",
				start.Line+1,
				start.Column+1,
				end.Line+1,
				end.Column+1,
			)

			cmds = append(cmds, statusCmd(statusInfo, summary))
		}
	} else if prevActive {
		cmds = append(cmds, statusCmd(statusInfo, "Selection cleared"))
	}

	return e, tea.Batch(cmds...)
}

func (e *requestEditor) ClearSelection() {
	e.clearSelection()
}

func (e requestEditor) ToggleVisual() (requestEditor, tea.Cmd) {
	if e.mode == selectionVisual {
		e.clearSelection()
		return e, nil
	}

	e.startSelection(e.caretPosition(), selectionVisual)
	return e, nil
}

func (e requestEditor) ToggleVisualLine() (requestEditor, tea.Cmd) {
	if e.mode == selectionVisualLine {
		e.clearSelection()
		return e, nil
	}

	e.startSelection(e.caretPosition(), selectionVisualLine)
	e.selection.active = true
	e.applySelectionHighlight()
	return e, nil
}

func (e requestEditor) YankSelection() (requestEditor, tea.Cmd) {
	text := e.selectedText()
	if text == "" {
		return e, statusCmd(statusWarn, "No selection to yank")
	}

	cmd := (&e).copyToClipboard(text, "")
	e.clearSelection()
	return e, cmd
}

func (e requestEditor) DeleteSelection() (requestEditor, tea.Cmd) {
	text := e.selectedText()
	if text == "" {
		return e, statusCmd(statusWarn, "No selection to delete")
	}

	if _, removed := (&e).removeSelection(); removed {
		status := (&e).writeClipboardWithFallback(text, "Deleted selection")
		if status.level == statusInfo && status.text == "Deleted selection" {
			status.text = "Selection deleted"
		}
		return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
	}
	return e, nil
}

func (e requestEditor) linewiseSpan(startLine, endLine int) (motionSpan, bool) {
	if startLine > endLine {
		startLine, endLine = endLine, startLine
	}
	lineStart, _, startIdx := e.lineBounds(startLine)
	_, lineEnd, endIdx := e.lineBounds(endLine)
	startOffset := e.clampOffset(lineStart)
	endOffset := e.clampOffset(lineEnd)
	if startOffset >= endOffset {
		return motionSpan{}, false
	}
	return motionSpan{
		startLine: startIdx,
		endLine:   endIdx,
		start:     startOffset,
		end:       endOffset,
		linewise:  true,
	}, true
}

func (e requestEditor) charwiseSpan(
	startOffset int,
	endOffset int,
	spec deleteMotionSpec,
	includeSpaceAfterWord bool,
	runes []rune,
) (motionSpan, bool) {
	if startOffset == endOffset {
		return motionSpan{}, false
	}
	if startOffset < endOffset {
		if spec.includeFinalForward {
			endOffset = nextRuneOffset(runes, endOffset)
		}
		// Delete uses Vim-like word motion that also consumes trailing spaces.
		if includeSpaceAfterWord && (spec.command == "w" || spec.command == "W") {
			for endOffset < len(runes) {
				r := runes[endOffset]
				if r == '\n' || !unicode.IsSpace(r) {
					break
				}
				endOffset++
			}
		}
	} else {
		startOffset, endOffset = endOffset, startOffset
	}

	startOffset = e.clampOffset(startOffset)
	endOffset = e.clampOffset(endOffset)
	if startOffset >= endOffset {
		return motionSpan{}, false
	}
	return motionSpan{start: startOffset, end: endOffset}, true
}

func (e requestEditor) motionSpan(
	anchor cursorPosition,
	target cursorPosition,
	spec deleteMotionSpec,
	includeSpaceAfterWord bool,
) (motionSpan, bool) {
	if spec.linewise {
		return e.linewiseSpan(anchor.Line, target.Line)
	}
	runes := []rune(e.Value())
	return e.charwiseSpan(
		anchor.Offset,
		target.Offset,
		spec,
		includeSpaceAfterWord,
		runes,
	)
}

func (e requestEditor) DeleteMotion(
	anchor cursorPosition,
	spec deleteMotionSpec,
) (requestEditor, tea.Cmd) {
	target := e.caretPosition()
	span, ok := e.motionSpan(anchor, target, spec, true)
	if !ok {
		return e, statusCmd(statusWarn, "Nothing to delete")
	}

	editorPtr := &e
	removed, ok := editorPtr.deleteRange(span.start, span.end)
	if !ok {
		return e, statusCmd(statusWarn, "Nothing to delete")
	}

	editorPtr.pendingMotion = ""
	summary := "Deleted text"
	if span.linewise {
		summary = "Deleted lines"
	}

	status := editorPtr.writeClipboardWithFallback(removed, summary)
	return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
}

func (e *requestEditor) changeLines(startLine, endLine int) (string, bool) {
	value := e.Value()
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		return "", false
	}
	if startLine > endLine {
		startLine, endLine = endLine, startLine
	}

	startOffset, _, startIdx := e.lineBounds(startLine)
	_, endOffset, endIdx := e.lineBounds(endLine)
	startLine = startIdx
	endLine = endIdx

	startOffset = e.clampOffset(startOffset)
	endOffset = e.clampOffset(endOffset)
	if startOffset >= endOffset {
		return "", false
	}

	runes := []rune(value)
	removed := string(runes[startOffset:endOffset])

	prevView := e.ViewStart()
	e.pushUndoSnapshot()

	if startLine < 0 {
		startLine = 0
	}
	if startLine >= len(lines) {
		startLine = len(lines) - 1
	}
	if endLine < startLine {
		endLine = startLine
	}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}

	// Replace the changed line range with a single blank line for insert.
	lines = append(lines[:startLine], append([]string{""}, lines[endLine+1:]...)...)
	newValue := strings.Join(lines, "\n")

	e.SetValue(newValue)
	e.SetViewStart(prevView)
	e.clearSelection()
	targetLine := startLine
	if targetLine >= len(lines) {
		targetLine = len(lines) - 1
	}
	if targetLine < 0 {
		targetLine = 0
	}
	e.moveCursorTo(targetLine, 0)
	e.applySelectionHighlight()
	return removed, true
}

func (e requestEditor) ChangeMotion(
	anchor cursorPosition,
	spec deleteMotionSpec,
) (requestEditor, tea.Cmd) {
	target := e.caretPosition()
	span, ok := e.motionSpan(anchor, target, spec, false)
	if !ok {
		return e, statusCmd(statusWarn, "Nothing to change")
	}

	editorPtr := &e
	var removed string
	if span.linewise {
		// Linewise change collapses the range into a single blank line.
		removed, ok = editorPtr.changeLines(span.startLine, span.endLine)
	} else {
		removed, ok = editorPtr.deleteRange(span.start, span.end)
	}
	if !ok {
		return e, statusCmd(statusWarn, "Nothing to change")
	}

	editorPtr.pendingMotion = ""
	summary := "Changed text"
	if span.linewise {
		summary = "Changed lines"
	}

	status := editorPtr.writeClipboardWithFallback(removed, summary)
	return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
}

func classifyMotion(keys []string, action string) (deleteMotionSpec, error) {
	if len(keys) == 0 {
		return deleteMotionSpec{}, fmt.Errorf("%s motion requires a target", action)
	}

	first := keys[0]
	spec := deleteMotionSpec{command: first}

	switch first {
	case "g":
		if len(keys) != 2 {
			return deleteMotionSpec{}, fmt.Errorf(
				"unsupported %s motion sequence %v",
				action,
				keys,
			)
		}
		switch keys[1] {
		case "g":
			spec.command = "gg"
			spec.linewise = true
			return spec, nil
		default:
			return deleteMotionSpec{}, fmt.Errorf(
				"unsupported %s motion sequence g%v",
				action,
				keys[1:],
			)
		}
	case "w", "W":
		return spec, nil
	case "e", "E":
		spec.includeFinalForward = true
		return spec, nil
	case "b", "B":
		return spec, nil
	case "j", "k", "G":
		spec.linewise = true
		return spec, nil
	case "$":
		return spec, nil
	case "f", "F":
		if len(keys) < 2 {
			return deleteMotionSpec{}, fmt.Errorf("find motion requires a target")
		}
		spec.includeFinalForward = true
		return spec, nil
	case "t", "T":
		if len(keys) < 2 {
			return deleteMotionSpec{}, fmt.Errorf("till motion requires a target")
		}
		spec.includeFinalForward = true
		return spec, nil
	default:
		if len(keys) > 1 {
			return deleteMotionSpec{}, fmt.Errorf(
				"unsupported %s motion sequence %v",
				action,
				keys,
			)
		}
	}

	return deleteMotionSpec{}, fmt.Errorf("unsupported %s motion %q", action, first)
}

func classifyDeleteMotion(keys []string) (deleteMotionSpec, error) {
	return classifyMotion(keys, "delete")
}

func mapChangeMotionKey(key string) string {
	switch key {
	case "w":
		// Vim cw behaves like ce: change to end of word without trailing spaces.
		return "e"
	case "W":
		// Same for WORD motion in Vim.
		return "E"
	default:
		return key
	}
}

func mapChangeMotionKeys(keys []string) []string {
	if len(keys) == 0 {
		return keys
	}
	// Avoid mutating the original motion slice when mapping cw/cW.
	mapped := make([]string, len(keys))
	copy(mapped, keys)
	mapped[0] = mapChangeMotionKey(mapped[0])
	return mapped
}

func classifyChangeMotion(keys []string) (deleteMotionSpec, error) {
	return classifyMotion(mapChangeMotionKeys(keys), "change")
}

func (e requestEditor) DeleteCurrentLine() (requestEditor, tea.Cmd) {
	if e.LineCount() == 0 {
		return e, statusCmd(statusWarn, "Nothing to delete")
	}

	prevView := e.ViewStart()
	value := e.Value()
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}

	line := e.Line()
	if line < 0 {
		line = 0
	}
	if line >= len(lines) {
		line = len(lines) - 1
	}

	e.pushUndoSnapshot()
	segment := lines[line]
	clip := segment + "\n"
	lines = append(lines[:line], lines[line+1:]...)
	var newValue string
	if len(lines) > 0 {
		newValue = strings.Join(lines, "\n")
	} else {
		newValue = ""
	}

	e.SetValue(newValue)
	e.SetViewStart(prevView)
	e.clearSelection()
	lineCount := e.LineCount()
	if lineCount <= 0 {
		lineCount = 1
	}

	target := line
	if target >= lineCount {
		target = lineCount - 1
	}
	if target < 0 {
		target = 0
	}

	editorPtr := &e
	editorPtr.moveCursorTo(target, 0)
	e.applySelectionHighlight()
	status := editorPtr.writeClipboardWithFallback(clip, "Deleted line")
	return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
}

func (e requestEditor) DeleteToLineEnd() (requestEditor, tea.Cmd) {
	prevView := e.ViewStart()
	cursor := e.caretPosition()
	start := cursor.Offset
	value := e.Value()
	runes := []rune(value)
	if start >= len(runes) {
		return e, statusCmd(statusWarn, "Nothing to delete")
	}

	lineLen := lineLength(value, cursor.Line)
	end := e.offsetForPosition(cursor.Line, lineLen)
	if end < start {
		end = start
	}
	if end <= start {
		if start < len(runes) && runes[start] == '\n' {
			end = start + 1
		}
	}
	if end > len(runes) {
		end = len(runes)
	}

	segment := string(runes[start:end])
	if segment == "" {
		return e, statusCmd(statusWarn, "Nothing to delete")
	}

	e.pushUndoSnapshot()
	runes = append(runes[:start], runes[end:]...)
	e.SetValue(string(runes))
	e.SetViewStart(prevView)
	e.clearSelection()
	editorPtr := &e
	editorPtr.moveCursorTo(cursor.Line, cursor.Column)
	e.applySelectionHighlight()
	status := (&e).writeClipboardWithFallback(segment, "Deleted to end of line")
	return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
}

func (e requestEditor) DeleteCharAtCursor() (requestEditor, tea.Cmd) {
	if e.hasSelection() {
		removed, ok := (&e).removeSelection()
		if !ok {
			return e, statusCmd(statusWarn, "Nothing to delete")
		}
		status := statusMsg{}
		if removed != "" {
			status = (&e).writeClipboardWithFallback(removed, "Deleted selection")
		} else {
			status = statusMsg{text: "Deleted selection", level: statusInfo}
		}
		return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
	}

	runes := []rune(e.Value())
	cursor := e.caretPosition()
	if cursor.Offset >= len(runes) {
		return e, statusCmd(statusWarn, "Nothing to delete")
	}

	prevView := e.ViewStart()
	e.pushUndoSnapshot()
	removed := runes[cursor.Offset]
	runes = append(runes[:cursor.Offset], runes[cursor.Offset+1:]...)

	e.SetValue(string(runes))
	e.SetViewStart(prevView)
	e.clearSelection()

	editorPtr := &e
	editorPtr.moveCursorTo(cursor.Line, cursor.Column)
	e.applySelectionHighlight()
	deletedChar := string([]rune{removed})
	status := (&e).writeClipboardWithFallback(deletedChar, "Deleted character")
	return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
}

func (e requestEditor) ChangeCurrentLine() (requestEditor, tea.Cmd) {
	if e.LineCount() == 0 {
		return e, statusCmd(statusWarn, "Nothing to change")
	}

	editorPtr := &e
	removed, ok := editorPtr.changeLines(e.Line(), e.Line())
	if !ok {
		return e, statusCmd(statusWarn, "Nothing to change")
	}
	status := editorPtr.writeClipboardWithFallback(removed, "Changed line")
	return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
}

func (e requestEditor) PasteClipboard(after bool) (requestEditor, tea.Cmd) {
	editorPtr := &e
	text, source, ok, failure := editorPtr.resolvePasteBuffer()
	if !ok {
		if failure != nil {
			return e, toEditorEventCmd(editorEvent{status: failure})
		}
		return e, nil
	}

	cursor := e.caretPosition()
	prevView := e.ViewStart()
	runes := []rune(e.Value())
	insert := []rune(text)
	index := cursor.Offset
	if index < 0 {
		index = 0
	}
	if index > len(runes) {
		index = len(runes)
	}

	insertPos := index
	linewise := strings.HasSuffix(text, "\n")
	if after {
		if linewise {
			if cursor.Line+1 >= e.LineCount() {
				insertPos = len(runes)
			} else {
				insertPos = e.offsetForPosition(cursor.Line+1, 0)
			}
		} else {
			if index < len(runes) {
				insertPos = index + 1
			} else {
				insertPos = len(runes)
			}
		}
	} else if linewise {
		insertPos = e.offsetForPosition(cursor.Line, 0)
	}

	if insertPos < 0 {
		insertPos = 0
	}
	if insertPos > len(runes) {
		insertPos = len(runes)
	}

	e.pushUndoSnapshot()
	runes = append(runes[:insertPos], append(insert, runes[insertPos:]...)...)
	newValue := string(runes)
	e.SetValue(newValue)
	e.SetViewStart(prevView)
	e.clearSelection()

	insertLen := len(insert)
	insertStart := insertPos
	insertEnd := insertPos + insertLen
	targetLine := 0
	targetCol := 0
	if linewise {
		targetLine, _ = positionForOffset(newValue, insertStart)
		lines := strings.Split(newValue, "\n")
		if targetLine >= 0 && targetLine < len(lines) {
			targetCol = firstNonWhitespaceColumn(lines[targetLine])
		}
	} else {
		destOffset := insertStart
		if insertLen > 0 {
			destOffset = insertEnd - 1
		}
		destOffset = editorPtr.clampOffset(destOffset)
		targetLine, targetCol = positionForOffset(newValue, destOffset)
	}

	editorPtr.moveCursorTo(targetLine, targetCol)
	e.applySelectionHighlight()
	status := statusMsg{
		level: statusInfo,
		text:  "Pasted",
	}
	switch source {
	case pasteSourceRegisterEmpty:
		status.text = "Pasted from editor register"
	case pasteSourceRegisterError:
		status = statusMsg{
			level: statusWarn,
			text:  "Clipboard unavailable; pasted from editor register",
		}
	}
	return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
}

func (e requestEditor) UndoLastChange() (requestEditor, tea.Cmd) {
	if len(e.undoStack) == 0 {
		e.undoCoalescing = false
		return e, statusCmd(statusInfo, "Nothing to undo")
	}

	current := e.captureSnapshot()
	last := e.undoStack[len(e.undoStack)-1]
	e.undoStack = e.undoStack[:len(e.undoStack)-1]
	e.redoStack = appendSnapshot(e.redoStack, current)
	e.restoreSnapshot(last)
	status := statusMsg{
		level: statusInfo,
		text:  "Undid last change",
	}
	return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
}

func (e requestEditor) RedoLastChange() (requestEditor, tea.Cmd) {
	if len(e.redoStack) == 0 {
		e.undoCoalescing = false
		return e, statusCmd(statusInfo, "Nothing to redo")
	}

	current := e.captureSnapshot()
	next := e.redoStack[len(e.redoStack)-1]
	e.redoStack = e.redoStack[:len(e.redoStack)-1]
	e.undoStack = appendSnapshot(e.undoStack, current)
	e.restoreSnapshot(next)
	status := statusMsg{
		level: statusInfo,
		text:  "Redid last change",
	}
	return e, toEditorEventCmd(editorEvent{dirty: true, status: &status})
}

func (e requestEditor) lineBounds(requested int) (start int, end int, idx int) {
	value := e.Value()
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		return 0, 0, 0
	}

	idx = requested
	if idx < 0 {
		idx = 0
	}
	if idx >= len(lines) {
		idx = len(lines) - 1
	}

	start = e.offsetForPosition(idx, 0)
	runes := []rune(value)
	if idx == len(lines)-1 {
		end = len(runes)
	} else {
		end = e.offsetForPosition(idx+1, 0)
	}
	return start, end, idx
}

func (e requestEditor) executeFindMotion(
	kind string,
	target rune,
) (requestEditor, tea.Cmd) {
	forward := kind == "f" || kind == "t"
	till := kind == "t" || kind == "T"
	cursor := e.caretPosition()
	lines := strings.Split(e.Value(), "\n")
	if cursor.Line < 0 || cursor.Line >= len(lines) {
		return e, nil
	}

	row := []rune(lines[cursor.Line])
	if len(row) == 0 {
		return e, statusCmd(statusWarn, "Line is empty")
	}

	index := -1
	if forward {
		start := cursor.Column + 1
		if start < 0 {
			start = 0
		}
		for i := start; i < len(row); i++ {
			if row[i] == target {
				index = i
				break
			}
		}
		if index >= 0 && till {
			index--
		}
	} else {
		start := cursor.Column - 1
		if start >= len(row) {
			start = len(row) - 1
		}
		for i := start; i >= 0; i-- {
			if row[i] == target {
				index = i
				break
			}
		}
		if index >= 0 && till {
			index++
		}
	}
	if index < 0 {
		msg := fmt.Sprintf("%q not found", string(target))
		return e, statusCmd(statusWarn, msg)
	}
	if index < 0 {
		index = 0
	}
	if index > len(row) {
		index = len(row)
	}

	editorPtr := &e
	editorPtr.moveCursorTo(cursor.Line, index)
	e.applySelectionHighlight()
	return e, nil
}

func decodeFindTarget(command string) (rune, bool) {
	switch command {
	case "":
		return 0, false
	case "space":
		return ' ', true
	case "tab":
		return '\t', true
	}

	runes := []rune(command)
	if len(runes) != 1 {
		return 0, false
	}
	return runes[0], true
}

func (e requestEditor) HandleMotion(
	command string,
) (requestEditor, tea.Cmd, bool) {
	if !e.motionsEnabled {
		return e, nil, false
	}
	switch e.pendingMotion {
	case "g":
		if command == "g" {
			e.pendingMotion = ""
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g', 'g'}}
			updated, cmd := e.Update(msg)
			updated.pendingMotion = ""
			return updated, cmd, true
		}
		e.pendingMotion = ""
	case "f", "t", "T":
		pending := e.pendingMotion
		e.pendingMotion = ""
		if command == "esc" || command == "ctrl+c" || command == "ctrl+g" {
			return e, nil, true
		}
		if target, ok := decodeFindTarget(command); ok {
			updated, cmd := e.executeFindMotion(pending, target)
			return updated, cmd, true
		}
		cmd := statusCmd(statusWarn, "Find target must be a single character")
		return e, cmd, true
	}

	switch command {
	case "g":
		e.pendingMotion = "g"
		return e, nil, true
	case "f", "t", "T":
		e.pendingMotion = command
		return e, nil, true
	case "G", "^", "e", "E", "w", "W", "b", "B", "ctrl+f", "ctrl+b", "ctrl+d", "ctrl+u":
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

func (e requestEditor) ApplySearch(
	query string,
	isRegex bool,
) (requestEditor, tea.Cmd) {
	trimmed := strings.TrimSpace(query)
	pe := &e
	pe.search.query = trimmed
	pe.search.isRegex = isRegex
	pe.search.matches = nil
	pe.search.index = -1
	pe.search.active = false

	if trimmed == "" {
		pe.applySelectionHighlight()
		return e, statusCmd(statusWarn, "Enter a search pattern")
	}

	matches, err := pe.buildSearchMatches(trimmed, isRegex)
	if err != nil {
		pe.applySelectionHighlight()
		msg := fmt.Sprintf("Invalid regex: %v", err)
		return e, statusCmd(statusError, msg)
	}

	pe.search.matches = matches
	if len(matches) == 0 {
		pe.applySelectionHighlight()
		msg := fmt.Sprintf("No matches for %q", trimmed)
		return e, statusCmd(statusWarn, msg)
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

	status := statusCmd(statusInfo, statusText)
	if moveCmd != nil {
		return e, tea.Batch(moveCmd, status)
	}
	return e, status
}

func (e requestEditor) NextSearchMatch() (requestEditor, tea.Cmd) {
	pe := &e
	trimmed := strings.TrimSpace(pe.search.query)
	if trimmed == "" {
		return e, statusCmd(statusWarn, "No active search")
	}

	if len(pe.search.matches) == 0 {
		matches, err := pe.buildSearchMatches(trimmed, pe.search.isRegex)
		if err != nil {
			msg := fmt.Sprintf("Invalid regex: %v", err)
			return e, statusCmd(statusError, msg)
		}
		pe.search.matches = matches
		pe.search.index = -1
		if len(matches) == 0 {
			msg := fmt.Sprintf("No matches for %q", trimmed)
			pe.applySelectionHighlight()
			return e, statusCmd(statusWarn, msg)
		}
	}

	if pe.search.index < 0 || pe.search.index >= len(pe.search.matches) {
		offset := pe.caretPosition().Offset
		index, wrapped := firstMatchIndex(pe.search.matches, offset)
		moveCmd := pe.jumpToSearchIndex(index)
		statusText := fmt.Sprintf(
			"Match %d/%d for %q",
			index+1,
			len(pe.search.matches),
			trimmed,
		)
		if wrapped {
			statusText += " (wrapped)"
		}

		status := statusCmd(statusInfo, statusText)
		if moveCmd != nil {
			return e, tea.Batch(moveCmd, status)
		}
		return e, status
	}

	nextIndex := pe.search.index + 1
	wrapped := false
	if nextIndex >= len(pe.search.matches) {
		nextIndex = 0
		wrapped = true
	}

	moveCmd := pe.jumpToSearchIndex(nextIndex)
	statusText := fmt.Sprintf(
		"Match %d/%d for %q",
		nextIndex+1,
		len(pe.search.matches),
		trimmed,
	)
	if wrapped {
		statusText += " (wrapped)"
	}

	status := statusCmd(statusInfo, statusText)
	if moveCmd != nil {
		return e, tea.Batch(moveCmd, status)
	}
	return e, status
}

func (e requestEditor) PrevSearchMatch() (requestEditor, tea.Cmd) {
	pe := &e
	trimmed := strings.TrimSpace(pe.search.query)
	if trimmed == "" {
		return e, statusCmd(statusWarn, "No active search")
	}

	if len(pe.search.matches) == 0 {
		matches, err := pe.buildSearchMatches(trimmed, pe.search.isRegex)
		if err != nil {
			msg := fmt.Sprintf("Invalid regex: %v", err)
			return e, statusCmd(statusError, msg)
		}
		pe.search.matches = matches
		pe.search.index = -1
		if len(matches) == 0 {
			msg := fmt.Sprintf("No matches for %q", trimmed)
			pe.applySelectionHighlight()
			return e, statusCmd(statusWarn, msg)
		}
	}

	if pe.search.index < 0 || pe.search.index >= len(pe.search.matches) {
		offset := pe.caretPosition().Offset
		index, wrapped := lastMatchIndex(pe.search.matches, offset)
		moveCmd := pe.jumpToSearchIndex(index)
		statusText := fmt.Sprintf(
			"Match %d/%d for %q",
			index+1,
			len(pe.search.matches),
			trimmed,
		)
		if wrapped {
			statusText += " (wrapped)"
		}

		status := statusCmd(statusInfo, statusText)
		if moveCmd != nil {
			return e, tea.Batch(moveCmd, status)
		}
		return e, status
	}

	prevIndex := pe.search.index - 1
	wrapped := false
	if prevIndex < 0 {
		prevIndex = len(pe.search.matches) - 1
		wrapped = true
	}

	moveCmd := pe.jumpToSearchIndex(prevIndex)
	statusText := fmt.Sprintf(
		"Match %d/%d for %q",
		prevIndex+1,
		len(pe.search.matches),
		trimmed,
	)
	if wrapped {
		statusText += " (wrapped)"
	}

	status := statusCmd(statusInfo, statusText)
	if moveCmd != nil {
		return e, tea.Batch(moveCmd, status)
	}
	return e, status
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

	startOffset, endOffset, ok := e.selectionOffsets()
	if !ok || startOffset == endOffset {
		return ""
	}

	content := []rune(e.Value())
	if startOffset < 0 {
		startOffset = 0
	}
	if endOffset > len(content) {
		endOffset = len(content)
	}
	if startOffset >= endOffset {
		return ""
	}
	return string(content[startOffset:endOffset])
}

func (e *requestEditor) removeSelection() (string, bool) {
	if !e.hasSelection() {
		return "", false
	}

	startOffset, endOffset, ok := e.selectionOffsets()
	if !ok || startOffset == endOffset {
		e.clearSelection()
		return "", false
	}

	prevView := e.Model.ViewStart()

	e.pushUndoSnapshot()

	runes := []rune(e.Value())
	if startOffset < 0 {
		startOffset = 0
	}
	if endOffset > len(runes) {
		endOffset = len(runes)
	}
	if startOffset >= endOffset {
		e.clearSelection()
		return "", false
	}

	removed := string(runes[startOffset:endOffset])
	updated := append([]rune{}, runes[:startOffset]...)
	updated = append(updated, runes[endOffset:]...)
	newValue := string(updated)

	e.SetValue(newValue)
	e.clearSelection()

	line, col := positionForOffset(newValue, startOffset)
	if e.isVisualLineMode() {
		col = 0
	}
	e.moveCursorTo(line, col)
	e.applySelectionHighlight()
	e.Model.SetViewStart(prevView)
	return removed, true
}

func nextRuneOffset(runes []rune, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset >= len(runes) {
		return len(runes)
	}
	return offset + 1
}

func (e *requestEditor) deleteRange(startOffset, endOffset int) (string, bool) {
	runes := []rune(e.Value())
	if startOffset < 0 {
		startOffset = 0
	}

	if endOffset > len(runes) {
		endOffset = len(runes)
	}
	if startOffset >= endOffset {
		return "", false
	}

	prevView := e.ViewStart()
	e.pushUndoSnapshot()

	removed := string(runes[startOffset:endOffset])
	updated := append([]rune{}, runes[:startOffset]...)
	updated = append(updated, runes[endOffset:]...)
	newValue := string(updated)

	e.SetValue(newValue)
	e.SetViewStart(prevView)
	e.clearSelection()
	line, col := positionForOffset(newValue, startOffset)
	e.moveCursorTo(line, col)
	e.applySelectionHighlight()
	return removed, true
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

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func wordClass(r rune, big bool) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if big || isWordRune(r) {
		return 1
	}
	return 2
}

func segEnd(runes []rune, idx int, big bool) int {
	cls := wordClass(runes[idx], big)
	for idx+1 < len(runes) && wordClass(runes[idx+1], big) == cls {
		idx++
	}
	return idx
}

func segStart(runes []rune, idx int, big bool) int {
	cls := wordClass(runes[idx], big)
	for idx-1 >= 0 && wordClass(runes[idx-1], big) == cls {
		idx--
	}
	return idx
}

func (e *requestEditor) moveToWordEnd(big bool) {
	value := e.Value()
	runes := []rune(value)
	if len(runes) == 0 {
		return
	}
	pos := e.caretPosition()
	idx := pos.Offset
	if idx < 0 {
		idx = 0
	}
	if idx >= len(runes) {
		idx = len(runes) - 1
	}

	if unicode.IsSpace(runes[idx]) {
		for idx < len(runes) && unicode.IsSpace(runes[idx]) {
			idx++
		}
		if idx >= len(runes) {
			return
		}
		idx = segEnd(runes, idx, big)
		line, col := positionForOffset(value, idx)
		e.moveCursorTo(line, col)
		return
	}

	cls := wordClass(runes[idx], big)
	if idx+1 < len(runes) && wordClass(runes[idx+1], big) == cls {
		idx = segEnd(runes, idx, big)
		line, col := positionForOffset(value, idx)
		e.moveCursorTo(line, col)
		return
	}

	idx++
	for idx < len(runes) && unicode.IsSpace(runes[idx]) {
		idx++
	}
	if idx >= len(runes) {
		return
	}
	idx = segEnd(runes, idx, big)
	line, col := positionForOffset(value, idx)
	e.moveCursorTo(line, col)
}

func (e *requestEditor) moveToWordNext(big bool) {
	value := e.Value()
	runes := []rune(value)
	if len(runes) == 0 {
		return
	}
	pos := e.caretPosition()
	idx := pos.Offset
	if idx < 0 {
		idx = 0
	}
	if idx >= len(runes) {
		return
	}
	if unicode.IsSpace(runes[idx]) {
		for idx < len(runes) && unicode.IsSpace(runes[idx]) {
			idx++
		}
	} else {
		idx = segEnd(runes, idx, big) + 1
		for idx < len(runes) && unicode.IsSpace(runes[idx]) {
			idx++
		}
	}
	line, col := positionForOffset(value, idx)
	e.moveCursorTo(line, col)
}

func (e *requestEditor) moveToWordStart(big bool) {
	value := e.Value()
	runes := []rune(value)
	if len(runes) == 0 {
		return
	}
	pos := e.caretPosition()
	idx := pos.Offset
	if idx < 0 {
		idx = 0
	}
	if idx >= len(runes) {
		idx = len(runes) - 1
	}

	if unicode.IsSpace(runes[idx]) {
		for idx >= 0 && unicode.IsSpace(runes[idx]) {
			idx--
		}
		if idx < 0 {
			return
		}
		idx = segStart(runes, idx, big)
		line, col := positionForOffset(value, idx)
		e.moveCursorTo(line, col)
		return
	}

	cls := wordClass(runes[idx], big)
	if idx-1 >= 0 && wordClass(runes[idx-1], big) == cls {
		idx = segStart(runes, idx, big)
		line, col := positionForOffset(value, idx)
		e.moveCursorTo(line, col)
		return
	}

	idx--
	for idx >= 0 && unicode.IsSpace(runes[idx]) {
		idx--
	}
	if idx < 0 {
		return
	}
	idx = segStart(runes, idx, big)
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

func lastMatchIndex(matches []searchMatch, offset int) (int, bool) {
	if len(matches) == 0 {
		return -1, false
	}
	for i := len(matches) - 1; i >= 0; i-- {
		match := matches[i]
		if offset > match.start {
			return i, false
		}
	}
	return len(matches) - 1, true
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

func (e requestEditor) buildSearchMatches(
	query string,
	isRegex bool,
) ([]searchMatch, error) {
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

func (e requestEditor) SearchActive() bool {
	if !e.search.active {
		return false
	}
	return strings.TrimSpace(e.search.query) != ""
}

func (e *requestEditor) ExitSearchMode() tea.Cmd {
	hadState := e.search.active || len(e.search.matches) > 0
	e.search.active = false
	e.search.matches = nil
	e.search.index = -1
	e.applySelectionHighlight()
	if !hadState {
		return nil
	}
	return statusCmd(statusInfo, "Search cleared")
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
	if len(msg.Runes) > 0 {
		return true
	}
	switch msg.String() {
	case "enter", "ctrl+m", "ctrl+j", "tab":
		return true
	default:
		return false
	}
}

func shouldRecordUndo(msg tea.KeyMsg) bool {
	if insertsText(msg) {
		return true
	}
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyDelete:
		return true
	}
	switch msg.String() {
	case "ctrl+v":
		return true
	default:
		return false
	}
}
