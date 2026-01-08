package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
)

func TestNavigatorTagChipsFilterMatchesQueryTokens(t *testing.T) {
	model := New(Config{})
	m := &model
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:    "file:/tmp/a",
			Title: "Requests file",
			Kind:  navigator.KindFile,
			Tags:  []string{"workspace"},
			Children: []*navigator.Node[any]{
				{
					ID:    "req:/tmp/a:0",
					Kind:  navigator.KindRequest,
					Title: "alpha req",
					Tags:  []string{"auth", "reqscope"},
				},
			},
		},
		{
			ID:    "file:/tmp/b",
			Title: "Req beta",
			Kind:  navigator.KindFile,
			Tags:  []string{"files"},
			Children: []*navigator.Node[any]{
				{
					ID:    "req:/tmp/b:0",
					Kind:  navigator.KindRequest,
					Title: "beta",
					Tags:  []string{"other", "reqbeta"},
				},
			},
		},
	})
	m.ensureNavigatorFilter()
	m.navigatorFilter.Focus()
	m.navigatorFilter.SetValue("req")
	m.navigator.SetFilter(m.navigatorFilter.Value())

	out := m.navigatorTagChips()
	if out == "" {
		t.Fatalf("expected tag chips to render")
	}
	clean := ansi.Strip(out)
	if strings.Contains(clean, "#workspace") || strings.Contains(clean, "#files") {
		t.Fatalf("expected unrelated tags to be filtered out, got %q", clean)
	}
	if !strings.Contains(clean, "#reqscope") || !strings.Contains(clean, "#reqbeta") {
		t.Fatalf("expected matching tags to remain, got %q", clean)
	}

	// When no prefix hits, we fall back to substring matching.
	m.navigatorFilter.SetValue("scope")
	out = m.navigatorTagChips()
	clean = ansi.Strip(out)
	if !strings.Contains(clean, "#reqscope") {
		t.Fatalf("expected substring fallback to keep reqscope, got %q", clean)
	}
}

func TestNavigatorTagChipsLimit(t *testing.T) {
	model := New(Config{})
	m := &model
	var tags []string
	for i := 0; i < 15; i++ {
		tags = append(tags, fmt.Sprintf("tag%d", i))
	}
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:    "file:/tmp/a",
			Title: "Requests file",
			Kind:  navigator.KindFile,
			Tags:  tags,
		},
	})
	m.ensureNavigatorFilter()
	m.navigatorFilter.Focus()

	out := m.navigatorTagChips()
	if out == "" {
		t.Fatalf("expected tag chips to render")
	}
	clean := ansi.Strip(out)
	parts := strings.Fields(clean)
	tagCount := 0
	for _, p := range parts {
		if strings.HasPrefix(p, "#") {
			tagCount++
		}
	}
	if tagCount != 10 {
		t.Fatalf("expected 10 tags rendered, got %d (%q)", tagCount, clean)
	}
	if !strings.Contains(clean, "...") {
		t.Fatalf("expected ellipsis when tags exceed limit, got %q", clean)
	}
}

func TestStatusBarShowsMinimizedIndicators(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir(), Version: "vTest"})
	model.width = 120
	model.height = 40
	model.ready = true
	_ = model.applyLayout()

	if res := model.setCollapseState(paneRegionSidebar, true); res.blocked {
		t.Fatalf("expected sidebar collapse to be allowed")
	}
	if res := model.setCollapseState(paneRegionEditor, true); res.blocked {
		t.Fatalf("expected editor collapse to be allowed")
	}

	bar := model.renderStatusBar()
	plain := ansi.Strip(bar)
	if strings.Contains(plain, "Editor:min") || strings.Contains(plain, "Response:min") {
		t.Fatalf("expected minimized indicators to replace legacy labels, got %q", plain)
	}
	if !strings.Contains(plain, "● Editor") || !strings.Contains(plain, "● Nav") {
		t.Fatalf("expected green dot indicators for minimized panes, got %q", plain)
	}
	trimmed := strings.TrimSpace(plain)
	if !strings.HasSuffix(trimmed, "vTest") {
		t.Fatalf("expected version to remain on the right, got %q", trimmed)
	}
	if strings.Contains(plain, "\n") {
		t.Fatalf("expected status bar to stay on one line, got %q", plain)
	}
}

func TestTabBadgeTextOmitsSpinner(t *testing.T) {
	m := &Model{}
	m.sending = true
	m.statusPulseFrame = 1
	got := m.tabBadgeText("Live")
	want := "LIVE"
	if got != want {
		t.Fatalf("expected badge %q, got %q", want, got)
	}
}

func TestTabBadgeShortOmitsSpinner(t *testing.T) {
	m := &Model{}
	m.sending = true
	m.statusPulseFrame = 0
	got := m.tabBadgeShort("Pinned")
	want := "P"
	if got != want {
		t.Fatalf("expected short badge %q, got %q", want, got)
	}
}

func TestResponsePaneShowsSendingSpinner(t *testing.T) {
	if len(tabSpinFrames) < 2 {
		t.Fatalf("expected tab spinner frames")
	}
	snap := &responseSnapshot{pretty: ensureTrailingNewline("ok"), ready: true}
	model := newModelWithResponseTab(responseTabPretty, snap)
	model.sending = true
	model.tabSpinIdx = 1
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 40
	pane.viewport.Height = 10

	view := model.renderResponseColumn(responsePanePrimary, true, 40)
	plain := ansi.Strip(view)
	if !strings.Contains(plain, responseSendingBase) {
		t.Fatalf("expected sending message, got %q", plain)
	}
	if !strings.Contains(plain, tabSpinFrames[1]) {
		t.Fatalf("expected spinner frame, got %q", plain)
	}
}
