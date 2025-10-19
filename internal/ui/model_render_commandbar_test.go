package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestRenderCommandBarContainerPreservesOuterPadding(t *testing.T) {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("#112233")).
		Padding(0, 1)

	out := renderCommandBarContainer(style, "Hi")

	if !strings.HasPrefix(out, " ") {
		t.Fatalf("expected left gutter to remain unstyled, got %q", out)
	}
	if !strings.HasSuffix(out, " ") {
		t.Fatalf("expected right gutter to remain unstyled, got %q", out)
	}
	if lipgloss.Width(out) != 4 { // 1 pad left + len(Hi) + 1 pad right
		t.Fatalf("expected rendered width 4, got %d", lipgloss.Width(out))
	}
}

func TestRenderCommandBarContainerRespectsWidthConstraints(t *testing.T) {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("#445566")).
		Padding(0, 2).
		Width(10)

	out := renderCommandBarContainer(style, "OK")

	if lipgloss.Width(out) != 10 {
		t.Fatalf("expected rendered width 10, got %d", lipgloss.Width(out))
	}
	if !strings.HasPrefix(out, "  ") {
		t.Fatalf("expected leading gutter to remain blank, got %q", out)
	}
}

func TestRenderCommandBarContainerWithColoredLeadingSpaces(t *testing.T) {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("#123456"))

	out := renderCommandBarContainer(style, "Hi", withColoredLeadingSpaces(2))

	if !strings.Contains(out, "  Hi") {
		t.Fatalf("expected colored leading spaces before content, got %q", out)
	}
	if lipgloss.Width(out) != 4 {
		t.Fatalf("expected rendered width 4, got %d", lipgloss.Width(out))
	}
}
