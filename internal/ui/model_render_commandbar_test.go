package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestRenderCommandBarContainerPreservesOuterPadding(t *testing.T) {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("#112233")).
		Padding(0, 1)

	out := renderCommandBarContainer(style, "Hi")

	if !strings.HasPrefix(out, " ") {
		t.Fatalf("expected left gutter to remain unstyled, got %q", out)
	}
	if !strings.HasSuffix(out, " ") {
		t.Fatalf("expected right gutter to remain unstyled, got %q", out)
	}
	if lipgloss.Width(out) != 4 { // 1 pad left + len(Hi) + 1 pad right
		t.Fatalf("expected rendered width 4, got %d", lipgloss.Width(out))
	}
}

func TestRenderCommandBarContainerRespectsWidthConstraints(t *testing.T) {
	style := lipgloss.NewStyle().
		Background(lipgloss.Color("#445566")).
		Padding(0, 2).
		Width(10)

	out := renderCommandBarContainer(style, "OK")

	if lipgloss.Width(out) != 10 {
		t.Fatalf("expected rendered width 10, got %d", lipgloss.Width(out))
	}
	if !strings.HasPrefix(out, "  ") {
		t.Fatalf("expected leading gutter to remain blank, got %q", out)
	}
}

func TestRenderSearchPromptAlignsToCommandBarStart(t *testing.T) {
	model := New(Config{})
	model.width = 80
	model.showSearchPrompt = true
	model.searchTarget = searchTargetEditor
	model.searchInput.Focus()

	out := ansi.Strip(model.renderSearchPrompt())

	if !strings.HasPrefix(out, " /pattern") {
		t.Fatalf("expected search prompt to align with command bar gutter, got %q", out)
	}
}

func TestRenderSearchPromptShowsLiteralGuideWhenEmpty(t *testing.T) {
	model := New(Config{})
	model.width = 96
	model.searchTarget = searchTargetEditor
	model.searchInput.Focus()

	out := ansi.Strip(model.renderSearchPrompt())
	expected := "/pattern LITERAL ^R regex"

	if !strings.Contains(out, expected) {
		t.Fatalf("expected empty literal editor search guide %q, got %q", expected, out)
	}
	if strings.Contains(out, "/p LITERAL") {
		t.Fatalf("expected full /pattern placeholder, got %q", out)
	}
}

func TestRenderSearchPromptShowsRegexGuideWhenEmpty(t *testing.T) {
	model := New(Config{})
	model.width = 96
	model.searchTarget = searchTargetEditor
	model.searchIsRegex = true
	model.searchInput.Focus()

	out := ansi.Strip(model.renderSearchPrompt())
	expected := "/pattern REGEX ^R literal"

	if !strings.Contains(out, expected) {
		t.Fatalf("expected empty regex editor search guide %q, got %q", expected, out)
	}
	if !strings.Contains(out, "^R literal") {
		t.Fatalf("expected regex guide to offer literal toggle, got %q", out)
	}
}

func TestRenderSearchPromptHidesGuideAfterTyping(t *testing.T) {
	model := New(Config{})
	model.width = 80
	model.searchTarget = searchTargetEditor
	model.searchInput.SetValue("pattern")
	model.searchInput.Focus()

	out := ansi.Strip(model.renderSearchPrompt())

	if !strings.Contains(out, "/pattern") {
		t.Fatalf("expected editor search input value, got %q", out)
	}
	assertSearchGuideHidden(t, out)
}

func TestRenderCommandBarDoesNotEchoResponseSearch(t *testing.T) {
	model := New(Config{})
	model.width = 80
	model.showSearchPrompt = true
	model.searchTarget = searchTargetResponse
	model.searchInput.SetValue("needle")
	model.searchInput.Focus()

	out := ansi.Strip(model.renderCommandBar())

	for _, unexpected := range []string{
		"Response Search",
		"needle",
		"^R",
	} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("expected command bar to hide response search %q, got %q", unexpected, out)
		}
	}
}

func TestRenderCommandButtonRendersFlush(t *testing.T) {
	out := ansi.Strip(renderCommandButton("Tab", "Focus", theme.CommandSegmentStyle{}, lipgloss.NoColor{}))

	if out != "Tab Focus" {
		t.Fatalf("expected flush shortcut cell, got %q", out)
	}
}

func TestRenderCommandKeycapWrapsKeyInChip(t *testing.T) {
	out := ansi.Strip(renderCommandKeycap("Tab", "Focus", theme.CommandSegmentStyle{
		Background: lipgloss.Color("#112233"),
	}, lipgloss.NoColor{}))

	if out != "▐Tab▌ Focus" {
		t.Fatalf("expected capped keycap chip, got %q", out)
	}
}

func TestRenderCommandHintFallsBackToFlatWithoutBackground(t *testing.T) {
	model := New(Config{})
	model.theme.CommandSegments = []theme.CommandSegmentStyle{{Key: lipgloss.Color("#FFFFFF")}}

	out := ansi.Strip(model.renderCommandHint(commandHint{key: "Tab", label: "Focus"}, 0, true, lipgloss.NoColor{}))

	if out != "Tab Focus" {
		t.Fatalf("expected flat hint without segment background, got %q", out)
	}
}

func TestRenderCommandBarOmitsGlobalShortcuts(t *testing.T) {
	model := New(Config{})
	model.width = 220
	model.focus = focusEditor

	out := ansi.Strip(model.renderCommandBar())
	for _, unwanted := range []string{"Tab Focus", "^Q Quit", ": Cmd", "? Help"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("command bar contains global shortcut %q: %q", unwanted, out)
		}
	}
}

func TestRenderCommandBarCustomBackgroundsEnableKeycaps(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	model := New(Config{})
	model.width = 220
	model.focus = focusEditor
	model.theme.CommandSegments = []theme.CommandSegmentStyle{
		{Background: lipgloss.Color("#112233")},
		{Background: lipgloss.Color("#223344")},
		{Background: lipgloss.Color("#334455")},
		{Background: lipgloss.Color("#445566")},
		{Background: lipgloss.Color("#556677")},
	}

	bar := model.renderCommandBar()
	plain := ansi.Strip(bar)
	backgrounds := renderedCellBackgrounds(bar)
	for idx, key := range []string{"i", "Enter", "Shift+K", "/", "^S"} {
		assertKeycapBackground(t, plain, backgrounds, key, model.theme.CommandSegment(idx).Background)
	}
	for _, flat := range []string{" Insert", " Send", " Docs", " Search", " Save"} {
		assertCommandHintBackground(t, plain, backgrounds, flat, nil)
	}
}

// A themed command_bar background must cover every cell inside the gutters or
// the bar shows seams.
func TestRenderCommandBarKeepsThemedBackground(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	model := New(Config{})
	model.width = 120
	model.focus = focusEditor
	barBg := lipgloss.Color("#e2e8f0")
	model.theme.CommandBar = model.theme.CommandBar.Background(barBg)

	bar := model.renderCommandBar()
	plain := []rune(ansi.Strip(bar))
	backgrounds := renderedCellBackgrounds(bar)

	want := sgrTrueColorBackground(t, barBg)
	for idx := 1; idx < len(backgrounds)-1; idx++ {
		if !slices.Equal(backgrounds[idx], want) {
			t.Fatalf("cell %d %q lost the bar background: got %v", idx, string(plain[idx]), backgrounds[idx])
		}
	}
}

// A command_divider background configured by a theme must survive even though
// hint runs set the bar background on themselves.
func TestRenderCommandBarKeepsDividerBackground(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	model := New(Config{})
	model.width = 120
	model.focus = focusEditor
	divBg := lipgloss.Color("#445566")
	model.theme.CommandDivider = model.theme.CommandDivider.Background(divBg)

	bar := model.renderCommandBar()
	plain := ansi.Strip(bar)
	backgrounds := renderedCellBackgrounds(bar)

	first := "i Insert"
	before, _, ok := strings.Cut(plain, first)
	if !ok {
		t.Fatalf("expected hint %q in %q", first, plain)
	}
	want := sgrTrueColorBackground(t, divBg)
	start := lipgloss.Width(before) + lipgloss.Width(first)
	for idx := start; idx < start+2; idx++ {
		if !slices.Equal(backgrounds[idx], want) {
			t.Fatalf("expected divider cell %d to keep background %v, got %v", idx, want, backgrounds[idx])
		}
	}
}

// assertKeycapBackground checks that the key cells inside the chip carry the
// segment background while the half-block caps stay on the bar background.
func assertKeycapBackground(
	t *testing.T,
	plain string,
	backgrounds [][]int,
	key string,
	bg lipgloss.Color,
) {
	t.Helper()

	chip := "▐" + key + "▌"
	before, _, ok := strings.Cut(plain, chip)
	if !ok {
		t.Fatalf("expected keycap %q in %q", chip, plain)
	}
	want := sgrTrueColorBackground(t, bg)
	start := lipgloss.Width(before)
	end := start + lipgloss.Width(chip)
	for idx := start; idx < end; idx++ {
		cell := backgrounds[idx]
		if idx == start || idx == end-1 {
			if len(cell) > 0 {
				t.Fatalf("expected %q cap cell %d on the bar background, got %v", chip, idx, cell)
			}
			continue
		}
		if !slices.Equal(cell, want) {
			t.Fatalf("expected %q key cell %d to use background %v, got %v", chip, idx, want, cell)
		}
	}
}

func sgrTrueColorBackground(t *testing.T, c lipgloss.Color) []int {
	t.Helper()

	var r, g, b int
	if _, err := fmt.Sscanf(strings.TrimPrefix(string(c), "#"), "%02x%02x%02x", &r, &g, &b); err != nil {
		t.Fatalf("parse segment color %q: %v", c, err)
	}
	return []int{sgrExtBackground, sgrExtRGB, r, g, b}
}

func assertCommandHintBackground(
	t *testing.T,
	plain string,
	backgrounds [][]int,
	hint string,
	want []int,
) {
	t.Helper()

	before, _, ok := strings.Cut(plain, hint)
	if !ok {
		t.Fatalf("expected command hint %q in %q", hint, plain)
	}
	cellStart := lipgloss.Width(before)
	for idx := cellStart; idx < cellStart+lipgloss.Width(hint); idx++ {
		if !slices.Equal(backgrounds[idx], want) {
			t.Fatalf(
				"expected %q cell %d to use background %v, got %v",
				hint,
				idx,
				want,
				backgrounds[idx],
			)
		}
	}
}

func TestRenderCommandBarUsesFocusedContext(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*Model)
		want   []string
		absent string
	}{
		{
			name: "files",
			setup: func(m *Model) {
				m.focus = focusFile
			},
			want: []string{"Enter Open", "Space Expand", "/ Filter", "^N New", "^O Open", "^T Temp"},
		},
		{
			name: "requests",
			setup: func(m *Model) {
				m.focus = focusRequests
			},
			want: []string{"Enter Run", "m Method", "t Tags", "l Jump", "g , Details"},
		},
		{
			name: "editor normal",
			setup: func(m *Model) {
				m.focus = focusEditor
				m.editorInsertMode = false
			},
			want:   []string{"i Insert", "Enter Send", "Shift+K Docs", "/ Search", "^S Save"},
			absent: "Ctrl+Enter",
		},
		{
			name: "editor insert",
			setup: func(m *Model) {
				m.focus = focusEditor
				m.editorInsertMode = true
			},
			want:   []string{"Esc Normal", "Ctrl+Enter Send", "Tab Complete"},
			absent: "Shift+K Docs",
		},
		{
			name: "response pretty",
			setup: func(m *Model) {
				m.focus = focusResponse
				m.responsePanes[0].activeTab = responseTabPretty
			},
			want: []string{"j/k Scroll", "g Shift+S Save"},
		},
		{
			name: "response raw",
			setup: func(m *Model) {
				m.focus = focusResponse
				m.responsePanes[0].activeTab = responseTabRaw
			},
			want: []string{"j/k Scroll", "g b View", "g Shift+D Dump", "g Shift+S Save"},
		},
		{
			name: "response history",
			setup: func(m *Model) {
				m.focus = focusResponse
				m.responsePanes[0].activeTab = responseTabHistory
			},
			want: []string{"c Scope", "s Sort", "Enter Load"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := New(Config{})
			model.width = 240
			tt.setup(&model)
			out := ansi.Strip(model.renderCommandBar())
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("expected context hint %q, got %q", want, out)
				}
			}
			if tt.absent != "" && strings.Contains(out, tt.absent) {
				t.Fatalf("did not expect context hint %q, got %q", tt.absent, out)
			}
		})
	}
}

func TestRenderCommandBarFitsNarrowWidth(t *testing.T) {
	model := New(Config{})
	model.width = 24
	model.focus = focusEditor

	out := ansi.Strip(model.renderCommandBar())
	if got := lipgloss.Width(out); got > model.width {
		t.Fatalf("expected command bar to fit width %d, got %d in %q", model.width, got, out)
	}
	if !strings.Contains(out, "i Insert") {
		t.Fatalf("expected the leading context hint to survive, got %q", out)
	}
}

func TestRenderCommandBarUsesCustomContextHelpBinding(t *testing.T) {
	dir := t.TempDir()
	data := []byte("[bindings]\nshow_context_help = [\"ctrl+k\"]\n")
	if err := os.WriteFile(filepath.Join(dir, "bindings.toml"), data, 0o644); err != nil {
		t.Fatalf("write bindings: %v", err)
	}
	bindingMap, _, err := bindings.Load(dir)
	if err != nil {
		t.Fatalf("load bindings: %v", err)
	}
	model := New(Config{Bindings: bindingMap})
	model.width = 180
	model.focus = focusEditor

	out := ansi.Strip(model.renderCommandBar())
	if !strings.Contains(out, "Ctrl+K Docs") {
		t.Fatalf("expected configured contextual-help binding, got %q", out)
	}
}

func TestRenderCommandBarHidesUnboundSoftContextHelp(t *testing.T) {
	dir := t.TempDir()
	data := []byte("[bindings]\nshow_globals = [\"shift+k\"]\n")
	if err := os.WriteFile(filepath.Join(dir, "bindings.toml"), data, 0o644); err != nil {
		t.Fatalf("write bindings: %v", err)
	}
	bindingMap, _, err := bindings.Load(dir)
	if err != nil {
		t.Fatalf("load bindings: %v", err)
	}
	model := New(Config{Bindings: bindingMap})
	model.width = 180
	model.focus = focusEditor

	out := ansi.Strip(model.renderCommandBar())
	if strings.Contains(out, "Docs") {
		t.Fatalf("did not expect an unbound contextual-help hint, got %q", out)
	}
	for _, section := range model.helpSections() {
		for _, entry := range section.entries {
			if strings.Contains(entry.description, "directive or keyword under the cursor") {
				t.Fatalf("did not expect unbound contextual help in help overlay: %+v", entry)
			}
		}
	}
}

func TestRenderResponseSearchPromptShowsLiteralGuideWhenEmpty(t *testing.T) {
	model := New(Config{})
	model.searchTarget = searchTargetResponse
	model.searchInput.Focus()

	out := ansi.Strip(model.renderResponseSearchPrompt(96))
	expected := "/pattern LITERAL ^R regex"

	if !strings.Contains(out, expected) {
		t.Fatalf("expected empty literal response search guide %q, got %q", expected, out)
	}
	if strings.Contains(out, "/p LITERAL") {
		t.Fatalf("expected full /pattern placeholder, got %q", out)
	}
}

func TestRenderResponseSearchPromptShowsRegexGuideWhenEmpty(t *testing.T) {
	model := New(Config{})
	model.searchTarget = searchTargetResponse
	model.searchIsRegex = true
	model.searchInput.Focus()

	out := ansi.Strip(model.renderResponseSearchPrompt(96))
	expected := "/pattern REGEX ^R literal"

	if !strings.Contains(out, expected) {
		t.Fatalf("expected empty regex response search guide %q, got %q", expected, out)
	}
	if !strings.Contains(out, "^R literal") {
		t.Fatalf("expected regex guide to offer literal toggle, got %q", out)
	}
}

func TestRenderResponseSearchPromptHidesGuideAfterTyping(t *testing.T) {
	model := New(Config{})
	model.searchTarget = searchTargetResponse
	model.searchInput.SetValue("pattern")
	model.searchInput.Focus()

	out := ansi.Strip(model.renderResponseSearchPrompt(48))

	if !strings.HasPrefix(out, "/pattern") {
		t.Fatalf("expected response search prompt to start at pane edge, got %q", out)
	}
	assertSearchGuideHidden(t, out)
}

func TestRenderResponseSearchPromptKeepsCursorVisibleForLongQuery(t *testing.T) {
	model := New(Config{})
	model.searchTarget = searchTargetResponse
	model.searchInput.SetValue(strings.Repeat("a", 30) + "TAIL")
	model.searchInput.CursorEnd()
	model.searchInput.Focus()

	out := ansi.Strip(model.renderResponseSearchPrompt(24))

	if !strings.Contains(out, "TAIL") {
		t.Fatalf("expected long response search to render cursor tail, got %q", out)
	}
	if lipgloss.Width(out) > 24 {
		t.Fatalf(
			"expected response search prompt to fit width 24, got width %d in %q",
			lipgloss.Width(out),
			out,
		)
	}
	assertSearchGuideHidden(t, out)
}

func assertSearchGuideHidden(t *testing.T, out string) {
	t.Helper()

	for _, unexpected := range []string{
		"LIT",
		"LITERAL",
		"REGEX",
		"^R regex",
		"^R literal",
	} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("expected typed search to hide %q, got %q", unexpected, out)
		}
	}
}
