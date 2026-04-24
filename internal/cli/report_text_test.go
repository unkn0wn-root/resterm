package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/runx/report"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestWriteTextStyledAppliesANSIColor(t *testing.T) {
	rep := &runfmt.Report{
		FilePath: "workflow.http",
		EnvName:  "dev",
		Results: []runfmt.Result{{
			Kind:     "workflow",
			Name:     "sample-order",
			Method:   "WORKFLOW",
			Status:   runfmt.StatusPass,
			Duration: 125 * time.Millisecond,
			Steps: []runfmt.Step{
				{
					Name:     "Login",
					Status:   runfmt.StatusPass,
					Duration: 25 * time.Millisecond,
					HTTP:     &runfmt.HTTP{Status: "200 OK", StatusCode: 200},
				},
				{
					Name:     "Checkout",
					Status:   runfmt.StatusFail,
					Summary:  "unexpected status code 500",
					Duration: 50 * time.Millisecond,
				},
			},
		}},
		Total:   1,
		Passed:  0,
		Failed:  1,
		Skipped: 0,
	}

	var plain strings.Builder
	if err := runfmt.WriteText(&plain, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}

	var colored strings.Builder
	if err := WriteTextStyled(&colored, rep, termcolor.TrueColor(), nil); err != nil {
		t.Fatalf("WriteTextStyled(...): %v", err)
	}

	out := colored.String()
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ansi output, got %q", out)
	}
	if got := ansi.Strip(out); got != plain.String() {
		t.Fatalf(
			"expected stripped output to match plain text\nwant:\n%s\n\ngot:\n%s",
			plain.String(),
			got,
		)
	}
}

func TestWriteTextStyledUsesThemePalette(t *testing.T) {
	rep := &runfmt.Report{
		FilePath: "workflow.http",
		EnvName:  "dev",
		Results: []runfmt.Result{{
			Kind:   "workflow",
			Name:   "sample-order",
			Method: "WORKFLOW",
			Status: runfmt.StatusPass,
		}},
		Total:  1,
		Passed: 1,
	}

	var dark strings.Builder
	if err := WriteTextStyled(&dark, rep, termcolor.TrueColor(), nil); err != nil {
		t.Fatalf("WriteTextStyled(...): %v", err)
	}

	lightTheme := theme.DefaultTheme()
	lightTheme.ExplainMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b"))
	lightTheme.PaneActiveForeground = lipgloss.Color("#0f172a")
	lightTheme.Success = lipgloss.NewStyle().Foreground(lipgloss.Color("#15803d"))
	lightTheme.Error = lipgloss.NewStyle().Foreground(lipgloss.Color("#b91c1c"))
	lightTheme.StatusBarKey = lipgloss.NewStyle().Foreground(lipgloss.Color("#b45309"))

	var light strings.Builder
	if err := WriteTextStyled(&light, rep, termcolor.TrueColor(), &theme.Definition{
		Key: "daybreak",
		Metadata: theme.Metadata{
			Name: "Daybreak",
			Tags: []string{"light"},
		},
		Theme: lightTheme,
	}); err != nil {
		t.Fatalf("WriteTextStyled(...): %v", err)
	}

	if dark.String() == light.String() {
		t.Fatalf("expected light theme text palette to differ from default dark palette")
	}
	if ansi.Strip(dark.String()) != ansi.Strip(light.String()) {
		t.Fatalf("expected theme palette changes to preserve text output")
	}
}
