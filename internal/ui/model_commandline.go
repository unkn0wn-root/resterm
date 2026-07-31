package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/intellisense"
)

type exCommandKind int

const (
	exCommandUnknown exCommandKind = iota
	exCommandEmpty
	exCommandTrailing
	exCommandWrite
	exCommandQuit
	exCommandWriteQuit
	exCommandExit
	exCommandEdit
	exCommandHelp
	exCommandNoHighlight
	exCommandMock
	exCommandDocs
)

type exCommand struct {
	kind exCommandKind
	name string
	bang bool
	args []string
}

func isCommandLineTriggerKey(msg tea.KeyMsg) bool {
	return msg.String() == ":"
}

// Modals, help, the search prompt and the navigator and history filters are
// already gated before handleKeyWithChord runs. Only states reachable there
// are checked.
func (m *Model) canOpenCommandLine(msg tea.KeyMsg) bool {
	return isCommandLineTriggerKey(msg) &&
		!m.streamFilterActive &&
		!m.websocketConsoleCapturesInput() &&
		(m.focus != focusEditor || (!m.editorInsertMode && !m.editor.awaitingFindTarget()))
}

func (m *Model) openCommandLine() tea.Cmd {
	m.resetChordState()
	m.clearOperatorState()
	m.editor.pendingMotion = ""
	m.closeSearchPrompt()
	m.showCommandLine = true
	m.commandLineJustOpened = true
	m.commandLineInput.SetValue("")
	m.commandLineInput.CursorEnd()
	m.refreshCommandSuggestions()
	return m.commandLineInput.Focus()
}

func (m *Model) closeCommandLine() {
	m.showCommandLine = false
	m.commandLineJustOpened = false
	m.commandLineInput.Blur()
	m.commandLineInput.SetValue("")
	m.commandSuggestions.reset(nil)
}

func (m *Model) handleCommandLineKey(msg tea.KeyMsg) tea.Cmd {
	keyStr := msg.String()
	if m.commandLineJustOpened {
		m.commandLineJustOpened = false
		if isCommandLineTriggerKey(msg) {
			return nil
		}
	}

	switch keyStr {
	case "esc", "ctrl+c", "ctrl+g":
		m.closeCommandLine()
		return nil
	case "ctrl+q", "ctrl+d":
		return tea.Quit
	case "enter":
		value := m.commandLineInput.Value()
		if item, ok := m.commandSuggestions.selected(); ok {
			value = item.insert
		}
		m.closeCommandLine()
		return m.executeExCommand(value)
	case "down", "ctrl+n":
		m.commandSuggestions.move(1)
		return nil
	case "up", "ctrl+p":
		m.commandSuggestions.move(-1)
		return nil
	case "tab":
		item, ok := m.commandSuggestions.completion()
		if !ok {
			return nil
		}
		m.commandLineInput.SetValue(item.insert)
		m.commandLineInput.CursorEnd()
		m.refreshCommandSuggestions()
		return nil
	}

	prev := m.commandLineInput.Value()
	var cmd tea.Cmd
	m.commandLineInput, cmd = m.commandLineInput.Update(msg)
	if m.commandLineInput.Value() != prev {
		m.refreshCommandSuggestions()
	}
	return cmd
}

func (m *Model) refreshCommandSuggestions() {
	m.commandSuggestions.reset(exCommands.Suggestions(m.commandLineInput.Value()))
}

func (m *Model) executeExCommand(input string) tea.Cmd {
	cmd := exCommands.Parse(input)
	switch cmd.kind {
	case exCommandEmpty:
		return statusCmd(statusWarn, "Enter a command")
	case exCommandTrailing:
		return statusCmd(statusWarn, "Trailing characters: "+cmd.name)
	case exCommandWrite:
		return m.saveFile()
	case exCommandQuit:
		return m.quitFromEx(cmd.bang)
	case exCommandWriteQuit:
		return m.writeQuitFromEx()
	case exCommandExit:
		if m.dirty {
			return m.writeQuitFromEx()
		}
		return tea.Quit
	case exCommandEdit:
		m.openOpenModal()
		return nil
	case exCommandHelp:
		return m.openHelpQuery(cmd.args)
	case exCommandNoHighlight:
		return m.clearSearchHighlightsFromEx()
	case exCommandMock:
		return m.executeMockCommand(cmd.args)
	case exCommandDocs:
		return m.openDocsQuery(cmd.args)
	default:
		return statusCmd(statusWarn, "Unknown command: "+cmd.name+" (try :help)")
	}
}

func (m *Model) quitFromEx(force bool) tea.Cmd {
	if !force && m.dirty {
		return statusCmd(statusWarn, "No write since last change (add ! to quit)")
	}
	return tea.Quit
}

func (m *Model) writeQuitFromEx() tea.Cmd {
	outcome, cmd := m.saveFileWithOutcome()
	switch outcome {
	case saveFileOutcomeSaved:
		return batchCommands(cmd, tea.Quit)
	case saveFileOutcomePending:
		// Must be set after saveFileWithOutcome: opening the save-as modal resets saveAsFollowUp.
		m.saveAsFollowUp = tea.Quit
		return cmd
	default:
		return cmd
	}
}

func (m *Model) clearSearchHighlightsFromEx() tea.Cmd {
	hadSearch := m.editor.ExitSearchMode() != nil

	var cmds []tea.Cmd
	for _, id := range []responsePaneID{responsePanePrimary, responsePaneSecondary} {
		pane := m.pane(id)
		if pane == nil {
			continue
		}
		if pane.search.clear() {
			hadSearch = true
			if cmd := m.syncResponsePane(id); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	if hadSearch {
		cmds = append(cmds, statusCmd(statusInfo, "Search cleared"))
	} else {
		cmds = append(cmds, statusCmd(statusInfo, "No search highlights"))
	}
	return batchCommands(cmds...)
}

const (
	commandSuggestionMaxRows  = 8
	commandSuggestionMaxWidth = 64
)

func (m Model) renderCommandSuggestionPopup(content string, y int) string {
	w := max(m.width, lipgloss.Width(content))
	h := lipgloss.Height(content)
	if w <= 0 || h <= y {
		return content
	}

	mx := m.editorHintBoxMetrics()
	limit := min(commandSuggestionMaxRows, max(h-y-mx.frameH, 0))
	items, selection, ok := m.commandSuggestions.display(limit)
	if !ok {
		return content
	}

	hints := make([]intellisense.Item, len(items))
	for i, item := range items {
		hints[i] = intellisense.Item{Label: item.label, Summary: item.summary}
	}
	labelW, summaryW := completionPopupPreference(hints)
	maxW := min(commandSuggestionMaxWidth, max(w-2, 0))
	lines := m.buildCompletionPopup(hints, selection, maxW, labelW, summaryW)
	if len(lines) == 0 {
		return content
	}

	return overlayHintPopup(content, lines, 1, y, w, h)
}
