package ui

import "testing"

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
