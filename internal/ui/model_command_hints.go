package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

type commandHint struct {
	key   string
	label string
}

func caretKey(h commandHint) commandHint {
	h.key = strings.ReplaceAll(h.key, "Ctrl+", "^")
	return h
}

func (m Model) contextCommandHints() []commandHint {
	switch m.focus {
	case focusFile:
		return []commandHint{
			{key: "Enter", label: "Open"},
			{key: "Space", label: "Expand"},
			{key: "/", label: "Filter"},
			caretKey(m.commandActionHint(bindings.ActionOpenNewFileModal, "New")),
			caretKey(m.commandActionHint(bindings.ActionOpenPathModal, "Open")),
			caretKey(m.commandActionHint(bindings.ActionOpenTempDocument, "Temp")),
		}
	case focusRequests:
		return []commandHint{
			{key: "Enter", label: "Run"},
			{key: "Space", label: "Preview"},
			{key: "/", label: "Filter"},
			{key: "m", label: "Method"},
			{key: "t", label: "Tags"},
			{key: "l", label: "Jump"},
			m.commandActionHint(bindings.ActionShowRequestDetails, "Details"),
		}
	case focusWorkflows:
		return []commandHint{
			{key: "Enter", label: "Run"},
			{key: "/", label: "Filter"},
			{key: "l", label: "Jump"},
		}
	case focusEditor:
		return m.editorCommandHints()
	case focusResponse:
		return m.responseCommandHints()
	default:
		return nil
	}
}

func (m Model) editorCommandHints() []commandHint {
	if m.editorInsertMode {
		return []commandHint{
			{key: "Esc", label: "Normal"},
			m.commandActionHint(bindings.ActionSendRequest, "Send"),
			{key: "Tab", label: "Complete"},
		}
	}
	// plain Enter sends from normal mode, see shouldSendEditorRequest
	return []commandHint{
		{key: "i", label: "Insert"},
		{key: "Enter", label: "Send"},
		m.commandActionHint(bindings.ActionShowContextHelp, "Docs"),
		{key: "/", label: "Search"},
		caretKey(m.commandActionHint(bindings.ActionSaveFile, "Save")),
	}
}

func (m Model) responseCommandHints() []commandHint {
	hints := []commandHint{
		{key: "j/k", label: "Scroll"},
		{key: "/", label: "Search"},
		m.commandActionHint(bindings.ActionCopyResponseTab, "Copy"),
	}
	pane := m.focusedPane()
	if pane == nil {
		return hints
	}

	switch pane.activeTab {
	case responseTabPretty:
		hints = append(hints, m.commandActionHint(bindings.ActionSaveResponseBody, "Save"))
	case responseTabRaw:
		hints = append(hints,
			m.commandActionHint(bindings.ActionCycleRawView, "View"),
			m.commandActionHint(bindings.ActionShowRawDump, "Dump"),
			m.commandActionHint(bindings.ActionSaveResponseBody, "Save"),
		)
	case responseTabHeaders:
		hints = append(hints, commandHint{key: "Enter", label: "Request/Response"})
	case responseTabStream:
		hints = append(hints,
			commandHint{key: "Ctrl+Space", label: "Pause"},
			commandHint{key: "Ctrl+F", label: "Filter"},
			commandHint{key: "Ctrl+B", label: "Bookmark"},
		)
	case responseTabHistory:
		hints = append(hints,
			commandHint{key: "c", label: "Scope"},
			commandHint{key: "s", label: "Sort"},
			commandHint{key: "Enter", label: "Load"},
		)
	case responseTabCompare:
		hints = append(hints, commandHint{key: "Enter", label: "Inspect"})
	}
	return hints
}

func (m Model) commandActionHint(action bindings.ActionID, label string) commandHint {
	items := m.bindingsMap.Bindings(action)
	if len(items) == 0 {
		return commandHint{}
	}
	return commandHint{key: formatHelpBinding(items[0]), label: label}
}

// Context hints lead because they change with the focused pane. The pinned
// group keeps a fixed position at the right edge. Every run sets the bar
// background itself because the resets inside rendered hints stop the
// container style from carrying it across the row.
func (m Model) renderCommandHints(style lipgloss.Style) string {
	limit := commandBarContentWidth(style)
	barBg := style.GetBackground()
	// Hint cells render flush, so the divider carries all spacing between them.
	dividerStyle := m.theme.CommandDivider
	if !theme.ColorDefined(dividerStyle.GetBackground()) {
		dividerStyle = dividerStyle.Background(barBg)
	}
	divider := dividerStyle.Render("  ")

	anchor := m.pinnedRow(limit, divider, barBg)
	if limit <= 0 {
		return renderCommandBarContainer(
			style,
			m.renderHintRow(m.contextCommandHints(), divider, 0, true, barBg)+divider+anchor,
		)
	}

	const gap = 2
	context := ""
	if avail := limit - lipgloss.Width(anchor) - gap; avail > 0 {
		context = m.renderHintRow(m.contextCommandHints(), divider, avail, true, barBg)
	}
	pad := max(limit-lipgloss.Width(context)-lipgloss.Width(anchor), 0)
	fill := lipgloss.NewStyle().Background(barBg).Render(strings.Repeat(" ", pad))
	return renderCommandBarContainer(style, context+fill+anchor)
}

// pinnedRow lays out the global shortcuts in display order Focus, Quit, Cmd,
// Help. Labels drop when the full row would claim more than half the bar.
// Focus, Cmd, and Help claim remaining space before Quit.
func (m Model) pinnedRow(limit int, divider string, barBg lipgloss.TerminalColor) string {
	core := []commandHint{
		m.commandActionHint(bindings.ActionCycleFocusNext, "Focus"),
		{key: ":", label: "Cmd"},
		m.commandActionHint(bindings.ActionToggleHelp, "Help"),
	}
	extras := []commandHint{
		caretKey(m.commandActionHint(bindings.ActionQuitApp, "Quit")),
	}

	full := m.renderHintRow(pinnedOrder(core, extras), divider, 0, false, barBg)
	if limit <= 0 {
		return full
	}
	if lipgloss.Width(full) > limit/2 {
		for i := range core {
			core[i].label = ""
		}
		for i := range extras {
			extras[i].label = ""
		}
	}

	width := lipgloss.Width(m.renderHintRow(pinnedOrder(core, nil), divider, limit, false, barBg))
	kept := make([]commandHint, 0, len(extras))
	for _, h := range extras {
		if h.key == "" {
			continue
		}
		w := lipgloss.Width(m.renderCommandHint(h, 0, false, barBg)) + lipgloss.Width(divider)
		if width+w > limit {
			continue
		}
		width += w
		kept = append(kept, h)
	}
	return m.renderHintRow(pinnedOrder(core, kept), divider, limit, false, barBg)
}

// pinnedOrder builds the display order from core Focus, Cmd, Help. Focus leads
// the row, Cmd and Help trail it, and extras sit in between so they can drop
// without moving the edges.
func pinnedOrder(core, extras []commandHint) []commandHint {
	row := make([]commandHint, 0, len(core)+len(extras))
	row = append(row, core[0])
	row = append(row, extras...)
	row = append(row, core[1:]...)
	return row
}

// renderHintRow joins bound hints with divider. A positive limit drops any
// hint that would push the row past it.
func (m Model) renderHintRow(
	hints []commandHint,
	divider string,
	limit int,
	keycap bool,
	barBg lipgloss.TerminalColor,
) string {
	var cells []string
	width, count := 0, 0
	for _, hint := range hints {
		if hint.key == "" {
			continue
		}
		cell := m.renderCommandHint(hint, count, keycap, barBg)
		w := lipgloss.Width(cell)
		if count > 0 {
			w += lipgloss.Width(divider)
		}
		if limit > 0 && width+w > limit {
			continue
		}
		if count > 0 {
			cells = append(cells, divider)
		}
		cells = append(cells, cell)
		width += w
		count++
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cells...)
}

func (m Model) renderCommandHint(hint commandHint, idx int, keycap bool, barBg lipgloss.TerminalColor) string {
	seg := m.theme.CommandSegment(idx)
	if keycap && seg.Background != "" {
		return renderCommandKeycap(hint.key, hint.label, seg, barBg)
	}
	return renderCommandButton(hint.key, hint.label, seg, barBg)
}
