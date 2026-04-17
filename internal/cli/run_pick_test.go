package cli

import (
	"bytes"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func TestPromptRunRequestChoiceTTYFallsBackToTextForNonTTYIO(t *testing.T) {
	choices := []RunRequestChoice{
		{Line: 3, Method: "GET", Name: "one", Target: "https://example.com/one", Label: "GET one"},
		{Line: 7, Method: "POST", Name: "two", Target: "https://example.com/two", Label: "POST two"},
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
	)

	p.appendDigit('2')
	if p.num != "2" || p.sel != 1 {
		t.Fatalf("expected jump to second choice, got num=%q sel=%d", p.num, p.sel)
	}

	p.appendDigit('1')
	if p.num != "1" || p.sel != 0 {
		t.Fatalf("expected invalid append to reset to first choice, got num=%q sel=%d", p.num, p.sel)
	}
	if p.note != "" {
		t.Fatalf("expected no validation note after reset, got %q", p.note)
	}
}

func TestRunRequestPickerViewHandlesEmptyChoices(t *testing.T) {
	p := newRunRequestPickerModel("many.http", nil, termcolor.TrueColor())

	out := xansi.Strip(p.View())
	if !strings.Contains(out, "No requests found.") {
		t.Fatalf("expected empty picker message, got %q", out)
	}
}
