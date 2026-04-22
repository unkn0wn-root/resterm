package cli

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestPromptRunRequestChoiceTTYFallsBackToTextForNonTTYIO(t *testing.T) {
	choices := []RunRequestChoice{
		{Line: 3, Method: "GET", Name: "one", Target: "https://example.com/one", Label: "GET one"},
		{
			Line:   7,
			Method: "POST",
			Name:   "two",
			Target: "https://example.com/two",
			Label:  "POST two",
		},
	}

	var out bytes.Buffer
	got, err := PromptRunRequestChoice(
		strings.NewReader("2\n"),
		&out,
		"many.http",
		choices,
		RunRequestPromptOptions{
			TTY:   true,
			Color: termcolor.TrueColor(),
		},
	)
	if err != nil {
		t.Fatalf("PromptRunRequestChoice: %v", err)
	}
	if got.Line != 7 {
		t.Fatalf("expected second choice, got %+v", got)
	}
	if s := xansi.Strip(out.String()); !strings.Contains(s, "Select request [1-2]:") {
		t.Fatalf("expected text prompt fallback, got %q", s)
	}
}

func TestRunRequestPickerModelMovesWithArrowKeys(t *testing.T) {
	m := newRunRequestPickerModel(
		"many.http",
		[]RunRequestChoice{
			{Line: 3, Label: "GET one"},
			{Line: 7, Label: "GET two"},
		},
		termcolor.TrueColor(),
		nil,
	)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got, ok := next.(*runRequestPickerModel)
	if !ok {
		t.Fatalf("expected picker model, got %T", next)
	}
	if cmd != nil {
		t.Fatalf("expected no quit command on move")
	}
	if got.sel != 1 {
		t.Fatalf("expected second row selected, got %d", got.sel)
	}
}

func TestRunRequestPickerModelCancelsOnQ(t *testing.T) {
	m := newRunRequestPickerModel(
		"many.http",
		[]RunRequestChoice{
			{Line: 3, Label: "GET one"},
			{Line: 7, Label: "GET two"},
		},
		termcolor.TrueColor(),
		nil,
	)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got, ok := next.(*runRequestPickerModel)
	if !ok {
		t.Fatalf("expected picker model, got %T", next)
	}
	if cmd == nil {
		t.Fatalf("expected quit command on cancel")
	}
	if !got.canceled {
		t.Fatalf("expected picker to be canceled")
	}
}

func TestRunRequestPickerViewFitsConfiguredWidth(t *testing.T) {
	p := newRunRequestPickerModel(
		"/very/long/path/to/examples/scripts.http",
		[]RunRequestChoice{
			{
				Line:   6,
				Method: "POST",
				Name:   "ReportsBootstrapWithAnExtraLongName",
				Target: "{{services.api.base}}/reports/sessions",
				Label:  "POST ReportsBootstrapWithAnExtraLongName",
			},
			{
				Line:   28,
				Method: "GET",
				Name:   "ReportsListWithAnExtraLongName",
				Target: "{{services.api.base}}/reports",
				Label:  "GET ReportsListWithAnExtraLongName",
			},
		},
		termcolor.TrueColor(),
		nil,
	)
	p.wid = 60

	for i, line := range strings.Split(p.View(), "\n") {
		if got := xansi.StringWidth(line); got > p.wid {
			t.Fatalf("line %d exceeds width: got %d want <= %d\n%q", i+1, got, p.wid, line)
		}
	}
}

func TestRunRequestPickerAppendDigitResetsInvalidJump(t *testing.T) {
	p := newRunRequestPickerModel(
		"many.http",
		[]RunRequestChoice{
			{Line: 3, Label: "GET one"},
			{Line: 7, Label: "GET two"},
		},
		termcolor.TrueColor(),
		nil,
	)

	p.appendDigit('2')
	if p.num != "2" || p.sel != 1 {
		t.Fatalf("expected jump to second choice, got num=%q sel=%d", p.num, p.sel)
	}

	p.appendDigit('1')
	if p.num != "1" || p.sel != 0 {
		t.Fatalf(
			"expected invalid append to reset to first choice, got num=%q sel=%d",
			p.num,
			p.sel,
		)
	}
	if p.note != "" {
		t.Fatalf("expected no validation note after reset, got %q", p.note)
	}
}

func TestRunRequestPickerViewHandlesEmptyChoices(t *testing.T) {
	p := newRunRequestPickerModel("many.http", nil, termcolor.TrueColor(), nil)

	out := xansi.Strip(p.View())
	if !strings.Contains(out, "No requests found.") {
		t.Fatalf("expected empty picker message, got %q", out)
	}
}

func TestRunRequestPickerAppliesLightThemeStyles(t *testing.T) {
	th := theme.DefaultTheme()
	th.HeaderTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1e40af")).Bold(true)
	th.HeaderValue = lipgloss.NewStyle().Foreground(lipgloss.Color("#0f172a"))
	th.ExplainMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("#64748b"))
	th.ExplainLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("#0369a1")).Bold(true)
	th.ListItemTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#0f172a"))
	th.ListItemDescription = lipgloss.NewStyle().Foreground(lipgloss.Color("#334155"))
	th.ListItemSelectedTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0f172a")).
		Background(lipgloss.Color("#bfdbfe")).
		Bold(true)
	th.ListItemSelectedDescription = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0f172a")).
		Background(lipgloss.Color("#bae6fd"))
	th.ResponseCursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280")).Bold(true)
	th.CommandBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#1f2933")).
		Background(lipgloss.Color("#f8fafc"))

	def := theme.Definition{
		Key: "daybreak",
		Metadata: theme.Metadata{
			Name: "Daybreak",
			Tags: []string{"light"},
		},
		Theme: th,
	}

	p := newRunRequestPickerModel(
		"many.http",
		[]RunRequestChoice{{Line: 3, Label: "GET one"}},
		termcolor.TrueColor(),
		&def,
	)

	if got := p.st.name.GetForeground(); got != lipgloss.Color("#0f172a") {
		t.Fatalf("expected light row text foreground, got %v", got)
	}
	if got := p.st.rowSel.GetBackground(); got != lipgloss.Color("#bfdbfe") {
		t.Fatalf("expected light selection background, got %v", got)
	}
	if got := p.st.rowSel.GetForeground(); got != lipgloss.Color("#0f172a") {
		t.Fatalf("expected light selection foreground, got %v", got)
	}
	if got := p.st.cursorSel.GetForeground(); got != lipgloss.Color("#1e40af") {
		t.Fatalf("expected accent cursor foreground, got %v", got)
	}
	if got := p.st.box.GetBackground(); got != lipgloss.Color("#f8fafc") {
		t.Fatalf("expected light box background, got %v", got)
	}
}
