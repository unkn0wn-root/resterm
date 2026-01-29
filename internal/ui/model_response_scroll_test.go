package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestScrollResponseToEdgeUpdatesCursor(t *testing.T) {
	snap := &responseSnapshot{id: "snap", ready: true}
	model := newModelWithResponseTab(responseTabPretty, snap)
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 40
	pane.viewport.Height = 3

	content := "one\ntwo\nthree\nfour\nfive\nsix"
	pane.snapshot.pretty = content
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, pane.viewport.Width),
	)
	pane.viewport.SetContent(content)
	pane.cursor = respCursor{
		on:   true,
		line: 4,
		tab:  responseTabPretty,
		sid:  "snap",
	}

	model.scrollResponseToTop()
	if pane.cursor.line != 0 {
		t.Fatalf("expected cursor to move to top line, got %d", pane.cursor.line)
	}

	model.scrollResponseToBottom()
	expected := len(strings.Split(content, "\n")) - 1
	if pane.cursor.line != expected {
		t.Fatalf("expected cursor to move to bottom line %d, got %d", expected, pane.cursor.line)
	}
}

func TestResponseArrowScrollKeepsCursorInView(t *testing.T) {
	snap := &responseSnapshot{id: "snap", ready: true}
	model := newModelWithResponseTab(responseTabPretty, snap)
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 20
	pane.viewport.Height = 3

	content := "one\ntwo\nthree\nfour\nfive"
	pane.snapshot.pretty = content
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, pane.viewport.Width),
	)
	pane.viewport.SetContent(content)
	pane.cursor = respCursor{
		on:   true,
		line: 0,
		tab:  responseTabPretty,
		sid:  "snap",
	}

	model.handleKey(tea.KeyMsg{Type: tea.KeyDown})

	cache := pane.wrapCache[responseTabPretty]
	expected := cache.rev[pane.viewport.YOffset]
	if pane.cursor.line != expected {
		t.Fatalf(
			"expected cursor to follow viewport to line %d, got %d",
			expected,
			pane.cursor.line,
		)
	}
}

func TestResponseArrowScrollPreservesCursorRow(t *testing.T) {
	snap := &responseSnapshot{id: "snap", ready: true}
	model := newModelWithResponseTab(responseTabPretty, snap)
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 20
	pane.viewport.Height = 3

	content := "one\ntwo\nthree\nfour\nfive\nsix"
	pane.snapshot.pretty = content
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, pane.viewport.Width),
	)
	pane.viewport.SetContent(content)
	pane.viewport.SetYOffset(1)
	pane.cursor = respCursor{
		on:   true,
		line: 2,
		tab:  responseTabPretty,
		sid:  "snap",
	}

	model.handleKey(tea.KeyMsg{Type: tea.KeyDown})

	if pane.cursor.line != 3 {
		t.Fatalf(
			"expected cursor to stay on middle row after scroll, got line %d",
			pane.cursor.line,
		)
	}
}

func TestResponseArrowScrollBringsCursorIntoView(t *testing.T) {
	snap := &responseSnapshot{id: "snap", ready: true}
	model := newModelWithResponseTab(responseTabPretty, snap)
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 20
	pane.viewport.Height = 3

	content := "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight"
	pane.snapshot.pretty = content
	pane.wrapCache[responseTabPretty] = wrapCache(
		responseTabPretty,
		content,
		responseWrapWidth(responseTabPretty, pane.viewport.Width),
	)
	pane.viewport.SetContent(content)
	pane.viewport.SetYOffset(3)
	pane.cursor = respCursor{
		on:   true,
		line: 0,
		tab:  responseTabPretty,
		sid:  "snap",
	}

	model.handleKey(tea.KeyMsg{Type: tea.KeyDown})

	cache := pane.wrapCache[responseTabPretty]
	expected := cache.rev[pane.viewport.YOffset]
	if pane.cursor.line != expected {
		t.Fatalf("expected cursor to snap into view at line %d, got %d", expected, pane.cursor.line)
	}
}
