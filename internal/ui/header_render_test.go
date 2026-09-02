package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func headerTestModel(t *testing.T, width int) *Model {
	t.Helper()

	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
	})

	model := newTestModelWithDoc(sampleRequestDoc)
	model.ready = true
	model.width = width
	model.height = 30
	model.ws.root = "/tmp/acme-api"
	model.activeRequestTitle = "GET create-user"
	return model
}

func TestHeaderRendersLayout(t *testing.T) {
	model := headerTestModel(t, 160)
	model.testResults = []scripts.TestResult{{Name: "failed", Passed: false}}
	model.headerTransport = headerTransportStatus{label: "201", level: statusSuccess}
	model.latencySeries.add(120 * time.Millisecond)

	view := ansi.Strip(model.renderHeader())
	lines := strings.Split(view, "\n")
	if len(lines) != 2 {
		t.Fatalf("header rows = %d, want 2:\n%s", len(lines), view)
	}
	for _, want := range []string{
		headerBrandName,
		iconHeaderEnv + " " + labelHeaderEnv + " default",
		iconHeaderWorkspace + " acme-api",
		iconHeaderRequests + " " + labelHeaderRequests,
		iconHeaderActive + " GET create-user",
		"✗ 1 fail",
		"201",
		latLabel,
		"? Help",
	} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("header is missing %q:\n%s", want, lines[0])
		}
	}
	for _, unwanted := range []string{"Tab Focus", "^Q Quit", ": Cmd"} {
		if strings.Contains(lines[0], unwanted) {
			t.Errorf("header contains %q:\n%s", unwanted, lines[0])
		}
	}
	for _, want := range []string{
		headerBrandName + headerGroupSep + iconHeaderEnv,
		iconHeaderEnv + " " + labelHeaderEnv + " default" + headerCellSep + iconHeaderWorkspace,
		"201 · " + latLabel,
	} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("header is missing separator layout %q:\n%s", want, lines[0])
		}
	}
	wantRule := " " + strings.Repeat("─", model.width-2) + " "
	if lines[1] != wantRule {
		t.Fatalf("header rule = %q, want %q", lines[1], wantRule)
	}
}

func TestHeaderDropsHelpBeforeLatency(t *testing.T) {
	model := headerTestModel(t, 160)

	for width := 70; width >= 30; width-- {
		model.width = width
		view := ansi.Strip(model.renderHeader())
		if strings.Contains(view, "?") {
			continue
		}
		if !strings.Contains(view, latLabel) {
			t.Fatalf("header dropped latency before Help at width %d:\n%s", width, view)
		}
		return
	}
	t.Fatal("expected Help to be hidden at a narrow width")
}

func TestHeaderShowsNoSelectedRequest(t *testing.T) {
	model := headerTestModel(t, 160)
	model.activeRequestTitle = ""

	if view := ansi.Strip(model.renderHeader()); !strings.Contains(view, iconHeaderActive+" none selected") {
		t.Fatalf("header does not show that no request is selected:\n%s", view)
	}

	model.width = 70
	if view := ansi.Strip(model.renderHeader()); !strings.Contains(view, iconHeaderActive+" none") {
		t.Fatalf("narrow header does not use the short empty value:\n%s", view)
	}
}

func TestHeaderDropsResponseCodeBeforeLatency(t *testing.T) {
	model := headerTestModel(t, 50)
	model.headerTransport = headerTransportStatus{label: "200", level: statusSuccess}
	model.latencySeries.add(180 * time.Millisecond)

	for width := 50; width >= 30; width-- {
		model.width = width
		view := ansi.Strip(model.renderHeader())
		if strings.Contains(view, "200") {
			continue
		}
		if !strings.Contains(view, latLabel) {
			t.Fatalf("header dropped latency with the status code at width %d:\n%s", width, view)
		}
		return
	}
	t.Fatal("expected the response code to be hidden at a narrow width")
}

func TestHeaderKeepsThemeBackground(t *testing.T) {
	model := headerTestModel(t, 160)
	background := lipgloss.Color("#010203")
	model.theme.Header = model.theme.Header.Background(background)
	model.headerTransport = headerTransportStatus{label: "404", level: statusWarn}
	model.latencySeries.add(750 * time.Millisecond)

	line := strings.SplitN(model.renderHeader(), "\n", 2)[0]
	backgrounds := renderedCellBackgrounds(line)
	want := renderedCellBackgrounds(lipgloss.NewStyle().Background(background).Render("x"))[0]
	for index, got := range backgrounds {
		if !slices.Equal(got, want) {
			t.Fatalf("header cell %d has background %v, want %v", index, got, want)
		}
	}
}

func TestHeaderFitsSupportedWidths(t *testing.T) {
	model := headerTestModel(t, 200)
	statuses := []headerTransportStatus{
		{},
		{label: "ResourceExhausted", level: statusWarn},
	}

	for _, status := range statuses {
		model.headerTransport = status
		for width := 3; width <= 200; width++ {
			model.width = width
			for _, line := range strings.Split(model.renderHeader(), "\n") {
				if got := lipgloss.Width(line); got > width {
					t.Fatalf("status %q width %d rendered %d cells", status.label, width, got)
				}
			}
		}
	}
}
