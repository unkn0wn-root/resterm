package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/intellisense"
)

func TestCompletionPopupLayoutPrefersBelowWhenItFits(t *testing.T) {
	m := New(Config{})
	got, ok := m.completionPopupLayout(50, 12, 10, 2, 4)
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

func TestCompletionPopupLayoutKeepsBelowWhenBothSidesFit(t *testing.T) {
	m := New(Config{})
	got, ok := m.completionPopupLayout(50, 18, 10, 8, 3)
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

func TestCompletionPopupLayoutCapsVisibleRows(t *testing.T) {
	m := New(Config{})
	got, ok := m.completionPopupLayout(80, 40, 10, 2, 50)
	if !ok {
		t.Fatal("expected popup layout")
	}
	if got.limit != completionPopupMaxRows {
		t.Fatalf("expected hint limit %d, got %d", completionPopupMaxRows, got.limit)
	}
}

func TestCompletionPopupLayoutStaysBelowWhenCappedRowsFit(t *testing.T) {
	m := New(Config{})
	got, ok := m.completionPopupLayout(80, 50, 10, 30, 50)
	if !ok {
		t.Fatal("expected popup layout")
	}
	if got.y != 31 {
		t.Fatalf("expected popup below cursor at row 31, got %d", got.y)
	}
	if got.limit != completionPopupMaxRows {
		t.Fatalf("expected hint limit %d, got %d", completionPopupMaxRows, got.limit)
	}
}

func TestCompletionPopupLayoutFlipsAboveWhenBelowIsTight(t *testing.T) {
	m := New(Config{})
	got, ok := m.completionPopupLayout(50, 12, 10, 8, 5)
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

func TestBuildCompletionPopupTruncatesSummaryWhenWidthIsTight(t *testing.T) {
	m := New(Config{})
	items := []intellisense.Item{
		{Label: "@variables", Summary: "project variables"},
		{Label: "@timeout", Summary: "request timeout"},
	}
	labelW, summaryW := completionPopupPreference(items)
	lines := m.buildCompletionPopup(
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
	for line := range strings.SplitSeq(plain, "\n") {
		if w := lipgloss.Width(line); w > 18 {
			t.Fatalf("expected popup width <= 18, got %d in %q", w, line)
		}
	}
}

func TestBuildCompletionPopupKeepsSummaryWithLongerVisibleItems(t *testing.T) {
	m := New(Config{})
	items := []intellisense.Item{
		{Label: "@if", Summary: "branch start"},
		{Label: "@elif", Summary: "branch alternative"},
		{Label: "@else", Summary: "fallback branch"},
		{Label: "@verylongdirective", Summary: "longer item should not hide help"},
	}
	labelW, summaryW := completionPopupPreference(items)
	lines := m.buildCompletionPopup(
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

func TestCompletionStatePopupPreferenceDoesNotShrinkWithinSession(t *testing.T) {
	var st completionState
	st.update(10, []intellisense.Item{
		{Label: "@verylongdirective", Summary: "very long summary"},
	}, intellisense.Context{})
	if st.popupLabelW == 0 || st.popupSummaryW == 0 {
		t.Fatalf("expected popup preference widths, got %d %d", st.popupLabelW, st.popupSummaryW)
	}

	labelW := st.popupLabelW
	summaryW := st.popupSummaryW
	st.update(10, []intellisense.Item{
		{Label: "@if", Summary: "short"},
	}, intellisense.Context{})
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

func TestBuildCompletionPopupKeepsWidthAcrossVisibleWindows(t *testing.T) {
	m := New(Config{})
	all := []intellisense.Item{
		{Label: "@if", Summary: "branch start"},
		{Label: "@elif", Summary: "branch alternative"},
		{Label: "@else", Summary: "fallback branch"},
		{Label: "@verylongdirective", Summary: "longer item should not widen on scroll"},
		{Label: "@default", Summary: "final branch"},
	}
	labelW, summaryW := completionPopupPreference(all)

	winA := m.buildCompletionPopup(all[:3], 0, 42, labelW, summaryW)
	winB := m.buildCompletionPopup(all[2:], 0, 42, labelW, summaryW)
	if len(winA) == 0 || len(winB) == 0 {
		t.Fatal("expected popup windows")
	}
	if completionPopupWidth(winA) != completionPopupWidth(winB) {
		t.Fatalf(
			"expected stable popup width across windows, got %d and %d",
			completionPopupWidth(winA),
			completionPopupWidth(winB),
		)
	}
}

func TestBuildCompletionPreviewShowsAliasesAndInsert(t *testing.T) {
	m := New(Config{})
	lines := m.buildCompletionPreview(
		intellisense.Item{
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

func TestRenderCompletionPopupFlipsAboveNearBottom(t *testing.T) {
	m := newCompletionPopupModel(t, 90, 18)
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
	setTestCompletions(&m, []intellisense.Item{
		{Label: "@body", Summary: "request body"},
		{Label: "@variables", Summary: "workspace variables"},
		{Label: "@trace", Summary: "trace settings"},
		{Label: "@k8s", Summary: "kubernetes target"},
	}, 0)

	content := m.editor.View()
	_, cy := m.editor.ViewportCursor()
	out := stripANSIEscape(m.renderCompletionPopup(content))
	lines := strings.Split(out, "\n")

	top := findLineContaining(lines, "╭")
	if top < 0 {
		t.Fatalf("expected popup border in rendered content, got %q", out)
	}
	if top >= cy {
		t.Fatalf("expected popup above cursor row %d, got top row %d", cy, top)
	}
}

func TestRenderCompletionPopupClampsLeftNearRightEdge(t *testing.T) {
	m := newCompletionPopupModel(t, 70, 16)
	m.editor.SetValue("# @verylongdirectivevaluewithmoretext")
	m.editor.moveCursorTo(0, len([]rune("# @verylongdirectivevaluewithmoretext")))
	setTestCompletions(&m, []intellisense.Item{
		{Label: "@variables", Summary: "workspace variables"},
		{Label: "@environment", Summary: "active environment"},
		{Label: "@timeout", Summary: "request timeout"},
	}, 0)

	content := m.editor.View()
	w := lipgloss.Width(content)
	cx, _ := m.editor.ViewportCursor()
	out := stripANSIEscape(m.renderCompletionPopup(content))
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

func TestRenderCompletionPopupPreviewShowsHelpForSelectedHint(t *testing.T) {
	m := newCompletionPopupModel(t, 110, 20)
	m.editor.SetValue("# @auth")
	m.editor.moveCursorTo(0, len([]rune("# @auth")))
	setTestCompletions(&m, []intellisense.Item{
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
	m.editor.completion.preview = true

	out := stripANSIEscape(m.renderCompletionPopup(m.editor.View()))
	for _, want := range []string{"Configure authentication", "@auth"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected preview content %q in %q", want, out)
		}
	}
	if strings.Count(out, "╭") < 2 {
		t.Fatalf("expected both list and preview boxes in %q", out)
	}
}

func newCompletionPopupModel(t *testing.T, width, height int) Model {
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

func setTestCompletions(m *Model, items []intellisense.Item, selection int) {
	m.editor.completion.update(0, items, intellisense.Context{})
	m.editor.completion.selection = selection
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

func TestEscDismissesCompletionBeforeLeavingInsertMode(t *testing.T) {
	m := newCompletionPopupModel(t, 120, 40)
	setTestCompletions(&m, []intellisense.Item{
		{Label: "@auth", Summary: "Configure authentication"},
		{Label: "@name", Summary: "Assign a name"},
	}, 0)
	if !m.editor.completion.active {
		t.Fatal("expected completion popup to be active")
	}

	// First esc retracts the popup but must stay in insert mode.
	_ = m.handleKey(keyMsgFor("esc"))
	if m.editor.completion.active {
		t.Fatal("expected esc to dismiss the completion popup")
	}
	if !m.editorInsertMode {
		t.Fatal("expected esc to keep the editor in insert mode while a popup was open")
	}

	// Second esc, with no popup, leaves insert mode as before.
	_ = m.handleKey(keyMsgFor("esc"))
	if m.editorInsertMode {
		t.Fatal("expected esc to leave insert mode once the popup was closed")
	}
}
