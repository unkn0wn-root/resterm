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

// Apply the background to each hint because ANSI resets clear the container background.
func (m Model) renderCommandHints(style lipgloss.Style) string {
	limit := commandBarContentWidth(style)
	barBg := style.GetBackground()
	// Hint cells render flush, so the divider carries all spacing between them.
	dividerStyle := m.theme.CommandDivider
	if !theme.ColorDefined(dividerStyle.GetBackground()) {
		dividerStyle = dividerStyle.Background(barBg)
	}
	divider := dividerStyle.Render("  ")

	context := m.renderHintRow(m.contextCommandHints(), divider, limit, true, barBg)
	if limit <= 0 {
		return renderCommandBarContainer(style, context)
	}

	pad := max(limit-lipgloss.Width(context), 0)
	fill := lipgloss.NewStyle().Background(barBg).Render(strings.Repeat(" ", pad))
	return renderCommandBarContainer(style, context+fill)
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
