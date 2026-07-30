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

// A and B both name their environments dev and prod, so their runtime scopes
// collide. C and F have no environment file, D's cannot be parsed, E has only
// staging and G holds only a lookalike name discovery never opens.
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
	write("G/dev.env.json", `{"e":{"who":"X"}}`)
	write("G/req.http", request)
	return base
}

// workspaceModel starts a session in workspace A through the real command
// line wiring, which once hid a --env value that never reached the model.
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

// firstStatus finds the status a command carries, bare or wrapped, since a
// test that only knows one shape silently reports no status at all.
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

// A workspace with no environment file has nothing to compare a move against,
// so reopening it once looked like a move between two env-less workspaces.
func TestReopeningEnvlessWorkspaceKeepsRuntimeState(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")
	m.applyOpenDirectory(filepath.Join(base, "C"))

	scope := m.ws.active.Scope()
	m.globalsStore().Set(scope, "captured.token", "KEEP", false)
	m.fileStore().Set(scope, filepath.Join(base, "C", "req.http"), "file.token", "KEEP", false)

	m.applyOpenDirectory(filepath.Join(base, "C"))

	if snap := m.globalsStore().Snapshot(scope); len(snap) == 0 {
		t.Fatal("reopening the same env-less workspace discarded globals")
	}
	if got := m.fileStore().Entries(); len(got) == 0 {
		t.Fatal("reopening the same env-less workspace discarded file values")
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

	// Picking an environment unblocks the same entry point.
	if err := m.selectEnvironment("staging", nil); err != nil {
		t.Fatal(err)
	}
	m.sendActiveRequest()
	if !m.sending {
		t.Fatal("an accepted run must start the sending indicators")
	}
}

// Cancellation makes the stream runner flush pending events and completion
// messages, all of which arrive after the move cleared the maps.
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

func TestOpenDirectoryClearsDocumentDerivedState(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")
	flow := "@doctoken = fromdoc\n\n# @workflow demo\n# @if response.ok run=First\n\n" +
		"### First\n# @name First\nGET http://example.test/{{who}}\n"
	path := filepath.Join(base, "A", "flow.http")
	if err := os.WriteFile(path, []byte(flow), 0o644); err != nil {
		t.Fatal(err)
	}
	m.applyOpenFilePath(path)

	hasVar := func(name string) bool {
		for _, v := range m.editor.scope.Variables {
			if v.Name == name {
				return true
			}
		}
		return false
	}
	if len(m.workflowItems) == 0 || !hasVar("doctoken") {
		t.Fatalf(
			"fixture must project a workflow and a doc variable, got %d workflows, doctoken=%v",
			len(m.workflowItems),
			hasVar("doctoken"),
		)
	}

	m.applyOpenDirectory(filepath.Join(base, "C"))

	if len(m.workflowItems) != 0 || m.showWorkflow {
		t.Fatal("workflow rows survived the directory move")
	}
	if len(m.requestItems) != 0 {
		t.Fatal("request rows survived the directory move")
	}
	if hasVar("doctoken") {
		t.Fatal("completion variables from the closed document survived the move")
	}
	if hasVar("auth.token") {
		t.Fatal("completion variables from the previous environment survived the move")
	}
}

func TestOpenWorkspaceWarnsAboutUndiscoveredEnvFiles(t *testing.T) {
	base := workspaceFixture(t)
	m := workspaceModel(t, base, "", "")

	status, ok := firstStatus(m.applyOpenDirectory(filepath.Join(base, "G")))
	if !ok {
		t.Fatal("no status message")
	}
	if status.level != statusWarn {
		t.Fatalf("level = %v, want warn: %q", status.level, status.text)
	}
	if !strings.Contains(status.text, "Workspace set to G") ||
		!strings.Contains(status.text, "Load dev.env.json with --env-file") {
		t.Fatalf("status = %q, want the move and the undiscovered file named together", status.text)
	}
}
