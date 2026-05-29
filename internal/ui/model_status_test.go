package ui

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestStatusPulseBaseUsesWarnText(t *testing.T) {
	m := Model{}
	m.sending = true
	m.statusPulseOn = true
	m.statusPulseBase = "Sending"

	msg := statusMsg{text: "Request skipped", level: statusWarn}
	m.setStatusMessage(msg)

	if m.statusPulseBase != "Request skipped" {
		t.Fatalf("expected pulse base to track warn text, got %q", m.statusPulseBase)
	}
}

func TestSetActiveRequestDoesNotReplaceStatusMessage(t *testing.T) {
	m := New(Config{})
	m.statusMessage = statusMsg{text: "Existing status", level: statusWarn}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Metadata: restfile.RequestMetadata{
			Tags: []string{"alpha"},
		},
	}
	m.setActiveRequest(req)

	if m.statusMessage.text != "Existing status" || m.statusMessage.level != statusWarn {
		t.Fatalf("expected active request not to replace status message, got %+v", m.statusMessage)
	}
	if m.activeRequestKey == "" {
		t.Fatal("expected active request state to update")
	}
}
