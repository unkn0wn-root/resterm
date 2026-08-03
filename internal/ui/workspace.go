package ui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Shared wording so the messages and the tests asserting on them cannot drift.
const (
	moveRefused   = "Workspace not changed"
	noEnvSelected = "No environment selected"
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
	envPinned  bool
	intent     vars.Intent
	active     vars.Environment
	unselected bool
}

func newWorkspace(root string, recursive bool, env vars.Config) workspace {
	w := workspace{
		root:      root,
		recursive: recursive,
		cat:       env.Catalog,
		sel:       env.Selection,
		envFile:   env.File,
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
	root       string
	cat        vars.Catalog
	sel        vars.Selection
	envFile    string
	unselected bool
	reset      bool
	status     statusMsg
}

// plan works out what a move to root would activate, without touching the
// current state. Discovery searches only the new root, and a root whose
// environment file does not load refuses the move the same way launching
// there would.
func (w workspace) plan(root string) (wsMove, error) {
	text := fmt.Sprintf("Workspace set to %s", filepath.Base(root))

	if w.envPinned {
		mv := wsMove{
			root:       root,
			cat:        w.cat,
			sel:        w.sel,
			envFile:    w.envFile,
			unselected: w.unselected,
			status:     statusMsg{text: text, level: statusInfo},
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
		root:    root,
		cat:     cat,
		envFile: envFile,
		reset:   !w.sameEnv(root, envFile),
		status:  statusMsg{text: text, level: statusInfo},
	}
	if cat.Empty() {
		return mv, nil
	}

	sel, ok := w.intent.Resolve(cat)
	if !ok {
		// Falling back to the new default could promote a dev session to
		// prod, so nothing is active until someone picks.
		mv.unselected = true
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

	mv.sel = sel
	if env, err := cat.Resolve(sel); err == nil {
		mv.status = statusMsg{
			text:  fmt.Sprintf("%s. Environment %s", text, env.Label()),
			level: statusInfo,
		}
	}
	return mv, nil
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
	return statusCmd(statusWarn, "Finish or cancel the running request before changing workspace")
}

func moveRefusedCmd(err error) tea.Cmd {
	return statusCmd(statusError, fmt.Sprintf("%s. %v", moveRefused, err))
}

// prepareMove runs every check a move must pass before anything is committed.
// A non nil cmd is the refusal to show, with the session untouched.
func (m *Model) prepareMove(dir, current string) (wsMove, []filesvc.FileEntry, tea.Cmd) {
	if cmd := m.moveBlocked(); cmd != nil {
		return wsMove{}, nil, cmd
	}
	mv, err := m.ws.plan(dir)
	if err != nil {
		return wsMove{}, nil, moveRefusedCmd(err)
	}
	entries, err := listWorkspaceEntries(dir, m.ws.recursive, mv.envFile, current, nil)
	if err != nil {
		return wsMove{}, nil, moveRefusedCmd(err)
	}

	// Surface the same environment file warnings launching with -w would,
	// unless the move already has something more specific to say.
	if mv.status.level == statusInfo {
		next := workspace{root: dir, recursive: m.ws.recursive, cat: mv.cat, envFile: mv.envFile}
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
	m.ws.cat = mv.cat
	m.ws.sel = mv.sel
	m.ws.envFile = mv.envFile
	m.ws.unselected = mv.unselected
	if mv.unselected {
		m.ws.active = vars.Environment{}
	} else {
		m.ws.active, _ = mv.cat.Resolve(mv.sel)
	}

	// Runs re-derive their base directory from the file they execute.
	m.cfg.HTTPOptions.BaseDir = ""

	m.envDraft = vars.Selection{}
	m.envList.ResetFilter()
	m.envList.SetItems(makeEnvItems(mv.cat, mv.sel))
	m.envList.SetDelegate(envDelegateForTheme(m.theme, mv.cat))
	// Document-derived state is the caller's to rebuild. Only it knows whether
	// the move clears the document or installs a new one.
	return mv.status, stop
}

// stopLiveStreams cancels every live session and drops their consoles, so a
// stream opened in one workspace cannot keep writing into the next.
func (m *Model) stopLiveStreams() {
	for _, s := range m.sessionHandles {
		s.Cancel()
	}
	m.sessionHandles = make(map[string]*stream.Session)
	m.liveSessions = make(map[string]*liveSession)
	m.wsSenders = make(map[string]*httpclient.WebSocketSender)
	m.wsConsole = nil
	m.requestSessions = make(map[*restfile.Request]string)
	m.sessionRequests = make(map[string]*restfile.Request)
	m.requestKeySessions = make(map[string]string)
}

// clearResponseState drops everything the previous workspace's requests
// produced. The engine seeds scripts with the last response, so leaving any of
// it in place lets one workspace read another's traffic.
func (m *Model) clearResponseState() {
	if m.responseRenderCancel != nil {
		m.responseRenderCancel()
		m.responseRenderCancel = nil
	}
	m.cancelResponseReflow()
	m.responseLoading = false
	m.responseRenderToken = ""
	m.respTasks = newRespTasks()

	m.lastResponse = nil
	m.lastGRPC = nil
	m.lastError = nil
	m.testResults = nil
	m.scriptError = nil
	m.responseLatest = nil
	m.responsePrevious = nil
	m.responsePending = nil
	m.responseTokens = make(map[string]*responseSnapshot)
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
