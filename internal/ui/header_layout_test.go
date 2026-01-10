package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBuildHeaderLineFitsWidth(t *testing.T) {
	left := []string{"RESTERM", "ENV", "WORKSPACE"}
	sep := " "
	right := latencyPlaceholder
	width := 20
	line := buildHeaderLine(left, sep, right, lipgloss.NewStyle(), width)
	if strings.Contains(line, "\n") {
		t.Fatalf("expected single-line header, got %q", line)
	}
	if got := lipgloss.Width(line); got > width {
		t.Fatalf("expected width <= %d, got %d", width, got)
	}
	if !strings.Contains(line, "▁") {
		t.Fatalf("expected right text to be present, got %q", line)
	}
}

func TestBuildHeaderLineRightOnly(t *testing.T) {
	sep := " "
	right := latencyPlaceholder
	width := 4
	line := buildHeaderLine(nil, sep, right, lipgloss.NewStyle(), width)
	if strings.Contains(line, "\n") {
		t.Fatalf("expected single-line header, got %q", line)
	}
	if got := lipgloss.Width(line); got > width {
		t.Fatalf("expected width <= %d, got %d", width, got)
	}
	if !strings.Contains(line, "▁") {
		t.Fatalf("expected right text to be present, got %q", line)
	}
}

func TestBuildHeaderLineDropsTrailingSegments(t *testing.T) {
	left := []string{"BRAND", "ONE", "TWO", "THREE"}
	sep := " "
	right := latencyPlaceholder
	width := 16
	line := buildHeaderLine(left, sep, right, lipgloss.NewStyle(), width)
	if strings.Contains(line, "THREE") {
		t.Fatalf("expected trailing segments to be dropped, got %q", line)
	}
	if got := lipgloss.Width(line); got > width {
		t.Fatalf("expected width <= %d, got %d", width, got)
	}
}

func TestBuildHeaderLineNarrowWidthDropsRight(t *testing.T) {
	left := []string{"BRAND", "ONE"}
	sep := " "
	right := latencyPlaceholder
	width := 4
	line := buildHeaderLine(left, sep, right, lipgloss.NewStyle(), width)
	if strings.Contains(line, "▁") {
		t.Fatalf("expected right text to be dropped, got %q", line)
	}
	if got := lipgloss.Width(line); got > width {
		t.Fatalf("expected width <= %d, got %d", width, got)
	}
}

func TestBuildHeaderLineLeftOnly(t *testing.T) {
	left := []string{"BRAND", "ONE"}
	sep := " "
	width := 10
	line := buildHeaderLine(left, sep, "", lipgloss.NewStyle(), width)
	if strings.Contains(line, "▁") {
		t.Fatalf("expected no right text, got %q", line)
	}
	if got := lipgloss.Width(line); got > width {
		t.Fatalf("expected width <= %d, got %d", width, got)
	}
}

func TestBuildHeaderLineRightStylePadding(t *testing.T) {
	left := []string{"BRAND"}
	sep := " "
	right := "LATENCY"
	width := 18
	style := lipgloss.NewStyle().Padding(0, 1)
	line := buildHeaderLine(left, sep, right, style, width)
	if got := lipgloss.Width(line); got > width {
		t.Fatalf("expected width <= %d, got %d", width, got)
	}
	if !strings.Contains(line, "LATENCY") {
		t.Fatalf("expected right text to be present, got %q", line)
	}
}
