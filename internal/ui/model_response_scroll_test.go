package ui

import (
	"strings"
	"testing"
)

func TestScrollResponseToTopAndBottom(t *testing.T) {
	model := newModelWithResponseTab(responseTabPretty, &responseSnapshot{ready: true})
	pane := model.pane(responsePanePrimary)
	pane.viewport.Height = 5
	pane.viewport.SetContent(strings.Repeat("line\n", 30))

	pane.viewport.GotoBottom()
	if pane.viewport.YOffset == 0 {
		t.Fatalf("expected bottom navigation to move offset")
	}

	model.scrollResponseToTop()
	if pane.viewport.YOffset != 0 {
		t.Fatalf("expected gg to jump to top, got offset %d", pane.viewport.YOffset)
	}

	model.scrollResponseToBottom()
	if pane.viewport.YOffset == 0 {
		t.Fatalf("expected G to jump to bottom")
	}
}

func TestScrollResponseIgnoresHistoryTab(t *testing.T) {
	model := newModelWithResponseTab(responseTabHistory, &responseSnapshot{ready: true})
	pane := model.pane(responsePanePrimary)
	pane.viewport.Height = 3
	pane.viewport.SetContent(strings.Repeat("item\n", 10))
	pane.viewport.GotoBottom()
	offset := pane.viewport.YOffset

	model.scrollResponseToTop()
	if pane.viewport.YOffset != offset {
		t.Fatalf("expected history tab to ignore gg, offset changed from %d to %d", offset, pane.viewport.YOffset)
	}
}
