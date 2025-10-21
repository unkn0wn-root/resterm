package ui

import "testing"

func TestWebSocketConsoleHistoryNavigation(t *testing.T) {
	console := newWebsocketConsole("session", nil, nil, "")
	if console.historyPrev() {
		t.Fatalf("expected no history yet")
	}
	console.prependHistory(consoleHistoryEntry{Mode: consoleModeText, Payload: "first"})
	console.prependHistory(consoleHistoryEntry{Mode: consoleModeJSON, Payload: "second"})

	if !console.historyPrev() {
		t.Fatalf("expected history prev to succeed")
	}
	if console.input.Value() != "second" {
		t.Fatalf("expected second entry, got %q", console.input.Value())
	}
	if !console.historyPrev() {
		t.Fatalf("expected history prev to succeed for first entry")
	}
	if console.input.Value() != "first" {
		t.Fatalf("expected first entry, got %q", console.input.Value())
	}
	if !console.historyNext() {
		t.Fatalf("expected history next to succeed")
	}
	if console.input.Value() != "second" {
		t.Fatalf("expected second entry after next, got %q", console.input.Value())
	}
	if !console.historyNext() {
		t.Fatalf("expected history next to clear input")
	}
	if console.input.Value() != "" {
		t.Fatalf("expected cleared input, got %q", console.input.Value())
	}
}

func TestWebSocketConsoleCycleMode(t *testing.T) {
	console := newWebsocketConsole("session", nil, nil, "")
	initial := console.mode
	console.cycleMode()
	if console.mode == initial {
		t.Fatalf("expected mode to change")
	}
	for i := 0; i < 5; i++ {
		console.cycleMode()
	}
	if console.mode != initial {
		t.Fatalf("expected mode to cycle back to initial")
	}
}
