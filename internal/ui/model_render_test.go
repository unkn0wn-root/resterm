package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/theme"
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

func TestNavigatorFilterBandShowsOnlyOnDemand(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir()})
	model.width = 100
	model.height = 32
	model.ready = true
	_ = model.applyLayout()
	model.ensureNavigatorFilter()

	view := ansi.Strip(model.renderFilePane(renderContext{}))
	if strings.Contains(view, "Filter:") {
		t.Fatalf("expected inactive empty filter to be hidden, got %q", view)
	}

	model.navigatorFilter.Focus()
	view = ansi.Strip(model.renderFilePane(renderContext{}))
	if !strings.Contains(view, "Filter:") {
		t.Fatalf("expected focused filter to be visible, got %q", view)
	}

	model.navigatorFilter.Blur()
	model.navigatorFilter.SetValue("auth")
	view = ansi.Strip(model.renderFilePane(renderContext{}))
	if !strings.Contains(view, "Filter:") {
		t.Fatalf("expected active text filter to remain visible, got %q", view)
	}

	model.navigatorFilter.SetValue("")
	model.navigator.ToggleMethodFilter("GET")
	view = ansi.Strip(model.renderFilePane(renderContext{}))
	if !strings.Contains(view, "Filter:") {
		t.Fatalf("expected active method filter to keep filter band visible, got %q", view)
	}
}

func TestCenteredModalUnderlayKeepsAppVisible(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	model := newTestModelWithDoc(sampleRequestDoc)
	model.width = 100
	model.height = 30
	model.ready = true
	model.focus = focusFile
	model.theme.PaneBorderFocusFile = lipgloss.Color("#111111")
	model.theme.PaneDivider = lipgloss.NewStyle().Foreground(lipgloss.Color("#222222"))
	_ = model.applyLayout()
	model.openErrorModal("network down")

	view := model.View()
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "Error") || !strings.Contains(plain, "network down") {
		t.Fatalf("expected error modal content, got %q", plain)
	}
	if !strings.Contains(plain, "RESTERM") {
		t.Fatalf("expected app content to remain visible behind modal, got %q", plain)
	}
	if strings.Contains(view, "\x1b[38;2;17;17;17m") {
		t.Fatalf("expected modal underlay to remove pane focus color")
	}
	if !strings.Contains(view, "\x1b[38;2;34;34;34m") {
		t.Fatalf("expected modal underlay to render inactive pane divider color")
	}
}

func TestCenteredModalUnderlayUsesBackdropColor(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	model := newTestModelWithDoc(sampleRequestDoc)
	model.width = 100
	model.height = 30
	model.ready = true
	model.theme.ModalBackdrop = lipgloss.Color("#0f1720")
	_ = model.applyLayout()
	model.openErrorModal("network down")

	view := model.View()
	if !strings.Contains(view, "\x1b[38;2;15;23;32m") {
		t.Fatalf("expected modal underlay to use modal backdrop color")
	}
}

func TestCenteredModalKeepsThemeSurface(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	model := newTestModelWithDoc(sampleRequestDoc)
	model.width = 100
	model.height = 30
	model.ready = true
	model.theme.ModalInputBackground = lipgloss.Color("#123456")
	_ = model.applyLayout()
	model.openNewFileModal()

	view := model.View()
	line := lineWith(view, "New Request File")
	if line == "" {
		t.Fatalf("expected new file modal title, got %q", ansi.Strip(view))
	}
	if strings.Contains(line, "48;2;18;52;86") {
		t.Fatalf("expected modal title to keep original surface, got %q", line)
	}
}

func TestModalTitlesAreCenteredWithinBorders(t *testing.T) {
	cases := []struct {
		name   string
		title  string
		setup  func(*Model)
		render func(*Model) string
	}{
		{
			name:   "help",
			title:  "Key Bindings",
			render: func(m *Model) string { return m.renderHelpOverlay() },
		},
		{
			name:   "new file",
			title:  "New Request File",
			render: func(m *Model) string { return m.renderNewFileModal() },
		},
		{
			name:   "open",
			title:  "Open File or Workspace",
			render: func(m *Model) string { return m.renderOpenModal() },
		},
		{
			name:   "save response",
			title:  "Save Response Body",
			render: func(m *Model) string { return m.renderResponseSaveModal() },
		},
		{
			name:  "error",
			title: "Error",
			setup: func(m *Model) {
				m.errorModalMessage = "network down"
			},
			render: func(m *Model) string { return m.renderErrorModal() },
		},
		{
			name:   "layout save",
			title:  "Save Layout",
			render: func(m *Model) string { return m.renderLayoutSaveModal() },
		},
		{
			name:  "file change",
			title: "File Change Detected",
			setup: func(m *Model) {
				m.fileChangeTitle = "File Change Detected"
				m.fileChangeMessage = "File changed outside this session."
			},
			render: func(m *Model) string { return m.renderFileChangeModal() },
		},
		{
			name:  "request details",
			title: "Request Details",
			setup: func(m *Model) {
				m.requestDetailTitle = "Request Details"
				m.requestDetailFields = []requestDetailField{
					{label: "Method", value: "GET"},
				}
			},
			render: func(m *Model) string { return m.renderRequestDetailsModal() },
		},
		{
			name:  "history preview",
			title: "History Entry",
			setup: func(m *Model) {
				m.historyPreviewTitle = "History Entry"
				m.historyPreviewContent = "{}"
			},
			render: func(m *Model) string { return m.renderHistoryPreviewModal() },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := newTestModelWithDoc(sampleRequestDoc)
			model.width = 100
			model.height = 32
			model.ready = true
			_ = model.applyLayout()
			if tc.setup != nil {
				tc.setup(model)
			}

			assertTitleCenteredWithinNearestBorders(t, tc.render(model), tc.title)
		})
	}
}

func TestNavigatorRenderStateMarksCurrentFileAndActiveRequest(t *testing.T) {
	content := "### first\n# @name first\nGET https://example.com/one\n\n### second\n# @name second\nPOST https://example.com/two\n"
	model := newTestModelWithDoc(content)
	file := "/tmp/nav-state.http"
	model.currentFile = file
	model.activeRequestKey = requestKey(model.doc.Requests[1])

	state := model.navigatorRenderState()
	if state.ActiveFilePath != file {
		t.Fatalf("expected active file %q, got %q", file, state.ActiveFilePath)
	}
	if want := navigatorRequestID(file, 1); state.ActiveNodeID != want {
		t.Fatalf("expected active request node %q, got %q", want, state.ActiveNodeID)
	}
}

func TestNavigatorRenderStateUsesNavigatorNodeIDForEquivalentRequestPath(t *testing.T) {
	content := "### first\n# @name first\nGET https://example.com/one\n\n### second\n# @name second\nPOST https://example.com/two\n"
	model := newTestModelWithDoc(content)
	file := filepath.Join(t.TempDir(), "nav-state.http")
	nodePath := filepath.Dir(file) + string(filepath.Separator) + "." +
		string(filepath.Separator) + filepath.Base(file)
	model.currentFile = file
	model.activeRequestKey = requestKey(model.doc.Requests[1])

	want := navigatorRequestID(nodePath, 1)
	model.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:       "file:" + nodePath,
			Kind:     navigator.KindFile,
			Expanded: true,
			Payload:  navigator.Payload[any]{FilePath: nodePath},
			Children: []*navigator.Node[any]{
				{
					ID:   navigatorRequestID(nodePath, 0),
					Kind: navigator.KindRequest,
					Payload: navigator.Payload[any]{
						FilePath: nodePath,
						Data:     model.doc.Requests[0],
					},
				},
				{
					ID:   want,
					Kind: navigator.KindRequest,
					Payload: navigator.Payload[any]{
						FilePath: nodePath,
						Data:     model.doc.Requests[1],
					},
				},
			},
		},
	})

	state := model.navigatorRenderState()
	if state.ActiveNodeID != want {
		t.Fatalf("expected navigator node id %q, got %q", want, state.ActiveNodeID)
	}
}

func TestNavigatorRenderStateSuppressesRequestMarkerForSelectedWorkflow(t *testing.T) {
	content := "### req\nGET https://example.com\n\n# @workflow sample\n# @step Fetch using=req\n"
	model := newTestModelWithDoc(content)
	model.currentFile = "/tmp/workflow.http"
	model.activeRequestKey = requestKey(model.doc.Requests[0])
	model.navigator = navigator.New[any]([]*navigator.Node[any]{
		{
			ID:   "wf:" + model.currentFile + ":0",
			Kind: navigator.KindWorkflow,
			Payload: navigator.Payload[any]{
				FilePath: model.currentFile,
				Data:     &model.doc.Workflows[0],
			},
		},
	})

	state := model.navigatorRenderState()
	if state.ActiveNodeID != "" {
		t.Fatalf(
			"expected selected workflow to suppress request marker, got %q",
			state.ActiveNodeID,
		)
	}
}

func TestStatusBarShowsMinimizedIndicators(t *testing.T) {
	model := New(Config{WorkspaceRoot: t.TempDir(), Version: "vTest"})
	model.width = 120
	model.height = 40
	model.ready = true
	model.statusUser = ""
	model.statusHost = ""
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
		t.Fatalf("expected dot indicators for minimized panes, got %q", plain)
	}
	if !strings.Contains(plain, "◇ vTest") {
		t.Fatalf("expected version icon on the right, got %q", plain)
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
	model.statusUser = ""
	model.statusHost = ""

	tests := []struct {
		name  string
		level statusLevel
		text  string
		bg    lipgloss.Color
	}{
		{"info", statusInfo, "Connected", lipgloss.Color("#2563EB")},
		{"warn", statusWarn, "Missing variable", lipgloss.Color("#B45309")},
		{"error", statusError, "Request failed", lipgloss.Color("#B91C1C")},
		{"success", statusSuccess, "Request saved", lipgloss.Color("#15803D")},
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
			palette := statusBarPalette(model.theme.StatusBarPalette)
			if got := statusBarStatusStyle(tt.level, palette).Background; got != tt.bg {
				t.Fatalf("expected %s status background %v, got %v", tt.name, tt.bg, got)
			}
		})
	}
}

func TestStatusBarUsesPlainLeftSections(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	model := New(Config{})
	model.width = 96
	model.currentFile = "/tmp/example.http"
	model.focus = focusEditor
	model.editorInsertMode = true
	model.statusUser = ""
	model.statusHost = ""

	bar := model.renderStatusBar()
	plain := ansi.Strip(bar)
	for _, want := range []string{"Ready", "⇄ example.http", "▣ Editor", "▸ INSERT"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected powerline status section %q in %q", want, plain)
		}
	}
	for _, legacy := range []string{"▤", "◉", "-- INSERT --"} {
		if strings.Contains(plain, legacy) {
			t.Fatalf("expected plain powerline sections without legacy marker %q in %q", legacy, plain)
		}
	}
	if !strings.Contains(bar, "48;2;0;0;0") {
		t.Fatalf("expected black status bar background, got %q", bar)
	}
}

func TestStatusBarUsesThemePalette(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	model := New(Config{})
	model.width = 80
	model.statusUser = ""
	model.statusHost = ""
	model.theme.StatusBarPalette = theme.DefaultStatusBarPalette()
	model.theme.StatusBarPalette.Base = lipgloss.Color("#101010")
	model.theme.StatusBarPalette.Info = theme.StatusBarSegmentStyle{
		Foreground: lipgloss.Color("#abcdef"),
		Background: lipgloss.Color("#123456"),
	}

	bar := model.renderStatusBar()
	if !strings.Contains(bar, "48;2;16;16;16") {
		t.Fatalf("expected custom base background, got %q", bar)
	}
	if !strings.Contains(bar, "38;2;171;205;239") ||
		!strings.Contains(bar, "48;2;18;52;86") {
		t.Fatalf("expected custom info segment colors, got %q", bar)
	}
}

func TestStatusBarContextText(t *testing.T) {
	tests := []struct {
		seg  statusBarSeg
		want string
	}{
		{statusBarSeg{key: "File", val: "example.http"}, "⇄ example.http"},
		{statusBarSeg{key: "Focus", val: "Editor"}, "▣ Editor"},
		{statusBarSeg{key: "Focus", val: "Response"}, "Response"},
		{statusBarSeg{key: "Mode", val: "VIEW"}, "□ VIEW"},
		{statusBarSeg{key: "Mode", val: "INSERT"}, "▸ INSERT"},
		{statusBarSeg{key: "Zoom", val: "Response"}, "Response"},
		{statusBarSeg{key: "Unknown", val: "fallback"}, "Unknown: fallback"},
	}

	for _, tt := range tests {
		if got := statusBarContextText(tt.seg); got != tt.want {
			t.Fatalf("expected %q, got %q", tt.want, got)
		}
	}
}

func TestStatusBarSectionsDoNotRenderExplicitSeparators(t *testing.T) {
	styleA := theme.StatusBarSegmentStyle{
		Foreground: lipgloss.Color("#ffffff"),
		Background: lipgloss.Color("#111111"),
	}
	styleB := theme.StatusBarSegmentStyle{
		Foreground: lipgloss.Color("#ffffff"),
		Background: lipgloss.Color("#222222"),
	}
	segs := []statusBarSection{
		{text: "A", style: styleA},
		{text: "B", style: styleB},
	}

	for name, view := range map[string]string{
		"left":  renderStatusBarSections(segs),
		"right": renderStatusBarSections(segs),
	} {
		plain := ansi.Strip(view)
		if plain != " A  B " {
			t.Fatalf("unexpected %s section layout %q", name, plain)
		}
		if strings.ContainsAny(plain, "│") {
			t.Fatalf("expected no explicit %s separators in %q", name, plain)
		}
		if got := lipgloss.Width(plain); got != statusBarSectionsWidth(segs) {
			t.Fatalf(
				"expected %s width %d, got %d",
				name,
				statusBarSectionsWidth(segs),
				got,
			)
		}
	}
}

func TestStatusBarRightShowsIdentity(t *testing.T) {
	model := New(Config{Version: "vTest"})
	model.width = 120
	model.statusUser = "david"
	model.statusHost = "workstation"

	bar := model.renderStatusBar()
	plain := ansi.Strip(bar)
	for _, want := range []string{"◇ vTest", "david", "workstation"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected right status section %q in %q", want, plain)
		}
	}
	if strings.ContainsAny(plain, "│") {
		t.Fatalf("expected no explicit separators in %q", plain)
	}
	if trimmed := strings.TrimSpace(plain); !strings.HasSuffix(trimmed, "workstation") {
		t.Fatalf("expected host to be the rightmost section, got %q", trimmed)
	}
}

func TestStatusBarKeepsMessageWhenIdentityIsLong(t *testing.T) {
	model := New(Config{Version: "vTest"})
	model.width = 36
	model.statusUser = "very-long-user-name"
	model.statusHost = "very-long-workstation-name"
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
	if !strings.Contains(plain, "Request") {
		t.Fatalf("expected status message to remain visible, got %q", plain)
	}
	if !strings.Contains(plain, "vTest") {
		t.Fatalf("expected version to remain visible, got %q", plain)
	}
	if strings.Contains(plain, "very-long-user-name") ||
		strings.Contains(plain, "very-long-workstation-name") {
		t.Fatalf("expected long identity to yield space first, got %q", plain)
	}
}

func TestStatusBarStyledMessageFitsNarrowWidth(t *testing.T) {
	model := New(Config{Version: "vTest"})
	model.width = 36
	model.statusUser = ""
	model.statusHost = ""
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

func TestCleanStatusUsername(t *testing.T) {
	tests := map[string]string{
		`DOMAIN\david`:  "david",
		`DOMAIN\ david`: "david",
		"domain/david":  `david`,
		" david ":       "david",
		"":              "",
	}

	for input, want := range tests {
		if got := cleanStatusUsername(input); got != want {
			t.Fatalf("cleanStatusUsername(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCleanStatusHost(t *testing.T) {
	tests := map[string]string{
		"workstation.local": "workstation",
		"host":              "host",
		" host ":            "host",
		"":                  "",
	}

	for input, want := range tests {
		if got := cleanStatusHost(input); got != want {
			t.Fatalf("cleanStatusHost(%q) = %q, want %q", input, got, want)
		}
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

func TestResponsePaneShowsExplainPreviewSpinner(t *testing.T) {
	if len(tabSpinFrames) < 2 {
		t.Fatalf("expected tab spinner frames")
	}
	snap := &responseSnapshot{pretty: withTrailingNewline("ok"), ready: true}
	model := newModelWithResponseTab(responseTabExplain, snap)
	model.sending = true
	model.sendingOverlayBase = responseExplainPreviewBase
	model.tabSpinIdx = 1
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 40
	pane.viewport.Height = 10

	view := model.renderResponseColumn(responsePanePrimary, true, 40)
	plain := ansi.Strip(view)
	if strings.Contains(plain, responseSendingBase) {
		t.Fatalf("did not expect send spinner message during explain preview, got %q", plain)
	}
	if !strings.Contains(plain, responseExplainPreviewBase) {
		t.Fatalf("expected explain preview spinner message, got %q", plain)
	}
	if !strings.Contains(plain, tabSpinFrames[1]) {
		t.Fatalf("expected spinner frame, got %q", plain)
	}
}

func TestResponseSearchPromptRendersAtColumnBottom(t *testing.T) {
	snap := &responseSnapshot{pretty: withTrailingNewline("short-body"), ready: true}
	model := newModelWithResponseTab(responseTabPretty, snap)
	model.showSearchPrompt = true
	model.searchTarget = searchTargetResponse
	model.searchResponsePane = responsePanePrimary
	model.searchInput.SetValue("needle")
	model.searchInput.Focus()

	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 36
	pane.viewport.Height = 5
	pane.viewport.SetContent("short-body")

	view := model.renderResponseColumn(responsePanePrimary, true, 36)
	lines := strings.Split(ansi.Strip(view), "\n")
	bodyLine := lineIndexContaining(lines, "short-body")
	searchLine := lineIndexContaining(lines, searchPromptIcon+" Search")
	if bodyLine < 0 {
		t.Fatalf("expected response body in column, got %q", ansi.Strip(view))
	}
	if searchLine < 0 {
		t.Fatalf("expected response search prompt in column, got %q", ansi.Strip(view))
	}
	if searchLine <= bodyLine {
		t.Fatalf(
			"expected search prompt below response body, got body line %d search line %d in %q",
			bodyLine,
			searchLine,
			ansi.Strip(view),
		)
	}
	if last := lastNonBlankLineIndex(lines); last != searchLine {
		t.Fatalf(
			"expected search prompt on bottom content line, got last line %d search line %d in %q",
			last,
			searchLine,
			ansi.Strip(view),
		)
	}
}

func TestResponseSearchPromptOnlyRendersInTargetSplitPane(t *testing.T) {
	model := newModelWithResponseTab(responseTabPretty, &responseSnapshot{ready: true})
	model.responseSplit = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	model.showSearchPrompt = true
	model.searchTarget = searchTargetResponse
	model.searchResponsePane = responsePaneSecondary
	model.searchInput.SetValue("needle")
	model.searchInput.Focus()

	primary := model.pane(responsePanePrimary)
	primary.viewport.Width = 32
	primary.viewport.Height = 5
	primary.viewport.SetContent("primary-body")
	secondary := model.pane(responsePaneSecondary)
	secondary.activeTab = responseTabPretty
	secondary.viewport.Width = 32
	secondary.viewport.Height = 5
	secondary.viewport.SetContent("secondary-body")

	primaryView := model.renderResponseColumn(responsePanePrimary, true, 32)
	if strings.Contains(ansi.Strip(primaryView), searchPromptIcon+" Search") {
		t.Fatalf("did not expect search prompt in primary pane, got %q", ansi.Strip(primaryView))
	}

	secondaryView := model.renderResponseColumn(responsePaneSecondary, false, 32)
	secondaryPlain := ansi.Strip(secondaryView)
	if !strings.Contains(secondaryPlain, searchPromptIcon+" Search") {
		t.Fatalf("expected search prompt in secondary pane, got %q", secondaryPlain)
	}
	secondaryLines := strings.Split(secondaryPlain, "\n")
	bodyLine := lineIndexContaining(secondaryLines, "secondary-body")
	searchLine := lineIndexContaining(secondaryLines, searchPromptIcon+" Search")
	if bodyLine < 0 || searchLine < 0 {
		t.Fatalf("expected secondary body and search prompt, got %q", secondaryPlain)
	}
	if searchLine <= bodyLine {
		t.Fatalf(
			"expected secondary search prompt below body, got body line %d search line %d in %q",
			bodyLine,
			searchLine,
			secondaryPlain,
		)
	}
}

func TestResponseColumnUsesFullViewportHeight(t *testing.T) {
	content := "line-1\nline-2\nline-3\nline-4\nline-5"
	snap := &responseSnapshot{pretty: withTrailingNewline(content), ready: true}
	model := newModelWithResponseTab(responseTabPretty, snap)

	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 36
	pane.viewport.Height = 5
	if cmd := model.syncResponsePane(responsePanePrimary); cmd != nil {
		cmd()
	}

	view := model.renderResponseColumn(responsePanePrimary, true, 36)
	lines := strings.Split(ansi.Strip(view), "\n")
	if got, want := len(lines), pane.viewport.Height+lipgloss.Height(model.renderPaneTabs(responsePanePrimary, true, 36)); got != want {
		t.Fatalf("expected response column height %d, got %d in %q", want, got, ansi.Strip(view))
	}
	if !strings.Contains(lines[len(lines)-1], "line-5") {
		t.Fatalf("expected last viewport row to show last content line, got %q in %q", lines[len(lines)-1], ansi.Strip(view))
	}
}

func TestInactiveEditorPaneKeepsCursorRuneStyle(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	model := New(Config{})
	model.ready = true
	model.focus = focusResponse
	model.editor.SetValue("GET https://google.no")
	model.editor.SetWidth(32)
	model.editor.SetHeight(2)
	model.editorContentHeight = 3
	model.editor.ShowLineNumbers = false
	model.editor.SetCursor(0)

	baseColor := lipgloss.Color("#ffffff")
	reqColor := lipgloss.Color("#ff0000")
	model.editor.BlurredStyle.Text = lipgloss.NewStyle().Foreground(baseColor)
	model.editor.BlurredStyle.CursorLine = lipgloss.NewStyle().Foreground(baseColor)
	model.editor.Blur()
	model.editor.SetRuneStyler(newMetadataRuneStyler(theme.EditorMetadataPalette{
		RequestLine: reqColor,
	}))

	view := model.renderEditorPane(renderContext{})
	line := lineWith(view, "GET")
	if line == "" {
		t.Fatalf("expected rendered editor to include request line, got %q", ansi.Strip(view))
	}
	if hasFaintSGR(line) {
		t.Fatalf("expected inactive editor body to avoid pane-level faint styling, got %q", line)
	}
	if strings.Contains(line, lipgloss.NewStyle().Foreground(baseColor).Render("G")) {
		t.Fatalf("expected cursor rune to keep request-line style, got %q", line)
	}
}

func TestInactiveSidebarFrameKeepsStyleOnFrame(t *testing.T) {
	model := New(Config{})
	model.theme.BrowserBorder = model.theme.BrowserBorder.
		Foreground(lipgloss.Color("#ffffff")).
		Bold(true).
		Faint(true)

	st := model.sidebarFrameStyle(false)
	if st.GetBold() || st.GetFaint() {
		t.Fatalf("expected inactive sidebar frame to strip text attrs")
	}
	if _, ok := st.GetForeground().(lipgloss.NoColor); !ok {
		t.Fatalf(
			"expected inactive sidebar frame to avoid text foreground, got %v",
			st.GetForeground(),
		)
	}
	if st.GetBorderLeftForeground() != model.theme.PaneDivider.GetForeground() {
		t.Fatalf("expected inactive sidebar border to use pane divider color")
	}
}

func TestFocusedPaneFramesUseThemeFocusColors(t *testing.T) {
	model := New(Config{})
	model.theme.PaneBorderFocusFile = lipgloss.Color("#111111")
	model.theme.PaneBorderFocusRequests = lipgloss.Color("#222222")
	model.theme.PaneBorderFocusEditor = lipgloss.Color("#333333")
	model.theme.PaneBorderFocusResponse = lipgloss.Color("#444444")

	model.focus = focusFile
	if got := model.sidebarFrameStyle(true).
		GetBorderLeftForeground(); got != lipgloss.Color(
		"#111111",
	) {
		t.Fatalf("expected file focus border color, got %v", got)
	}

	model.focus = focusRequests
	if got := model.sidebarFrameStyle(true).
		GetBorderLeftForeground(); got != lipgloss.Color(
		"#222222",
	) {
		t.Fatalf("expected requests focus border color, got %v", got)
	}

	if got := model.editorFrameStyle(true).
		GetBorderLeftForeground(); got != lipgloss.Color(
		"#333333",
	) {
		t.Fatalf("expected editor focus border color, got %v", got)
	}
	if got := model.respFrameStyle(true).
		GetBorderLeftForeground(); got != lipgloss.Color(
		"#444444",
	) {
		t.Fatalf("expected response focus border color, got %v", got)
	}
}

func TestPaneTitleStyleUsesFocusedFrameColor(t *testing.T) {
	model := New(Config{})
	model.theme.PaneTitle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#111111"))
	model.theme.PaneTitleEditor = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#222222"))
	frame := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#333333"))

	inactive := model.paneTitleStyle(model.theme.PaneTitleEditor, frame, false)
	if got := inactive.GetForeground(); got != lipgloss.Color("#222222") {
		t.Fatalf("expected inactive title to keep pane title color, got %v", got)
	}
	if theme.ColorDefined(inactive.GetBackground()) {
		t.Fatalf(
			"expected inactive title background to stay unset, got %v",
			inactive.GetBackground(),
		)
	}

	active := model.paneTitleStyle(model.theme.PaneTitleEditor, frame, true)
	if got := active.GetForeground(); got != lipgloss.Color("#333333") {
		t.Fatalf("expected active title foreground from focused frame, got %v", got)
	}
	if theme.ColorDefined(active.GetBackground()) {
		t.Fatalf("expected active title background to stay unset, got %v", active.GetBackground())
	}
	if !active.GetBold() {
		t.Fatalf("expected active title to be bold")
	}
}

func TestTitledPaneFrameRendersTitleOnTopBorder(t *testing.T) {
	frame := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#abcdef")).
		Width(18).
		Height(3)

	tests := []struct {
		title string
		want  string
	}{
		{title: filePaneTitle, want: "─ Files ─╮"},
		{title: editorPaneTitle, want: "─ Editor ─╮"},
		{title: responsePaneTitle, want: "─ Response ─╮"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			rendered := renderTitledPaneFrame(frame, lipgloss.NewStyle(), tt.title, "body")
			lines := strings.Split(ansi.Strip(rendered), "\n")
			if len(lines) < 1 {
				t.Fatalf("expected rendered frame to have a top border")
			}
			if !strings.HasPrefix(lines[0], "╭─") {
				t.Fatalf("expected titled rounded top border to keep left corner, got %q", lines[0])
			}
			if !strings.Contains(lines[0], tt.want) {
				t.Fatalf("expected title %q on top border, got %q", tt.want, lines[0])
			}
			if !strings.HasSuffix(lines[0], "╮") {
				t.Fatalf("expected top border to keep right corner, got %q", lines[0])
			}
			if got, want := ansi.StringWidth(
				lines[0],
			), lipgloss.Width(
				frame.Render("body"),
			); got != want {
				t.Fatalf("expected titled top border width %d, got %d", want, got)
			}
		})
	}
}

func TestInactiveResponsePaneKeepsPrettyContentReadable(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	snap := &responseSnapshot{
		pretty: withTrailingNewline("{\n  \"ok\": true\n}"),
		ready:  true,
	}
	model := newModelWithResponseTab(responseTabPretty, snap)
	model.focus = focusEditor
	model.responseContentHeight = 8
	pane := model.pane(responsePanePrimary)
	pane.viewport.Width = 32
	pane.viewport.Height = 6
	pane.viewport.SetContent(snap.pretty)

	view := model.renderResponsePane(40, renderContext{})
	line := lineWith(view, "{")
	if line == "" {
		t.Fatalf("expected rendered response to include opening brace, got %q", ansi.Strip(view))
	}
	if hasFaintSGR(line) {
		t.Fatalf("expected inactive pane to leave Pretty content unfainted, got %q", line)
	}
}

func TestUnfocusedSplitResponseColumnKeepsPrettyContentReadable(t *testing.T) {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	model := newModelWithResponseTab(responseTabPretty, &responseSnapshot{ready: true})
	model.responseSplit = true
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePaneSecondary)
	pane.activeTab = responseTabPretty
	pane.viewport.Width = 32
	pane.viewport.Height = 6
	pane.viewport.SetContent("{\n  \"ok\": true\n}")

	view := model.renderResponseColumn(responsePaneSecondary, false, 32)
	line := lineWith(view, "{")
	if line == "" {
		t.Fatalf("expected rendered response to include opening brace, got %q", ansi.Strip(view))
	}
	if hasFaintSGR(line) {
		t.Fatalf("expected unfocused split column to leave Pretty content unfainted, got %q", line)
	}
}

func lineWith(view, needle string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(ansi.Strip(line), needle) {
			return line
		}
	}
	return ""
}

func lineIndexContaining(lines []string, needle string) int {
	for idx, line := range lines {
		if strings.Contains(line, needle) {
			return idx
		}
	}
	return -1
}

func lastNonBlankLineIndex(lines []string) int {
	for idx := len(lines) - 1; idx >= 0; idx-- {
		if strings.TrimSpace(lines[idx]) != "" {
			return idx
		}
	}
	return -1
}

func assertTitleCenteredWithinNearestBorders(t *testing.T, view, title string) {
	t.Helper()

	plain := ansi.Strip(view)
	line := lineWith(plain, title)
	if line == "" {
		t.Fatalf("expected modal title %q in view, got %q", title, plain)
	}

	titleStart := strings.Index(line, title)
	if titleStart < 0 {
		t.Fatalf("expected modal title %q in line %q", title, line)
	}
	titleEnd := titleStart + len(title)
	leftBorder := strings.LastIndex(line[:titleStart], "│")
	rightOffset := strings.Index(line[titleEnd:], "│")
	if leftBorder < 0 || rightOffset < 0 {
		t.Fatalf("expected line with %q to have modal borders, got %q", title, line)
	}
	rightBorder := titleEnd + rightOffset

	leftPad := lipgloss.Width(line[leftBorder+len("│") : titleStart])
	rightPad := lipgloss.Width(line[titleEnd:rightBorder])
	if leftPad > rightPad+1 || rightPad > leftPad+1 {
		t.Fatalf(
			"expected %q centered between modal borders, got left=%d right=%d line=%q",
			title,
			leftPad,
			rightPad,
			line,
		)
	}
}

func hasFaintSGR(s string) bool {
	return strings.Contains(s, "\x1b[2m") || strings.Contains(s, "\x1b[2;")
}
