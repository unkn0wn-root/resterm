package ui

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	_ = model.startMockServer("127.0.0.1:0")
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
	_ = model.startMockServer("127.0.0.1:0")
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
