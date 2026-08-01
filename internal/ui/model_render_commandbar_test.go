package ui

import (
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

func TestRenderCommandButtonUsesSingleCellHorizontalPadding(t *testing.T) {
	out := ansi.Strip(renderCommandButton("Tab", "Focus", theme.CommandSegmentStyle{}))

	if out != " Tab Focus " {
		t.Fatalf("expected single-cell shortcut padding, got %q", out)
	}
}

func TestRenderCommandBarKeepsStableAnchors(t *testing.T) {
	model := New(Config{})
	model.width = 220
	model.focus = focusEditor

	out := ansi.Strip(model.renderCommandBar())
	for _, unexpected := range []string{"^N New", "^S Save"} {
		if strings.Contains(out, unexpected) {
			t.Fatalf("did not expect pinned hint %q, got %q", unexpected, out)
		}
	}
	prev := -1
	for _, want := range []string{
		"Tab Focus", "^Q Quit", ": Cmd", "? Help",
	} {
		idx := strings.Index(out, want)
		if idx < 0 {
			t.Fatalf("expected stable command-bar anchor %q, got %q", want, out)
		}
		if idx < prev {
			t.Fatalf("expected anchor %q after the previous one, got %q", want, out)
		}
		prev = idx
	}
	if !strings.HasSuffix(strings.TrimRight(out, " "), "? Help") {
		t.Fatalf("expected anchors pinned to the right edge, got %q", out)
	}
	if strings.Index(out, "i Insert") > strings.Index(out, "Tab Focus") {
		t.Fatalf("expected context hints before the anchors, got %q", out)
	}
}

func TestRenderCommandBarHintsHaveNoSegmentBackground(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

	model := New(Config{})
	model.width = 220
	model.focus = focusEditor

	bar := model.renderCommandBar()
	plain := ansi.Strip(bar)
	backgrounds := renderedCellBackgrounds(bar)
	if len(backgrounds) != lipgloss.Width(plain) {
		t.Fatalf(
			"expected %d rendered cell backgrounds, got %d",
			lipgloss.Width(plain),
			len(backgrounds),
		)
	}

	for _, hint := range []string{
		"i Insert", "Enter Send", "/ Search",
		"Tab Focus", "^Q Quit", ": Cmd", "? Help",
	} {
		assertCommandHintBackground(t, plain, backgrounds, hint, nil)
	}
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
			name: "requests",
			setup: func(m *Model) {
				m.focus = focusRequests
			},
			want: []string{"Enter Run", "m Method", "t Tags", "l Jump"},
		},
		{
			name: "editor normal",
			setup: func(m *Model) {
				m.focus = focusEditor
				m.editorInsertMode = false
			},
			want:   []string{"i Insert", "Enter Send", "Shift+K Docs", "/ Search"},
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
			name: "response raw",
			setup: func(m *Model) {
				m.focus = focusResponse
				m.responsePanes[0].activeTab = responseTabRaw
			},
			want: []string{"j/k Scroll", "g b View", "g Shift+D Dump"},
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

func TestRenderCommandBarCompactsStableAnchorsAtNarrowWidth(t *testing.T) {
	model := New(Config{})
	model.width = 24
	model.focus = focusEditor

	out := ansi.Strip(model.renderCommandBar())
	if got := lipgloss.Width(out); got > model.width {
		t.Fatalf("expected command bar to fit width %d, got %d in %q", model.width, got, out)
	}
	for _, want := range []string{"Tab", ":", "?"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected compact anchor %q, got %q", want, out)
		}
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
