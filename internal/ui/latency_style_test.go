package ui

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestLatencyPlaceholder(t *testing.T) {
	m := Model{
		theme:         theme.DefaultTheme(),
		themeRuntime:  newThemeRuntime(theme.DefaultDefinition()),
		latencySeries: newLatencySeries(latCap),
	}

	if got := m.latencyText(); got != "▁▂▄▆█ ◷" {
		t.Fatalf("expected placeholder text, got %q", got)
	}
	if !m.latencyStyle().GetFaint() {
		t.Fatalf("expected faint placeholder style on dark theme")
	}
}

func TestLatencyStyleAfterSample(t *testing.T) {
	m := &Model{
		theme:         theme.DefaultTheme(),
		themeRuntime:  newThemeRuntime(theme.DefaultDefinition()),
		latencySeries: newLatencySeries(latCap),
	}

	m.latencySeries.add(120 * time.Millisecond)
	if got := m.latencyText(); got == latPlaceholder {
		t.Fatalf("expected series render after sample, got %q", got)
	}

	st := m.latencyStyle()
	if st.GetFaint() {
		t.Fatalf("expected active style after sample")
	}
	if fg := st.GetForeground(); fg != latFg(m.theme.Success, latOkFg) {
		t.Fatalf("expected ok colour, got %v", fg)
	}
}

func TestLatStyleThresholds(t *testing.T) {
	th := theme.DefaultTheme()
	if fg := latStyle(th, latOkMax).GetForeground(); fg != latFg(th.Success, latOkFg) {
		t.Fatalf("expected ok colour at threshold, got %v", fg)
	}
	if fg := latStyle(th, latOkMax+time.Millisecond).GetForeground(); fg != latWarnFg {
		t.Fatalf("expected warn colour, got %v", fg)
	}
	if fg := latStyle(th, latWarnMax+time.Millisecond).GetForeground(); fg != latFg(th.Error, latErrFg) {
		t.Fatalf("expected error colour, got %v", fg)
	}
}
