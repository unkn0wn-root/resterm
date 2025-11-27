package ui

import (
	"strings"
	"testing"

	"github.com/atotto/clipboard"
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
		pretty:  ensureTrailingNewline("\x1b[31mStatus\x1b[0m 200 OK"),
		raw:     ensureTrailingNewline("raw"),
		headers: ensureTrailingNewline("Headers:\nX-Test: ok"),
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
		pretty:  ensureTrailingNewline("pretty"),
		raw:     ensureTrailingNewline("raw"),
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
		pretty:         ensureTrailingNewline("pretty"),
		raw:            ensureTrailingNewline("raw"),
		headers:        ensureTrailingNewline("Headers:\nX-Resp: ok"),
		requestHeaders: ensureTrailingNewline("Headers:\nCookie: demo=1"),
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

func TestCopyResponseTabWritesClipboard(t *testing.T) {
	if err := clipboard.WriteAll(""); err != nil {
		t.Skipf("clipboard unavailable: %v", err)
	}
	body := ensureTrailingNewline("Status: 200 OK\n\n{}")
	snap := &responseSnapshot{
		pretty: body,
		raw:    ensureTrailingNewline("raw-body"),
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
		pretty: ensureTrailingNewline("status"),
		raw:    ensureTrailingNewline("raw"),
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
