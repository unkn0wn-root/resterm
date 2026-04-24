package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/ui/hint"
)

func TestMetadataHintPopupLayoutPrefersBelowWhenItFits(t *testing.T) {
	m := New(Config{})
	got, ok := m.metadataHintPopupLayout(50, 12, 10, 2, 4)
	if !ok {
		t.Fatal("expected popup layout")
	}
	if got.y != 3 {
		t.Fatalf("expected popup below cursor at row 3, got %d", got.y)
	}
	if got.limit != 4 {
		t.Fatalf("expected all hints to fit below, got %d", got.limit)
	}
}

func TestMetadataHintPopupLayoutKeepsBelowWhenBothSidesFit(t *testing.T) {
	m := New(Config{})
	got, ok := m.metadataHintPopupLayout(50, 18, 10, 8, 3)
	if !ok {
		t.Fatal("expected popup layout")
	}
	if got.y != 9 {
		t.Fatalf("expected popup to stay below cursor, got row %d", got.y)
	}
	if got.limit != 3 {
		t.Fatalf("expected all hints to fit below, got %d", got.limit)
	}
}

func TestMetadataHintPopupLayoutCapsVisibleRows(t *testing.T) {
	m := New(Config{})
	got, ok := m.metadataHintPopupLayout(80, 40, 10, 2, 50)
	if !ok {
		t.Fatal("expected popup layout")
	}
	if got.limit != metadataHintMaxRows {
		t.Fatalf("expected hint limit %d, got %d", metadataHintMaxRows, got.limit)
	}
}

func TestMetadataHintPopupLayoutStaysBelowWhenCappedRowsFit(t *testing.T) {
	m := New(Config{})
	got, ok := m.metadataHintPopupLayout(80, 50, 10, 30, 50)
	if !ok {
		t.Fatal("expected popup layout")
	}
	if got.y != 31 {
		t.Fatalf("expected popup below cursor at row 31, got %d", got.y)
	}
	if got.limit != metadataHintMaxRows {
		t.Fatalf("expected hint limit %d, got %d", metadataHintMaxRows, got.limit)
	}
}

func TestMetadataHintPopupLayoutFlipsAboveWhenBelowIsTight(t *testing.T) {
	m := New(Config{})
	got, ok := m.metadataHintPopupLayout(50, 12, 10, 8, 5)
	if !ok {
		t.Fatal("expected popup layout")
	}
	if got.y != 1 {
		t.Fatalf("expected popup to flip above cursor, got row %d", got.y)
	}
	if got.limit != 5 {
		t.Fatalf("expected all hints to fit above, got %d", got.limit)
	}
}

func TestBuildMetadataHintPopupTruncatesSummaryWhenWidthIsTight(t *testing.T) {
	m := New(Config{})
	items := []hint.Hint{
		{Label: "@variables", Summary: "project variables"},
		{Label: "@timeout", Summary: "request timeout"},
	}
	labelW, summaryW := metadataHintPopupPreference(items)
	lines := m.buildMetadataHintPopup(
		items,
		0,
		18,
		labelW,
		summaryW,
	)
	if len(lines) == 0 {
		t.Fatal("expected popup lines")
	}

	plain := stripANSIEscape(strings.Join(lines, "\n"))
	if !strings.Contains(plain, "@variables pro") {
		t.Fatalf("expected summary text to remain visible, got %q", plain)
	}
	if !strings.Contains(plain, "@variables") {
		t.Fatalf("expected label to remain visible, got %q", plain)
	}
	for _, line := range strings.Split(plain, "\n") {
		if w := lipgloss.Width(line); w > 18 {
			t.Fatalf("expected popup width <= 18, got %d in %q", w, line)
		}
	}
}

func TestBuildMetadataHintPopupKeepsSummaryWithLongerVisibleItems(t *testing.T) {
	m := New(Config{})
	items := []hint.Hint{
		{Label: "@if", Summary: "branch start"},
		{Label: "@elif", Summary: "branch alternative"},
		{Label: "@else", Summary: "fallback branch"},
		{Label: "@verylongdirective", Summary: "longer item should not hide help"},
	}
	labelW, summaryW := metadataHintPopupPreference(items)
	lines := m.buildMetadataHintPopup(
		items,
		1,
		34,
		labelW,
		summaryW,
	)
	if len(lines) == 0 {
		t.Fatal("expected popup lines")
	}

	plain := stripANSIEscape(strings.Join(lines, "\n"))
	for _, want := range []string{"branch", "fallback", "longer"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected visible summary fragment %q in %q", want, plain)
		}
	}
}

func TestMetadataHintStatePopupPreferenceDoesNotShrinkWithinSession(t *testing.T) {
	var st metadataHintState
	st.update(10, []hint.Hint{
		{Label: "@verylongdirective", Summary: "very long summary"},
	}, hint.Context{})
	if st.popupLabelW == 0 || st.popupSummaryW == 0 {
		t.Fatalf("expected popup preference widths, got %d %d", st.popupLabelW, st.popupSummaryW)
	}

	labelW := st.popupLabelW
	summaryW := st.popupSummaryW
	st.update(10, []hint.Hint{
		{Label: "@if", Summary: "short"},
	}, hint.Context{})
	if st.popupLabelW != labelW || st.popupSummaryW != summaryW {
		t.Fatalf(
			"expected session popup widths to stay locked, got %d/%d want %d/%d",
			st.popupLabelW,
			st.popupSummaryW,
			labelW,
			summaryW,
		)
	}
}

func TestBuildMetadataHintPopupKeepsWidthAcrossVisibleWindows(t *testing.T) {
	m := New(Config{})
	all := []hint.Hint{
		{Label: "@if", Summary: "branch start"},
		{Label: "@elif", Summary: "branch alternative"},
		{Label: "@else", Summary: "fallback branch"},
		{Label: "@verylongdirective", Summary: "longer item should not widen on scroll"},
		{Label: "@default", Summary: "final branch"},
	}
	labelW, summaryW := metadataHintPopupPreference(all)

	winA := m.buildMetadataHintPopup(all[:3], 0, 42, labelW, summaryW)
	winB := m.buildMetadataHintPopup(all[2:], 0, 42, labelW, summaryW)
	if len(winA) == 0 || len(winB) == 0 {
		t.Fatal("expected popup windows")
	}
	if metadataHintPopupWidth(winA) != metadataHintPopupWidth(winB) {
		t.Fatalf(
			"expected stable popup width across windows, got %d and %d",
			metadataHintPopupWidth(winA),
			metadataHintPopupWidth(winB),
		)
	}
}

func TestBuildMetadataHintPreviewShowsAliasesAndInsert(t *testing.T) {
	m := New(Config{})
	lines := m.buildMetadataHintPreview(
		hint.Hint{
			Label:   "@auth",
			Aliases: []string{"@authorization"},
			Summary: "Configure authentication",
			Insert:  "auth bearer {{token}}",
		},
		40,
		12,
	)
	if len(lines) == 0 {
		t.Fatal("expected preview lines")
	}

	plain := stripANSIEscape(strings.Join(lines, "\n"))
	for _, want := range []string{
		"Configure authentication",
		"Aliases",
		"@authorization",
		"Insert",
		"auth bearer {{token}}",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected preview content %q in %q", want, plain)
		}
	}
}

func TestRenderMetadataHintPopupFlipsAboveNearBottom(t *testing.T) {
	m := newHintPopupModel(t, 90, 18)
	m.editor.SetValue(strings.Join([]string{
		"GET https://example.com",
		"# @body",
		"# @body",
		"# @body",
		"# @body",
		"# @body",
		"# @body",
		"# @body",
	}, "\n"))
	m.editor.moveCursorTo(7, len([]rune("# @body")))
	setTestMetadataHints(&m, []hint.Hint{
		{Label: "@body", Summary: "request body"},
		{Label: "@variables", Summary: "workspace variables"},
		{Label: "@trace", Summary: "trace settings"},
		{Label: "@k8s", Summary: "kubernetes target"},
	}, 0)

	content := m.editor.View()
	_, cy := m.editor.ViewportCursor()
	out := stripANSIEscape(m.renderMetadataHintPopup(content))
	lines := strings.Split(out, "\n")

	top := findLineContaining(lines, "╭")
	if top < 0 {
		t.Fatalf("expected popup border in rendered content, got %q", out)
	}
	if top >= cy {
		t.Fatalf("expected popup above cursor row %d, got top row %d", cy, top)
	}
}

func TestRenderMetadataHintPopupClampsLeftNearRightEdge(t *testing.T) {
	m := newHintPopupModel(t, 70, 16)
	m.editor.SetValue("# @verylongdirectivevaluewithmoretext")
	m.editor.moveCursorTo(0, len([]rune("# @verylongdirectivevaluewithmoretext")))
	setTestMetadataHints(&m, []hint.Hint{
		{Label: "@variables", Summary: "workspace variables"},
		{Label: "@environment", Summary: "active environment"},
		{Label: "@timeout", Summary: "request timeout"},
	}, 0)

	content := m.editor.View()
	w := lipgloss.Width(content)
	cx, _ := m.editor.ViewportCursor()
	out := stripANSIEscape(m.renderMetadataHintPopup(content))
	lines := strings.Split(out, "\n")

	top := findLineContaining(lines, "╭")
	if top < 0 {
		t.Fatalf("expected popup border in rendered content, got %q", out)
	}
	left := runeIndex(lines[top], '╭')
	if left >= cx {
		t.Fatalf("expected popup to shift left of cursor %d, got %d", cx, left)
	}
	for _, line := range lines {
		if ww := lipgloss.Width(line); ww > w {
			t.Fatalf("expected rendered content width <= %d, got %d in %q", w, ww, line)
		}
	}
}

func TestRenderMetadataHintPopupPreviewShowsHelpForSelectedHint(t *testing.T) {
	m := newHintPopupModel(t, 110, 20)
	m.editor.SetValue("# @auth")
	m.editor.moveCursorTo(0, len([]rune("# @auth")))
	setTestMetadataHints(&m, []hint.Hint{
		{
			Label:   "@auth",
			Aliases: []string{"@authorization"},
			Summary: "Configure authentication",
			Insert:  "auth bearer {{token}}",
		},
		{
			Label:   "@assert",
			Summary: "Evaluate an assertion",
		},
	}, 0)
	m.editor.metadataHints.preview = true

	out := stripANSIEscape(m.renderMetadataHintPopup(m.editor.View()))
	for _, want := range []string{"Configure authentication", "@auth"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected preview content %q in %q", want, out)
		}
	}
	if strings.Count(out, "╭") < 2 {
		t.Fatalf("expected both list and preview boxes in %q", out)
	}
}

func newHintPopupModel(t *testing.T, width, height int) Model {
	t.Helper()

	m := New(Config{WorkspaceRoot: t.TempDir()})
	m.width = width
	m.height = height
	m.ready = true
	m.focus = focusEditor
	m.editorInsertMode = true
	if cmd := m.applyLayout(); cmd != nil {
		_ = cmd
	}
	return m
}

func setTestMetadataHints(m *Model, items []hint.Hint, selection int) {
	m.editor.metadataHints.update(0, items, hint.Context{})
	m.editor.metadataHints.selection = selection
}

func findLineContaining(lines []string, needle string) int {
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

func runeIndex(s string, target rune) int {
	for i, r := range []rune(s) {
		if r == target {
			return i
		}
	}
	return -1
}
