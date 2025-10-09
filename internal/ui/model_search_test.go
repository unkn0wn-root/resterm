package ui

import (
	"strings"
	"testing"
)

func TestRetreatResponseSearchWrap(t *testing.T) {
	model := New(Config{})
	model.responsePaneFocus = responsePanePrimary
	model.searchResponsePane = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane to be available")
	}
	pane.viewport.Width = 80
	pane.snapshot = &responseSnapshot{
		id:      "snap-1",
		pretty:  ensureTrailingNewline("foo bar\nfoo baz\nfoo"),
		raw:     ensureTrailingNewline("foo bar\nfoo baz\nfoo"),
		headers: ensureTrailingNewline("Status: 200 OK"),
		ready:   true,
	}

	if status := statusFromCmd(t, model.applyResponseSearch("foo", false)); status == nil {
		t.Fatal("expected initial response search status")
	}

	cmd := model.retreatResponseSearch()
	status := statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after retreating search")
	}
	if status.level != statusInfo {
		t.Fatalf("expected info level, got %v", status.level)
	}
	if !strings.Contains(status.text, "Match 3/3") {
		t.Fatalf("expected wrap to last match, got %q", status.text)
	}
	if !strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("expected wrapped indicator, got %q", status.text)
	}
	if pane.search.index != 2 {
		t.Fatalf("expected search index 2, got %d", pane.search.index)
	}

	cmd = model.retreatResponseSearch()
	status = statusFromCmd(t, cmd)
	if status == nil {
		t.Fatal("expected status after moving to previous match")
	}
	if !strings.Contains(status.text, "Match 2/3") {
		t.Fatalf("expected to move to second match, got %q", status.text)
	}
	if strings.Contains(status.text, "(wrapped)") {
		t.Fatalf("did not expect wrapped indicator on second match, got %q", status.text)
	}
	if pane.search.index != 1 {
		t.Fatalf("expected search index 1, got %d", pane.search.index)
	}
}
