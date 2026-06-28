package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

func TestMatchesFilterSSE(t *testing.T) {
	evt := &stream.Event{
		Kind:      stream.KindSSE,
		Direction: stream.DirReceive,
		Payload:   []byte("hello world"),
		SSE: stream.SSEMetadata{
			Name:    "greeting",
			Comment: "friendly",
		},
	}
	if !matchesFilter("hello", evt) {
		t.Fatalf("expected filter to match payload")
	}
	if !matchesFilter("greet", evt) {
		t.Fatalf("expected filter to match event name")
	}
	if matchesFilter("bye", evt) {
		t.Fatalf("did not expect filter to match")
	}
}

func TestLiveSessionPause(t *testing.T) {
	ls := newLiveSession("s", 10)
	evt := &stream.Event{Kind: stream.KindSSE, Direction: stream.DirReceive, Payload: []byte("one")}
	ls.append([]*stream.Event{evt})
	if len(ls.events) != 1 {
		t.Fatalf("expected one event while running")
	}
	ls.setPaused(true)
	if !ls.paused {
		t.Fatalf("expected paused flag to set")
	}
	if ls.pausedIndex != 1 {
		t.Fatalf("expected paused index to capture current position, got %d", ls.pausedIndex)
	}
	ls.append(
		[]*stream.Event{
			{Kind: stream.KindSSE, Direction: stream.DirReceive, Payload: []byte("two")},
		},
	)
	if len(ls.events) != 2 {
		t.Fatalf("expected buffered events to grow while paused")
	}
	if ls.pausedIndex != 1 {
		t.Fatalf("expected pause boundary to stay fixed while paused, got %d", ls.pausedIndex)
	}
	ls.setPaused(false)
	if ls.pausedIndex != -1 {
		t.Fatalf("expected paused index reset after resume, got %d", ls.pausedIndex)
	}
	if len(ls.events) != 2 {
		t.Fatalf("expected all events available after resume")
	}
}

func TestBookmarkLabelFallback(t *testing.T) {
	bm := streamBookmark{Label: "", Created: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)}
	label := bookmarkLabel(bm)
	if label == "" {
		t.Fatalf("expected fallback label")
	}
}

func TestStreamFilterPromptClearsOnEsc(t *testing.T) {
	model := newStreamFilterPromptModel(t)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	model = updated.(Model)
	if model.statusMessage.text != streamFilterPromptStatus {
		t.Fatalf("expected stream filter prompt, got %q", model.statusMessage.text)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if model.streamFilterActive {
		t.Fatalf("expected stream filter mode to close")
	}
	if model.statusMessage.text != "" {
		t.Fatalf("expected stream filter prompt to clear, got %q", model.statusMessage.text)
	}
}

func TestStreamFilterPromptClearsWhenFocusLeavesResponse(t *testing.T) {
	model := newStreamFilterPromptModel(t)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	model = updated.(Model)
	if model.statusMessage.text != streamFilterPromptStatus {
		t.Fatalf("expected stream filter prompt, got %q", model.statusMessage.text)
	}

	_ = model.setFocus(focusEditor)
	if model.streamFilterActive {
		t.Fatalf("expected stream filter mode to close")
	}
	if model.statusMessage.text != "" {
		t.Fatalf("expected focus change to clear stream filter prompt, got %q", model.statusMessage.text)
	}
}

func newStreamFilterPromptModel(t *testing.T) Model {
	t.Helper()
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary response pane")
	}
	pane.activeTab = responseTabStream

	req := &restfile.Request{Method: "GET", URL: "https://example.com/events"}
	model.currentRequest = req
	model.requestSessions[req] = "stream-1"
	model.liveSessions["stream-1"] = newLiveSession("stream-1", 10)
	return model
}
