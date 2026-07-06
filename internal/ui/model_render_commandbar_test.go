package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
