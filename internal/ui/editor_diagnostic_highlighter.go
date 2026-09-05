package ui

import (
	"slices"

	"github.com/charmbracelet/lipgloss"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui/textarea"
)

type sourceStyler interface {
	SetSource(source string)
	sourceLine(line int) parser.SourceLine
}

// Decorated lines are copied to preserve the base styler's cache.
type editorDiagnosticStyler struct {
	base             textarea.RuneStyler
	snapshot         *diagnosticSnapshot
	warning, failure lipgloss.Style
}

func newEditorDiagnosticStyler(base textarea.RuneStyler, th theme.Theme) *editorDiagnosticStyler {
	return &editorDiagnosticStyler{
		base:    base,
		warning: th.EditorDiagnosticWarning,
		failure: th.EditorDiagnosticError,
	}
}

func (s *editorDiagnosticStyler) SetSource(source string) {
	s.snapshot = nil
	if base, ok := s.base.(sourceStyler); ok {
		base.SetSource(source)
	}
}

func (s *editorDiagnosticStyler) sourceLine(line int) parser.SourceLine {
	if base, ok := s.base.(sourceStyler); ok {
		return base.sourceLine(line)
	}
	return parser.SourceLine{Kind: parser.SourceLineLiteral}
}

func (s *editorDiagnosticStyler) StylesForLine(line []rune, index int) []lipgloss.Style {
	styles := s.base.StylesForLine(line, index)
	if s.snapshot == nil {
		return styles
	}
	copied := false
	for _, r := range s.snapshot.lines[index] {
		start, end := min(r.start, len(line)), min(r.end, len(line))
		if start >= end {
			continue
		}
		if !copied {
			if len(styles) == len(line) {
				styles = slices.Clone(styles)
			} else {
				styles = make([]lipgloss.Style, len(line))
			}
			copied = true
		}
		decoration := s.style(r.severity)
		for i := start; i < end; i++ {
			styles[i] = decoration.Inherit(styles[i])
		}
	}
	return styles
}

func (s *editorDiagnosticStyler) style(severity diag.Severity) lipgloss.Style {
	if severity == diag.SeverityError {
		return s.failure
	}
	return s.warning
}

func (s *editorDiagnosticStyler) LineNumberStyle(line int) (lipgloss.Style, bool) {
	if s.snapshot == nil {
		return lipgloss.Style{}, false
	}
	severity, ok := s.snapshot.marks[line]
	if !ok {
		return lipgloss.Style{}, false
	}
	return s.style(severity), true
}
