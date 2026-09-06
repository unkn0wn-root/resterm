package ui

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/intellisense"
)

type completionState struct {
	active        bool
	anchorOffset  int
	replaceEnd    int
	selection     int
	preview       bool
	popupLabelW   int
	popupSummaryW int
	filtered      []intellisense.Item
	ctx           intellisense.Context
}

func (s *completionState) deactivate() {
	*s = completionState{}
}

func (s *completionState) update(
	anchor int,
	filtered []intellisense.Item,
	ctx intellisense.Context,
) {
	if len(filtered) == 0 {
		s.deactivate()
		return
	}
	reset := !s.active || s.anchorOffset != anchor
	if reset || s.selection >= len(filtered) {
		s.selection = 0
	}
	s.active = true
	s.anchorOffset = anchor
	s.replaceEnd = anchor + max(ctx.End-ctx.Start, 0)
	s.filtered = filtered
	s.ctx = ctx
	labelW, summaryW := completionPopupPreference(filtered)
	if reset {
		s.popupLabelW = labelW
		s.popupSummaryW = summaryW
		return
	}
	s.popupLabelW = max(s.popupLabelW, labelW)
	s.popupSummaryW = max(s.popupSummaryW, summaryW)
}

func (s *completionState) move(delta int) {
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

func (s *completionState) setPreview(open bool) {
	if !s.active || len(s.filtered) == 0 {
		s.preview = false
		return
	}
	s.preview = open
}

func (s *completionState) togglePreview() {
	s.setPreview(!s.preview)
}

func (s completionState) display(limit int) (items []intellisense.Item, selected int, ok bool) {
	if !s.active || len(s.filtered) == 0 || limit <= 0 {
		return nil, 0, false
	}
	start, end := popupWindow(s.selection, limit, len(s.filtered))
	return s.filtered[start:end], s.selection - start, true
}

func completionPopupPreference(items []intellisense.Item) (int, int) {
	labelW := 0
	summaryW := 0
	for _, item := range items {
		labelW = max(labelW, visibleWidth(item.Label))
		summaryW = max(summaryW, visibleWidth(item.Summary))
	}
	return labelW, summaryW
}

func (e *requestEditor) SetCompletionEnabled(enabled bool) {
	e.completionEnabled = enabled
	if !enabled {
		e.closeCompletions()
	}
}

// SetCompletionScope stores document and environment data for completion.
// Call it when either changes.
func (e *requestEditor) SetCompletionScope(scope intellisense.Scope) {
	e.scope = scope
}

func (e requestEditor) hasActiveCompletion() bool {
	return e.completion.active && len(e.completion.filtered) > 0
}

func (e *requestEditor) handleCompletionKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !e.hasActiveCompletion() {
		return false, nil
	}
	switch msg.String() {
	case "down", "ctrl+n":
		e.completion.move(1)
		return true, nil
	case "up", "ctrl+p", "shift+tab":
		e.completion.move(-1)
		return true, nil
	case "right":
		e.completion.setPreview(true)
		return true, nil
	case "left":
		if e.completion.preview {
			e.completion.setPreview(false)
			return true, nil
		}
		return false, nil
	case "ctrl+l", "?", "shift+/":
		e.completion.togglePreview()
		return true, nil
	case "esc":
		if e.completion.preview {
			e.completion.setPreview(false)
			return true, nil
		}
		return false, nil
	case "tab", "enter", "ctrl+m":
		cmd := e.applyCompletion()
		return true, cmd
	default:
		return false, nil
	}
}

func (e *requestEditor) dismissCompletion() bool {
	if !e.completion.active {
		return false
	}
	if e.completion.preview {
		e.completion.setPreview(false)
		return true
	}
	e.closeCompletions()
	return true
}

func (e *requestEditor) updateCompletions(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter", "ctrl+m", "esc":
		e.closeCompletions()
		return nil
	case " ":
		// A space ends most tokens, but inside a path it is ordinary text.
		if ctx, line, ok := e.completionAt(); ok && ctx.Kind == intellisense.KindPath {
			return e.showCompletions(ctx, line)
		}
		e.closeCompletions()
		return nil
	case "backspace", "ctrl+h", "delete":
		if !e.completion.active && !e.editingPath() {
			return nil
		}
		return e.refreshCompletions()
	default:
		return e.refreshCompletions()
	}
}

func (e *requestEditor) refreshCompletions() tea.Cmd {
	ctx, line, ok := e.completionAt()
	if !ok {
		e.closeCompletions()
		return nil
	}
	// Typing inside a token does not open the popup, but an open popup follows the edit.
	if !e.completion.active && e.caretInsideToken() {
		return nil
	}
	return e.showCompletions(ctx, line)
}

func (e *requestEditor) openCompletions() tea.Cmd {
	ctx, line, ok := e.completionAt()
	if !ok {
		e.closeCompletions()
		return nil
	}
	return e.showCompletions(ctx, line)
}

func (e *requestEditor) completionAt() (intellisense.Context, int, bool) {
	if !e.completionEnabled {
		return intellisense.Context{}, 0, false
	}
	line := e.Line()
	ctx, ok := intellisense.Analyze(e, line, e.caretColumn())
	return ctx, line, ok
}

func (e *requestEditor) editingPath() bool {
	ctx, _, ok := e.completionAt()
	return ok && ctx.Kind == intellisense.KindPath
}

func (e *requestEditor) caretColumn() int {
	info := e.LineInfo()
	return min(max(info.StartColumn+info.ColumnOffset, 0), len(e.LineRunes(e.Line())))
}

func (e *requestEditor) caretInsideToken() bool {
	cur := e.LineRunes(e.Line())
	col := e.caretColumn()
	return col < len(cur) && intellisense.IsTokenRune(cur[col])
}

func (e *requestEditor) showCompletions(ctx intellisense.Context, line int) tea.Cmd {
	if ctx.Kind == intellisense.KindPath {
		return e.showPathCompletions(ctx, line)
	}
	e.paths.reset()
	items := e.engine.Suggest(ctx, e.scope)
	if len(items) == 0 {
		e.completion.deactivate()
		return nil
	}
	e.completion.update(e.offsetForPosition(line, ctx.Start), items, ctx)
	return nil
}

func (e *requestEditor) closeCompletions() {
	e.completion.deactivate()
	e.paths.reset()
}

// NextCompletion selects the next item or opens suggestions at the caret.
func (e requestEditor) NextCompletion() (requestEditor, tea.Cmd) {
	if e.hasActiveCompletion() {
		e.completion.move(1)
		return e, nil
	}
	return e, e.openCompletions()
}

func (e *requestEditor) applyCompletion() tea.Cmd {
	if !e.hasActiveCompletion() {
		return nil
	}
	selected := e.completion.filtered[e.completion.selection]
	start := e.completion.anchorOffset
	end := e.completion.replaceEnd
	caret := e.caretPosition()
	runes := []rune(e.Value())
	if start < 0 || end < start || end > len(runes) || caret.Offset < start {
		e.closeCompletions()
		return nil
	}
	text := []rune(selected.InsertText())
	if selected.AppendsSpace(e.completion.ctx.Kind) {
		if end < len(runes) && runes[end] == ' ' {
			end++
		}
		text = append(text, ' ')
	}
	after := runes[end:]
	e.pushUndoSnapshot()

	updated := slices.Concat(runes[:start], text)
	exit := len(updated)
	updated = append(updated, after...)

	prevView := e.ViewStart()
	if value := string(updated); value != e.Value() {
		e.storeValue(value) // Keep the current path session.
	}
	e.SetViewStart(prevView)

	caretOffset := exit - selected.CursorBack
	if from, to, ok := selected.PlaceholderRange(); ok {
		e.startPlaceholder(start+from, start+to, exit)
		caretOffset = start + from
	} else {
		e.clearSelection()
	}
	line, col := e.positionForOffset(caretOffset)
	e.moveCursorTo(line, col)
	e.applySelectionHighlight()
	if selected.Placeholder == "" && selected.Continue {
		e.completion.deactivate()
		return e.openCompletions()
	}
	e.closeCompletions()
	return nil
}
