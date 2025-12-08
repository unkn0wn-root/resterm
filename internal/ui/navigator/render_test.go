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
	if !strings.Contains(clean, "SSE") || !strings.Contains(clean, "SCRIPT") || !strings.Contains(clean, "WS") {
		t.Fatalf("expected all badge labels to render, got %q", clean)
	}
}
