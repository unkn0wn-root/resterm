package ui

import (
	"net/http"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/update"
)

func TestEnqueueUpdateCheck(t *testing.T) {
	cl, err := update.NewClient(&http.Client{}, "unkn0wn-root/resterm")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	m := Model{updateEnabled: true, updateVersion: "1.0.0", updateClient: cl}
	if m.updateBusy {
		t.Fatal("expected updateBusy false")
	}
	if cmd := m.enqueueUpdateCheck(); cmd == nil {
		t.Fatal("expected command when updates enabled")
	}
	if !m.updateBusy {
		t.Fatal("expected updateBusy true after enqueue")
	}
}

func TestEnqueueUpdateCheckDisabled(t *testing.T) {
	m := Model{updateEnabled: false, updateVersion: "1.0.0"}
	if cmd := m.enqueueUpdateCheck(); cmd != nil {
		t.Fatal("expected nil command when disabled")
	}
}
