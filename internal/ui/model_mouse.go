package ui

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
	"github.com/unkn0wn-root/resterm/internal/util"
)

const (
	mouseWheelLines       = 3
	mouseDoubleClickDelay = 500 * time.Millisecond
)

const (
	mouseAreaNavigator mouseArea = iota + 1
	mouseAreaHistory
	mouseAreaEditor
)

type mouseArea int

type mouseClickState struct {
	area   mouseArea
	target string
	x      int
	y      int
	at     time.Time
}

type mouseDragState struct {
	active   bool
	area     mouseArea
	outer    mouseRect
	anchor   cursorPosition
	lineMode bool
	startX   int
	startY   int
	dragged  bool
}

type editorMouseHit struct {
	pos  cursorPosition
	body mouseRect
	ok   bool
}

type tabRange struct {
	start int
	end   int
	next  int
	ok    bool
}

func (m *Model) handleMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	if !m.ready {
		return nil, false
	}
	if m.showHelp {
		return m.handleHelpMouse(msg)
	}
	if m.mouseModalActive() {
		return nil, false
	}

	if msg.Action == tea.MouseActionRelease {
		return m.handleMouseRelease(msg)
	}
	if msg.Action == tea.MouseActionMotion {
		return m.handleMouseMotion(msg)
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return m.handleMouseWheel(msg, -mouseWheelLines)
	case tea.MouseButtonWheelDown:
		return m.handleMouseWheel(msg, mouseWheelLines)
	case tea.MouseButtonLeft:
		if msg.Action == tea.MouseActionPress {
			return m.handleMouseClick(msg)
		}
	}
	return nil, false
}

func (m *Model) consumeMouseClick(area mouseArea, target string, x, y int) bool {
	now := time.Now()
	last := m.lastMouseClick
	m.lastMouseClick = mouseClickState{
		area:   area,
		target: target,
		x:      x,
		y:      y,
		at:     now,
	}

	if target == "" || last.target == "" {
		return false
	}
	if last.area != area || last.target != target {
		return false
	}
	if now.Sub(last.at) > mouseDoubleClickDelay {
		return false
	}

	dx := last.x - x
	dy := last.y - y
	return max(dx, -dx) <= 1 && max(dy, -dy) <= 1
}

func (m *Model) mouseModalActive() bool {
	return m.modalCapturesGlobalKeys() || m.showHelp || m.showSearchPrompt || m.showCommandLine
}

func (m *Model) handleMouseClick(msg tea.MouseMsg) (tea.Cmd, bool) {
	m.mouseDrag = mouseDragState{}
	ly := m.currentMouseLayout()
	x, y := msg.X, msg.Y

	if ly.file.Contains(x, y) {
		return m.handleNavigatorMouseClick(ly.file, x, y)
	}
	if ly.editor.Contains(x, y) {
		return m.handleEditorMouseClick(ly.editor, x, y)
	}
	if ly.response.Contains(x, y) {
		return m.handleResponseMouseClick(ly.response, x, y)
	}
	return nil, false
}

func (m *Model) handleMouseWheel(msg tea.MouseMsg, delta int) (tea.Cmd, bool) {
	m.mouseDrag = mouseDragState{}
	ly := m.currentMouseLayout()
	x, y := msg.X, msg.Y

	switch {
	case ly.file.Contains(x, y):
		return m.scrollNavigatorBy(mouseWheelRowDelta(delta)), true
	case ly.editor.Contains(x, y):
		return m.scrollEditorBy(delta), true
	case ly.response.Contains(x, y):
		hit := m.responseMouseHit(ly.response, x, y)
		if !hit.ok {
			return nil, true
		}
		m.focusResponsePane(hit.id)
		_ = m.setFocus(focusResponse)
		return m.scrollResponseBy(hit.id, delta), true
	default:
		return nil, false
	}
}

func mouseWheelRowDelta(delta int) int {
	switch {
	case delta > 0:
		return mouseWheelLines
	case delta < 0:
		return -mouseWheelLines
	default:
		return 0
	}
}

func (m *Model) handleHelpMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	delta := 0
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		delta = -mouseWheelLines
	case tea.MouseButtonWheelDown:
		delta = mouseWheelLines
	default:
		return nil, false
	}
	if msg.Action != tea.MouseActionPress {
		return nil, true
	}

	m.mouseDrag = mouseDragState{}
	_, body := m.helpOverlayBox()
	if !body.Contains(msg.X, msg.Y) {
		return nil, true
	}
	if vp := m.helpViewport; vp != nil {
		if delta > 0 {
			vp.ScrollDown(delta)
		} else {
			vp.ScrollUp(-delta)
		}
	}
	return nil, true
}

func (m *Model) handleMouseMotion(msg tea.MouseMsg) (tea.Cmd, bool) {
	if !m.mouseDrag.active || m.mouseDrag.area != mouseAreaEditor {
		return nil, false
	}
	if msg.X != m.mouseDrag.startX || msg.Y != m.mouseDrag.startY {
		m.mouseDrag.dragged = true
	}
	return m.updateEditorMouseSelection(m.mouseDrag.outer, msg.X, msg.Y, true), true
}

func (m *Model) handleMouseRelease(msg tea.MouseMsg) (tea.Cmd, bool) {
	if !m.mouseDrag.active {
		return nil, false
	}
	drag := m.mouseDrag
	if drag.area != mouseAreaEditor {
		m.mouseDrag = mouseDragState{}
		return nil, true
	}
	var cmd tea.Cmd
	dragged := drag.dragged || msg.X != drag.startX || msg.Y != drag.startY
	if dragged || drag.lineMode {
		cmd = m.updateEditorMouseSelection(drag.outer, msg.X, msg.Y, true)
	} else {
		m.editor.ClearSelection()
	}
	if !drag.lineMode && !m.editor.SelectionActive() {
		m.editor.ClearSelection()
	}
	m.mouseDrag = mouseDragState{}
	return cmd, true
}

func (m *Model) handleNavigatorMouseClick(outer mouseRect, x, y int) (tea.Cmd, bool) {
	if m.navigator == nil {
		return m.setFocus(focusFile), true
	}
	body := m.paneBodyRect(outer, m.sidebarFrameStyle(m.navigatorPaneFocused()))
	if !body.Contains(x, y) {
		return m.setFocus(focusFile), true
	}

	row := y - body.y
	if m.navigatorFilterVisible() {
		filterH := 2
		if row < filterH {
			m.ensureNavigatorFilter()
			m.navigatorFilter.Focus()
			_ = m.setFocus(focusFile)
			return nil, true
		}
		row -= filterH
	}
	rows := m.navigator.VisibleRows()
	if row >= 0 && row < len(rows) {
		n := rows[row].Node
		if n != nil {
			m.navigator.SelectByID(n.ID)
			m.syncNavigatorSelection()
			if m.consumeMouseClick(mouseAreaNavigator, n.ID, x, y) {
				return m.activateNavigatorMouseNode(n), true
			}
			return nil, true
		}
	}

	return m.setFocus(focusFile), true
}

func (m *Model) activateNavigatorMouseNode(n *navigator.Node[any]) tea.Cmd {
	if m.navigator == nil || n == nil {
		return nil
	}
	switch n.Kind {
	case navigator.KindDir:
		m.navExpandDir(n, true)
		return nil
	case navigator.KindFile:
		return m.activateNavigatorMouseFile(n)
	case navigator.KindRequest:
		return m.activateNavigatorMouseRequest()
	case navigator.KindWorkflow:
		return m.sendNavigatorWorkflow()
	default:
		m.navigator.ToggleExpanded()
		return nil
	}
}

func (m *Model) activateNavigatorMouseRequest() tea.Cmd {
	res := m.navJumpCmd("l")
	if !res.ok {
		return res.cmd
	}
	var cmds []tea.Cmd
	if res.cmd != nil {
		cmds = append(cmds, res.cmd)
	}
	if res.focus {
		if cmd := m.setFocus(focusEditor); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if m.focus == focusEditor {
			m.suppressEditorKey = true
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *Model) activateNavigatorMouseFile(n *navigator.Node[any]) tea.Cmd {
	path := n.Payload.FilePath
	if path == "" {
		return nil
	}

	var cmds []tea.Cmd
	if !util.SamePath(path, m.currentFile) {
		if !m.confirmCrossFileNavigation(
			n,
			navActionOpenFile,
			"Double-click again to open.",
		) {
			return nil
		}
		if cmd := m.openFile(path); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if filesvc.IsRequestFile(path) {
		if refreshed := m.navigator.Find(n.ID); refreshed != nil {
			n = refreshed
		}
		m.navExpandFile(n, false)
		if len(cmds) > 0 {
			return tea.Batch(cmds...)
		}
		return nil
	}

	if cmd := m.setFocus(focusEditor); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *Model) handleEditorMouseClick(outer mouseRect, x, y int) (tea.Cmd, bool) {
	cmd := m.setFocus(focusEditor)
	hit := m.editorMousePosition(outer, x, y, false)
	if !hit.ok {
		return cmd, true
	}
	lineMode := x-hit.body.x < m.editorMouseReservedWidth()
	m.editor.ClearSelection()
	m.editor.moveCursorTo(hit.pos.Line, hit.pos.Column)
	m.mouseDrag = mouseDragState{
		active:   true,
		area:     mouseAreaEditor,
		outer:    outer,
		anchor:   hit.pos,
		lineMode: lineMode,
		startX:   x,
		startY:   y,
	}
	if lineMode {
		m.editor.StartVisualLineSelection(hit.pos)
	}
	m.syncNavigatorWithEditorCursor()
	return cmd, true
}

func (m *Model) updateEditorMouseSelection(outer mouseRect, x, y int, clamp bool) tea.Cmd {
	hit := m.editorMousePosition(outer, x, y, clamp)
	if !hit.ok {
		return nil
	}
	if m.mouseDrag.lineMode {
		m.editor.UpdateVisualLineSelection(m.mouseDrag.anchor, hit.pos)
		return nil
	}

	anchor, caret := m.editorMouseSelectionPositions(m.mouseDrag.anchor, hit.pos)
	m.editor.SetManualSelection(anchor, caret)
	return nil
}

func (m *Model) editorMousePosition(
	outer mouseRect,
	x, y int,
	clamp bool,
) editorMouseHit {
	body := m.paneBodyRect(outer, m.editorFrameStyle(m.focus == focusEditor))
	if body.Empty() {
		return editorMouseHit{body: body}
	}
	if !clamp && !body.Contains(x, y) {
		return editorMouseHit{body: body}
	}

	relY := max(y-body.y, 0)
	if relY >= body.h {
		relY = body.h - 1
	}

	line := max(m.editor.ViewStart()+relY, 0)
	n := m.editor.LineCount()
	if n <= 0 {
		return editorMouseHit{body: body}
	}
	if line >= n {
		line = n - 1
	}

	cell := max(x-body.x-m.editorMouseReservedWidth(), 0)
	col := m.editor.ColumnForVisibleCell(line, cell)
	return editorMouseHit{
		pos: cursorPosition{
			Line:   line,
			Column: col,
			Offset: m.editor.offsetForPosition(line, col),
		},
		body: body,
		ok:   true,
	}
}

func (m *Model) editorMouseSelectionPositions(
	anchor cursorPosition,
	pos cursorPosition,
) (cursorPosition, cursorPosition) {
	if pos.Offset <= anchor.Offset {
		return m.editorMouseRightBoundary(anchor), pos
	}
	return anchor, m.editorMouseRightBoundary(pos)
}

func (m *Model) editorMouseRightBoundary(pos cursorPosition) cursorPosition {
	lineLen := m.editor.LineLength(pos.Line)
	if pos.Column >= lineLen {
		return pos
	}
	col := pos.Column + 1
	return cursorPosition{
		Line:   pos.Line,
		Column: col,
		Offset: m.editor.offsetForPosition(pos.Line, col),
	}
}

func (m *Model) editorMouseReservedWidth() int {
	width := lipgloss.Width(m.editor.Prompt)
	if !m.editor.ShowLineNumbers {
		return width
	}
	line := max(m.editor.LineCount(), 1)
	if h := m.editor.Height(); h > line {
		line = h
	}
	digits := max(len(strconv.Itoa(line)), 2)
	return width + digits + 2
}

func (m *Model) handleResponseMouseClick(outer mouseRect, x, y int) (tea.Cmd, bool) {
	hit := m.responseMouseHit(outer, x, y)
	if !hit.ok {
		return m.setFocus(focusResponse), true
	}
	m.focusResponsePane(hit.id)
	cmd := m.setFocus(focusResponse)

	if y == hit.rect.y {
		if tab, ok := m.responseTabAt(hit.id, x-hit.rect.x); ok {
			if pane := m.pane(hit.id); pane != nil && pane.activeTab != tab {
				pane.setActiveTab(tab)
				if tab == responseTabHistory {
					m.historyJumpToLatest = true
				}
				return batchCommands(cmd, m.syncResponsePane(hit.id)), true
			}
		}
	}

	pane := m.pane(hit.id)
	if pane != nil && pane.activeTab == responseTabHistory {
		if historyCmd, ok := m.selectHistoryAtMouseY(hit.rect, x, y); ok {
			return batchCommands(cmd, historyCmd), true
		}
	}
	return cmd, true
}

func (m *Model) responseTabAt(id responsePaneID, x int) (responseTab, bool) {
	pane := m.pane(id)
	if pane == nil || x < 0 {
		return responseTabPretty, false
	}
	width := max(pane.viewport.Width, 1)
	row := m.renderPaneTabs(id, true, width)
	line := firstLine(ansi.Strip(row))
	off := 0
	for _, tab := range m.availableResponseTabs() {
		label := responseTabLabelForSnapshot(tab, pane.snapshot)
		r := findVisibleTabRange(line, label, off)
		if !r.ok {
			continue
		}
		if x >= r.start && x < r.end {
			return tab, true
		}
		off = r.next
	}
	return responseTabPretty, false
}

func findVisibleTabRange(line, label string, off int) tabRange {
	if label == "" {
		return tabRange{next: off}
	}
	if off < 0 {
		off = 0
	}
	if off > len(line) {
		off = len(line)
	}
	rs := []rune(label)
	cands := make([]string, 0, len(rs)*2)
	for n := len(rs); n >= 1; n-- {
		part := string(rs[:n])
		cands = append(cands, tabIndicatorPrefix+part, part)
	}

	start := -1
	end := -1
	startCell := 0
	endCell := 0
	for _, cand := range cands {
		idx := strings.Index(line[off:], cand)
		if idx < 0 {
			continue
		}
		idx += off
		candEnd := idx + len(cand)
		if start == -1 || idx < start || (idx == start && candEnd > end) {
			start = idx
			end = candEnd
			startCell = lipgloss.Width(line[:idx])
			endCell = lipgloss.Width(line[:candEnd])
		}
	}
	if start < 0 {
		return tabRange{next: off}
	}
	return tabRange{
		start: startCell,
		end:   endCell,
		next:  end,
		ok:    true,
	}
}

func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before
	}
	return s
}

func (m *Model) selectHistoryAtMouseY(r mouseRect, x, y int) (tea.Cmd, bool) {
	pane := m.focusedPane()
	if pane == nil || pane.activeTab != responseTabHistory {
		return nil, false
	}
	off := y - r.y - responseTabsHeight - m.historyHeaderHeight()
	if off < 0 {
		return nil, false
	}

	row := off / historyMouseRowHeight()
	items := m.historyList.VisibleItems()
	if len(items) == 0 || row < 0 {
		return nil, false
	}

	idx := m.historyList.Paginator.Page*m.historyList.Paginator.PerPage + row
	if idx < 0 || idx >= len(items) {
		return nil, false
	}
	m.historyList.Select(idx)
	m.captureHistorySelection()
	target := "history:" + strconv.Itoa(idx)
	if it, ok := items[idx].(historyItem); ok && it.entry.ID != "" {
		target = it.entry.ID
	}

	if m.consumeMouseClick(mouseAreaHistory, target, x, y) {
		return m.loadHistorySelection(false), true
	}
	return nil, true
}

func (m *Model) scrollNavigatorBy(delta int) tea.Cmd {
	if m.navigator == nil || delta == 0 {
		return nil
	}
	_ = m.setFocus(focusFile)
	m.navigator.Move(delta)
	m.syncNavigatorSelection()
	return nil
}

func (m *Model) scrollEditorBy(delta int) tea.Cmd {
	if delta == 0 {
		return nil
	}
	_ = m.setFocus(focusEditor)
	m.editor.SetViewStart(m.editor.ViewStart() + delta)
	return nil
}

func (m *Model) scrollResponseBy(id responsePaneID, delta int) tea.Cmd {
	if delta == 0 {
		return nil
	}
	p := m.pane(id)
	if p == nil {
		return nil
	}
	if p.activeTab == responseTabHistory {
		return m.scrollHistoryBy(mouseWheelRowDelta(delta))
	}
	if p.activeTab == responseTabStats {
		if stats := workflowStatsFromPane(p); stats != nil {
			if stats.detailFocus {
				return m.scrollWorkflowStatsDetail(delta)
			}
			return m.jumpWorkflowStatsSelection(delta)
		}
	}
	if !isScrollableResponsePane(p) {
		return nil
	}
	return m.scrollResponseViewport(p, func() {
		if delta > 0 {
			p.viewport.ScrollDown(delta)
		} else {
			p.viewport.ScrollUp(-delta)
		}
	})
}

func (m *Model) scrollHistoryBy(delta int) tea.Cmd {
	if delta == 0 {
		return nil
	}
	m.moveHistoryCursor(delta)
	m.captureHistorySelection()
	return nil
}
