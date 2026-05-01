package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/ui/navigator"
)

func TestNavigatorTagChipsFilterMatchesQueryTokens(t *testing.T) {
	model := New(Config{})
	m := &model
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:    "file:/tmp/a",
			Title: "Requests file",
			Kind:  navigator.KindFile,
			Tags:  []string{"workspace"},
			Children: []*navigator.Node[any]{
				{
					ID:    "req:/tmp/a:0",
					Kind:  navigator.KindRequest,
					Title: "alpha req",
					Tags:  []string{"auth", "reqscope"},
				},
			},
		},
		{
			ID:    "file:/tmp/b",
			Title: "Req beta",
			Kind:  navigator.KindFile,
			Tags:  []string{"files"},
			Children: []*navigator.Node[any]{
				{
					ID:    "req:/tmp/b:0",
					Kind:  navigator.KindRequest,
					Title: "beta",
					Tags:  []string{"other", "reqbeta"},
				},
			},
		},
	})
	m.ensureNavigatorFilter()
	m.navigatorFilter.Focus()
	m.navigatorFilter.SetValue("req")
	m.navigator.SetFilter(m.navigatorFilter.Value())

	out := m.navigatorTagChips()
	if out == "" {
		t.Fatalf("expected tag chips to render")
	}
	clean := ansi.Strip(out)
	if strings.Contains(clean, "#workspace") || strings.Contains(clean, "#files") {
		t.Fatalf("expected unrelated tags to be filtered out, got %q", clean)
	}
	if !strings.Contains(clean, "#reqscope") || !strings.Contains(clean, "#reqbeta") {
		t.Fatalf("expected matching tags to remain, got %q", clean)
	}

	// When no prefix hits, we fall back to substring matching.
	m.navigatorFilter.SetValue("scope")
	out = m.navigatorTagChips()
	clean = ansi.Strip(out)
	if !strings.Contains(clean, "#reqscope") {
		t.Fatalf("expected substring fallback to keep reqscope, got %q", clean)
	}
}

func TestNavigatorTagChipsLimit(t *testing.T) {
	model := New(Config{})
	m := &model
	var tags []string
	for i := 0; i < 15; i++ {
		tags = append(tags, fmt.Sprintf("tag%d", i))
	}
	m.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:    "file:/tmp/a",
			Title: "Requests file",
			Kind:  navigator.KindFile,
			Tags:  tags,
		},
	})
	m.ensureNavigatorFilter()
	m.navigatorFilter.Focus()

	out := m.navigatorTagChips()
	if out == "" {
		t.Fatalf("expected tag chips to render")
	}
	clean := ansi.Strip(out)
	parts := strings.Fields(clean)
	tagCount := 0
	for _, p := range parts {
		if strings.HasPrefix(p, "#") {
			tagCount++
		}
	}
	if tagCount != 10 {
		t.Fatalf("expected 10 tags rendered, got %d (%q)", tagCount, clean)
	}
	if !strings.Contains(clean, "...") {
		t.Fatalf("expected ellipsis when tags exceed limit, got %q", clean)
	}
}

func TestStatusBarShowsMinimizedIndicators(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir(), Version: "vTest"})
	model.width = 120
	model.height = 40
	model.ready = true
	_ = model.applyLayout()

	if res := model.setCollapseState(paneRegionSidebar, true); res.blocked {
		t.Fatalf("expected sidebar collapse to be allowed")
	}
	if res := model.setCollapseState(paneRegionEditor, true); res.blocked {
		t.Fatalf("expected editor collapse to be allowed")
	}

	bar := model.renderStatusBar()
	plain := ansi.Strip(bar)
	if strings.Contains(plain, "Editor:min") || strings.Contains(plain, "Response:min") {
		t.Fatalf("expected minimized indicators to replace legacy labels, got %q", plain)
	}
	if !strings.Contains(plain, "● Editor") || !strings.Contains(plain, "● Nav") {
		t.Fatalf("expected green dot indicators for minimized panes, got %q", plain)
	}
	trimmed := strings.TrimSpace(plain)
	if !strings.HasSuffix(trimmed, "vTest") {
		t.Fatalf("expected version to remain on the right, got %q", trimmed)
	}
	if strings.Contains(plain, "\n") {
		t.Fatalf("expected status bar to stay on one line, got %q", plain)
	}
}

func TestStatusBarMessageLevelsRenderStyled(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	model := New(Config{})
	model.width = 96
	model.theme.StatusBar = lipgloss.NewStyle()
	// source theme styles may be bold
	// statusbar messages should keep only
	// their foreground color and render at regular weight
	model.theme.StatusBarInfo = lipgloss.NewStyle().Bold(true)
	model.theme.StatusBarKey = lipgloss.NewStyle().Bold(true)
	model.theme.StatusBarValue = lipgloss.NewStyle()
	model.theme.Error = lipgloss.NewStyle().Bold(true)
	model.theme.Success = lipgloss.NewStyle().Bold(true)

	tests := []struct {
		name  string
		level statusLevel
		text  string
		color lipgloss.Color
	}{
		{"info", statusInfo, "Connected", lipgloss.Color(statusInfoDarkColor)},
		{"warn", statusWarn, "Missing variable", lipgloss.Color(statusWarnDarkColor)},
		{"error", statusError, "Request failed", lipgloss.Color(statusErrorDarkColor)},
		{"success", statusSuccess, "Request saved", lipgloss.Color(statusSuccessDarkColor)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model.statusMessage = statusMsg{text: tt.text, level: tt.level}

			bar := model.renderStatusBar()
			plain := ansi.Strip(bar)
			if !strings.Contains(plain, tt.text) {
				t.Fatalf("expected status text in bar, got %q", plain)
			}
			if strings.Contains(plain, "\n") {
				t.Fatalf("expected one-line status bar, got %q", plain)
			}
			if !strings.Contains(bar, "\x1b[") {
				t.Fatalf("expected rendered status bar to include ANSI styling, got %q", bar)
			}
			if got := model.statusBarMessageStyle(tt.level).GetForeground(); got != tt.color {
				t.Fatalf("expected %s color %v, got %v", tt.name, tt.color, got)
			}
			if model.statusBarMessageStyle(tt.level).GetBold() {
				t.Fatalf("expected %s status message to render at regular weight", tt.name)
			}
		})
	}
}

func TestStatusBarInfoUsesThemeForeground(t *testing.T) {
	model := New(Config{})
	model.theme.StatusBarInfo = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#0ea5e9")).
		Bold(true)

	style := model.statusBarMessageStyle(statusInfo)
	if got := style.GetForeground(); got != lipgloss.Color("#0ea5e9") {
		t.Fatalf("expected status info foreground override, got %v", got)
	}
	if style.GetBold() {
		t.Fatal("expected status info message to render at regular weight")
	}
}

func TestRenderStatusBarLeftUsesExplicitParts(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	model := New(Config{})
	model.theme.StatusBarInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("#0ea5e9"))
	model.theme.StatusBarKey = lipgloss.NewStyle().Foreground(lipgloss.Color("#f59e0b"))
	model.theme.StatusBarValue = lipgloss.NewStyle().Foreground(lipgloss.Color("#cbd5e1"))

	segs := []statusBarSeg{{key: "Focus", val: "Editor"}}
	full := model.renderStatusBarLeft(statusBarLeft{
		msg:   "Ready",
		level: statusInfo,
		ctx:   statusBarSegmentsText(segs),
		segs:  segs,
	})
	if plain := ansi.Strip(full); plain != "Ready"+statusBarSep+"Focus: Editor" {
		t.Fatalf("unexpected full status text %q", plain)
	}
	if want := model.theme.StatusBarKey.Render("Focus:"); !strings.Contains(full, want) {
		t.Fatalf("expected full context key style %q in %q", want, full)
	}

	truncated := model.renderStatusBarLeft(statusBarLeft{
		msg:          "Ready",
		level:        statusInfo,
		ctx:          "Focus: Ed...",
		ctxTruncated: true,
	})
	if want := model.theme.StatusBarValue.Render("Focus: Ed..."); !strings.Contains(truncated, want) {
		t.Fatalf("expected truncated context value style %q in %q", want, truncated)
	}
}

func TestStatusBarLabelsCurrentFileSegment(t *testing.T) {
	model := New(Config{})
	model.currentFile = "/tmp/example.http"

	got := statusBarSegmentsText(model.statusBarSegments())
	if !strings.Contains(got, "File: example.http") {
		t.Fatalf("expected file segment to be labeled, got %q", got)
	}
}

func TestStatusBarOmitsFileSegmentWithoutCurrentFile(t *testing.T) {
	model := New(Config{})

	got := statusBarSegmentsText(model.statusBarSegments())
	if strings.Contains(got, "File:") {
		t.Fatalf("expected no file segment without current file, got %q", got)
	}
	if !strings.Contains(got, "Focus:") {
		t.Fatalf("expected focus segment to remain labeled, got %q", got)
	}
}

func TestStatusBarStyledMessageFitsNarrowWidth(t *testing.T) {
	model := New(Config{Version: "vTest"})
	model.width = 36
	model.statusMessage = statusMsg{
		text:  "Request failed because the upstream service returned a very long error",
		level: statusError,
	}

	bar := model.renderStatusBar()
	plain := ansi.Strip(bar)
	if strings.Contains(plain, "\n") {
		t.Fatalf("expected one-line status bar, got %q", plain)
	}
	if got := lipgloss.Width(plain); got > model.width {
		t.Fatalf("expected status bar width <= %d, got %d (%q)", model.width, got, plain)
	}
	if !strings.Contains(plain, "vTest") {
		t.Fatalf("expected version to remain visible, got %q", plain)
	}
}

func TestTabBadgeTextOmitsSpinner(t *testing.T) {
	m := &Model{}
	m.sending = true
	m.statusPulseFrame = 1
	got := m.tabBadgeText("Live")
	want := "LIVE"
	if got != want {
		t.Fatalf("expected badge %q, got %q", want, got)
	}
}

func TestTabBadgeShortOmitsSpinner(t *testing.T) {
	m := &Model{}
	m.sending = true
	m.statusPulseFrame = 0
	got := m.tabBadgeShort("Pinned")
	want := "P"
	if got != want {
		t.Fatalf("expected short badge %q, got %q", want, got)
	}
}

func TestRenderTabSegmentConsumesIndicatorPadding(t *testing.T) {
	st := lipgloss.NewStyle().Padding(0, 2)

	if got := stripANSIEscape(renderTabSegment(st, "Raw", false)); got != "  Raw  " {
		t.Fatalf("expected unmarked tab spacing, got %q", got)
	}
	if got := stripANSIEscape(renderTabSegment(st, "Raw", true)); got != " ▹ Raw  " {
		t.Fatalf("expected marked tab spacing, got %q", got)
	}
}

func TestRenderTabSegmentHandlesCompactPadding(t *testing.T) {
	compact := lipgloss.NewStyle().Padding(0, 1)
	if got := stripANSIEscape(renderTabSegment(compact, "Raw", true)); got != "▹ Raw " {
		t.Fatalf("expected compact marked tab spacing, got %q", got)
	}

	tight := lipgloss.NewStyle().Padding(0)
	if got := stripANSIEscape(renderTabSegment(tight, "Raw", true)); got != "▹ Raw" {
		t.Fatalf("expected tight marked tab spacing, got %q", got)
	}
}

func TestAdaptiveTabRowUsesSingleIndicator(t *testing.T) {
	m := Model{}
	states := []tabLabelState{
		{
			runes:     []rune("Raw"),
			isActive:  true,
			length:    3,
			maxLength: 3,
		},
	}
	plan := tabRowPlan{
		activeStyle:   lipgloss.NewStyle().Padding(0, 1),
		inactiveStyle: lipgloss.NewStyle(),
		badgeStyle:    lipgloss.NewStyle(),
	}

	row, width := m.renderTabRowFromStates(states, plan, true)
	if got := stripANSIEscape(row); got != "▹ Raw " {
		t.Fatalf("expected adaptive marked tab spacing, got %q", got)
	}
	if width != lipgloss.Width("▹ Raw ") {
		t.Fatalf("expected adaptive width to match rendered text, got %d", width)
	}
}

func TestResponsePaneShowsSendingSpinner(t *testing.T) {
	if len(tabSpinFrames) < 2 {
		t.Fatalf("expected tab spinner frames")
	}
	snap := &responseSnapshot{pretty: withTrailingNewline("ok"), ready: true}
	model := newModelWithResponseTab(responseTabPretty, snap)
	model.sending = true
	model.tabSpinIdx = 1
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 40
	pane.viewport.Height = 10

	view := model.renderResponseColumn(responsePanePrimary, true, 40)
	plain := ansi.Strip(view)
	if !strings.Contains(plain, responseSendingBase) {
		t.Fatalf("expected sending message, got %q", plain)
	}
	if !strings.Contains(plain, tabSpinFrames[1]) {
		t.Fatalf("expected spinner frame, got %q", plain)
	}
}
