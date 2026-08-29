package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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

// Two panes can show the same session. The filter is typed into one of them,
// so only the focused pane grows the prompt line.
func TestStreamFilterPromptRendersOnlyInFocusedPane(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.width = 160
	model.height = 40
	model.responseSplit = true
	_ = model.applyLayout()

	ls := newLiveSession("sse-1", 100)
	ls.kind = stream.KindSSE
	model.liveSessions[ls.id] = ls
	for _, id := range model.visiblePaneIDs() {
		pane := model.pane(id)
		pane.streamID = ls.id
		pane.setActiveTab(responseTabStream)
	}

	model.responsePaneFocus = responsePaneSecondary
	model.streamFilterActive = true
	model.streamFilterInput.SetValue("needle")
	model.refreshStreamPanes()

	primary := ansi.Strip(model.pane(responsePanePrimary).viewport.View())
	secondary := ansi.Strip(model.pane(responsePaneSecondary).viewport.View())
	if !strings.Contains(secondary, "needle") {
		t.Fatalf("focused pane lost the filter prompt: %q", secondary)
	}
	if strings.Contains(primary, "needle") {
		t.Fatalf("filter prompt leaked into the unfocused pane: %q", primary)
	}
}

func TestStreamPauseDoesNotPinResponsePane(t *testing.T) {
	model := newStreamFilterPromptModel(t)
	pane := model.pane(responsePanePrimary)
	if !pane.followLatest {
		t.Fatal("expected primary response pane to start live")
	}

	if _, ok := model.handleStreamKey(tea.KeyMsg{Type: tea.KeyCtrlAt}); !ok {
		t.Fatal("expected pause shortcut to be handled")
	}
	if !pane.followLatest {
		t.Fatal("pausing a stream pinned the response pane")
	}
	if pane.tail {
		t.Fatal("expected pausing to stop tail follow")
	}

	if _, ok := model.handleStreamKey(tea.KeyMsg{Type: tea.KeyCtrlAt}); !ok {
		t.Fatal("expected resume shortcut to be handled")
	}
	if !pane.followLatest || !pane.tail {
		t.Fatal("expected resume to follow the stream without changing response mode")
	}
}

func TestFailedStreamSurvivesMainSplitReflow(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.width = 140
	model.height = 42
	_ = model.applyLayout()

	pane := model.pane(responsePanePrimary)
	pane.activeTab = responseTabStream
	pane.streamID = "grpc-1"
	pane.viewport.Height = 6
	ls := newLiveSession("grpc-1", 100)
	ls.kind = stream.KindGRPC
	for range 20 {
		ls.append([]*stream.Event{{
			Kind:      stream.KindGRPC,
			Direction: stream.DirReceive,
			Payload:   []byte(`{"item":"value"}`),
		}})
	}
	model.liveSessions[ls.id] = ls
	snap := newTextSnapshot("request failed", "test")
	model.bindSnapshotStream(snap, ls.id)
	model.setPaneSnapshot(responsePanePrimary, snap)
	pane.setActiveTab(responseTabStream)
	pane.tail = true
	model.refreshStreamPanes()
	if pane.viewport.YOffset == 0 {
		t.Fatal("expected live stream to follow the tail before failure")
	}

	err := errors.New("stream deadline exceeded")
	model.handleStreamState(streamStateMsg{sessionID: ls.id, state: stream.StateFailed, err: err})
	if pane.tail {
		t.Fatal("expected failure to stop tail follow")
	}
	if pane.viewport.YOffset != 0 {
		t.Fatalf("expected failure to reveal the error header, offset=%d", pane.viewport.YOffset)
	}
	model.handleStreamComplete(streamCompleteMsg{sessionID: ls.id})
	if model.liveSession(ls.id) != nil {
		t.Fatal("expected completed bound session to leave the live registry")
	}

	model.mainSplitOrientation = mainSplitHorizontal
	_ = model.setMainSplitOrientation(mainSplitVertical)
	plain := ansi.Strip(pane.viewport.View())
	if !strings.Contains(plain, err.Error()) {
		t.Fatalf("expected failed transcript after reflow, got %q", plain)
	}
	if strings.Contains(plain, "Waiting for stream session") {
		t.Fatalf("completed stream lost its response ownership after reflow: %q", plain)
	}
	if model.statusMessage.level != statusWarn {
		t.Fatalf("expected failed completion warning, got level %v", model.statusMessage.level)
	}
	if !strings.Contains(model.statusMessage.text, err.Error()) {
		t.Fatalf("expected failure status to include the cause, got %q", model.statusMessage.text)
	}
}

func TestStaleStreamAttachmentIsCanceled(t *testing.T) {
	model := New(Config{})
	model.streamGen = 2
	sess := stream.NewSession(context.Background(), stream.KindSSE, stream.Config{})

	model.handleStreamAttach(streamAttachMsg{session: sess, gen: 1})
	if model.liveSession(sess.ID()) != nil {
		t.Fatal("stale attachment entered the live registry")
	}
	select {
	case <-sess.Context().Done():
	default:
		t.Fatal("stale session was not canceled")
	}
}

func TestFailedStreamResponseDoesNotResumeTail(t *testing.T) {
	model := New(Config{})
	ls := newLiveSession("grpc-1", 10)
	ls.setState(stream.StateFailed, errors.New("deadline exceeded"))
	model.liveSessions[ls.id] = ls
	snap := newTextSnapshot("failed", "")
	model.bindSnapshotStream(snap, ls.id)

	model.setPaneSnapshot(responsePanePrimary, snap)
	pane := model.pane(responsePanePrimary)
	if pane.tail {
		t.Fatal("late response ownership resumed tail follow for a failed stream")
	}
}

func TestStreamResponseKeepsRunTargetBeforeAttachment(t *testing.T) {
	model := New(Config{})
	model.responseSplit = true
	model.responseLastFocused = responsePanePrimary

	target := model.responseTargetForMsg(responseMsg{
		streamID: "grpc-1",
		target:   paneRunTarget(responsePaneSecondary),
	})
	if target != responsePaneSecondary {
		t.Fatalf("expected captured secondary target, got %v", target)
	}
}

func TestRunTargetIgnoresSecondaryPaneWithoutSplit(t *testing.T) {
	model := New(Config{})
	model.responseLastFocused = responsePanePrimary

	target := model.responseTargetForMsg(responseMsg{
		target: paneRunTarget(responsePaneSecondary),
	})
	if target != responsePanePrimary {
		t.Fatalf("expected the closed split to fall back to primary, got %v", target)
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
	pane.streamID = "stream-1"

	req := &restfile.Request{Method: "GET", URL: "https://example.com/events"}
	model.currentRequest = req
	model.requestSessions[req] = "stream-1"
	model.liveSessions["stream-1"] = newLiveSession("stream-1", 10)
	return model
}

func TestStreamPaneReportsEveryLossSource(t *testing.T) {
	model := New(Config{})
	sess := stream.NewSession(context.Background(), stream.KindSSE, stream.Config{BufferSize: 1})
	sess.MarkOpen()
	sess.Publish(&stream.Event{Kind: stream.KindSSE, Payload: []byte("evicted")})
	sess.Publish(&stream.Event{Kind: stream.KindSSE, Payload: []byte("kept")})

	ls := model.attachStreamSession(sess)
	if ls == nil {
		t.Fatal("the session did not attach")
	}
	t.Cleanup(sess.Cancel)
	if ls.lost.evicted != 1 {
		t.Fatalf("lost.evicted = %d, want the event the session had already dropped", ls.lost.evicted)
	}

	model.handleStreamEvents(streamEventMsg{
		sessionID: ls.id,
		events:    []*stream.Event{{Kind: stream.KindSSE, Payload: []byte("late")}},
		dropped:   2,
	})
	if ls.lost.listener != 2 {
		t.Fatalf("lost.listener = %d, want the listener's running total", ls.lost.listener)
	}

	ls.maxEvents = 1
	ls.append(streamEvents(3, 8))
	if ls.lost.trimmed == 0 {
		t.Fatal("the pane trimmed events without recording the loss")
	}

	view := model.formatStreamContent(ls)
	if !strings.Contains(view, "Transcript incomplete") {
		t.Fatalf("stream pane does not mark the loss:\n%s", view)
	}
	if want := fmt.Sprintf("%d events dropped", ls.lost.total()); !strings.Contains(view, want) {
		t.Fatalf("stream pane does not name the loss %q:\n%s", want, view)
	}
}

func TestStreamPaneStaysQuietWhenNothingIsLost(t *testing.T) {
	model := New(Config{})
	ls := newLiveSession("sse-1", 10)
	ls.append([]*stream.Event{{Kind: stream.KindSSE, Payload: []byte("one")}})

	if view := model.formatStreamContent(ls); strings.Contains(view, "Transcript incomplete") {
		t.Fatalf("a complete transcript was marked incomplete:\n%s", view)
	}
}

func TestStreamCompletionCarriesTheFinalDropCount(t *testing.T) {
	model := New(Config{})
	ls := newLiveSession("sse-1", 10)
	model.liveSessions[ls.id] = ls

	model.handleStreamComplete(streamCompleteMsg{sessionID: ls.id, dropped: 7})
	if ls.lost.listener != 7 {
		t.Fatalf("lost.listener = %d, want the count the listener finished on", ls.lost.listener)
	}
}
