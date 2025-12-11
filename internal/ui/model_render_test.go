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
				{ID: "req:/tmp/a:0", Kind: navigator.KindRequest, Title: "alpha req", Tags: []string{"auth", "reqscope"}},
			},
		},
		{
			ID:    "file:/tmp/b",
			Title: "Req beta",
			Kind:  navigator.KindFile,
			Tags:  []string{"files"},
			Children: []*navigator.Node[any]{
				{ID: "req:/tmp/b:0", Kind: navigator.KindRequest, Title: "beta", Tags: []string{"other", "reqbeta"}},
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
