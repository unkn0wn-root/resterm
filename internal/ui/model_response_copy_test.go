package ui

import (
	"net/http"
	"strings"
	"testing"

	"github.com/atotto/clipboard"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
)

func newModelWithResponseTab(tab responseTab, snap *responseSnapshot) *Model {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	pane.activeTab = tab
	pane.viewport.Width = 80
	pane.viewport.Height = 20
	if snap != nil {
		cloned := *snap
		cloned.ready = true
		pane.snapshot = &cloned
	}
	return &model
}

func TestResponseCopyPayloadStripsANSI(t *testing.T) {
	snap := &responseSnapshot{
		pretty:  withTrailingNewline("\x1b[31mStatus\x1b[0m 200 OK"),
		raw:     withTrailingNewline("raw"),
		headers: withTrailingNewline("Headers:\nX-Test: ok"),
		ready:   true,
	}
	model := newModelWithResponseTab(responseTabPretty, snap)

	label, text, status := model.responseCopyPayload()
	if status != nil {
		t.Fatalf("expected nil status, got %+v", status)
	}
	if label != "Pretty" {
		t.Fatalf("expected label Pretty, got %q", label)
	}
	if strings.Contains(text, "\x1b[") {
		t.Fatalf("expected ANSI codes stripped, got %q", text)
	}
	if !strings.Contains(text, "Status 200 OK") {
		t.Fatalf("expected response summary in text, got %q", text)
	}
}

func TestResponseCopyPayloadHeadersFallback(t *testing.T) {
	snap := &responseSnapshot{
		pretty:  withTrailingNewline("pretty"),
		raw:     withTrailingNewline("raw"),
		headers: "",
		ready:   true,
	}
	model := newModelWithResponseTab(responseTabHeaders, snap)

	label, text, status := model.responseCopyPayload()
	if status != nil {
		t.Fatalf("expected nil status, got %+v", status)
	}
	if label != "Headers" {
		t.Fatalf("expected Headers label, got %q", label)
	}
	if !strings.Contains(text, "<no headers>") {
		t.Fatalf("expected fallback header text, got %q", text)
	}
}

func TestResponseCopyPayloadRequestHeaders(t *testing.T) {
	snap := &responseSnapshot{
		pretty:         withTrailingNewline("pretty"),
		raw:            withTrailingNewline("raw"),
		headers:        withTrailingNewline("Headers:\nX-Resp: ok"),
		requestHeaders: withTrailingNewline("Headers:\nCookie: demo=1"),
		ready:          true,
	}
	model := newModelWithResponseTab(responseTabHeaders, snap)
	pane := model.pane(responsePanePrimary)
	pane.headersView = headersViewRequest

	label, text, status := model.responseCopyPayload()
	if status != nil {
		t.Fatalf("expected nil status, got %+v", status)
	}
	if label != "Headers" {
		t.Fatalf("expected Headers label, got %q", label)
	}
	if !strings.Contains(text, "Cookie: demo=1") {
		t.Fatalf("expected request headers in copy, got %q", text)
	}
	if strings.Contains(text, "X-Resp") {
		t.Fatalf("unexpected response header in request copy, got %q", text)
	}
}

func TestResponseCopyPayloadHeadersCopiesPlainHeaders(t *testing.T) {
	val := strings.Repeat("b", 80)
	resp := &httpclient.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Headers: http.Header{
			"X-Long-Copy-Test": {val},
		},
	}
	snap := &responseSnapshot{
		pretty:  withTrailingNewline("pretty"),
		raw:     withTrailingNewline("raw"),
		headers: withTrailingNewline("Status: 200 OK\n\n1 HEADER\nX-Decorated: no"),
		source:  newHTTPResponseRenderSource(resp, nil, nil),
		ready:   true,
	}
	model := newModelWithResponseTab(responseTabHeaders, snap)
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 18

	label, text, status := model.responseCopyPayload()
	if status != nil {
		t.Fatalf("expected nil status, got %+v", status)
	}
	if label != "Headers" {
		t.Fatalf("expected Headers label, got %q", label)
	}
	want := withTrailingNewline("X-Long-Copy-Test: " + val)
	if text != want {
		t.Fatalf("expected plain response headers copy %q, got %q", want, text)
	}
	for _, bad := range []string{"HEADER", "Status:", "X-Decorated"} {
		if strings.Contains(text, bad) {
			t.Fatalf("expected no %q in copied headers, got %q", bad, text)
		}
	}
}

func TestResponseCopyPayloadRequestHeadersCopiesPlainHeaders(t *testing.T) {
	val := strings.Repeat("b", 80)
	resp := &httpclient.Response{
		Status:     "200 OK",
		StatusCode: 200,
		RequestHeaders: http.Header{
			"X-Long-Request-Copy-Test": {val},
		},
	}
	snap := &responseSnapshot{
		pretty:  withTrailingNewline("pretty"),
		raw:     withTrailingNewline("raw"),
		headers: withTrailingNewline("Stored response headers\nX-Resp: ok"),
		requestHeaders: withTrailingNewline(
			"Request GET http://example.test\n\n1 HEADER\nX-Decorated-Req: no",
		),
		source: newHTTPResponseRenderSource(resp, nil, nil),
		ready:  true,
	}
	model := newModelWithResponseTab(responseTabHeaders, snap)
	pane := model.pane(responsePanePrimary)
	pane.headersView = headersViewRequest
	pane.viewport.Width = 18

	label, text, status := model.responseCopyPayload()
	if status != nil {
		t.Fatalf("expected nil status, got %+v", status)
	}
	if label != "Headers" {
		t.Fatalf("expected Headers label, got %q", label)
	}
	want := withTrailingNewline("X-Long-Request-Copy-Test: " + val)
	if text != want {
		t.Fatalf("expected plain request headers copy %q, got %q", want, text)
	}
	for _, bad := range []string{"HEADER", "Request ", "X-Decorated-Req"} {
		if strings.Contains(text, bad) {
			t.Fatalf("expected no %q in copied request headers, got %q", bad, text)
		}
	}
}

func TestCopyResponseTabWritesClipboard(t *testing.T) {
	if err := clipboard.WriteAll(""); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	body := withTrailingNewline("Status: 200 OK\n\n{}")
	snap := &responseSnapshot{
		pretty: body,
		raw:    withTrailingNewline("raw-body"),
		ready:  true,
	}
	model := newModelWithResponseTab(responseTabPretty, snap)

	cmd := model.copyResponseTab()
	if cmd == nil {
		t.Fatalf("expected copy command")
	}
	msg := cmd()
	event, ok := msg.(editorEvent)
	if !ok {
		t.Fatalf("expected editorEvent, got %T", msg)
	}
	if event.status == nil || !strings.Contains(event.status.text, "Copied Pretty tab") {
		t.Fatalf("expected Pretty copy status, got %+v", event.status)
	}
	got, err := clipboard.ReadAll()
	if err != nil {
		t.Fatalf("read clipboard: %v", err)
	}
	if got != body {
		t.Fatalf("expected clipboard %q, got %q", body, got)
	}
}

func TestCopyResponseTabRequiresFocus(t *testing.T) {
	snap := &responseSnapshot{
		pretty: withTrailingNewline("status"),
		raw:    withTrailingNewline("raw"),
		ready:  true,
	}
	model := newModelWithResponseTab(responseTabPretty, snap)
	model.focus = focusEditor

	cmd := model.copyResponseTab()
	if cmd == nil {
		t.Fatalf("expected status command when not focused")
	}
	msg := cmd()
	status, ok := msg.(statusMsg)
	if !ok {
		t.Fatalf("expected statusMsg, got %T", msg)
	}
	if !strings.Contains(status.text, "Focus the response pane") {
		t.Fatalf("unexpected status text %q", status.text)
	}
}
