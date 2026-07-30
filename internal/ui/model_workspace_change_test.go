package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/util"
)

// A and B both name their environments dev and prod, which is what makes their
// runtime scopes collide. C and F have no environment file, D's cannot be
// parsed, and E has only staging, so neither dev nor prod can be replayed there.
func workspaceFixture(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	request := "### r\n# @name r\nGET http://example.test/{{who}}\n"
	write(
		"A/resterm.env.json",
		`{"dev":{"who":"A-dev","auth.token":"A-DEV"},"prod":{"who":"A-prod","auth.token":"A-PROD"}}`,
	)
	write("A/req.http", request)
	write(
		"B/resterm.env.json",
		`{"dev":{"who":"B-dev","auth.token":"B-DEV"},"prod":{"who":"B-prod","auth.token":"B-PROD"}}`,
	)
	write("B/req.http", request)
	write("C/req.http", request)
	write("D/resterm.env.json", `{"dev": "not an object"}`)
	write("D/req.http", request)
	write("E/resterm.env.json", `{"staging":{"who":"E","auth.token":"E-STAGING"}}`)
	write("E/req.http", request)
	write("F/req.http", request)
	return base
}

// workspaceModel starts a session in workspace A through the real command line
// wiring. Building ui.Config by hand here is what once hid a --env value that
// never reached the model.
func workspaceModel(t *testing.T, base, envName, explicitEnvFile string) Model {
	t.Helper()
	root := filepath.Join(base, "A")
	flags := cli.NewExecFlags()
	flags.EnvName = envName
	flags.EnvFile = explicitEnvFile
	flags.Workspace = root
	cfg, err := flags.Resolve("")
	if err != nil {
		t.Fatalf("resolve flags: %v", err)
	}
	return New(Config{Env: cfg.Env, WorkspaceRoot: cfg.Workspace, Recursive: cfg.Recursive})
}

// firstStatus finds the status a command carries. Status reaches Update either
// bare or wrapped in an editorEvent, depending on which helper produced it, and a
// test that only knows one shape silently reports "no status message".
func firstStatus(cmd tea.Cmd) (statusMsg, bool) {
	if cmd == nil {
		return statusMsg{}, false
	}
	switch msg := cmd().(type) {
	case statusMsg:
		return msg, true
	case editorEvent:
		if msg.status != nil {
			return *msg.status, true
		}
	case tea.BatchMsg:
		for _, sub := range msg {
			if status, ok := firstStatus(sub); ok {
				return status, true
			}
		}
	}
	return statusMsg{}, false
}

// The environment moves with the workspace, and what carries across is the
// environment that was asked for rather than the one that was resolved. Falling
// back to the new workspace's default could promote a dev session to prod.
func TestOpenWorkspaceReplaysSelectionIntent(t *testing.T) {
	for _, tc := range []struct{ envName, label, token string }{
		{envName: "", label: "dev", token: "B-DEV"},
		{envName: "prod", label: "prod", token: "B-PROD"},
	} {
		t.Run("env "+tc.envName, func(t *testing.T) {
			base := workspaceFixture(t)
			m := workspaceModel(t, base, tc.envName, "")

			status, ok := firstStatus(m.applyOpenDirectory(filepath.Join(base, "B")))
			if !ok {
				t.Fatal("no status message")
			}
			if status.level != statusInfo {
				t.Fatalf("level = %v, want info: %q", status.level, status.text)
			}
			if got := m.ws.active.Label(); got != tc.label {
				t.Fatalf("label = %q, want %q", got, tc.label)
			}
			if got := m.ws.active.Values()["auth.token"]; got != tc.token {
				t.Fatalf("token = %q, want the new workspace's %q", got, tc.token)
			}
			if !strings.Contains(m.ws.envFile, filepath.Join("B", "resterm.env.json")) {
				t.Fatalf("environment file = %q, want B's", m.ws.envFile)
			}
		})
	}
}

// The picker choice becomes the intent, so it is what a later move replays.
func TestOpenWorkspaceReplaysPickerChoice(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")

	m.openEnvironmentSelector()
	for i, item := range m.envList.Items() {
		if env, ok := item.(envItem); ok && env.name == "prod" {
			m.envList.Select(i)
		}
	}
	m.applyEnvironmentSelection()
	if got := m.ws.active.Label(); got != "prod" {
		t.Fatalf("label = %q, want the picked environment", got)
	}

	m.applyOpenDirectory(filepath.Join(base, "B"))
	if got := m.ws.active.Label(); got != "prod" {
		t.Fatalf("label = %q, want the picked choice to carry across", got)
	}
}

// Scopes name their environment file, so A and B no longer collide. The reset is
// the second layer: nothing the previous workspace stored stays resident,
// whatever it was keyed under. The last response goes too, because the engine
// seeds scripts with it.
func TestOpenWorkspaceForgetsPreviousState(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")

	scope := m.ws.active.Scope()
	m.globalsStore().Set(scope, "leaked.token", "A-DEV", true)
	m.lastResponse = &httpclient.Response{Status: "200 OK"}
	if m.cookieStore().Jar(scope) == nil {
		t.Fatal("expected a cookie jar for the active scope")
	}

	m.applyOpenDirectory(filepath.Join(base, "B"))

	if got := m.ws.active.Scope(); got == scope {
		t.Fatalf("scope = %q, want the two workspaces to differ", got)
	}
	if snap := m.globalsStore().Snapshot(scope); len(snap) != 0 {
		t.Fatalf("globals survived the workspace change: %v", snap)
	}
	if got := m.globalsStore().Entries(); len(got) != 0 {
		t.Fatalf("globals survived under some other scope: %v", got)
	}
	if m.lastResponse != nil {
		t.Fatalf("last response survived the move: %v", m.lastResponse.Status)
	}
}

// Reopening the same workspace resolves the same environment file, so there is
// nothing scoped to forget.
func TestReopeningSameWorkspaceKeepsRuntimeState(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")

	scope := m.ws.active.Scope()
	m.globalsStore().Set(scope, "captured.token", "KEEP", false)

	m.applyOpenDirectory(filepath.Join(base, "A"))

	if snap := m.globalsStore().Snapshot(scope); len(snap) == 0 {
		t.Fatal("reopening the same workspace discarded runtime state")
	}
}

func TestOpenWorkspaceWithoutEnvironmentClearsIt(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")

	status, _ := firstStatus(m.applyOpenDirectory(filepath.Join(base, "C")))
	if status.level != statusInfo {
		t.Fatalf("level = %v, want info: %q", status.level, status.text)
	}
	if got := m.ws.active.Values()["auth.token"]; got != "" {
		t.Fatalf("token = %q, want the previous credentials gone", got)
	}
	if m.ws.envFile != "" {
		t.Fatalf("environment file = %q, want none", m.ws.envFile)
	}
}

// A broken environment file refuses the move outright, the same way launching
// with -w there would. Entering anyway would run requests with no environment.
func TestOpenWorkspaceWithBrokenEnvironmentRefusesMove(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")

	status, ok := firstStatus(m.applyOpenDirectory(filepath.Join(base, "D")))
	if !ok {
		t.Fatal("no status message")
	}
	if status.level != statusError || !strings.Contains(status.text, "not changed") {
		t.Fatalf("status = %q (level %v), want a refusal", status.text, status.level)
	}
	if filepath.Base(m.ws.root) != "A" {
		t.Fatalf("workspace moved to %q despite the broken environment", m.ws.root)
	}
	if got := m.ws.active.Values()["auth.token"]; got != "A-DEV" {
		t.Fatalf("token = %q, want A's environment still fully active", got)
	}
	if cmd := m.runBlocked(); cmd != nil {
		t.Fatal("staying in A must keep runs allowed")
	}
}

func TestOpenWorkspaceKeepsExplicitEnvFile(t *testing.T) {
	base := workspaceFixture(t)
	explicit := filepath.Join(base, "A", "resterm.env.json")
	m := workspaceModel(t, base, "", explicit)

	status, ok := firstStatus(m.applyOpenDirectory(filepath.Join(base, "B")))
	if !ok {
		t.Fatal("no status message")
	}
	if status.level != statusWarn || !strings.Contains(status.text, "--env-file") {
		t.Fatalf("status = %q (level %v), want a warning that names the reason", status.text, status.level)
	}
	if got := m.ws.active.Values()["auth.token"]; got != "A-DEV" {
		t.Fatalf("token = %q, want the explicit choice to survive", got)
	}
	if !util.SamePath(m.ws.envFile, explicit) {
		t.Fatalf("environment file = %q, want %q", m.ws.envFile, explicit)
	}
}

// Opening a file outside the current root moves the workspace, so it has to move
// the environment too.
func TestOpenExternalFileMovesEnvironment(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")
	if got := m.ws.active.Values()["auth.token"]; got != "A-DEV" {
		t.Fatalf("token = %q, want A's before the move", got)
	}

	m.applyOpenFilePath(filepath.Join(base, "B", "req.http"))

	if got := m.ws.active.Values()["auth.token"]; got != "B-DEV" {
		t.Fatalf("token = %q, want the opened file's workspace", got)
	}
	if !strings.Contains(m.ws.envFile, filepath.Join("B", "resterm.env.json")) {
		t.Fatalf("environment file = %q, want B's", m.ws.envFile)
	}
}

// When the intent cannot be honoured nothing is active until someone picks, so
// the catalog still loads but requests are refused.
func TestOpenWorkspaceLeavesNothingSelectedWhenIntentIsMissing(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "prod", "")

	status, ok := firstStatus(m.applyOpenDirectory(filepath.Join(base, "E")))
	if !ok {
		t.Fatal("no status message")
	}
	if status.level != statusWarn || !strings.Contains(status.text, "No environment selected") {
		t.Fatalf("status = %q (level %v), want a warning about the missing intent", status.text, status.level)
	}
	if !m.ws.unselected {
		t.Fatal("expected the session to be left unselected")
	}
	if got := m.ws.active.Values()["auth.token"]; got != "" {
		t.Fatalf("token = %q, want nothing active", got)
	}
	if m.ws.cat.Empty() {
		t.Fatal("expected the new catalog to be available for picking")
	}
	if got := m.headerEnvVariants()[0]; !strings.Contains(got, "none") {
		t.Fatalf("header = %q, want it to show nothing is selected", got)
	}

	// Every path onto the wire is refused, not only the ordinary send.
	for name, cmd := range map[string]tea.Cmd{
		"send":     runCmd(m.startRun(runSpec{sel: m.ws.sel})),
		"profile":  m.startProfileRun(nil, &restfile.Request{}, httpclient.Options{}),
		"compare":  m.startCompareRun(nil, nil, nil, httpclient.Options{}),
		"workflow": m.startWorkflowRun(nil, restfile.Workflow{}, httpclient.Options{}),
		"foreach":  m.startForEachRun(nil, nil, httpclient.Options{}),
	} {
		refused, ok := firstStatus(cmd)
		if !ok || !strings.Contains(refused.text, "No environment selected") {
			t.Fatalf("%s status = %q, want a refusal", name, refused.text)
		}
	}

	m.openEnvironmentSelector()
	m.envList.Select(0)
	m.applyEnvironmentSelection()
	if m.ws.unselected {
		t.Fatal("picking an environment should clear the unselected state")
	}
	if got := m.ws.active.Values()["auth.token"]; got != "E-STAGING" {
		t.Fatalf("token = %q, want the picked environment", got)
	}
}

// History replay selects an environment without going through the picker, so it
// has to leave the session in the same state a picked one would.
func TestHistoryReplaySelectionBecomesTheIntent(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "prod", "")
	m.applyOpenDirectory(filepath.Join(base, "E"))
	if !m.ws.unselected {
		t.Fatal("expected the session to be left unselected")
	}

	if err := m.selectEnvironment("staging", nil); err != nil {
		t.Fatalf("select staging: %v", err)
	}
	if m.ws.unselected {
		t.Fatal("selecting an environment should clear the unselected state")
	}

	m.applyOpenDirectory(filepath.Join(base, "B"))
	if !m.ws.unselected {
		t.Fatalf("label = %q, want staging replayed and missing in B", m.ws.active.Label())
	}
}

// An in-flight request holds the engine config and can write runtime values back,
// so the move waits for it.
func TestOpenWorkspaceRefusedWhileRunning(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")
	m.sending = true

	before := m.ws.envFile
	status, ok := firstStatus(m.applyOpenDirectory(filepath.Join(base, "B")))
	if !ok {
		t.Fatal("no status message")
	}
	if status.level != statusWarn || !strings.Contains(status.text, "running request") {
		t.Fatalf("status = %q, want a refusal", status.text)
	}
	if m.ws.envFile != before {
		t.Fatalf("environment file changed to %q despite the refusal", m.ws.envFile)
	}
	if filepath.Base(m.ws.root) != "A" {
		t.Fatalf("workspace moved to %q despite the refusal", m.ws.root)
	}
}

// Discovery for a move searches only the new root. The directory the session
// was launched from held an environment once, and rediscovering it there is how
// a workspace without one ended up running another's credentials.
func TestOpenWorkspaceDoesNotRediscoverFromWorkingDirectory(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")
	t.Chdir(filepath.Join(base, "A"))

	m.applyOpenDirectory(filepath.Join(base, "C"))

	if m.ws.envFile != "" {
		t.Fatalf("environment file = %q, want none discovered for C", m.ws.envFile)
	}
	if got := m.ws.active.Values()["auth.token"]; got != "" {
		t.Fatalf("token = %q, want the working directory's environment ignored", got)
	}
}

// Two workspaces without environment files share the unnamed scope, so a move
// between them still crosses a boundary and resets runtime state.
func TestMoveBetweenEnvlessWorkspacesResets(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")
	m.applyOpenDirectory(filepath.Join(base, "C"))

	m.globalsStore().Set(m.ws.active.Scope(), "leak", "C", false)

	m.applyOpenDirectory(filepath.Join(base, "F"))
	if got := m.globalsStore().Entries(); len(got) != 0 {
		t.Fatalf("globals survived the move between env-less workspaces: %v", got)
	}
}

// A failed listing aborts the move before anything is committed, so workspace,
// document and environment stay from one context.
func TestOpenWorkspaceListingFailureCommitsNothing(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")
	root, envFile := m.ws.root, m.ws.envFile

	status, ok := firstStatus(m.applyOpenDirectory(filepath.Join(base, "missing")))
	if !ok || status.level != statusError {
		t.Fatalf("status = %q (level %v), want an error", status.text, status.level)
	}
	if m.ws.root != root || m.ws.envFile != envFile {
		t.Fatalf("workspace committed despite the failure: root %q env %q", m.ws.root, m.ws.envFile)
	}
	if got := m.ws.active.Values()["auth.token"]; got != "A-DEV" {
		t.Fatalf("token = %q, want A's environment still active", got)
	}
}

// The move commits everything at once: streams stop, panes and response state
// drop, and the launch BaseDir stops pointing at the old workspace.
func TestCommitMoveClearsResponseAndStreamState(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")

	sess := stream.NewSession(context.Background(), stream.KindSSE, stream.Config{})
	m.sessionHandles[sess.ID()] = sess
	m.liveSessions[sess.ID()] = newLiveSession(sess.ID(), 10)
	m.responsePanes[0].snapshot = &responseSnapshot{}
	m.responseTokens["t"] = &responseSnapshot{}
	m.testResults = []scripts.TestResult{{Name: "x", Passed: true}}
	m.scriptError = errors.New("old")
	m.lastError = errors.New("old")
	m.cfg.HTTPOptions.BaseDir = filepath.Join(base, "A")

	m.applyOpenDirectory(filepath.Join(base, "B"))

	if sess.Context().Err() == nil {
		t.Fatal("the live stream kept running after the move")
	}
	if len(m.liveSessions) != 0 || len(m.sessionHandles) != 0 {
		t.Fatal("stream state survived the move")
	}
	if m.responsePanes[0].snapshot != nil {
		t.Fatal("pane snapshot survived the move")
	}
	if m.responseTokens == nil || len(m.responseTokens) != 0 {
		t.Fatal("response tokens must be cleared but stay usable")
	}
	if m.testResults != nil || m.scriptError != nil || m.lastError != nil {
		t.Fatal("test and error state survived the move")
	}
	if m.cfg.HTTPOptions.BaseDir != "" {
		t.Fatalf("BaseDir = %q, want it re-derived per run after a move", m.cfg.HTTPOptions.BaseDir)
	}
}

// runOptions points BaseDir at the file being run rather than the launch file.
func TestRunOptionsDerivesBaseDirFromCurrentFile(t *testing.T) {
	m := Model{cfg: Config{HTTPOptions: httpclient.Options{BaseDir: "/launch"}}}
	if got := m.runOptions().BaseDir; got != "/launch" {
		t.Fatalf("BaseDir = %q, want the launch dir with no file open", got)
	}
	m.currentFile = filepath.Join("/ws", "b", "req.http")
	if got := m.runOptions().BaseDir; got != filepath.Join("/ws", "b") {
		t.Fatalf("BaseDir = %q, want the open file's directory", got)
	}
}

// A refused run must leave no progress state behind: a stuck spinner reads as
// an active run and blocks the workspace change that would fix the situation.
func TestRefusedRunLeavesNoProgressState(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "prod", "")
	m.applyOpenDirectory(filepath.Join(base, "E"))
	if !m.ws.unselected {
		t.Fatal("expected the session to be left unselected")
	}

	m.editor.SetValue("### r\n# @name r\nGET http://example.test/x\n")
	m.editor.SetCursor(0)

	for name, run := range map[string]func() tea.Cmd{
		"send":    m.sendActiveRequest,
		"explain": m.explainActiveRequest,
	} {
		status, ok := firstStatus(run())
		if !ok || !strings.Contains(status.text, "No environment selected") {
			t.Fatalf("%s status = %q, want a refusal", name, status.text)
		}
		if m.sending {
			t.Fatalf("%s left sending set with no response on the way", name)
		}
		if cmd := m.moveBlocked(); cmd != nil {
			t.Fatalf("%s left the workspace locked", name)
		}
	}

	// Picking an environment unblocks the same entry point, and only then do
	// the sending indicators start.
	if err := m.selectEnvironment("staging", nil); err != nil {
		t.Fatal(err)
	}
	m.sendActiveRequest()
	if !m.sending {
		t.Fatal("an accepted run must start the sending indicators")
	}
}

// Messages from a canceled stream must not resurrect its session in the next
// workspace. Cancellation makes the runner flush pending events and emit state
// and completion, all of which arrive after the maps were cleared.
func TestStaleStreamMessagesAreDropped(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")

	sess := stream.NewSession(context.Background(), stream.KindSSE, stream.Config{})
	m.attachStreamSession(sess)
	id := sess.ID()
	if m.liveSession(id) == nil {
		t.Fatal("attach must track the session")
	}

	m.applyOpenDirectory(filepath.Join(base, "B"))
	if len(m.liveSessions) != 0 {
		t.Fatal("move must drop live sessions")
	}

	before := m.statusMessage
	m.handleStreamEvents(streamEventMsg{sessionID: id, events: []*stream.Event{{}}})
	m.handleStreamState(streamStateMsg{sessionID: id, state: stream.StateClosed})
	m.handleStreamComplete(streamCompleteMsg{sessionID: id})
	m.handleStreamReady(streamReadyMsg{sessionID: id})

	if len(m.liveSessions) != 0 {
		t.Fatal("a stale stream message resurrected its session")
	}
	if m.statusMessage != before {
		t.Fatalf("a stale stream overwrote the status: %q", m.statusMessage.text)
	}
}

// A file that cannot be read aborts the open before anything is committed, so
// the environment cannot move under an editor still showing the old workspace.
func TestOpenExternalUnreadableFileCommitsNothing(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")
	m.applyOpenFilePath(filepath.Join(base, "A", "req.http"))
	if filepath.Base(m.currentFile) != "req.http" {
		t.Fatalf("fixture file not open: %q", m.currentFile)
	}

	status, ok := firstStatus(m.applyOpenFilePath(filepath.Join(base, "B", "missing.http")))
	if !ok || status.level != statusError {
		t.Fatalf("status = %q (level %v), want an open error", status.text, status.level)
	}
	if !strings.Contains(m.ws.envFile, filepath.Join("A", "resterm.env.json")) {
		t.Fatalf("environment file = %q, want A's untouched", m.ws.envFile)
	}
	if got := m.ws.active.Values()["auth.token"]; got != "A-DEV" {
		t.Fatalf("token = %q, want A's environment", got)
	}
	if filepath.Base(filepath.Dir(m.currentFile)) != "A" {
		t.Fatalf("editor file = %q, want A's file still open", m.currentFile)
	}
}

// A mock server serves the workspace it was started in, so a move stops it and
// drops reloads already in flight rather than leaving it rooted in the old tree.
func TestMoveStopsMockServer(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")
	path := filepath.Join(base, "A", "mock.http")
	if err := os.WriteFile(path, []byte(mockTestDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	m.applyOpenFilePath(path)
	_ = m.startMockServer("127.0.0.1:0")
	srv := m.activeMockServer()
	if srv == nil {
		t.Fatal("mock server was not started")
	}
	gen := m.mock.gen

	mv, err := m.ws.plan(filepath.Join(base, "B"))
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	_, stop := m.commitMove(mv)

	if m.mock.server != nil || m.mock.reloader != nil {
		t.Fatal("the mock server survived the workspace move")
	}
	if m.handleMockReload(mockReloadResultMsg{server: srv, generation: gen}) != nil {
		t.Fatal("a reload in flight for the old server was not dropped")
	}
	if stop == nil {
		t.Fatal("no close command for the running mock server")
	}
	if closed, ok := stop().(mockServerClosedMsg); !ok || closed.err != nil {
		t.Fatalf("close result = %+v", closed)
	}
}
