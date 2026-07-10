package ui

import (
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestLatencyPlaceholder(t *testing.T) {
	m := Model{
		theme:         theme.DefaultTheme(),
		themeRuntime:  newThemeRuntime(theme.DefaultDefinition()),
		latencySeries: newLatencySeries(latCap),
	}

	if got := m.latencyText(); got != "▁▂▄▆█ ms" {
		t.Fatalf("expected placeholder text, got %q", got)
	}
	if got := ansi.Strip(m.renderLatency()); got != "Latency ▁▂▄▆█ ms" {
		t.Fatalf("expected labeled placeholder, got %q", got)
	}
	if !m.latMutedStyle().GetFaint() {
		t.Fatalf("expected faint placeholder style on dark theme")
	}
}

func TestRenderLatencyAfterSample(t *testing.T) {
	m := &Model{
		theme:         theme.DefaultTheme(),
		themeRuntime:  newThemeRuntime(theme.DefaultDefinition()),
		latencySeries: newLatencySeries(latCap),
	}

	m.latencySeries.add(120 * time.Millisecond)
	if got := m.latencyText(); got == latPlaceholder {
		t.Fatalf("expected series render after sample, got %q", got)
	}
	if got := ansi.Strip(m.renderLatency()); got != "Latency ▁▁▁▁█ 120ms · p95 120ms" {
		t.Fatalf("expected labeled latency summary, got %q", got)
	}

	st := latStyle(m.theme, 120*time.Millisecond)
	if st.GetFaint() {
		t.Fatalf("expected active style after sample")
	}
	if fg := st.GetForeground(); fg != m.theme.HeaderValue.GetForeground() {
		t.Fatalf("expected neutral active colour, got %v", fg)
	}
}

func TestLatStyleThresholds(t *testing.T) {
	th := theme.DefaultTheme()
	if fg := latStyle(th, latOKMax).GetForeground(); fg != th.HeaderValue.GetForeground() {
		t.Fatalf("expected neutral colour at threshold, got %v", fg)
	}
	if fg := latStyle(th, latOKMax+time.Millisecond).GetForeground(); fg != latWarnFg {
		t.Fatalf("expected warn colour, got %v", fg)
	}
	if fg := latStyle(th, latWarnMax+time.Millisecond).GetForeground(); fg != th.Error.GetForeground() {
		t.Fatalf("expected error colour, got %v", fg)
	}

	th.Error = lipgloss.NewStyle()
	if fg := latStyle(th, latWarnMax+time.Millisecond).GetForeground(); fg != latErrFg {
		t.Fatalf("expected fallback error colour, got %v", fg)
	}
}
