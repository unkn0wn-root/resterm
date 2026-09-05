package ui

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

type diagnosticKey struct {
	revision uint64
	path     string
}

type diagnosticRange struct {
	start, end int // zero-based rune columns, end exclusive
	item       int
	severity   diag.Severity
}

// Snapshots are immutable after delivery. Ranges paint errors last; marks
// records the worst severity for empty locations such as blank lines or EOF.
type diagnosticSnapshot struct {
	key       diagnosticKey
	report    diag.Report
	lines     map[int][]diagnosticRange
	marks     map[int]diag.Severity
	positions []cursorPosition
	errors    int
	warnings  int
}

func newDiagnosticSnapshot(key diagnosticKey, rep diag.Report) *diagnosticSnapshot {
	s := &diagnosticSnapshot{
		key:    key,
		report: rep,
		lines:  make(map[int][]diagnosticRange),
		marks:  make(map[int]diag.Severity),
	}
	lines := strings.Split(string(rep.Source), "\n")
	for i, item := range rep.Items {
		if item.Severity == diag.SeverityError {
			s.errors++
		} else {
			s.warnings++
		}
		s.positions = append(s.positions, s.addSpan(lines, item.Span, i, item.Severity))
		for _, label := range item.Labels {
			s.addSpan(lines, label.Span, i, item.Severity)
		}
	}
	for _, ranges := range s.lines {
		slices.SortStableFunc(ranges, func(a, b diagnosticRange) int {
			return cmp.Compare(severityRank(b.severity), severityRank(a.severity))
		})
	}
	slices.SortFunc(s.positions, compareDiagnosticPositions)
	s.positions = slices.CompactFunc(s.positions, func(a, b cursorPosition) bool {
		return compareDiagnosticPositions(a, b) == 0
	})
	return s
}

func severityRank(severity diag.Severity) int {
	if severity == diag.SeverityError {
		return 0
	}
	return 1
}

func compareDiagnosticPositions(a, b cursorPosition) int {
	return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column))
}

func (s *diagnosticSnapshot) addSpan(
	lines []string,
	span diag.Span,
	item int,
	severity diag.Severity,
) cursorPosition {
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
			if prev, ok := s.marks[line]; !ok || severityRank(severity) < severityRank(prev) {
				s.marks[line] = severity
			}
		}
		s.lines[line] = append(s.lines[line], diagnosticRange{start: from, end: to, item: item, severity: severity})
	}
	return pos
}

// at lists the items on the caret's line. Items under the caret come first,
// then errors before warnings.
func (s *diagnosticSnapshot) at(pos cursorPosition) []int {
	if s == nil {
		return nil
	}
	var items []int
	touching := make(map[int]bool)
	for _, r := range s.lines[pos.Line] {
		if !slices.Contains(items, r.item) {
			items = append(items, r.item)
		}
		if r.start <= pos.Column && pos.Column < r.end {
			touching[r.item] = true
		}
	}
	slices.SortStableFunc(items, func(a, b int) int {
		if touching[a] != touching[b] {
			if touching[a] {
				return -1
			}
			return 1
		}
		return cmp.Compare(severityRank(s.report.Items[a].Severity), severityRank(s.report.Items[b].Severity))
	})
	return items
}

func (s *diagnosticSnapshot) next(pos cursorPosition, backwards bool) (cursorPosition, bool) {
	if s == nil || len(s.positions) == 0 {
		return cursorPosition{}, false
	}
	if backwards {
		for _, candidate := range slices.Backward(s.positions) {
			if compareDiagnosticPositions(candidate, pos) < 0 {
				return candidate, true
			}
		}
		return s.positions[len(s.positions)-1], true
	}
	for _, candidate := range s.positions {
		if compareDiagnosticPositions(candidate, pos) > 0 {
			return candidate, true
		}
	}
	return s.positions[0], true
}
