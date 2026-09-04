package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/mock"
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

func assertHeaderFits(t *testing.T, model *Model, label string) {
	t.Helper()

	for width := 3; width <= 200; width++ {
		model.width = width
		view := model.renderHeader()
		if got := lipgloss.Height(view); got != 2 {
			t.Fatalf("%s width %d rendered %d rows, want 2: %q", label, width, got, ansi.Strip(view))
		}
		for line := range strings.SplitSeq(view, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("%s width %d rendered %d cells: %q", label, width, got, ansi.Strip(line))
			}
		}
	}
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
		iconHeaderWorkspace + " " + labelHeaderWorkspace + " acme-api",
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
		iconHeaderEnv + " " + labelHeaderEnv + " default  " + iconHeaderWorkspace + " " +
			labelHeaderWorkspace,
		iconHeaderRequests + " " + labelHeaderRequests + " 0 " + iconHeaderActive + " GET create-user",
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

func TestHeaderMockVariantsSummariseSourceScope(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name string
		src  mock.Sources
		want []string
	}{
		{name: "workspace", src: mock.Sources{Path: root}, want: []string{"workspace"}},
		{
			name: "recursive workspace",
			src:  mock.Sources{Path: root, Recursive: true},
			want: []string{"workspace/**"},
		},
		{
			name: "one file",
			src: mock.Sources{
				Path:  root,
				Files: []string{filepath.Join(root, "mock.http")},
			},
			want: []string{"mock.http"},
		},
		{
			name: "several files",
			src: mock.Sources{
				Path: root,
				Files: []string{
					filepath.Join(root, "nested", "users.http"),
					filepath.Join(root, "payments.http"),
					filepath.Join(root, "errors.rest"),
				},
			},
			want: []string{filepath.Join("nested", "users.http") + " +2", "users.http +2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := headerMockVariants(tt.src); !slices.Equal(got, tt.want) {
				t.Fatalf("headerMockVariants() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHeaderShowsMockScopeOnlyWhileRunning(t *testing.T) {
	model := headerTestModel(t, 160)
	model.mock.src = mock.Sources{
		Path:  model.ws.root,
		Files: []string{filepath.Join(model.ws.root, "fixtures", "mock.http")},
	}
	want := iconHeaderMock + " " + labelHeaderMock + " " + filepath.Join("fixtures", "mock.http")

	if view := ansi.Strip(model.renderHeader()); strings.Contains(view, want) {
		t.Fatalf("stopped header contains mock scope %q:\n%s", want, view)
	}

	model.mock.server = &mock.Server{}
	if view := ansi.Strip(model.renderHeader()); !strings.Contains(view, want) {
		t.Fatalf("running header is missing mock scope %q:\n%s", want, view)
	}

	model.mock.server = nil
	if view := ansi.Strip(model.renderHeader()); strings.Contains(view, want) {
		t.Fatalf("stopped header kept mock scope %q:\n%s", want, view)
	}
}

func TestHeaderShortensMockScopeBeforeTruncating(t *testing.T) {
	model := headerTestModel(t, 160)
	model.mock.server = &mock.Server{}
	model.mock.src = mock.Sources{
		Path: model.ws.root,
		Files: []string{
			filepath.Join(model.ws.root, "nested", "users.http"),
			filepath.Join(model.ws.root, "payments.http"),
			filepath.Join(model.ws.root, "errors.rest"),
		},
	}
	full := filepath.Join("nested", "users.http") + " +2"
	base := "users.http +2"

	for width := 160; width >= 30; width-- {
		model.width = width
		view := ansi.Strip(model.renderHeader())
		if strings.Contains(view, full) {
			continue
		}
		if !strings.Contains(view, base) {
			t.Fatalf("mock scope lost characters before shortening at width %d:\n%s", width, view)
		}
		return
	}
	t.Fatal("expected the nested mock scope to shorten at a narrow width")
}

func TestHeaderWithMockFitsSupportedWidths(t *testing.T) {
	model := headerTestModel(t, 200)
	model.mock.server = &mock.Server{}
	model.mock.src = mock.Sources{
		Path: model.ws.root,
		Files: []string{
			filepath.Join(model.ws.root, "fixtures", strings.Repeat("long-source-", 4)+"users.http"),
			filepath.Join(model.ws.root, "payments.http"),
		},
	}
	model.testResults = []scripts.TestResult{{Name: "passed", Passed: true}}

	assertHeaderFits(t, model, "running mock")
}

func TestHeaderKeepsUnprintableNamesOnOneRow(t *testing.T) {
	const raw = "\n\x1b[2J"

	tests := []struct {
		name  string
		apply func(*Model)
	}{
		{
			name:  "workspace",
			apply: func(m *Model) { m.ws.root = "/tmp/ac" + raw + "me-api" },
		},
		{
			name: "mock source",
			apply: func(m *Model) {
				m.mock.server = &mock.Server{}
				m.mock.src = mock.Sources{
					Path:  m.ws.root,
					Files: []string{filepath.Join(m.ws.root, "us"+raw+"ers.http")},
				}
			},
		},
		{
			name:  "active request",
			apply: func(m *Model) { m.activeRequestTitle = "GET cre" + raw + "ate-user" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := headerTestModel(t, 160)
			tt.apply(model)
			assertHeaderFits(t, model, tt.name)

			for width := 3; width <= 200; width++ {
				model.width = width
				if view := model.renderHeader(); strings.Contains(view, "\x1b[2J") {
					t.Fatalf("%s width %d printed a raw escape: %q", tt.name, width, view)
				}
			}
		})
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

func TestHeaderCompactsWorkspaceLabelBeforeDroppingWorkspace(t *testing.T) {
	model := headerTestModel(t, 160)

	for width := 159; width >= 30; width-- {
		model.width = width
		view := ansi.Strip(model.renderHeader())
		if strings.Contains(view, labelHeaderWorkspace) {
			continue
		}
		if !strings.Contains(view, iconHeaderWorkspace+" acme-api") {
			t.Fatalf("workspace label and value disappeared together at width %d:\n%s", width, view)
		}
		return
	}
	t.Fatal("expected the workspace label to be hidden at a narrow width")
}

func TestHeaderOmitsActiveRequestWhenNoneSelected(t *testing.T) {
	tests := []struct {
		name  string
		items []requestListItem
		count string
	}{
		{name: "no requests", count: "0"},
		{name: "requests without a selection", items: make([]requestListItem, 17), count: "17"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := headerTestModel(t, 160)
			model.currentRequest = nil
			model.activeRequestTitle = ""
			model.requestItems = tt.items
			model.testResults = []scripts.TestResult{{Name: "passed", Passed: true}}

			for _, width := range []int{160, 70} {
				model.width = width
				view := ansi.Strip(model.renderHeader())
				if strings.Contains(view, iconHeaderActive) {
					t.Fatalf("width %d header contains an active-request cell:\n%s", width, view)
				}
				if width == 160 {
					want := iconHeaderRequests + " " + labelHeaderRequests + " " + tt.count +
						headerCellSep + iconTestPass
					if !strings.Contains(view, want) {
						t.Fatalf("header does not join request count to the following cell with %q:\n%s", want, view)
					}
				}
			}
		})
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
	activeRequests := []string{"GET create-user", ""}
	statuses := []headerTransportStatus{
		{},
		{label: "ResourceExhausted", level: statusWarn},
	}

	for _, activeRequest := range activeRequests {
		model.activeRequestTitle = activeRequest
		for _, status := range statuses {
			model.headerTransport = status
			assertHeaderFits(
				t,
				model,
				fmt.Sprintf("active request %q status %q", activeRequest, status.label),
			)
		}
	}
}
