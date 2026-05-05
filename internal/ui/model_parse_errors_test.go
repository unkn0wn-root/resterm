package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

func TestStyleLinesRendersChainWithWarmDiagnosticColor(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prevProfile)

	model := New(Config{})
	chain := `╰─> Get "https://api.local"`
	note := "help: No response payload was received."
	st := model.errSty()

	gotChain := st.line(diag.Line{Kind: diag.LineChain, Text: chain})
	wantChain := model.chainLineStyle().Render(chain)
	if gotChain != wantChain {
		t.Fatalf("chain line style = %q, want warm diagnostic style %q", gotChain, wantChain)
	}
	if strings.Contains(gotChain, "\x1b[2m") {
		t.Fatalf("chain line should not use faint style, got %q", gotChain)
	}
	if gotChain == model.statusBarFg(
		model.theme.Error,
		statusErrorLightColor,
		statusErrorDarkColor,
	).Render(chain) {
		t.Fatalf("chain line should not use error color, got %q", gotChain)
	}
	if gotChain == model.themeRuntime.subtleTextStyle(model.theme).Render(chain) {
		t.Fatalf("chain line should not use info/subtle color, got %q", gotChain)
	}

	gotNote := st.line(diag.Line{Kind: diag.LineHelp, Text: note})
	if gotNote == gotChain || !strings.Contains(gotNote, "\x1b[2m") {
		t.Fatalf("note line should stay subtle; note=%q chain=%q", gotNote, gotChain)
	}
}
