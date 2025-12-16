package ui

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	model.workspaceRoot = dir
	model.lastResponseSaveDir = dir

	if cmd := model.saveResponseBody(); cmd != nil {
		collectMsgs(cmd)
	}
	if !model.showResponseSaveModal {
		t.Fatalf("expected save modal to be visible")
	}
	value := model.responseSaveInput.Value()
	if !strings.HasPrefix(value, dir) || !strings.HasSuffix(value, "demo.bin") {
		t.Fatalf("expected prefilled path with workspace and filename, got %q", value)
	}

	target := filepath.Join(dir, "out.bin")
	model.responseSaveInput.SetValue(target)
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
