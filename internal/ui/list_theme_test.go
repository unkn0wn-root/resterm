package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestMergeListStylePreservesBaseAndAppliesOverride(t *testing.T) {
	base := lipgloss.NewStyle().MarginLeft(2)
	override := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff0000"))

	merged := mergeListStyle(base, override)

	if got := merged.GetMarginLeft(); got != 2 {
		t.Fatalf("expected margin left to remain 2, got %d", got)
	}
	if got := merged.GetForeground(); got != lipgloss.Color("#ff0000") {
		t.Fatalf("expected foreground #ff0000, got %v", got)
	}
}

func TestMergeListStyleWithEmptyOverrideReturnsBase(t *testing.T) {
	base := lipgloss.NewStyle().MarginLeft(3).Foreground(lipgloss.Color("#00ff00"))
	override := lipgloss.Style{}

	merged := mergeListStyle(base, override)

	if got := merged.GetMarginLeft(); got != 3 {
		t.Fatalf("expected margin left 3, got %d", got)
	}
	if got := merged.GetForeground(); got != lipgloss.Color("#00ff00") {
		t.Fatalf("expected foreground #00ff00, got %v", got)
	}
}
