package ui

import (
	"cmp"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

type diagnosticKey struct {
	revision uint64
	path     string
}

// Snapshots are immutable after delivery and are the only diagnostic data used
// for actions.
type diagnosticSnapshot struct {
	key       diagnosticKey
	report    diag.Report
	display   *diagnosticDisplay
	positions []cursorPosition
}

func newDiagnosticSnapshot(key diagnosticKey, rep diag.Report) *diagnosticSnapshot {
	s := &diagnosticSnapshot{
		key:    key,
		report: rep,
		display: &diagnosticDisplay{
			path:        key.path,
			sourceLines: strings.Split(string(rep.Source), "\n"),
			lines:       make(map[int][]diagnosticRange),
			marks:       make(map[int]diag.Severity),
		},
	}

	for i, item := range rep.Items {
		if item.Severity == diag.SeverityError {
			s.display.errors++
		} else {
			s.display.warnings++
		}
		s.positions = append(s.positions, s.display.addSpan(item.Span, i, item.Severity))
		for _, label := range item.Labels {
			s.display.addSpan(label.Span, i, item.Severity)
		}
	}
	for _, ranges := range s.display.lines {
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

// at lists the items on the caret's line. Items under the caret come first,
// then errors before warnings.
func (s *diagnosticSnapshot) at(pos cursorPosition) []int {
	if s == nil {
		return nil
	}
	var items []int
	touching := make(map[int]bool)
	for _, r := range s.display.lines[pos.Line] {
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
