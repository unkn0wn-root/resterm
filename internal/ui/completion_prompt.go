package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/prompt"
)

// How long a prompt waits for the directory it opens on before handing the read
// back to the update loop.
const firstListingWait = 20 * time.Millisecond

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
	id        promptID
	input     textinput.Model
	menu      prompt.Menu
	paths     prompt.PathSession
	dismissed bool
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
	var pending tea.Cmd
	if load := p.fill(src); load.Pending() {
		read, later := listDirSoon(p.id, load, readDir, firstListingWait)
		if later != nil {
			pending = later
		} else {
			p.deliver(read)
		}
	}
	return batchCommands(p.input.Focus(), pending)
}

func (p *completionPrompt) close() {
	p.input.Blur()
	p.input.SetValue("")
	p.menu.Reset(nil)
	p.paths.Reset()
}

func (p *completionPrompt) value() string { return p.input.Value() }

func (p *completionPrompt) dismiss() bool {
	if len(p.menu.Items()) == 0 {
		return false
	}
	p.menu.Reset(nil)
	p.dismissed = true
	return true
}

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
		return p.refresh(src), nil
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
	p.dismissed = false
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
	items, ok := p.paths.Deliver(read)
	if ok && !p.dismissed {
		p.menu.Reset(items)
	}
}

func readDir(load prompt.DirLoad) prompt.DirRead {
	entries, err := prompt.ReadDir(load.Dir)
	return prompt.DirRead{DirLoad: load, Entries: entries, Err: err}
}

func readPathDir(id promptID, load prompt.DirLoad) tea.Cmd {
	return func() tea.Msg {
		return pathReadMsg{id: id, read: readDir(load)}
	}
}

// listDirSoon gives a fast directory long enough to land in the frame that
// opens the prompt. A slow one comes back through the update loop instead, so
// a network mount cannot hold the whole UI.
func listDirSoon(
	id promptID,
	load prompt.DirLoad,
	read func(prompt.DirLoad) prompt.DirRead,
	wait time.Duration,
) (prompt.DirRead, tea.Cmd) {
	done := make(chan prompt.DirRead, 1)
	go func() { done <- read(load) }()

	select {
	case read := <-done:
		return read, nil
	case <-time.After(wait):
		return prompt.DirRead{}, func() tea.Msg {
			return pathReadMsg{id: id, read: <-done}
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
