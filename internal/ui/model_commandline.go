package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type exCommandKind int

const (
	exCommandUnknown exCommandKind = iota
	exCommandInvalid
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
	err  error
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
	return m.commandLine.open(m.commandSource())
}

func (m *Model) closeCommandLine() {
	m.showCommandLine = false
	m.commandLineJustOpened = false
	m.commandLine.close()
}

func (m *Model) handleCommandLineKey(msg tea.KeyMsg) tea.Cmd {
	if m.commandLineJustOpened {
		m.commandLineJustOpened = false
		if isCommandLineTriggerKey(msg) {
			return nil
		}
	}

	src := m.commandSource()
	switch msg.String() {
	case "esc", "ctrl+c", "ctrl+g":
		m.closeCommandLine()
		return nil
	case "ctrl+q", "ctrl+d":
		return tea.Quit
	case "enter":
		cmd, more, err := m.commandLine.accept(src)
		if err != nil {
			return statusCmd(statusError, err.Error())
		}
		if more {
			return cmd
		}
		value := m.commandLine.value()
		m.closeCommandLine()
		return m.executeExCommand(value)
	}

	cmd, err := m.commandLine.handleKey(msg, src)
	if err != nil {
		return statusCmd(statusError, err.Error())
	}
	return cmd
}

func (m *Model) executeExCommand(input string) tea.Cmd {
	cmd := exCommands.Parse(input)
	switch cmd.kind {
	case exCommandEmpty:
		return statusCmd(statusWarn, "Enter a command")
	case exCommandInvalid:
		return statusCmd(statusWarn, cmd.err.Error())
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
		switch len(cmd.args) {
		case 0:
			return m.openOpenModal()
		case 1:
			return m.openPathFromCommand(cmd.args[0])
		default:
			return statusCmd(statusWarn, "Usage: :edit [path]")
		}
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

// The popup takes only the width its rows need. The cap stops a verbose row
// from spanning the whole screen.
const (
	commandSuggestionMaxRows  = 10
	commandSuggestionMaxWidth = 96
)

func (m *Model) renderCommandSuggestionPopup(content string, y int) string {
	w := max(m.width, lipgloss.Width(content))
	h := lipgloss.Height(content)
	if w <= 0 || h <= y {
		return content
	}

	mx := m.editorHintBoxMetrics()
	limit := min(commandSuggestionMaxRows, max(h-y-mx.frameH, 0))
	maxW := min(commandSuggestionMaxWidth, max(w-2, 0))
	lines := m.buildMenuPopup(m.commandLine.menu, limit, maxW)
	if len(lines) == 0 {
		return content
	}

	return overlayHintPopup(content, lines, 1, y, w, h)
}
