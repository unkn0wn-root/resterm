package navigator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/filesvc"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

const selectedTestBackgroundSGR = "48;2;36;27;51"

func selectedRenderTheme(t *testing.T) theme.Theme {
	t.Helper()
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prevProfile)
	})

	th := theme.DefaultTheme()
	th.NavigatorTitleSelected = th.NavigatorTitleSelected.Background(lipgloss.Color("#241B33"))
	th.NavigatorSubtitleSelected = th.NavigatorSubtitleSelected.Background(lipgloss.Color("#241B33"))
	return th
}

func assertSelectedBackgroundSegments(t *testing.T, rendered string, min int) {
	t.Helper()
	if got := strings.Count(rendered, selectedTestBackgroundSGR); got < min {
		t.Fatalf("expected at least %d selected background segments, got %d in %q", min, got, rendered)
	}
}

func assertSelectedRow(t *testing.T, rendered string, width int) {
	t.Helper()
	if got := lipgloss.Width(rendered); got != width {
		t.Fatalf("expected selected row width %d, got %d in %q", width, got, rendered)
	}
	assertSelectedBackgroundSegments(t, rendered, 3)
}

func TestRowActiveMatchesRelativeAndAbsoluteFilePath(t *testing.T) {
	abs, err := filepath.Abs("api.http")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	row := Flat[any]{
		Node: &Node[any]{
			Kind:    KindFile,
			Payload: Payload[any]{FilePath: "api.http"},
		},
	}

	if !rowActive(row, RenderState{ActiveFilePath: abs}) {
		t.Fatalf("expected relative path to match absolute path %q", abs)
	}
}

func TestRenderBadgesUsesCommaSeparator(t *testing.T) {
	th := theme.DefaultTheme()
	out := renderBadges([]string{"  SSE  ", "SCRIPT", "WS"}, th, false)
	clean := ansi.Strip(out)

	if strings.Count(clean, ",") != 2 {
		t.Fatalf("expected comma separators between badges, got %q", clean)
	}
	if strings.Contains(clean, "  ,") || strings.Contains(clean, ",  ") {
		t.Fatalf("expected comma separators without extra spacing, got %q", clean)
	}
	if strings.HasSuffix(clean, ",") {
		t.Fatalf("expected no trailing comma, got %q", clean)
	}
	if !strings.Contains(clean, "SSE") || !strings.Contains(clean, "SCRIPT") ||
		!strings.Contains(clean, "WS") {
		t.Fatalf("expected all badge labels to render, got %q", clean)
	}
}

func TestRenderWorkflowShowsBadgeNoCaret(t *testing.T) {
	th := theme.DefaultTheme()
	node := Flat[any]{
		Node: &Node[any]{
			Kind:   KindWorkflow,
			Title:  "sample-order",
			Badges: []string{"4 steps"},
			Tags:   []string{"demo", "workflow"},
		},
	}
	out := renderRow(node, false, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if strings.Contains(clean, iconCaretClosed) || strings.Contains(clean, iconCaretOpen) {
		t.Fatalf("expected workflow row without caret, got %q", clean)
	}
	if !strings.Contains(clean, "WF") {
		t.Fatalf("expected workflow badge, got %q", clean)
	}
	if !strings.Contains(clean, "WF  sample-order") {
		t.Fatalf("expected padded workflow badge before title, got %q", clean)
	}
}

func TestRenderRowShowsBadgesButOmitsTags(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:   KindRequest,
			Title:  "Fetch user",
			Method: "GET",
			Target: "https://example.com/users/1",
			Tags:   []string{"beta", "users"},
			Badges: []string{"AUTH", "gRPC"},
		},
	}
	out := renderRow(row, false, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if strings.Contains(clean, "#beta") || strings.Contains(clean, "#users") {
		t.Fatalf("expected tags to be omitted from list row, got %q", clean)
	}
	if !strings.Contains(clean, "AUTH") || !strings.Contains(clean, "gRPC") {
		t.Fatalf("expected badges to render in list row, got %q", clean)
	}
	if !strings.Contains(clean, "Fetch user") ||
		!strings.Contains(clean, "https://example.com/users/1") {
		t.Fatalf("expected request summary to remain in list row, got %q", clean)
	}
}

func TestRenderSelectedRequestShowsMarker(t *testing.T) {
	th := selectedRenderTheme(t)
	row := Flat[any]{
		Node: &Node[any]{
			Kind:   KindRequest,
			Title:  "getStatus",
			Method: "GET",
		},
	}
	out := renderRow(row, true, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.HasPrefix(clean, "> GET  getStatus") {
		t.Fatalf("expected selected request marker before method badge, got %q", clean)
	}
	assertSelectedRow(t, out, 80)
}

func TestRenderRowShowsDirIcon(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:     KindDir,
			Title:    "rts",
			Expanded: false,
		},
	}
	out := renderRow(row, false, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.Contains(clean, iconDirClosed) {
		t.Fatalf("expected directory icon, got %q", clean)
	}
	if strings.Contains(clean, iconCaretClosed) || strings.Contains(clean, iconCaretOpen) {
		t.Fatalf("expected directory row without caret, got %q", clean)
	}
}

func TestRenderSelectedFileShowsMarkerAndCaret(t *testing.T) {
	th := selectedRenderTheme(t)
	row := Flat[any]{
		Node: &Node[any]{
			Kind:     KindFile,
			Title:    "api.http",
			Expanded: false,
			Count:    2,
		},
	}
	out := renderRow(row, true, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.HasPrefix(clean, iconSelected+" "+iconCaretClosed+" api.http (2)") {
		t.Fatalf("expected selected file marker, caret, and title, got %q", clean)
	}
	assertSelectedRow(t, out, 80)
}

func TestRenderSelectedDirShowsMarkerAndIcon(t *testing.T) {
	th := selectedRenderTheme(t)
	row := Flat[any]{
		Node: &Node[any]{
			Kind:  KindDir,
			Title: "queries",
		},
	}
	out := renderRow(row, true, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.HasPrefix(clean, iconSelected+" "+iconDirClosed+" queries") {
		t.Fatalf("expected selected directory marker, icon, and title, got %q", clean)
	}
	assertSelectedRow(t, out, 80)
}

func TestRenderSelectedWorkflowShowsMarker(t *testing.T) {
	th := selectedRenderTheme(t)
	row := Flat[any]{
		Node: &Node[any]{
			Kind:  KindWorkflow,
			Title: "sample-order",
		},
	}
	out := renderRow(row, true, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.HasPrefix(clean, "> WF  sample-order") {
		t.Fatalf("expected selected workflow marker before badge, got %q", clean)
	}
	assertSelectedRow(t, out, 80)
}

func TestRenderActiveFileShowsSubtleMarker(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:     KindFile,
			Title:    "api.http",
			Expanded: false,
			Count:    2,
		},
	}
	out := renderRowState(row, false, true, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.HasPrefix(clean, iconActive+" "+iconCaretClosed+" api.http (2)") {
		t.Fatalf("expected active file marker, caret, and title, got %q", clean)
	}
}

func TestRenderActiveRequestShowsSubtleMarker(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:   KindRequest,
			Title:  "getStatus",
			Method: "GET",
		},
	}
	out := renderRowState(row, false, true, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.HasPrefix(clean, iconActive+" GET  getStatus") {
		t.Fatalf("expected active request marker before method badge, got %q", clean)
	}
}

func TestRenderSelectedActivePrefersSelectionMarker(t *testing.T) {
	th := selectedRenderTheme(t)
	row := Flat[any]{
		Node: &Node[any]{
			Kind:     KindFile,
			Title:    "api.http",
			Expanded: false,
		},
	}
	out := renderRowState(row, true, true, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.HasPrefix(clean, iconSelected+" "+iconCaretClosed+" api.http") {
		t.Fatalf("expected selected marker to win over active marker, got %q", clean)
	}
	if strings.HasPrefix(clean, iconActive) {
		t.Fatalf("expected active marker to be hidden on selected row, got %q", clean)
	}
}

func TestRenderRowShowsRTSIcon(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:  KindFile,
			Title: "apply_patch.rts",
			Payload: Payload[any]{
				FilePath: "apply_patch.rts",
			},
		},
	}
	out := renderRow(row, false, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.Contains(clean, iconRTS) {
		t.Fatalf("expected rts icon, got %q", clean)
	}
	if strings.Contains(clean, "•") {
		t.Fatalf("expected rts row without bullet icon, got %q", clean)
	}
}

func TestRenderRTSUsesModuleIndicator(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:    KindFile,
			Title:   "mod.rts",
			Payload: Payload[any]{FilePath: "/tmp/mod.rts"},
		},
	}
	out := renderRow(row, false, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if strings.Contains(clean, iconCaretClosed) || strings.Contains(clean, iconCaretOpen) {
		t.Fatalf("expected rts row without caret, got %q", clean)
	}
	if !strings.Contains(clean, iconRTS) {
		t.Fatalf("expected rts icon, got %q", clean)
	}
}

func TestRenderRowShowsEnvIcon(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:   KindFile,
			Title:  "resterm.env.json",
			Badges: []string{"ENV", "ACTIVE"},
			Payload: Payload[any]{
				FilePath: "/tmp/resterm.env.json",
				Data: filesvc.FileEntry{
					Name: "resterm.env.json",
					Path: "/tmp/resterm.env.json",
					Kind: filesvc.FileKindEnv,
				},
			},
		},
	}
	out := renderRow(row, false, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.Contains(clean, iconEnv) {
		t.Fatalf("expected env icon, got %q", clean)
	}
	if strings.Contains(clean, iconCaretClosed) || strings.Contains(clean, iconCaretOpen) {
		t.Fatalf("expected env row without caret, got %q", clean)
	}
	if !strings.Contains(clean, "ENV") || !strings.Contains(clean, "ACTIVE") {
		t.Fatalf("expected env badges, got %q", clean)
	}
}

func TestRenderRowShowsGraphQLIcon(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:  KindFile,
			Title: "query.graphql",
			Payload: Payload[any]{
				FilePath: "/tmp/query.graphql",
				Data: filesvc.FileEntry{
					Name: "query.graphql",
					Path: "/tmp/query.graphql",
					Kind: filesvc.FileKindGraphQL,
				},
			},
		},
	}
	out := renderRow(row, false, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.Contains(clean, iconGraphQL) {
		t.Fatalf("expected graphql icon, got %q", clean)
	}
	if strings.Contains(clean, iconCaretClosed) || strings.Contains(clean, iconCaretOpen) {
		t.Fatalf("expected graphql row without caret, got %q", clean)
	}
	if strings.Contains(clean, "GQL") {
		t.Fatalf("expected graphql row without extension badge, got %q", clean)
	}
}

func TestRenderRowShowsJSONIcon(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:  KindFile,
			Title: "variables.json",
			Payload: Payload[any]{
				FilePath: "/tmp/variables.json",
				Data: filesvc.FileEntry{
					Name: "variables.json",
					Path: "/tmp/variables.json",
					Kind: filesvc.FileKindJSON,
				},
			},
		},
	}
	out := renderRow(row, false, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.Contains(clean, iconJSON) {
		t.Fatalf("expected json icon, got %q", clean)
	}
	if strings.Contains(clean, iconCaretClosed) || strings.Contains(clean, iconCaretOpen) {
		t.Fatalf("expected json row without caret, got %q", clean)
	}
	if strings.Contains(clean, "JSON") {
		t.Fatalf("expected json row without extension badge, got %q", clean)
	}
}

func TestRenderRowShowsJavaScriptIcon(t *testing.T) {
	th := theme.DefaultTheme()
	row := Flat[any]{
		Node: &Node[any]{
			Kind:  KindFile,
			Title: "pre.js",
			Payload: Payload[any]{
				FilePath: "/tmp/pre.js",
				Data: filesvc.FileEntry{
					Name: "pre.js",
					Path: "/tmp/pre.js",
					Kind: filesvc.FileKindJavaScript,
				},
			},
		},
	}
	out := renderRow(row, false, th, 80, true, false, theme.AppearanceUnknown)
	clean := ansi.Strip(out)
	if !strings.Contains(clean, iconJavaScript) {
		t.Fatalf("expected javascript icon, got %q", clean)
	}
	if strings.Contains(clean, iconCaretClosed) || strings.Contains(clean, iconCaretOpen) {
		t.Fatalf("expected javascript row without caret, got %q", clean)
	}
}
