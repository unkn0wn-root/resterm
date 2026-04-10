package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
)

func emitQueuedMsg(ch chan tea.Msg, msg tea.Msg) {
	if msg == nil || ch == nil {
		return
	}
	ch <- msg
}

func (m *Model) emitRunMsg(msg tea.Msg) {
	emitQueuedMsg(m.runMsgChan, msg)
}

func (m *Model) nextRunMsgCmd() tea.Cmd {
	if m.runMsgChan == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-m.runMsgChan
		if !ok {
			return nil
		}
		return msg
	}
}

func runSink(ch chan tea.Msg) core.Sink {
	return core.SinkFunc(func(_ context.Context, e core.Evt) error {
		emitQueuedMsg(ch, runEvtMsg{evt: e})
		return nil
	})
}

func (m *Model) startRunWorker(id string, fn func(context.Context) error) tea.Cmd {
	if fn == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.sendCancel = cancel
	ch := m.runMsgChan
	return func() tea.Msg {
		go func() {
			defer cancel()
			emitQueuedMsg(ch, runWorkerDoneMsg{runID: id, err: fn(ctx)})
		}()
		return nil
	}
}
