package ui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Shared wording so the messages and the tests asserting on them cannot drift.
const (
	moveRefused   = "Workspace not changed"
	noEnvSelected = "No environment selected"
	envFixHint    = "Fix and save the file to load its environments"
)

// workspace is the live workspace state. A move goes through plan, which
// touches nothing, and commitMove, which applies everything at once, so a
// failed move leaves the session exactly as it was.
type workspace struct {
	root       string
	recursive  bool
	cat        vars.Catalog
	sel        vars.Selection
	envFile    string
	envErr     error
	envPinned  bool
	intent     vars.Intent
	active     vars.Environment
	unselected bool
}

type envState struct {
	cat        vars.Catalog
	sel        vars.Selection
	envFile    string
	envErr     error
	unselected bool
}

func newWorkspace(root string, recursive bool, env vars.Config) workspace {
	w := workspace{
		root:      root,
		recursive: recursive,
		cat:       env.Catalog,
		sel:       env.Selection,
		envFile:   env.File,
		envErr:    env.FileErr,
		envPinned: env.FileExplicit,
		intent:    env.Intent,
	}
	w.active, _ = w.cat.Resolve(w.sel)
	return w
}

// use installs env as the active selection. The applied choice becomes the
// intent a later workspace change replays.
func (w *workspace) use(env vars.Environment) {
	w.sel = env.Selection()
	w.active = env
	w.intent = w.cat.IntentFor(env)
	w.unselected = false
}

type wsMove struct {
	root   string
	env    envState
	reset  bool
	status statusMsg
}

// plan validates a workspace move without changing the current state. A broken
// environment file causes the move to fail.
func (w workspace) plan(root string) (wsMove, error) {
	text := fmt.Sprintf("Workspace set to %s", filepath.Base(root))

	if w.envPinned {
		mv := wsMove{
			root: root,
			env: envState{
				cat:        w.cat,
				sel:        w.sel,
				envFile:    w.envFile,
				envErr:     w.envErr,
				unselected: w.unselected,
			},
			status: statusMsg{text: text, level: statusInfo},
		}
		if own := vars.DiscoverPath(root); own != "" && !util.SameFile(own, w.envFile) {
			mv.status = statusMsg{
				text:  text + ". Keeping the environment file passed with --env-file",
				level: statusWarn,
			}
		}
		return mv, nil
	}

	cat, envFile, err := vars.Discover(root)
	if err != nil {
		return wsMove{}, fmt.Errorf("environment file failed to load: %w", err)
	}

	mv := wsMove{
		root:   root,
		env:    envState{cat: cat, envFile: envFile},
		reset:  !w.sameEnv(root, envFile),
		status: statusMsg{text: text, level: statusInfo},
	}
	if cat.Empty() {
		return mv, nil
	}

	sel, ok := w.intent.Resolve(cat)
	if !ok {
		// Falling back to the new default could promote a dev session to
		// prod, so nothing is active until someone picks.
		mv.env.unselected = true
		mv.status = statusMsg{
			text: fmt.Sprintf(
				"%s. %s, %s is not available here",
				text,
				noEnvSelected,
				w.intent.Describe(),
			),
			level: statusWarn,
		}
		return mv, nil
	}

	mv.env.sel = sel
	if env, err := cat.Resolve(sel); err == nil {
		mv.status = statusMsg{
			text:  fmt.Sprintf("%s. Environment %s", text, env.Label()),
			level: statusInfo,
		}
	}
	return mv, nil
}

func (w workspace) ownsEnvFile(path string) bool {
	if w.envFile != "" {
		return sameEnvFile(path, w.envFile)
	}
	own := vars.DiscoverPath(w.root)
	return own != "" && util.SameFile(path, own)
}

// reloadEnv reads the environment file and reapplies the current selection
// intent. An unavailable selection never falls back to the file's default.
func (w workspace) reloadEnv() (envState, statusMsg, error) {
	cat, envFile, err := w.loadEnv()
	if err != nil {
		return envState{}, statusMsg{}, err
	}

	next := envState{cat: cat, envFile: envFile}
	if cat.Empty() {
		return next, statusMsg{text: "No environments loaded", level: statusWarn}, nil
	}

	sel, ok := w.intent.Resolve(cat)
	if !ok {
		next.unselected = true
		return next, statusMsg{
			text:  fmt.Sprintf("%s, %s is not available here", noEnvSelected, w.intent.Describe()),
			level: statusWarn,
		}, nil
	}

	next.sel = sel
	env, _ := cat.Resolve(sel)
	return next, statusMsg{text: "Environment " + env.Label(), level: statusInfo}, nil
}

// loadEnv reloads the active file. It uses discovery only before a file is
// active because an explicit file may be outside the workspace.
func (w workspace) loadEnv() (vars.Catalog, string, error) {
	if w.envFile != "" {
		cat, err := vars.LoadEnvironmentFile(w.envFile)
		return cat, w.envFile, err
	}
	return vars.Discover(w.root)
}

// sameEnv reports whether a move to root keeps the environment the runtime
// values were captured under. Two workspaces without an environment file share
// nothing nameable, so a move between them still crosses a boundary and resets,
// but reopening the same root is not a crossing at all.
func (w workspace) sameEnv(root, envFile string) bool {
	if sameEnvFile(envFile, w.envFile) {
		return true
	}
	return envFile == "" && w.envFile == "" && util.SameFile(root, w.root)
}

func sameEnvFile(a, b string) bool {
	return a != "" && b != "" && util.SameFile(a, b)
}

// moveBlocked refuses a move while a request is in flight, because that
// request can still write runtime values back and undo the reset.
func (m *Model) moveBlocked() tea.Cmd {
	if !m.hasActiveRun() {
		return nil
	}
	return statusCmd(statusWarn, "Finish or cancel the running request first")
}

func moveRefusedCmd(err error) tea.Cmd {
	return statusCmd(statusError, fmt.Sprintf("%s. %v", moveRefused, err))
}

// prepareMove runs every check a move must pass before anything is committed.
// A non nil cmd is the refusal to show, with the session untouched.
func (m *Model) prepareMove(dir, current string) (wsMove, []files.Entry, tea.Cmd) {
	if cmd := m.moveBlocked(); cmd != nil {
		return wsMove{}, nil, cmd
	}
	mv, err := m.ws.plan(dir)
	if err != nil {
		return wsMove{}, nil, moveRefusedCmd(err)
	}
	entries, err := listWorkspaceEntries(dir, m.ws.recursive, mv.env.envFile, current, nil)
	if err != nil {
		return wsMove{}, nil, moveRefusedCmd(err)
	}

	// Surface the same environment file warnings launching with -w would,
	// unless the move already has something more specific to say.
	if mv.status.level == statusInfo {
		next := workspace{
			root:      dir,
			recursive: m.ws.recursive,
			cat:       mv.env.cat,
			envFile:   mv.env.envFile,
		}
		if warn := envFileWarning(entries, next); warn.text != "" {
			mv.status = statusMsg{text: mv.status.text + ". " + warn.text, level: statusWarn}
		}
	}
	return mv, entries, nil
}

// commitMove applies a planned move in one step. The returned command finishes
// teardown that has to happen off the update loop.
func (m *Model) commitMove(mv wsMove) (statusMsg, tea.Cmd) {
	var stop tea.Cmd
	if server := m.mock.server; server != nil {
		// The reloader walks the old root and would keep serving its mocks
		// against the new editor.
		addr := m.mockAddress()
		m.detachMockServer(server)
		stop = closeMockServerCmd(server, mockStartSpec{addr: addr}, false)
	}
	m.forgetMockScope()

	if mv.reset {
		if rt := m.runtimeSvc(); rt != nil {
			rt.ResetSharedSecrets()
		}
	}
	m.stopLiveStreams()
	m.clearResponseState()

	m.ws.root = mv.root
	m.applyEnv(mv.env)

	// Runs re-derive their base directory from the file they execute.
	m.cfg.HTTPOptions.BaseDir = ""

	// Document-derived state is the caller's to rebuild. Only it knows whether
	// the move clears the document or installs a new one.
	return mv.status, stop
}

func (m *Model) applyEnv(next envState) {
	m.ws.cat = next.cat
	m.ws.sel = next.sel
	m.ws.envFile = next.envFile
	m.ws.envErr = next.envErr
	m.ws.unselected = next.unselected
	// Resolve would choose the default when the requested selection is
	// unavailable, so leave the active environment empty in that case.
	m.ws.active = vars.Environment{}
	if !next.unselected {
		m.ws.active, _ = next.cat.Resolve(next.sel)
	}

	m.envDraft = vars.Selection{}
	m.envList.ResetFilter()
	m.envList.SetItems(makeEnvItems(next.cat, next.sel))
	m.envList.SetDelegate(envDelegateForTheme(m.theme, next.cat))
}

// reloadEnvFile reloads path when it is the active environment file. If the
// reload fails after a successful load, the previous catalog stays active. If
// no catalog has loaded yet, the error is stored and requests remain blocked.
func (m *Model) reloadEnvFile(path string) statusMsg {
	if !m.ws.ownsEnvFile(path) {
		return statusMsg{}
	}

	next, status, err := m.ws.reloadEnv()
	if err != nil {
		if m.ws.cat.Empty() {
			m.ws.envErr = err
		}
		return statusMsg{text: envLoadFailed(err), level: statusError}
	}

	m.applyEnv(next)
	m.syncRequestList(m.doc)
	m.syncHistory()
	return status
}

// stopLiveStreams cancels every live session and drops their consoles, so a
// stream opened in one workspace cannot keep writing into the next.
func (m *Model) stopLiveStreams() {
	m.streamGen++
	for _, s := range m.sessionHandles {
		s.Cancel()
	}
	m.sessionHandles = make(map[string]*stream.Session)
	m.liveSessions = make(map[string]*liveSession)
	m.wsSenders = make(map[string]*httpx.WebSocketSender)
	m.wsConsole = nil
	m.requestSessions = make(map[*restfile.Request]string)
	m.sessionRequests = make(map[string]*restfile.Request)
	m.requestKeySessions = make(map[string]string)
}

// clearResponseState drops everything the previous workspace's requests
// produced. The engine seeds scripts with the last response, so leaving any of
// it in place lets one workspace read another's traffic.
func (m *Model) clearResponseState() {
	m.resetPendingResponse()
	m.cancelResponseReflow()
	m.respTasks = newRespTasks()

	m.lastResponse = nil
	m.lastGRPC = nil
	m.lastError = nil
	m.testResults = nil
	m.scriptError = nil
	m.responseLatest = nil
	m.responsePrevious = nil
	m.compareBundle = nil
	m.resetCompareState()
	m.latencySeries.reset()
	m.resetResponsePanes()
}

// resetResponsePanes rebuilds both panes empty. Layout is a view preference
// and stays, content goes.
func (m *Model) resetResponsePanes() {
	for i := range m.responsePanes {
		vp := m.responsePanes[i].viewport
		vp.SetContent(logoPlaceholder(vp.Width, vp.Height))
		m.responsePanes[i] = newResponsePaneState(vp, i == 0)
	}
	m.setLivePane(responsePanePrimary)
}
