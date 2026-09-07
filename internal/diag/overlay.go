package diag

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Cell is a zero-based line and rune column in an editor buffer, unlike Pos.
type Cell struct {
	Line, Col int
}

// LineSource reads editor lines without copying the buffer.
type LineSource interface {
	LineCount() int
	LineRunes(line int) []rune
}

// Range is a decorated run of runes on one line, end exclusive. Item indexes
// the report the overlay was built from.
type Range struct {
	Start, End int
	Item       int
	Severity   Severity
}

// Overlay maps a report onto editor lines for rendering and navigation.
type Overlay struct {
	source    []string
	lines     map[int][]Range
	marks     map[int]Severity
	errors    int
	warnings  int
	positions []Cell
}

func NewOverlay(rep Report) *Overlay {
	o := &Overlay{
		source: strings.Split(string(rep.Source), "\n"),
		lines:  make(map[int][]Range),
		marks:  make(map[int]Severity),
	}
	for i, item := range rep.Items {
		if item.Severity == SeverityError {
			o.errors++
		} else {
			o.warnings++
		}
		o.positions = append(o.positions, o.add(item.Span, i, item.Severity))
		for _, label := range item.Labels {
			o.add(label.Span, i, item.Severity)
		}
	}
	for _, ranges := range o.lines {
		slices.SortStableFunc(ranges, func(a, b Range) int {
			return cmp.Compare(rank(b.Severity), rank(a.Severity))
		})
	}
	slices.SortFunc(o.positions, compareCells)
	o.positions = slices.Compact(o.positions)
	return o
}

// A span past the last line marks that line. A span that continues onto later
// lines is drawn to the end of its first line only.
func (o *Overlay) add(span Span, item int, severity Severity) Cell {
	line := min(max(span.Start.Line-1, 0), len(o.source)-1)
	raw := strings.TrimSuffix(o.source[line], "\r")
	start, end := len(raw), len(raw)
	if span.Start.Line <= len(o.source) {
		start = min(max(span.Start.Col-1, 0), len(raw))
		if span.End.Line <= span.Start.Line {
			end = min(max(span.End.Col-1, start), len(raw))
		}
	}
	from, to := utf8.RuneCountInString(raw[:start]), utf8.RuneCountInString(raw[:end])
	if from == to {
		if prev, ok := o.marks[line]; !ok || rank(severity) < rank(prev) {
			o.marks[line] = severity
		}
	}
	o.lines[line] = append(o.lines[line], Range{Start: from, End: to, Item: item, Severity: severity})
	return Cell{Line: line, Col: from}
}

// Ranges lists a line's decorations in paint order, so errors come last and win overlaps.
func (o *Overlay) Ranges(line int) []Range {
	return o.lines[line]
}

// Mark reports the worst severity at a location with no text to decorate,
// such as a blank line or the end of the file.
func (o *Overlay) Mark(line int) (Severity, bool) {
	severity, ok := o.marks[line]
	return severity, ok
}

func (o *Overlay) Counts() (errors, warnings int) {
	return o.errors, o.warnings
}

// At lists the items on a line. Items under the cell come first, then errors
// before warnings.
func (o *Overlay) At(at Cell) []int {
	var items []int
	under := make(map[int]bool)
	severity := make(map[int]Severity)
	for _, r := range o.lines[at.Line] {
		if _, seen := severity[r.Item]; !seen {
			items = append(items, r.Item)
			severity[r.Item] = r.Severity
		}
		if r.Start <= at.Col && at.Col < r.End {
			under[r.Item] = true
		}
	}
	slices.SortStableFunc(items, func(a, b int) int {
		if under[a] != under[b] {
			if under[a] {
				return -1
			}
			return 1
		}
		return cmp.Compare(rank(severity[a]), rank(severity[b]))
	})
	return items
}

// Next returns the closest item after the cell, or before it when backwards,
// wrapping around the buffer.
func (o *Overlay) Next(at Cell, backwards bool) (Cell, bool) {
	if len(o.positions) == 0 {
		return Cell{}, false
	}
	if backwards {
		for _, c := range slices.Backward(o.positions) {
			if compareCells(c, at) < 0 {
				return c, true
			}
		}
		return o.positions[len(o.positions)-1], true
	}
	for _, c := range o.positions {
		if compareCells(c, at) > 0 {
			return c, true
		}
	}
	return o.positions[0], true
}

// Retain keeps the ranges whose text is unchanged in src, and the counts of the
// result they came from. Navigation is dropped because positions may have moved.
func (o *Overlay) Retain(src LineSource) *Overlay {
	if len(o.lines) == 0 {
		return o
	}
	next := &Overlay{
		source:   o.source,
		lines:    make(map[int][]Range),
		marks:    make(map[int]Severity),
		errors:   o.errors,
		warnings: o.warnings,
	}
	count := src.LineCount()
	for line, ranges := range o.lines {
		if line >= len(o.source) || line >= count {
			continue
		}
		edited := src.LineRunes(line)
		if n := len(edited); n > 0 && edited[n-1] == '\r' {
			edited = edited[:n-1]
		}
		same, whole := commonPrefix(strings.TrimSuffix(o.source[line], "\r"), edited)
		for _, r := range ranges {
			// Empty ranges have no text to anchor them and need the whole line.
			if r.Start == r.End && !whole || r.End > same {
				continue
			}
			next.lines[line] = append(next.lines[line], r)
		}
		if severity, ok := o.marks[line]; ok && whole {
			next.marks[line] = severity
		}
	}
	return next
}

// Number of leading runes both lines share, and whether they are identical.
func commonPrefix(before string, after []rune) (int, bool) {
	n := 0
	for _, r := range after {
		expected, size := utf8.DecodeRuneInString(before)
		if size == 0 || r != expected {
			return n, false
		}
		before = before[size:]
		n++
	}
	return n, before == ""
}

func rank(severity Severity) int {
	if severity == SeverityError {
		return 0
	}
	return 1
}

func compareCells(a, b Cell) int {
	return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Col, b.Col))
}
