package navigator

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestRenderBadgesUsesCommaSeparator(t *testing.T) {
	th := theme.DefaultTheme()
	out := renderBadges([]string{"  SSE  ", "SCRIPT", "WS"}, th)
	clean := ansi.Strip(out)

	if strings.Count(clean, ",") != 2 {
		t.Fatalf("expected comma separators between badges, got %q", clean)
	}
	if strings.Contains(clean, "  ,") || strings.Contains(clean, ",  ") {
		t.Fatalf("expected comma separators without extra spacing, got %q", clean)
	}
	if strings.HasSuffix(clean, ",") {
		t.Fatalf("expected no trailing comma, got %q", clean)
	}
	if !strings.Contains(clean, "SSE") || !strings.Contains(clean, "SCRIPT") ||
		!strings.Contains(clean, "WS") {
		t.Fatalf("expected all badge labels to render, got %q", clean)
	}
}

func TestRenderWorkflowShowsBadgeNoCaret(t *testing.T) {
	th := theme.DefaultTheme()
	node := Flat[any]{
		Node: &Node[any]{
			Kind:   KindWorkflow,
			Title:  "sample-order",
			Badges: []string{"4 steps"},
			Tags:   []string{"demo", "workflow"},
		},
	}
	out := renderRow(node, false, th, 80, true, false)
	clean := ansi.Strip(out)
	if strings.Contains(clean, iconCaretClosed) || strings.Contains(clean, iconCaretOpen) {
		t.Fatalf("expected workflow row without caret, got %q", clean)
	}
	if !strings.Contains(clean, "WF") {
		t.Fatalf("expected workflow badge, got %q", clean)
	}
	if !strings.Contains(clean, "WF  sample-order") {
		t.Fatalf("expected padded workflow badge before title, got %q", clean)
	}
}

func TestRenderRowShowsBadgesButOmitsTags(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:   KindRequest,
			Title:  "Fetch user",
			Method: "GET",
			Target: "https://example.com/users/1",
			Tags:   []string{"beta", "users"},
			Badges: []string{"AUTH", "gRPC"},
		},
	}
	out := renderRow(row, false, th, 80, true, false)
	clean := ansi.Strip(out)
	if strings.Contains(clean, "#beta") || strings.Contains(clean, "#users") {
		t.Fatalf("expected tags to be omitted from list row, got %q", clean)
	}
	if !strings.Contains(clean, "AUTH") || !strings.Contains(clean, "gRPC") {
		t.Fatalf("expected badges to render in list row, got %q", clean)
	}
	if !strings.Contains(clean, "Fetch user") ||
		!strings.Contains(clean, "https://example.com/users/1") {
		t.Fatalf("expected request summary to remain in list row, got %q", clean)
	}
}

func TestRenderRowShowsDirIcon(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:     KindDir,
			Title:    "rts",
			Expanded: false,
		},
	}
	out := renderRow(row, false, th, 80, true, false)
	clean := ansi.Strip(out)
	if !strings.Contains(clean, iconDirClosed) {
		t.Fatalf("expected directory icon, got %q", clean)
	}
	if strings.Contains(clean, iconCaretClosed) || strings.Contains(clean, iconCaretOpen) {
		t.Fatalf("expected directory row without caret, got %q", clean)
	}
}

func TestRenderRowShowsRTSIcon(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:  KindFile,
			Title: "apply_patch.rts",
			Payload: Payload[any]{
				FilePath: "apply_patch.rts",
			},
		},
	}
	out := renderRow(row, false, th, 80, true, false)
	clean := ansi.Strip(out)
	if !strings.Contains(clean, iconRTS) {
		t.Fatalf("expected rts icon, got %q", clean)
	}
	if strings.Contains(clean, "•") {
		t.Fatalf("expected rts row without bullet icon, got %q", clean)
	}
}

func TestRenderRTSUsesModuleIndicator(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:    KindFile,
			Title:   "mod.rts",
			Payload: Payload[any]{FilePath: "/tmp/mod.rts"},
		},
	}
	out := renderRow(row, false, th, 80, true, false)
	clean := ansi.Strip(out)
	if strings.Contains(clean, iconCaretClosed) || strings.Contains(clean, iconCaretOpen) {
		t.Fatalf("expected rts row without caret, got %q", clean)
	}
	if !strings.Contains(clean, iconRTS) {
		t.Fatalf("expected rts icon, got %q", clean)
	}
}
