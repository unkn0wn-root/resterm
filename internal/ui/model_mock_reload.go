package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/mock"
)

type mockReloadRequest struct {
	server     *mock.Server
	generation uint64
	path       string
	overlay    []byte
}

type mockReloadResultMsg struct {
	server     *mock.Server
	generation uint64
	handler    *mock.Handler
	err        error
}

type mockReloadTickMsg struct {
	server     *mock.Server
	generation uint64
}

func (m *Model) scheduleMockReload(delay time.Duration) tea.Cmd {
	server := m.mock.server
	if server == nil {
		return nil
	}
	m.mock.gen++
	gen := m.mock.gen
	if delay > 0 {
		return tea.Tick(delay, func(time.Time) tea.Msg {
			return mockReloadTickMsg{server: server, generation: gen}
		})
	}
	req := mockReloadRequest{
		server: server, generation: gen,
		path: m.currentFile, overlay: []byte(m.editor.Value()),
	}
	if m.mock.inFlightGen != 0 {
		m.mock.pending = &req
		return nil
	}
	return m.startMockReload(req)
}

func (m *Model) startMockReload(req mockReloadRequest) tea.Cmd {
	// the closure runs on a goroutine and must not touch m
	reloader := m.mock.reloader
	m.mock.inFlightGen = req.generation
	return func() tea.Msg {
		handler, err := reloader.Reload(req.path, req.overlay)
		return mockReloadResultMsg{
			server: req.server, generation: req.generation, handler: handler, err: err,
		}
	}
}

func (m *Model) handleMockReloadTick(msg mockReloadTickMsg) tea.Cmd {
	if !m.currentMockReload(msg.server, msg.generation) {
		return nil
	}
	if m.showMockLogs {
		m.syncMockLogs()
	}
	return m.scheduleMockReload(0)
}

func (m *Model) handleMockReload(msg mockReloadResultMsg) tea.Cmd {
	if msg.server == nil || msg.server != m.mock.server || msg.generation != m.mock.inFlightGen {
		return nil
	}
	m.mock.inFlightGen = 0
	if pending := m.mock.pending; pending != nil {
		m.mock.pending = nil
		return m.startMockReload(*pending)
	}
	// no pending means nothing was scheduled while this reload ran (every
	// schedule during flight lands in pending), so this is the newest generation
	if msg.err != nil {
		m.recordMockReloadError(msg.err)
		return m.scheduleMockReload(mockReloadInterval)
	}

	m.mock.reloadErr = ""
	if msg.handler != nil {
		m.mock.server.Reload(msg.handler)
		m.setStatusMessage(statusMsg{
			text: fmt.Sprintf(
				"Mock routes reloaded (%d routes, %d scenarios)",
				msg.handler.Routes(),
				msg.handler.Scenarios(),
			),
			level: statusSuccess,
		})
	}
	return m.scheduleMockReload(mockReloadInterval)
}

func (m *Model) currentMockReload(server *mock.Server, generation uint64) bool {
	return server != nil && server == m.mock.server && generation == m.mock.gen
}

func (m *Model) recordMockReloadError(err error) {
	text := oneLine(err.Error())
	if text == m.mock.reloadErr {
		return
	}
	m.mock.reloadErr = text
	m.mock.server.RecordReload(err)
	m.setStatusMessage(statusMsg{
		text:    "Mock reload failed; serving last valid routes: " + text,
		level:   statusWarn,
		noModal: true,
	})
}
