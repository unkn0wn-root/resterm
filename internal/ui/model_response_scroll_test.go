package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
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

func TestScrollResponseIgnoredInEditorFocus(t *testing.T) {
	model := newModelWithResponseTab(responseTabPretty, &responseSnapshot{ready: true})
	pane := model.pane(responsePanePrimary)
	pane.viewport.Height = 5
	pane.viewport.SetContent(strings.Repeat("line\n", 30))
	pane.viewport.GotoBottom()
	offset := pane.viewport.YOffset
	if offset == 0 {
		t.Fatalf("expected bottom navigation to move offset")
	}

	model.focus = focusEditor
	if cmd, handled := model.scrollShortcutToEdge(true); handled {
		t.Fatalf("expected editor focus to ignore gg, but handled with cmd %v", cmd)
	}
	if pane.viewport.YOffset != offset {
		t.Fatalf(
			"expected editor focus to ignore gg, offset changed from %d to %d",
			offset,
			pane.viewport.YOffset,
		)
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
		t.Fatalf(
			"expected history tab to ignore gg, offset changed from %d to %d",
			offset,
			pane.viewport.YOffset,
		)
	}
}

func TestScrollHistoryShortcutToEdge(t *testing.T) {
	model := New(Config{})
	model.focus = focusResponse

	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary pane")
	}
	pane.activeTab = responseTabHistory

	model.historyEntries = []history.Entry{
		{ID: "1"},
		{ID: "2"},
		{ID: "3"},
	}
	model.historyList.SetItems(makeHistoryItems(model.historyEntries, model.historyScope))
	model.historyList.Select(1)

	if _, handled := model.scrollShortcutToEdge(true); !handled {
		t.Fatalf("expected gg to be handled for history")
	}
	if idx := model.historyList.Index(); idx != 0 {
		t.Fatalf("expected history gg to select first item, got %d", idx)
	}

	if _, handled := model.scrollShortcutToEdge(false); !handled {
		t.Fatalf("expected G to be handled for history")
	}
	if idx := model.historyList.Index(); idx != len(model.historyEntries)-1 {
		t.Fatalf("expected history G to select last item, got %d", idx)
	}
}

func TestScrollShortcutUsesNavigatorWhenFocused(t *testing.T) {
	model := newModelWithResponseTab(responseTabPretty, &responseSnapshot{ready: true})
	pane := model.pane(responsePanePrimary)
	pane.viewport.Height = 4
	pane.viewport.SetContent(strings.Repeat("line\n", 12))
	pane.viewport.GotoBottom()
	responseOffset := pane.viewport.YOffset
	if responseOffset == 0 {
		t.Fatalf("expected bottom navigation to move response offset")
	}

	nodes := []*navigator.Node[any]{
		{
			ID:       "file:one",
			Kind:     navigator.KindFile,
			Expanded: true,
			Children: []*navigator.Node[any]{
				{ID: "req:1", Kind: navigator.KindRequest},
				{ID: "req:2", Kind: navigator.KindRequest},
			},
		},
	}
	model.navigator = navigator.New[any](nodes)
	model.navigator.SetViewportHeight(3)
	model.focus = focusRequests
	model.navigator.SelectLast()
	model.syncNavigatorSelection()

	if _, handled := model.scrollShortcutToEdge(true); !handled {
		t.Fatalf("expected navigator gg to be handled")
	}
	if sel := model.navigator.Selected(); sel == nil || sel.ID != "file:one" {
		t.Fatalf("expected navigator selection to move to top, got %+v", sel)
	}
	if pane.viewport.YOffset != responseOffset {
		t.Fatalf(
			"expected navigator gg to leave response offset %d, got %d",
			responseOffset,
			pane.viewport.YOffset,
		)
	}

	if _, handled := model.scrollShortcutToEdge(false); !handled {
		t.Fatalf("expected navigator G to be handled")
	}
	if sel := model.navigator.Selected(); sel == nil || sel.ID != "req:2" {
		t.Fatalf("expected navigator selection to move to bottom, got %+v", sel)
	}
}
