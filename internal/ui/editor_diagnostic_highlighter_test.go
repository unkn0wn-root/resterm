package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestDiagnosticStylerCopiesSyntaxCacheAndClearsAfterEdit(t *testing.T) {
	th := theme.DefaultTheme()
	source := "\u3000# @sse max-event=5"
	line := []rune(source)
	base := newMetadataRuneStyler(th.EditorMetadata)
	styler := newEditorDiagnosticStyler(base, th)
	styler.SetSource(source)
	before := base.StylesForLine(line, 0)
	styler.overlay = newDiagnosticSnapshot(
		diagnosticKey{},
		parser.Diagnostics(parser.Parse("test.http", []byte(source))),
	).overlay
	styles := styler.StylesForLine(line, 0)
	start := utf8.RuneCountInString(source[:strings.Index(source, "max-event")])
	for i := range line {
		marked := i >= start && i < start+len("max-event")
		if styles[i].GetUnderline() != marked {
			t.Fatalf("rune %d underline=%t, want %t", i, styles[i].GetUnderline(), marked)
		}
		if before[i].GetUnderline() {
			t.Fatal("diagnostic mutated syntax cache")
		}
		if marked && styles[i].GetForeground() != th.EditorDiagnosticWarning.GetForeground() {
			t.Fatal("warning color missing")
		}
	}
	styler.SetSource("# @sse max-events=5")
	if styler.overlay != nil {
		t.Fatal("source change retained old decorations")
	}
}

func TestDiagnosticOverlapsPreferErrors(t *testing.T) {
	source := "abcdefgh"
	span := func(a, b int) diag.Span {
		return diag.Span{Start: diag.Pos{Line: 1, Col: a}, End: diag.Pos{Line: 1, Col: b}}
	}
	rep := diag.Report{Source: []byte(source), Items: []diag.Diagnostic{
		{Severity: diag.SeverityWarning, Span: span(1, 7), Labels: []diag.Label{{Span: span(7, 9)}}},
		{Severity: diag.SeverityError, Span: span(3, 5)},
	}}
	s := newDiagnosticSnapshot(diagnosticKey{}, rep)
	th := theme.DefaultTheme()
	styler := newEditorDiagnosticStyler(newMetadataRuneStyler(th.EditorMetadata), th)
	styler.overlay = s.overlay
	styles := styler.StylesForLine([]rune(source), 0)
	if styles[2].GetForeground() != th.EditorDiagnosticError.GetForeground() ||
		styles[6].GetForeground() != th.EditorDiagnosticWarning.GetForeground() {
		t.Fatal("overlap precedence wrong")
	}
}

func TestDiagnosticEmptyAndEOFRangesUseLineNumbers(t *testing.T) {
	rep := diag.Report{Source: []byte("abc\n"), Items: []diag.Diagnostic{
		{
			Severity: diag.SeverityError,
			Span:     diag.Span{Start: diag.Pos{Line: 2, Col: 1}, End: diag.Pos{Line: 2, Col: 1}},
		},
		{Severity: diag.SeverityWarning, Span: diag.Span{Start: diag.Pos{Line: 9, Col: 1}}},
	}}
	s := newDiagnosticSnapshot(diagnosticKey{}, rep)
	styler := newEditorDiagnosticStyler(
		newMetadataRuneStyler(theme.DefaultTheme().EditorMetadata),
		theme.DefaultTheme(),
	)
	styler.overlay = s.overlay
	if _, ok := styler.LineNumberStyle(0); ok {
		t.Fatal("unaffected line marked")
	}
	if style, ok := styler.LineNumberStyle(1); !ok || style.GetForeground() != styler.failure.GetForeground() {
		t.Fatal("empty/EOF location not marked")
	}
}

func TestDiagnosticThemeChangeRetainsSnapshot(t *testing.T) {
	m := newDiagnosticModel(t, "# @nmae typo")
	snapshot := m.currentDiagnostics()
	ticket := m.diagnostics.ticket
	m.theme.EditorDiagnosticWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("#112233")).Underline(true)
	m.updateEditorStyler(m.currentFile)
	styler := m.editor.styler
	if styler.overlay != snapshot.overlay || m.diagnostics.ticket != ticket {
		t.Fatal("theme rebuild discarded/reparsed diagnostics")
	}
	if got := styler.StylesForLine(m.editor.LineRunes(0), 0)[2].GetForeground(); got != lipgloss.Color("#112233") {
		t.Fatalf("color=%v", got)
	}
}

func BenchmarkDiagnosticViewportStyles(b *testing.B) {
	th := theme.DefaultTheme()
	source := strings.Repeat("# @sse max-event=5\nGET http://x\n### r\n", 1000)
	base := newMetadataRuneStyler(th.EditorMetadata)
	styler := newEditorDiagnosticStyler(base, th)
	styler.SetSource(source)
	styler.overlay = newDiagnosticSnapshot(
		diagnosticKey{},
		parser.Diagnostics(parser.Parse("large.http", []byte(source))),
	).overlay
	line := []rune("# @sse max-event=5")
	b.ReportAllocs()
	for b.Loop() {
		for i := range 20 {
			styler.StylesForLine(line, i*3)
		}
	}
}
