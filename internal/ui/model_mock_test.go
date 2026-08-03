package ui

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const mockTestDocument = `### Mock user
# @mock method=GET path=/users/{id} name=found default=true
HTTP/1.1 200 OK
Content-Type: application/json

{"id":"old"}
`

func newMockTestModel(t *testing.T, content string) *Model {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "api.http")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	value := New(Config{
		FilePath:       path,
		InitialContent: content,
		WorkspaceRoot:  dir,
	})
	model := &value
	t.Cleanup(func() { _ = model.Close() })
	return model
}

func TestTUIStartsReloadsAndStopsMockServer(t *testing.T) {
	model := newMockTestModel(t, mockTestDocument)
	_ = model.startMockServer(mockStartSpec{addr: "127.0.0.1:0"})
	if model.activeMockServer() == nil {
		t.Fatal("mock server was not started")
	}

	request := func() string {
		t.Helper()
		response, err := http.Get("http://" + model.activeMockServer().Addr() + "/users/42")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	if got := request(); got != `{"id":"old"}` {
		t.Fatalf("initial body = %q", got)
	}

	updated := strings.Replace(mockTestDocument, `{"id":"old"}`, `{"id":"new"}`, 1)
	model.editor.SetValue(updated)
	message := model.scheduleMockReload(0)().(mockReloadResultMsg)
	_ = model.handleMockReload(message)
	if got := request(); got != `{"id":"new"}` {
		t.Fatalf("reloaded body = %q", got)
	}

	model.editor.SetValue(strings.Replace(updated, "HTTP/1.1 200 OK", "not a status", 1))
	message = model.scheduleMockReload(0)().(mockReloadResultMsg)
	_ = model.handleMockReload(message)
	if model.mock.reloadErr == "" {
		t.Fatal("invalid edit did not report a reload error")
	}
	if got := request(); got != `{"id":"new"}` {
		t.Fatalf("invalid reload replaced last valid response: %q", got)
	}
	if !strings.Contains(model.mockLogText(), "RELOAD error") {
		t.Fatalf("reload error missing from logs: %q", model.mockLogText())
	}

	stop := model.stopMockServer()
	if model.activeMockServer() != nil {
		t.Fatal("mock server was not stopped")
	}
	if closed, ok := stop().(mockServerClosedMsg); !ok || closed.err != nil {
		t.Fatalf("stop result = %+v", closed)
	}
}

func TestTUIMockResetVerifyAndClear(t *testing.T) {
	model := newMockTestModel(t, `### Poll
# @mock method=GET path=/poll sequence=polling
# @expect calls=1
HTTP/1.1 503 Service Unavailable

pending
---
HTTP/1.1 200 OK

done
`)
	_ = model.startMockServer(mockStartSpec{addr: "127.0.0.1:0"})
	server := model.activeMockServer()
	if server == nil {
		t.Fatal("mock server was not started")
	}
	call := func() int {
		t.Helper()
		response, err := http.Get("http://" + server.Addr() + "/poll")
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	if got := call(); got != http.StatusServiceUnavailable {
		t.Fatalf("first status = %d", got)
	}

	verify := model.executeMockCommand([]string{"verify"})
	if verify == nil {
		t.Fatal("verify command is nil")
	}
	result, ok := verify().(mockVerifyMsg)
	if !ok {
		t.Fatal("verify command did not produce a mockVerifyMsg")
	}
	if message := mockCommandStatus(t, model.handleMockVerify(result)); message.level != statusSuccess {
		t.Fatalf("verify status = %#v", message)
	}
	if !model.showMockVerification || !strings.Contains(model.mockVerificationText, "PASS") {
		t.Fatalf("verification modal = %t %q", model.showMockVerification, model.mockVerificationText)
	}
	model.closeMockVerification()

	if got := call(); got != http.StatusOK {
		t.Fatalf("second status = %d", got)
	}
	reset := model.executeMockCommand([]string{"reset", "polling"})
	if message := mockCommandStatus(t, reset); message.level != statusSuccess {
		t.Fatalf("reset status = %#v", message)
	}
	if got := call(); got != http.StatusServiceUnavailable {
		t.Fatalf("status after reset = %d", got)
	}

	clear := model.executeMockCommand([]string{"clear"})
	if message := mockCommandStatus(t, clear); message.level != statusInfo {
		t.Fatalf("clear status = %#v", message)
	}
	count, err := server.Count(context.Background(), mock.RequestPattern{})
	if err != nil || count != 0 || len(server.Logs()) != 0 {
		t.Fatalf("after clear count=%d err=%v logs=%d", count, err, len(server.Logs()))
	}
}

func TestTUIMockStartWithSourceSubset(t *testing.T) {
	dir := t.TempDir()
	usersDoc := "### Users\n# @mock method=GET path=/users\nHTTP/1.1 200 OK\n\nusers\n"
	files := map[string]string{
		"users.http":    usersDoc,
		"payments.http": "### Payments\n# @mock method=GET path=/payments\nHTTP/1.1 200 OK\n\npayments\n",
		"errors.rest":   "### Errors\n# @mock method=GET path=/errors\nHTTP/1.1 200 OK\n\nboom\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	value := New(Config{
		FilePath:       filepath.Join(dir, "users.http"),
		InitialContent: usersDoc,
		WorkspaceRoot:  dir,
	})
	model := &value
	t.Cleanup(func() { _ = model.Close() })

	if cmd := model.executeMockCommand(
		[]string{"start", "--addr", "127.0.0.1:0", "--source", "users.http"},
	); cmd == nil {
		t.Fatal("start command is nil")
	}
	if model.activeMockServer() == nil {
		t.Fatal("mock server was not started")
	}
	get := func(path string) int {
		t.Helper()
		response, err := http.Get("http://" + model.activeMockServer().Addr() + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	if got := get("/users"); got != http.StatusOK {
		t.Fatalf("scoped /users = %d", got)
	}
	if got := get("/payments"); got != http.StatusNotFound {
		t.Fatalf("unlisted /payments = %d, want 404", got)
	}
	if status := model.mockStatus(); !strings.Contains(status, "sources: users.http") {
		t.Fatalf("status = %q, want source list", status)
	}

	warn := mockCommandStatus(t, model.executeMockCommand([]string{"start", "--source", "payments.http"}))
	if warn.level != statusWarn || !strings.Contains(warn.text, ":mock restart") {
		t.Fatalf("scoped start while running = %#v, want restart hint", warn)
	}

	restart := func(args ...string) {
		t.Helper()
		cmd := model.executeMockCommand(append([]string{"restart"}, args...))
		closed, ok := cmd().(mockServerClosedMsg)
		if !ok || closed.err != nil {
			t.Fatalf("restart close = %+v", closed)
		}
		_ = model.handleMockServerClosed(closed)
		if model.activeMockServer() == nil {
			t.Fatal("mock server did not restart")
		}
	}
	restart()
	if got := get("/payments"); got != http.StatusNotFound {
		t.Fatalf("/payments after plain restart = %d, want inherited scope", got)
	}

	restart("-s", "users.http,payments.http")
	if got := get("/payments"); got != http.StatusOK {
		t.Fatalf("comma list /payments = %d", got)
	}
	if got := get("/errors"); got != http.StatusNotFound {
		t.Fatalf("comma list /errors = %d, want 404", got)
	}

	restart("--all")
	if got := get("/errors"); got != http.StatusOK {
		t.Fatalf("/errors after restart --all = %d", got)
	}
}

// Recursion is part of the scope a bare start inherits, so a workspace that
// launched without it does not quietly drop nested routes on the next restart
// or toggle. Naming a scope again is what changes it.
func TestMockRecursiveScopeOutlivesRestartAndStop(t *testing.T) {
	dir := t.TempDir()
	usersDoc := "### Users\n# @mock method=GET path=/users\nHTTP/1.1 200 OK\n\nusers\n"
	nested := filepath.Join(dir, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{
		filepath.Join(dir, "users.http"): usersDoc,
		filepath.Join(nested, "orders.http"): "### Orders\n" +
			"# @mock method=GET path=/orders\nHTTP/1.1 200 OK\n\norders\n",
	}
	for path, content := range docs {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	value := New(Config{
		FilePath:       filepath.Join(dir, "users.http"),
		InitialContent: usersDoc,
		WorkspaceRoot:  dir,
	})
	model := &value
	t.Cleanup(func() { _ = model.Close() })

	get := func(path string) int {
		t.Helper()
		response, err := http.Get("http://" + model.activeMockServer().Addr() + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	restart := func(args ...string) {
		t.Helper()
		closed, ok := model.executeMockCommand(append([]string{"restart"}, args...))().(mockServerClosedMsg)
		if !ok || closed.err != nil {
			t.Fatalf("restart close = %+v", closed)
		}
		_ = model.handleMockServerClosed(closed)
		if model.activeMockServer() == nil {
			t.Fatal("mock server did not restart")
		}
	}
	stop := func() {
		t.Helper()
		if closed, ok := model.stopMockServer()().(mockServerClosedMsg); !ok || closed.err != nil {
			t.Fatalf("stop result = %+v", closed)
		}
		// Rebinding the exact port the last server held races with its
		// teardown; the scope, not the address, is under test.
		model.mock.addr = "127.0.0.1:0"
	}

	if cmd := model.executeMockCommand([]string{"start", "-a", "127.0.0.1:0", "--recursive"}); cmd == nil {
		t.Fatal("start command is nil")
	}
	if model.activeMockServer() == nil {
		t.Fatal("mock server was not started")
	}
	if got := get("/orders"); got != http.StatusOK {
		t.Fatalf("nested /orders = %d, want the recursive scope", got)
	}

	restart()
	if got := get("/orders"); got != http.StatusOK {
		t.Fatalf("/orders after plain restart = %d, want the inherited recursive scope", got)
	}

	stop()
	if !model.mockSources().Recursive {
		t.Fatalf("stopped mockSources = %+v, want the remembered recursion", model.mockSources())
	}
	if cmd := model.toggleMockServer(); cmd == nil {
		t.Fatal("toggle command is nil")
	}
	if model.activeMockServer() == nil {
		t.Fatal("toggle did not start the mock server")
	}
	if got := get("/orders"); got != http.StatusOK {
		t.Fatalf("/orders after toggle = %d, want the remembered recursive scope", got)
	}

	// Naming whole workspace scope again drops recursion back to the
	// workspace default, which is how a session widens or narrows on purpose.
	restart("--all")
	if got := get("/users"); got != http.StatusOK {
		t.Fatalf("/users after restart --all = %d", got)
	}
	if got := get("/orders"); got != http.StatusNotFound {
		t.Fatalf("/orders after restart --all = %d, want 404", got)
	}
}

// A file scope outlives the server that introduced it, the way the address
// does, so a stop and start cannot silently widen what is served. A workspace
// move is the exception, because the remembered paths name the old root.
func TestMockFileScopeOutlivesStopButNotWorkspaceMove(t *testing.T) {
	base := workspaceFixture(t)
	value := workspaceModel(t, base, "", "")
	model := &value
	t.Cleanup(func() { _ = model.Close() })

	root := filepath.Join(base, "A")
	payments := filepath.Join(root, "payments.http")
	docs := map[string]string{
		filepath.Join(root, "users.http"): "### Users\n# @mock method=GET path=/users\nHTTP/1.1 200 OK\n\nusers\n",
		payments:                          "### Payments\n# @mock method=GET path=/payments\nHTTP/1.1 200 OK\n\npayments\n",
	}
	for path, content := range docs {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	get := func(path string) int {
		t.Helper()
		response, err := http.Get("http://" + model.activeMockServer().Addr() + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	stop := func() {
		t.Helper()
		if closed, ok := model.stopMockServer()().(mockServerClosedMsg); !ok || closed.err != nil {
			t.Fatalf("stop result = %+v", closed)
		}
	}

	if cmd := model.executeMockCommand([]string{"start", "-a", "127.0.0.1:0", "-s", "payments.http"}); cmd == nil {
		t.Fatal("start command is nil")
	}
	if model.activeMockServer() == nil {
		t.Fatal("mock server was not started")
	}
	stop()

	if status := model.mockStatus(); !strings.Contains(status, "sources: payments.http") {
		t.Fatalf("stopped status = %q, want the remembered scope", status)
	}
	if !slices.Equal(model.mockSources().Files, []string{payments}) {
		t.Fatalf("stopped mockSources = %+v, want the remembered scope", model.mockSources())
	}

	// Rebinding the exact port the last server held races with its teardown,
	// so take an ephemeral one; the scope, not the address, is under test.
	model.mock.addr = "127.0.0.1:0"
	if cmd := model.toggleMockServer(); cmd == nil {
		t.Fatal("toggle command is nil")
	}
	if model.activeMockServer() == nil {
		t.Fatal("toggle did not start the mock server")
	}
	if got := get("/payments"); got != http.StatusOK {
		t.Fatalf("/payments after toggle = %d, want the remembered scope", got)
	}
	if got := get("/users"); got != http.StatusNotFound {
		t.Fatalf("/users after toggle = %d, want 404", got)
	}
	stop()

	mv, err := model.ws.plan(filepath.Join(base, "B"))
	if err != nil {
		t.Fatal(err)
	}
	model.commitMove(mv)
	if len(model.mock.src.Files) != 0 {
		t.Fatalf("mock scope after move = %+v, want it forgotten", model.mock.src)
	}
	if src := model.mockSources(); src.Path != filepath.Join(base, "B") || src.Files != nil {
		t.Fatalf("mockSources after move = %+v, want the new workspace", src)
	}
}

func TestMockCommandReportsArgumentProblems(t *testing.T) {
	model := newMockTestModel(t, mockTestDocument)

	status := mockCommandStatus(t, model.executeMockCommand([]string{"start", "--sauce"}))
	if status.level != statusWarn || !strings.HasPrefix(status.text, ":mock start: ") {
		t.Fatalf("unknown flag status = %#v", status)
	}
	status = mockCommandStatus(t, model.executeMockCommand([]string{"restart", "--help"}))
	if status.level != statusWarn || !strings.HasPrefix(status.text, "Usage: :mock restart ") {
		t.Fatalf("usage request status = %#v", status)
	}
	status = mockCommandStatus(t, model.executeMockCommand([]string{"start", "--source", "notes.txt"}))
	if status.level != statusWarn || !strings.HasPrefix(status.text, ":mock start: ") {
		t.Fatalf("unusable source status = %#v", status)
	}
	// A source that only fails once it is read is a start failure, not an
	// argument problem.
	status = mockCommandStatus(t, model.executeMockCommand([]string{"start", "--source", "missing.http"}))
	if status.level != statusWarn || !strings.HasPrefix(status.text, "Mock server not started: ") {
		t.Fatalf("missing source status = %#v", status)
	}
}

// A capture is validated against what the server serves plus the edited file.
// An unlisted overlay never joins a scoped set, so leaving it out would check
// the new mock against everything except itself.
func TestCaptureSourcesFollowsRunningScope(t *testing.T) {
	dir := t.TempDir()
	usersDoc := "### Users\n# @mock method=GET path=/users\nHTTP/1.1 200 OK\n\nusers\n"
	users := filepath.Join(dir, "users.http")
	payments := filepath.Join(dir, "payments.http")
	docs := map[string]string{
		users:    usersDoc,
		payments: "### Payments\n# @mock method=GET path=/payments\nHTTP/1.1 200 OK\n\npayments\n",
	}
	for path, content := range docs {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	value := New(Config{FilePath: users, InitialContent: usersDoc, WorkspaceRoot: dir})
	model := &value
	t.Cleanup(func() { _ = model.Close() })

	if src := model.captureSources(); len(src.Files) != 0 {
		t.Fatalf("stopped captureSources = %+v, want workspace scope", src)
	}

	if cmd := model.executeMockCommand([]string{"start", "-a", "127.0.0.1:0", "-s", "payments.http"}); cmd == nil {
		t.Fatal("start command is nil")
	}
	if model.activeMockServer() == nil {
		t.Fatal("mock server was not started")
	}
	if src := model.captureSources(); !slices.Equal(src.Files, []string{payments, users}) {
		t.Fatalf("captureSources = %+v, want the served file plus the edited one", src.Files)
	}
}

func mockCommandStatus(t *testing.T, command tea.Cmd) statusMsg {
	t.Helper()
	event, ok := command().(editorEvent)
	if !ok || event.status == nil {
		t.Fatalf("mock command result = %#v, want editor status event", event)
	}
	return *event.status
}

func TestCaptureFocusedHTTPResponseAsMock(t *testing.T) {
	const input = `### Pay
# @name payment
POST https://api.example.test/payments
`
	model := newMockTestModel(t, input)
	response := &httpclient.Response{
		Status:       "202 Accepted",
		StatusCode:   http.StatusAccepted,
		ReqMethod:    http.MethodPost,
		EffectiveURL: "https://api.example.test/payments?source=tui",
		Headers: http.Header{
			"Content-Type":   {"application/json"},
			"Content-Length": {"35"},
			"Set-Cookie":     {"session=secret"},
		},
		Body: []byte(`{"id":"pay_123","status":"pending","template":"{{literal}}"}`),
		Request: &restfile.Request{
			Method: http.MethodPost,
			Metadata: restfile.RequestMetadata{
				Name: "payment",
			},
		},
	}
	model.responsePanes[responsePanePrimary].snapshot = &responseSnapshot{
		ready:  true,
		source: newHTTPResponseRenderSource(response, nil, nil),
	}
	model.responseLastFocused = responsePanePrimary

	_ = model.captureMockResponse()
	if !model.dirty {
		t.Fatal("capture should leave the editor dirty")
	}
	if len(model.doc.Errors) > 0 || len(model.doc.Mocks) != 1 {
		t.Fatalf("captured document errors=%+v mocks=%d", model.doc.Errors, len(model.doc.Mocks))
	}
	spec := model.doc.Mocks[0]
	if spec.Method != http.MethodPost || spec.Path != "/payments" ||
		spec.Responses[0].Status != http.StatusAccepted || spec.Name != "payment" || !spec.Default {
		t.Fatalf("captured mock = %+v", spec)
	}
	if !spec.DisableInterpolation {
		t.Fatal("captured literal template was not preserved")
	}
	if spec.Responses[0].Headers.Get("Content-Length") != "" ||
		spec.Responses[0].Headers.Get("Set-Cookie") != "" {
		t.Fatalf("captured headers = %v", spec.Responses[0].Headers)
	}
	if !strings.Contains(model.editor.Value(), `{"id":"pay_123"`) {
		t.Fatalf("captured body missing from editor: %q", model.editor.Value())
	}
	if !strings.Contains(model.editor.Value(), "interpolate=false") {
		t.Fatalf("captured interpolation option missing from editor: %q", model.editor.Value())
	}
}
