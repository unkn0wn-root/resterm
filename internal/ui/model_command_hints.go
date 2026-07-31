package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/bindings"
)

type commandHint struct {
	key          string
	label        string
	noBackground bool
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
		}
	case focusRequests:
		return []commandHint{
			{key: "Enter", label: "Run"},
			{key: "Space", label: "Preview"},
			{key: "/", label: "Filter"},
			{key: "m", label: "Method"},
			{key: "t", label: "Tags"},
			{key: "l", label: "Jump"},
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
	case responseTabRaw:
		hints = append(hints,
			m.commandActionHint(bindings.ActionCycleRawView, "View"),
			m.commandActionHint(bindings.ActionShowRawDump, "Dump"),
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
// group keeps a fixed position at the right edge.
func (m Model) renderCommandHints(style lipgloss.Style) string {
	limit := commandBarContentWidth(style)
	divider := m.theme.CommandDivider.Render(" ")

	anchorRow := m.pinnedRow(limit, divider)
	if limit <= 0 {
		row := lipgloss.JoinHorizontal(lipgloss.Top, m.fitCommandHints(nil, m.contextCommandHints(), divider, 0)...)
		return renderCommandBarContainer(style, row+anchorRow)
	}

	const gap = 2
	contextRow := ""
	if avail := limit - lipgloss.Width(anchorRow) - gap; avail > 0 {
		contextRow = lipgloss.JoinHorizontal(lipgloss.Top, m.fitCommandHints(nil, m.contextCommandHints(), divider, avail)...)
	}
	pad := max(limit-lipgloss.Width(contextRow)-lipgloss.Width(anchorRow), 0)
	return renderCommandBarContainer(style, contextRow+strings.Repeat(" ", pad)+anchorRow)
}

// pinnedRow lays out the global shortcuts in display order Focus, New, Save,
// Quit, Cmd, Help. Labels drop when the full row would claim more than half
// the bar. Focus, Cmd, and Help claim remaining space before the middle three.
func (m Model) pinnedRow(limit int, divider string) string {
	focus := m.commandActionHint(bindings.ActionCycleFocusNext, "Focus")
	core := []commandHint{
		focus,
		{key: ":", label: "Cmd"},
		m.commandActionHint(bindings.ActionToggleHelp, "Help"),
	}
	extras := []commandHint{
		caretKey(m.commandActionHint(bindings.ActionOpenNewFileModal, "New")),
		caretKey(m.commandActionHint(bindings.ActionSaveFile, "Save")),
		caretKey(m.commandActionHint(bindings.ActionQuitApp, "Quit")),
	}
	for i := range core {
		core[i].noBackground = true
	}
	for i := range extras {
		extras[i].noBackground = true
	}
	assemble := func() []commandHint {
		out := []commandHint{core[0]}
		out = append(out, extras...)
		return append(out, core[1], core[2])
	}

	fullRow := lipgloss.JoinHorizontal(lipgloss.Top, m.fitCommandHints(nil, assemble(), divider, 0)...)
	if limit <= 0 {
		return fullRow
	}
	if lipgloss.Width(fullRow) > limit/2 {
		for i := range core {
			core[i].label = ""
		}
		for i := range extras {
			extras[i].label = ""
		}
	}

	width := lipgloss.Width(lipgloss.JoinHorizontal(lipgloss.Top, m.fitCommandHints(nil, core, divider, limit)...))
	kept := extras[:0]
	for _, h := range extras {
		if h.key == "" {
			continue
		}
		w := lipgloss.Width(m.renderCommandHint(h, 0)) + lipgloss.Width(divider)
		if width+w > limit {
			continue
		}
		width += w
		kept = append(kept, h)
	}
	extras = kept
	return lipgloss.JoinHorizontal(lipgloss.Top, m.fitCommandHints(nil, assemble(), divider, limit)...)
}

func (m Model) fitCommandHints(
	buttons []string,
	hints []commandHint,
	divider string,
	limit int,
) []string {
	width := lipgloss.Width(lipgloss.JoinHorizontal(lipgloss.Top, buttons...))
	for _, hint := range hints {
		if hint.key == "" {
			continue
		}
		idx := (len(buttons) + 1) / 2
		button := m.renderCommandHint(hint, idx)
		extra := lipgloss.Width(button)
		if len(buttons) > 0 {
			extra += lipgloss.Width(divider)
		}
		if limit > 0 && width+extra > limit {
			continue
		}
		if len(buttons) > 0 {
			buttons = append(buttons, divider)
		}
		buttons = append(buttons, button)
		width += extra
	}
	return buttons
}

func (m Model) renderCommandHint(hint commandHint, idx int) string {
	segment := m.theme.CommandSegment(idx)
	if hint.noBackground {
		segment.Background = ""
	}
	return renderCommandButton(hint.key, hint.label, segment)
}
