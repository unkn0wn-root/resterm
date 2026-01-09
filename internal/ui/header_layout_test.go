package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBuildHeaderLineFitsWidth(t *testing.T) {
	left := []string{"RESTERM", "ENV", "WORKSPACE"}
	sep := " "
	right := "Latency: -"
	width := 20
	line := buildHeaderLine(left, sep, right, lipgloss.NewStyle(), width)
	if strings.Contains(line, "\n") {
		t.Fatalf("expected single-line header, got %q", line)
	}
	if got := lipgloss.Width(line); got > width {
		t.Fatalf("expected width <= %d, got %d", width, got)
	}
	if !strings.Contains(line, "Latency") {
		t.Fatalf("expected right text to be present, got %q", line)
	}
}

func TestBuildHeaderLineDropsTrailingSegments(t *testing.T) {
	left := []string{"BRAND", "ONE", "TWO", "THREE"}
	sep := " "
	right := "Latency: -"
	width := 16
	line := buildHeaderLine(left, sep, right, lipgloss.NewStyle(), width)
	if strings.Contains(line, "THREE") {
		t.Fatalf("expected trailing segments to be dropped, got %q", line)
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
	if strings.Contains(line, "Latency") {
		t.Fatalf("expected no right text, got %q", line)
	}
	if got := lipgloss.Width(line); got > width {
		t.Fatalf("expected width <= %d, got %d", width, got)
	}
}
