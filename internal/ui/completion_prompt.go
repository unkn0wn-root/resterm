package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/prompt"
)

type completionSource interface {
	prompt.PathProvider
	Suggest(input string) []prompt.Item
}

type promptID uint8

const (
	promptCommandLine promptID = iota + 1
	promptOpenPath
	promptResponseSave
)

type pathReadMsg struct {
	id   promptID
	read prompt.DirRead
}

type completionPrompt struct {
	id    promptID
	input textinput.Model
	menu  prompt.Menu
	paths prompt.PathSession
}

func newCompletionPrompt(id promptID, placeholder string) completionPrompt {
	return completionPrompt{id: id, input: newPromptInput(placeholder, "")}
}

func (p *completionPrompt) open(src completionSource) tea.Cmd {
	return p.openWith(src, "")
}

func (p *completionPrompt) openWith(src completionSource, value string) tea.Cmd {
	p.input.SetValue(value)
	p.input.CursorEnd()
	p.menu.Reset(nil)
	p.paths.Reset()

	// The directory a prompt opens on is listed here instead of off the update
	// loop, so the prompt draws complete in the frame it opens in rather than
	// growing into its suggestions a frame later. Every keystroke after this
	// one goes back to reading off the loop.
	if load := p.fill(src); load.Pending() {
		entries, err := prompt.ReadDir(load.Dir)
		p.deliver(prompt.DirRead{DirLoad: load, Entries: entries, Err: err})
	}
	return p.input.Focus()
}

func (p *completionPrompt) close() {
	p.input.Blur()
	p.input.SetValue("")
	p.menu.Reset(nil)
	p.paths.Reset()
}

func (p *completionPrompt) value() string { return p.input.Value() }

func (p *completionPrompt) handleKey(msg tea.KeyMsg, src completionSource) (tea.Cmd, error) {
	switch msg.String() {
	case "down", "ctrl+n":
		p.menu.Move(1)
		return nil, nil
	case "up", "ctrl+p":
		p.menu.Move(-1)
		return nil, nil
	case "tab":
		return p.complete(src)
	}

	value, cursor := p.input.Value(), p.input.Position()
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	if p.input.Value() == value && p.input.Position() == cursor {
		return cmd, nil
	}
	return batchCommands(cmd, p.refresh(src)), nil
}

func (p *completionPrompt) complete(src completionSource) (tea.Cmd, error) {
	item, ok := p.menu.Preferred()
	if !ok {
		return nil, nil
	}
	if err := p.write(item); err != nil {
		return nil, err
	}
	return p.refresh(src), nil
}

func (p *completionPrompt) accept(src completionSource) (cmd tea.Cmd, more bool, err error) {
	item, ok := p.menu.Selected()
	if !ok {
		return nil, false, nil
	}
	if err := p.write(item); err != nil {
		return nil, false, err
	}
	if !item.Continue {
		return nil, false, nil
	}
	return p.refresh(src), true, nil
}

func (p *completionPrompt) write(item prompt.Item) error {
	value, cursor, err := item.Edit.Apply(p.input.Value())
	if err != nil {
		return err
	}
	p.input.SetValue(value)
	p.input.SetCursor(cursor)
	return nil
}

func (p *completionPrompt) refresh(src completionSource) tea.Cmd {
	load := p.fill(src)
	if !load.Pending() {
		return nil
	}
	return readPathDir(p.id, load)
}

// fill sets the menu from what is known already and reports the directory that
// still has to be listed, if there is one.
func (p *completionPrompt) fill(src completionSource) prompt.DirLoad {
	input, cursor := p.input.Value(), p.input.Position()
	items, load, isPath := p.paths.Suggest(src, input, cursor)
	if !isPath {
		p.menu.Reset(src.Suggest(input))
		return prompt.DirLoad{}
	}

	p.menu.Reset(items)
	return load
}

func (p *completionPrompt) deliver(read prompt.DirRead) {
	if items, ok := p.paths.Deliver(read); ok {
		p.menu.Reset(items)
	}
}

func readPathDir(id promptID, load prompt.DirLoad) tea.Cmd {
	return func() tea.Msg {
		entries, err := prompt.ReadDir(load.Dir)
		return pathReadMsg{
			id:   id,
			read: prompt.DirRead{DirLoad: load, Entries: entries, Err: err},
		}
	}
}

func (m *Model) handlePathRead(msg pathReadMsg) {
	switch msg.id {
	case promptCommandLine:
		m.commandLine.deliver(msg.read)
	case promptOpenPath:
		m.openPathPrompt.deliver(msg.read)
	case promptResponseSave:
		m.responseSavePrompt.deliver(msg.read)
	}
}
