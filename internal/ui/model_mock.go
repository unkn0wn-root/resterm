package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/mock"
)

const (
	mockReloadInterval = time.Second
	mockCloseTimeout   = 3 * time.Second
	mockStatusSep      = " · "
)

type mockServerState struct {
	server      *mock.Server
	inspector   *mockInspector
	addr        string
	src         mock.Sources // scope of the running server
	reloader    mockReloader
	reloadErr   string
	gen         uint64 // newest scheduled reload generation
	inFlightGen uint64 // generation being reloaded, 0 when idle
	pending     *mockReloadRequest
}

// mockInspector resolves the live server at call time, so scripts from a run
// that outlives a :mock stop or restart never count against a stale journal.
type mockInspector struct {
	srv atomic.Pointer[mock.Server]
}

func (i *mockInspector) Count(ctx context.Context, pattern mock.RequestPattern) (uint64, error) {
	if s := i.srv.Load(); s != nil {
		return s.Count(ctx, pattern)
	}
	return 0, mock.ErrInspectorUnavailable
}

func (m *Model) mockInspector() *mockInspector {
	if m.mock.inspector == nil {
		m.mock.inspector = &mockInspector{}
	}
	return m.mock.inspector
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
	spec    mockStartSpec
	err     error
	restart bool
}

// mockStartSpec is a resolved start request. An empty addr falls back to the
// remembered address and an empty src to workspace scope.
type mockStartSpec struct {
	addr string
	src  mock.Sources
}

// resolveStartSpec inherits the scope in effect when the arguments name none,
// so a bare start behaves like the toggle key. The workspace recursion default
// applies to whole-directory scope only, because listed files already name the
// exact set to serve.
func (m *Model) resolveStartSpec(args mockStartArgs) (mockStartSpec, error) {
	if !args.scoped() {
		return mockStartSpec{addr: args.addr, src: m.mockSources()}, nil
	}
	src, err := mock.NewSources(m.mockRoot(), args.recursive, args.sources)
	if err != nil {
		return mockStartSpec{}, err
	}
	if len(src.Files) == 0 {
		src.Recursive = src.Recursive || m.ws.recursive
	}
	return mockStartSpec{addr: args.addr, src: src}, nil
}

func (m *Model) executeMockCommand(args []string) tea.Cmd {
	if len(args) == 0 {
		return statusCmd(statusInfo, m.mockStatus())
	}

	command := strings.ToLower(args[0])
	args = args[1:]
	def, ok := exCommands.Mock(command)
	if !ok {
		return statusCmd(
			statusWarn,
			"Unknown :mock command (use "+strings.Join(exCommands.mockNames(), ", ")+")",
		)
	}
	if def.tooManyArgs(len(args)) {
		return m.mockCommandUsage(def)
	}
	switch command {
	case "status":
		return statusCmd(statusInfo, m.mockStatus())
	case "start":
		return m.mockStartFromArgs(def, args)
	case "stop":
		return m.stopMockServer()
	case "restart":
		return m.mockRestartFromArgs(def, args)
	case "logs":
		return m.openMockLogs()
	case "clear":
		if m.mock.server == nil {
			return statusCmd(statusInfo, "Mock server is stopped")
		}
		m.mock.server.Clear()
		m.syncMockLogs()
		return statusCmd(statusInfo, "Mock request journal and logs cleared")
	case "reset":
		return m.resetMockSequences(args)
	case "verify":
		return m.verifyMockRequests()
	case "capture":
		return m.captureMockResponse()
	}
	return nil
}

func (m *Model) mockCommandUsage(def mockCommandDef) tea.Cmd {
	return statusCmd(statusWarn, "Usage: :mock "+def.usage())
}

func (m *Model) mockArgsError(def mockCommandDef, err error) tea.Cmd {
	if errors.Is(err, errMockArgsUsage) {
		return m.mockCommandUsage(def)
	}
	return statusCmd(statusWarn, ":mock "+def.name+": "+oneLine(err.Error()))
}

func (m *Model) toggleMockServer() tea.Cmd {
	if m.mock.server != nil {
		return m.stopMockServer()
	}
	return m.startMockServer(mockStartSpec{})
}

func (m *Model) mockStartFromArgs(def mockCommandDef, args []string) tea.Cmd {
	parsed, err := parseMockStartArgs(args)
	if err != nil {
		return m.mockArgsError(def, err)
	}
	if m.mock.server != nil {
		if (parsed.addr != "" && parsed.addr != m.mock.addr) || parsed.scoped() {
			return statusCmd(
				statusWarn,
				"Mock server is already running on "+m.mock.addr+". Use :mock restart "+strings.Join(args, " "),
			)
		}
		return statusCmd(statusInfo, m.mockStatus())
	}
	spec, err := m.resolveStartSpec(parsed)
	if err != nil {
		return m.mockArgsError(def, err)
	}
	return m.startMockServer(spec)
}

func (m *Model) mockRestartFromArgs(def mockCommandDef, args []string) tea.Cmd {
	parsed, err := parseMockStartArgs(args)
	if err != nil {
		return m.mockArgsError(def, err)
	}
	spec, err := m.resolveStartSpec(parsed)
	if err != nil {
		return m.mockArgsError(def, err)
	}
	if spec.addr == "" {
		spec.addr = m.mockAddress()
	}
	server := m.mock.server
	if server == nil {
		return m.startMockServer(spec)
	}
	m.detachMockServer(server)
	return closeMockServerCmd(server, spec, true)
}

func (m *Model) startMockServer(spec mockStartSpec) tea.Cmd {
	if m.mock.server != nil {
		return statusCmd(statusInfo, m.mockStatus())
	}
	addr := strings.TrimSpace(spec.addr)
	if addr == "" {
		addr = m.mockAddress()
	}
	src := spec.src
	if src.Path == "" {
		src = m.mockSources()
	}

	reloader := mock.NewReloader(src)
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
	m.mockInspector().srv.Store(server)
	m.mock.addr = server.Addr()
	m.mock.src = src
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
		text += mockStatusSep + warning
	}
	if !mock.IsLoopbackAddr(addr) {
		level = statusWarn
		text += mockStatusSep + "server is exposed beyond this machine"
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
	return closeMockServerCmd(server, mockStartSpec{addr: addr}, false)
}

func closeMockServerCmd(server *mock.Server, spec mockStartSpec, restart bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mockCloseTimeout)
		defer cancel()
		return mockServerClosedMsg{spec: spec, err: server.Close(ctx), restart: restart}
	}
}

func (m *Model) handleMockServerClosed(msg mockServerClosedMsg) tea.Cmd {
	if msg.err != nil {
		cmd := statusCmd(statusWarn, "Mock server stop failed: "+oneLine(msg.err.Error()))
		if msg.restart {
			return batchCommands(cmd, m.startMockServer(msg.spec))
		}
		return cmd
	}
	if msg.restart {
		return m.startMockServer(msg.spec)
	}
	return statusCmd(statusInfo, "Mock server stopped (last address "+msg.spec.addr+")")
}

func (m *Model) detachMockServer(server *mock.Server) {
	if server == nil || server != m.mock.server {
		return
	}
	m.mock.gen++
	m.mock.server = nil
	m.mock.inspector.srv.Store(nil)
	m.mock.reloader = nil
	m.mock.resetReload()
	m.showMockLogs = false
	m.closeMockVerification()
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
	if root := strings.TrimSpace(m.ws.root); root != "" {
		return root
	}
	return "."
}

// forgetMockScope drops a remembered scope. Its root and paths only mean
// something under the workspace that produced them.
func (m *Model) forgetMockScope() {
	m.mock.src = mock.Sources{}
}

// mockSources reports the scope a mock server would serve now. A scope outlives
// the server that introduced it, the way the address does, so a stop and start
// serves the same files as before. Naming a scope again or leaving the
// workspace replaces it. Before any of that the workspace is the scope.
func (m *Model) mockSources() mock.Sources {
	if m.mock.src.Path != "" {
		return m.mock.src
	}
	return mock.Sources{Path: m.mockRoot(), Recursive: m.ws.recursive}
}

func (m *Model) mockStatus() string {
	if m.mock.server == nil {
		return strings.Join(compactStrings(
			"Mock server stopped",
			"next address "+m.mockAddress(),
			mockSourceSummary(m.mock.src),
		), mockStatusSep)
	}
	stats := m.mock.server.Stats()
	parts := compactStrings(
		fmt.Sprintf(
			"Mock http://%s: %d routes, %d scenarios, %d calls",
			stats.Addr,
			stats.Routes,
			stats.Scenarios,
			stats.Calls,
		),
		mockSourceSummary(m.mock.src),
	)
	if m.mock.reloadErr != "" {
		parts = append(parts, "reload error: "+m.mock.reloadErr)
	}
	return strings.Join(parts, mockStatusSep)
}

// mockSourceSummary names a listed file scope for status text. Whole workspace
// scope adds nothing.
func mockSourceSummary(src mock.Sources) string {
	names := mockSourceNames(src)
	if len(names) == 0 {
		return ""
	}
	label := "source"
	if len(names) > 1 {
		label = "sources"
	}
	return label + " " + strings.Join(names, ", ")
}

func mockSourceNames(src mock.Sources) []string {
	names := make([]string, len(src.Files))
	for i, f := range src.Files {
		names[i] = mockSourceName(src.Path, f)
	}
	return names
}

func mockSourceName(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && filepath.IsLocal(rel) {
		return rel
	}
	return path
}

// headerMockVariants names the served scope for the header, widest first. Only
// the first file is named, and :mock status lists them all.
func headerMockVariants(src mock.Sources) []string {
	if len(src.Files) == 0 {
		if src.Recursive {
			return []string{"workspace/**"}
		}
		return []string{"workspace"}
	}

	first := mockSourceName(src.Path, src.Files[0])
	more := ""
	if rest := len(src.Files) - 1; rest > 0 {
		more = fmt.Sprintf(" +%d", rest)
	}
	if base := filepath.Base(first); base != first {
		return []string{first + more, base + more}
	}
	return []string{first + more}
}
