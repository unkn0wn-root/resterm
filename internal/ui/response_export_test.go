package ui

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/launch"
)

func TestResponseSaveModalPrefillAndSaveWire(t *testing.T) {
	dir := t.TempDir()
	body := []byte{0xAA, 0xBB, 0xCC}
	snap := &responseSnapshot{
		body: body,
		responseHeaders: http.Header{
			"Content-Disposition": {"attachment; filename=\"demo.bin\""},
		},
		contentType:  "application/octet-stream",
		effectiveURL: "https://example.com/demo.bin",
		ready:        true,
	}
	model := newModelWithResponseTab(responseTabPretty, snap)
	model.ws.root = dir
	model.lastResponseSaveDir = dir

	if cmd := model.saveResponseBody(); cmd != nil {
		collectMsgs(cmd)
	}
	if !model.showResponseSaveModal {
		t.Fatalf("expected save modal to be visible")
	}
	value := model.responseSavePrompt.value()
	if !strings.HasPrefix(value, dir) || !strings.HasSuffix(value, "demo.bin") {
		t.Fatalf("expected prefilled path with workspace and filename, got %q", value)
	}

	target := filepath.Join(dir, "out.bin")
	model.responseSavePrompt.input.SetValue(target)
	if cmd := model.submitResponseSave(); cmd != nil {
		collectMsgs(cmd)
	}
	if model.showResponseSaveModal {
		t.Fatalf("expected save modal to close after submit")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file to be written: %v", err)
	}
	if !bytes.Equal(data, body) {
		t.Fatalf("expected saved data to match body, got %v", data)
	}
	if model.lastResponseSaveDir != dir {
		t.Fatalf("expected lastResponseSaveDir to update, got %q", model.lastResponseSaveDir)
	}
}

func TestOpenResponseExternallyRefusesUnviewableBody(t *testing.T) {
	dir := t.TempDir()
	snap := &responseSnapshot{
		body: []byte("MZ\x90\x00\x03\x00\x00\x00"),
		responseHeaders: http.Header{
			"Content-Disposition": {`attachment; filename="run.exe"`},
		},
		contentType: "application/octet-stream",
		ready:       true,
	}
	model := newModelWithResponseTab(responseTabPretty, snap)
	opener := &recordingOpener{}
	model.launcher = opener
	model.spool = launch.NewSpool(dir)

	if cmd := model.openResponseExternally(); cmd != nil {
		t.Fatal("expected no open command for an unviewable body")
	}
	if len(opener.links) != 0 {
		t.Fatalf("opener called with %q", opener.links)
	}
	if model.statusMessage.level != statusWarn || !strings.Contains(model.statusMessage.text, "g Shift+S") {
		t.Fatalf("unexpected status: %+v", model.statusMessage)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("spool wrote %d entries", len(entries))
	}
}

func TestOpenResponseExternallyOpensViewerName(t *testing.T) {
	body := []byte("%PDF-1.7")
	snap := &responseSnapshot{
		body: body,
		responseHeaders: http.Header{
			"Content-Disposition": {`attachment; filename="invoice.exe"`},
		},
		contentType: "application/pdf",
		ready:       true,
	}
	model := newModelWithResponseTab(responseTabPretty, snap)
	opener := &recordingOpener{}
	model.launcher = opener
	model.spool = launch.NewSpool(t.TempDir())

	cmd := model.openResponseExternally()
	if cmd == nil {
		t.Fatal("expected an open command")
	}
	if model.statusMessage.text != "Opening invoice.pdf" {
		t.Fatalf("unexpected status before open: %+v", model.statusMessage)
	}
	msg, ok := cmd().(responseOpenedMsg)
	if !ok {
		t.Fatal("expected responseOpenedMsg")
	}
	if len(opener.links) != 1 || filepath.Base(opener.links[0]) != "invoice.pdf" {
		t.Fatalf("opener links = %q, want one invoice.pdf", opener.links)
	}
	if data, err := os.ReadFile(opener.links[0]); err != nil || !bytes.Equal(data, body) {
		t.Fatalf("opened file = %q, %v, want %q", data, err, body)
	}

	model.handleResponseOpened(msg)
	if model.statusMessage.level != statusInfo || !strings.Contains(model.statusMessage.text, "Opened invoice.pdf") {
		t.Fatalf("unexpected status after open: %+v", model.statusMessage)
	}

	if err := model.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(opener.links[0]); !os.IsNotExist(err) {
		t.Fatalf("Stat() after Close error = %v, want the spooled file removed", err)
	}
}

func TestOpenResponseExternallyReportsOpenerFailure(t *testing.T) {
	snap := &responseSnapshot{body: []byte(`{"ok":true}`), contentType: "application/json", ready: true}
	model := newModelWithResponseTab(responseTabPretty, snap)
	model.launcher = &recordingOpener{err: errors.New("open: exit status 1")}
	model.spool = launch.NewSpool(t.TempDir())

	msg, ok := model.openResponseExternally()().(responseOpenedMsg)
	if !ok {
		t.Fatal("expected responseOpenedMsg")
	}
	model.handleResponseOpened(msg)
	want := "Open failed: open: exit status 1. Save it with g Shift+S instead."
	if model.statusMessage.level != statusWarn || model.statusMessage.text != want {
		t.Fatalf("status = %+v, want %q", model.statusMessage, want)
	}
}
