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

// Shared wording. Every aborted move starts with moveRefused so the guarantee
// it stands for, nothing was committed, reads the same everywhere; noEnvSelected
// opens both messages the unselected state produces.
const (
	moveRefused   = "Workspace not changed"
	noEnvSelected = "No environment selected"
)

// workspace is the live workspace state: the root being browsed and the
// environment its requests run against. A move goes through plan, which touches
// nothing, and commitMove, which applies everything at once, so a failed move
// leaves the session exactly as it was.
type workspace struct {
	root      string
	recursive bool
	cat       vars.Catalog
	sel       vars.Selection
	envFile   string
	// envPinned marks an --env-file environment, which names the environment
	// for the session rather than for one workspace and so outlives moves.
	envPinned bool
	intent    vars.Intent
	active    vars.Environment
	// unselected means the workspace has environments but none is active,
	// because the session's intent does not resolve here. Runs are refused
	// until someone picks.
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

// wsMove is a planned workspace change. A half applied move is what would leave
// one workspace's credentials pointed at another's requests, so nothing here
// reaches the model until commitMove.
type wsMove struct {
	root       string
	cat        vars.Catalog
	sel        vars.Selection
	envFile    string
	unselected bool
	reset      bool
	status     statusMsg
}

// plan works out the environment a move to root would activate, without
// touching the current state. Discovery searches only the new root: reaching
// further, into the directory the session was launched from, is how a workspace
// without an environment file ends up running another one's credentials. A root
// whose environment file does not load returns an error and no move: entering it
// anyway would run its requests with no environment at all, which launching with
// -w would have refused.
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
		reset:   !sameEnvFile(envFile, w.envFile),
		status:  statusMsg{text: text, level: statusInfo},
	}
	if cat.Empty() {
		return mv, nil
	}

	sel, ok := w.intent.Resolve(cat)
	if !ok {
		// This workspace does not have what the session asked for. Falling back
		// to its default could promote a dev session to prod, so nothing is
		// active until someone picks.
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

// sameEnvFile reports whether two workspaces read their environment from one
// file on disk. Two workspaces without a file share nothing nameable, so a move
// between them still crosses a boundary and resets.
func sameEnvFile(a, b string) bool {
	return a != "" && b != "" && util.SameFile(a, b)
}

// moveBlocked refuses a workspace change while a request is in flight, because
// that request can still write runtime values back and undo the reset.
func (m *Model) moveBlocked() tea.Cmd {
	if !m.hasActiveRun() {
		return nil
	}
	return statusCmd(statusWarn, "Finish or cancel the running request before changing workspace")
}

func moveRefusedCmd(err error) tea.Cmd {
	return statusCmd(statusError, fmt.Sprintf("%s. %v", moveRefused, err))
}

// prepareMove runs every check a workspace change must pass before anything is
// committed: no run in flight, a loadable environment, a listable root. A non
// nil cmd is the refusal to show, with the session untouched.
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
	return mv, entries, nil
}

// commitMove applies a planned move in one step: forget what the previous
// workspace produced, then install the new root and environment together. The
// returned command finishes teardown that has to happen off the update loop,
// currently closing a mock server still rooted in the old workspace.
func (m *Model) commitMove(mv wsMove) (statusMsg, tea.Cmd) {
	var stop tea.Cmd
	if server := m.mock.server; server != nil {
		// The reloader walks the old root and would keep serving its mocks
		// against the new editor. Detaching first bumps the reload generation,
		// so results and ticks already in flight are dropped.
		addr := m.mockAddress()
		m.detachMockServer(server)
		stop = closeMockServerCmd(server, addr, false)
	}

	if mv.reset {
		if rt := m.runtimeSvc(); rt != nil {
			rt.ResetSecrets()
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

	// The launch file's directory stops meaning anything under another root.
	// Each run derives its base from the file it executes.
	m.cfg.HTTPOptions.BaseDir = ""

	m.envDraft = vars.Selection{}
	m.envList.ResetFilter()
	m.envList.SetItems(makeEnvItems(mv.cat, mv.sel))
	m.envList.SetDelegate(envDelegateForTheme(m.theme, mv.cat))
	m.refreshCompletionScope()
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
// produced: the engine seeds scripts with the last response, and pane snapshots
// keep bodies renderable and exportable, so leaving either in place lets one
// workspace read another's traffic.
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

// resetResponsePanes rebuilds both panes empty while keeping the layout the
// user arranged: sizes, split and orientation are view preferences, snapshots
// and caches are content.
func (m *Model) resetResponsePanes() {
	for i := range m.responsePanes {
		vp := m.responsePanes[i].viewport
		vp.SetContent(logoPlaceholder(vp.Width, vp.Height))
		m.responsePanes[i] = newResponsePaneState(vp, i == 0)
	}
	m.setLivePane(responsePanePrimary)
}
