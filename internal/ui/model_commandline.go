package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
)

type exCommand struct {
	kind exCommandKind
	name string
	bang bool
}

func isCommandLineTriggerKey(msg tea.KeyMsg) bool {
	return msg.String() == ":"
}

// Modals, help, the search prompt and the navigator/history filters are already
// gated before handleKeyWithChord runs; only states reachable there are checked.
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
	return m.commandLineInput.Focus()
}

func (m *Model) closeCommandLine() {
	m.showCommandLine = false
	m.commandLineJustOpened = false
	m.commandLineInput.Blur()
	m.commandLineInput.SetValue("")
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
		m.closeCommandLine()
		return m.executeExCommand(value)
	}

	var cmd tea.Cmd
	m.commandLineInput, cmd = m.commandLineInput.Update(msg)
	return cmd
}

func parseExCommand(input string) exCommand {
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(input), ":"))
	if len(fields) == 0 {
		return exCommand{kind: exCommandEmpty}
	}

	name := fields[0]
	bang := strings.HasSuffix(name, "!")
	kind := exCommandKindFor(strings.ToLower(strings.TrimSuffix(name, "!")))
	if kind == exCommandUnknown {
		return exCommand{kind: exCommandUnknown, name: strings.Join(fields, " ")}
	}
	if len(fields) > 1 {
		return exCommand{kind: exCommandTrailing, name: strings.Join(fields[1:], " ")}
	}
	return exCommand{kind: kind, bang: bang}
}

func exCommandKindFor(name string) exCommandKind {
	switch name {
	case "w", "write":
		return exCommandWrite
	case "q", "quit", "qa", "qall":
		return exCommandQuit
	case "wq":
		return exCommandWriteQuit
	case "x", "xit", "exit":
		return exCommandExit
	case "e", "edit":
		return exCommandEdit
	case "h", "help":
		return exCommandHelp
	case "noh", "nohlsearch":
		return exCommandNoHighlight
	default:
		return exCommandUnknown
	}
}

func (m *Model) executeExCommand(input string) tea.Cmd {
	cmd := parseExCommand(input)
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
		if !m.showHelp {
			m.toggleHelp()
		}
		return nil
	case exCommandNoHighlight:
		return m.clearSearchHighlightsFromEx()
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
