package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

type diagnosticRange struct {
	start, end int // zero-based rune columns, end exclusive
	item       int
	severity   diag.Severity
}

// diagnosticDisplay contains the render-only part of the last completed
// result. It can outlive that result while a newer parse is pending.
// Ranges paint errors last; marks records the worst severity for empty
// locations such as blank lines or EOF. Source lines are shared and immutable.
type diagnosticDisplay struct {
	path        string
	sourceLines []string
	lines       map[int][]diagnosticRange
	marks       map[int]diag.Severity
	errors      int
	warnings    int
}

// diagnosticSource exposes physical buffer lines without serializing the editor.
// LineRunes returns read-only slices; filtering neither retains nor modifies them.
type diagnosticSource interface {
	LineCount() int
	LineRunes(int) []rune
}

func (d *diagnosticDisplay) addSpan(
	span diag.Span,
	item int,
	severity diag.Severity,
) cursorPosition {
	lines := d.sourceLines
	first := clamp(span.Start.Line-1, 0, len(lines)-1)
	last := clamp(max(span.Start.Line, span.End.Line)-1, first, len(lines)-1)
	pos := cursorPosition{Line: first}
	for line := first; line <= last; line++ {
		raw := strings.TrimSuffix(lines[line], "\r")
		start, end := 0, len(raw)
		if line == first {
			start = clamp(span.Start.Col-1, 0, len(raw))
		}
		if line == last {
			end = clamp(span.End.Col-1, start, len(raw))
		}
		// A location beyond EOF is represented by the last line number.
		if span.Start.Line > len(lines) {
			start, end = len(raw), len(raw)
		}
		if line > first && start == end {
			continue
		}
		from, to := utf8.RuneCountInString(raw[:start]), utf8.RuneCountInString(raw[:end])
		if line == first {
			pos.Column = from
		}
		if from == to {
			if prev, ok := d.marks[line]; !ok || severityRank(severity) < severityRank(prev) {
				d.marks[line] = severity
			}
		}
		d.lines[line] = append(d.lines[line], diagnosticRange{start: from, end: to, item: item, severity: severity})
	}
	return pos
}

// afterEdit keeps the completed result's counts and drops decorations whose
// coordinates may have changed. A fresh parse is required to restore them.
func (d *diagnosticDisplay) afterEdit(source diagnosticSource) *diagnosticDisplay {
	if len(d.lines) == 0 {
		return d
	}

	next := &diagnosticDisplay{
		path:        d.path,
		sourceLines: d.sourceLines,
		lines:       make(map[int][]diagnosticRange),
		marks:       make(map[int]diag.Severity),
		errors:      d.errors,
		warnings:    d.warnings,
	}

	lineCount := source.LineCount()
	for line, ranges := range d.lines {
		if line >= len(d.sourceLines) || line >= lineCount {
			continue
		}
		oldLine := strings.TrimSuffix(d.sourceLines[line], "\r")
		newLine := source.LineRunes(line)
		if len(newLine) > 0 && newLine[len(newLine)-1] == '\r' {
			newLine = newLine[:len(newLine)-1]
		}
		for _, r := range ranges {
			if !r.stillAligned(oldLine, newLine) {
				continue
			}
			next.lines[line] = append(next.lines[line], r)
			if r.start == r.end {
				if severity, marked := d.marks[line]; marked {
					next.marks[line] = severity
				}
			}
		}
	}
	return next
}

// A range remains at the same coordinates only when the source through its
// end is unchanged. Empty ranges have no textual anchor and require the whole
// line to match.
func (r diagnosticRange) stillAligned(before string, after []rune) bool {
	wholeLine := r.start == r.end
	if !wholeLine {
		if r.end > len(after) {
			return false
		}
		after = after[:r.end]
	}
	for _, actual := range after {
		expected, size := utf8.DecodeRuneInString(before)
		if size == 0 || actual != expected {
			return false
		}
		before = before[size:]
	}
	return !wholeLine || before == ""
}
