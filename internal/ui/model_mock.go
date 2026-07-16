package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/mock"
)

const (
	mockReloadInterval = time.Second
	mockCloseTimeout   = 3 * time.Second
)

type mockServerState struct {
	server      *mock.Server
	addr        string
	reloader    mockReloader
	reloadErr   string
	gen         uint64 // newest scheduled reload generation
	inFlightGen uint64 // generation being reloaded, 0 when idle
	pending     *mockReloadRequest
}

func (s *mockServerState) resetReload() {
	s.reloadErr = ""
	s.inFlightGen = 0
	s.pending = nil
}

type mockReloader interface {
	Reload(path string, overlay []byte) (*mock.Handler, error)
}

type mockServerDoneMsg struct {
	server *mock.Server
	err    error
}

type mockServerClosedMsg struct {
	addr    string
	err     error
	restart bool
}

func (m *Model) executeMockCommand(args []string) tea.Cmd {
	if len(args) == 0 {
		return statusCmd(statusInfo, m.mockStatus())
	}

	command := strings.ToLower(args[0])
	args = args[1:]
	switch command {
	case "status":
		if len(args) != 0 {
			return mockCommandUsage("status")
		}
		return statusCmd(statusInfo, m.mockStatus())
	case "start":
		if len(args) > 1 {
			return mockCommandUsage("start [host:port]")
		}
		addr := ""
		if len(args) == 1 {
			addr = args[0]
		}
		return m.startMockServer(addr)
	case "stop":
		if len(args) != 0 {
			return mockCommandUsage("stop")
		}
		return m.stopMockServer()
	case "restart":
		if len(args) > 1 {
			return mockCommandUsage("restart [host:port]")
		}
		addr := m.mockAddress()
		if len(args) == 1 {
			addr = args[0]
		}
		server := m.mock.server
		if server == nil {
			return m.startMockServer(addr)
		}
		m.detachMockServer(server)
		return closeMockServerCmd(server, addr, true)
	case "logs":
		if len(args) != 0 {
			return mockCommandUsage("logs")
		}
		return m.openMockLogs()
	case "clear":
		if len(args) != 0 {
			return mockCommandUsage("clear")
		}
		if m.mock.server == nil {
			return statusCmd(statusInfo, "Mock server is stopped")
		}
		m.mock.server.ClearLogs()
		m.syncMockLogs()
		return statusCmd(statusInfo, "Mock request log cleared")
	case "capture":
		if len(args) != 0 {
			return mockCommandUsage("capture")
		}
		return m.captureMockResponse()
	default:
		return statusCmd(
			statusWarn,
			"Unknown :mock command (use start, stop, restart, status, logs, clear, or capture)",
		)
	}
}

func mockCommandUsage(usage string) tea.Cmd {
	return statusCmd(statusWarn, "Usage: :mock "+usage)
}

func (m *Model) toggleMockServer() tea.Cmd {
	if m.mock.server != nil {
		return m.stopMockServer()
	}
	return m.startMockServer("")
}

func (m *Model) startMockServer(addr string) tea.Cmd {
	addr = strings.TrimSpace(addr)
	if m.mock.server != nil {
		if addr != "" && addr != m.mock.addr {
			return statusCmd(
				statusWarn,
				"Mock server is already running on "+m.mock.addr+"; use :mock restart "+addr,
			)
		}
		return statusCmd(statusInfo, m.mockStatus())
	}
	if addr == "" {
		addr = m.mockAddress()
	}

	reloader := mock.NewReloader(m.mockRoot(), m.workspaceRecursive)
	handler, err := reloader.Reload(m.currentFile, []byte(m.editor.Value()))
	if err != nil {
		return mockStartError(err)
	}
	if handler.Routes() == 0 {
		return mockStartError(errors.New("no # @mock scenarios found"))
	}
	cors, warning, err := mock.ResolveCORS("auto", addr)
	if err != nil {
		return mockStartError(err)
	}

	server, err := mock.Start(addr, handler, mock.Options{CORS: cors, Logs: mock.DefaultLogs})
	if err != nil {
		return mockStartError(err)
	}

	m.mock.server = server
	m.mock.addr = server.Addr()
	m.mock.reloader = reloader
	m.mock.resetReload()
	m.syncMockLogs()

	text := fmt.Sprintf(
		"Mock server listening on http://%s (%d routes, %d scenarios)",
		server.Addr(),
		handler.Routes(),
		handler.Scenarios(),
	)
	level := statusSuccess
	if warning != "" {
		level = statusWarn
		text += "; " + warning
	}
	if !mock.IsLoopbackAddr(addr) {
		level = statusWarn
		text += "; server is exposed beyond this machine"
	}

	return batchCommands(
		statusCmd(level, text),
		m.scheduleMockReload(mockReloadInterval),
		waitMockServerDone(server),
	)
}

func mockStartError(err error) tea.Cmd {
	return statusCmd(statusWarn, "Mock server not started: "+oneLine(err.Error()))
}

func waitMockServerDone(server *mock.Server) tea.Cmd {
	return func() tea.Msg {
		<-server.Done()
		return mockServerDoneMsg{server: server, err: server.Err()}
	}
}

func (m *Model) handleMockServerDone(msg mockServerDoneMsg) tea.Cmd {
	if msg.server != m.mock.server {
		return nil
	}
	m.detachMockServer(msg.server)
	text := "Mock server stopped unexpectedly"
	if msg.err != nil {
		text += ": " + oneLine(msg.err.Error())
	}
	return statusCmd(statusError, text)
}

func (m *Model) stopMockServer() tea.Cmd {
	server := m.mock.server
	if server == nil {
		return statusCmd(statusInfo, "Mock server is already stopped")
	}
	addr := m.mockAddress()
	m.detachMockServer(server)
	return closeMockServerCmd(server, addr, false)
}

func closeMockServerCmd(server *mock.Server, addr string, restart bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mockCloseTimeout)
		defer cancel()
		return mockServerClosedMsg{addr: addr, err: server.Close(ctx), restart: restart}
	}
}

func (m *Model) handleMockServerClosed(msg mockServerClosedMsg) tea.Cmd {
	if msg.err != nil {
		cmd := statusCmd(statusWarn, "Mock server stop failed: "+oneLine(msg.err.Error()))
		if msg.restart {
			return batchCommands(cmd, m.startMockServer(msg.addr))
		}
		return cmd
	}
	if msg.restart {
		return m.startMockServer(msg.addr)
	}
	return statusCmd(statusInfo, "Mock server stopped (last address "+msg.addr+")")
}

func (m *Model) detachMockServer(server *mock.Server) {
	if server == nil || server != m.mock.server {
		return
	}
	m.mock.gen++
	m.mock.server = nil
	m.mock.reloader = nil
	m.mock.resetReload()
	m.showMockLogs = false
}

func (m *Model) Close() error {
	server := m.mock.server
	if server == nil {
		return nil
	}
	m.detachMockServer(server)
	ctx, cancel := context.WithTimeout(context.Background(), mockCloseTimeout)
	defer cancel()
	return server.Close(ctx)
}

func (m *Model) activeMockServer() *mock.Server { return m.mock.server }

func (m *Model) mockAddress() string {
	if m.mock.addr == "" {
		return mock.DefaultAddr
	}
	return m.mock.addr
}

func (m *Model) mockRoot() string {
	if root := strings.TrimSpace(m.workspaceRoot); root != "" {
		return root
	}
	return "."
}

func (m *Model) mockStatus() string {
	if m.mock.server == nil {
		return "Mock server stopped; next address " + m.mockAddress()
	}
	stats := m.mock.server.Stats()
	text := fmt.Sprintf(
		"Mock http://%s: %d routes, %d scenarios, %d calls",
		stats.Addr,
		stats.Routes,
		stats.Scenarios,
		stats.Calls,
	)
	if m.mock.reloadErr != "" {
		text += "; reload error: " + m.mock.reloadErr
	}
	return text
}
