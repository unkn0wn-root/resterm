package diag_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

type lineSource struct {
	lines [][]rune
	read  []int
}

func newLineSource(source string) *lineSource {
	s := &lineSource{}
	for line := range strings.SplitSeq(source, "\n") {
		s.lines = append(s.lines, []rune(line))
	}
	return s
}

func (s *lineSource) LineCount() int { return len(s.lines) }

func (s *lineSource) LineRunes(line int) []rune {
	s.read = append(s.read, line)
	return s.lines[line]
}

func span(line, start, end int) diag.Span {
	return diag.Span{Start: diag.Pos{Line: line, Col: start}, End: diag.Pos{Line: line, Col: end}}
}

func errorAt(sp diag.Span) diag.Diagnostic {
	return diag.Diagnostic{Severity: diag.SeverityError, Span: sp}
}

func TestOverlayRetainsOnlyMatchingPrefixes(t *testing.T) {
	const source = "　# @mock path=/x\r\nsecond"
	overlay := diag.NewOverlay(diag.Report{
		Source: []byte(source),
		Items:  []diag.Diagnostic{errorAt(span(1, 6, 11))},
	})
	for _, tt := range []struct {
		name, source string
		keep         bool
	}{
		{"unchanged", source, true},
		{"argument edited", "　# @mock path=/漢\nsecond", true},
		{"argument removed", "　# @mock\nsecond", true},
		{"other line edited", "　# @mock path=/x\nchanged", true},
		{"prefix edited", " # @mock path=/x\nsecond", false},
		{"token edited", "　# @name path=/x\nsecond", false},
		{"shortened", "　# @moc", false},
		{"line shifted", "first\n" + source, false},
		{"line removed", "second", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			retained := overlay.Retain(newLineSource(tt.source))
			if got := len(retained.Ranges(0)) == 1; got != tt.keep {
				t.Fatalf("decoration retained = %t, want %t", got, tt.keep)
			}
			if errs, _ := retained.Counts(); errs != 1 {
				t.Fatal("editing changed the last completed result")
			}
			if tt.keep {
				retained.Ranges(0)[0].Start = 99
			} else if restored := retained.Retain(newLineSource(source)); len(restored.Ranges(0)) != 0 {
				t.Fatal("dropped decorations reappeared without a fresh parse")
			}
			if ranges := overlay.Ranges(0); len(ranges) != 1 || ranges[0].Start != 3 {
				t.Fatal("filtering mutated the original overlay")
			}
		})
	}
}

func TestOverlayEmptyRangesRequireMatchingLine(t *testing.T) {
	overlay := diag.NewOverlay(diag.Report{
		Source: []byte("abc\n\r\n"),
		Items:  []diag.Diagnostic{errorAt(span(2, 1, 1))},
	})
	for _, tt := range []struct {
		name, source string
		keep         bool
	}{
		{"unchanged", "abc\n\n", true},
		{"changed", "abc\nx\n", false},
		{"removed", "abc", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			retained := overlay.Retain(newLineSource(tt.source))
			if _, ok := retained.Mark(1); ok != tt.keep {
				t.Fatalf("gutter mark retained = %t, want %t", ok, tt.keep)
			}
			if got := len(retained.Ranges(1)) == 1; got != tt.keep {
				t.Fatalf("empty range retained = %t, want %t", got, tt.keep)
			}
			if _, ok := overlay.Mark(1); !ok || len(overlay.Ranges(1)) != 1 {
				t.Fatal("filtering mutated the original overlay")
			}
		})
	}
}

func TestOverlayRetainReadsOnlyMarkedLines(t *testing.T) {
	source := strings.Repeat("# @mock path=/x\n", 1000)
	overlay := diag.NewOverlay(diag.Report{
		Source: []byte(source),
		Items:  []diag.Diagnostic{errorAt(span(999, 3, 8))},
	})
	edited := newLineSource(source + "x")
	retained := overlay.Retain(edited)
	if !slices.Equal(edited.read, []int{998}) || len(retained.Ranges(998)) != 1 {
		t.Fatal("filtering read unrelated lines or dropped an unchanged range")
	}
	shortened := newLineSource("# @mock path=/x")
	retained = retained.Retain(shortened)
	if len(shortened.read) != 0 || len(retained.Ranges(998)) != 0 {
		t.Fatal("filtering read a removed line or retained its decoration")
	}
	retained.Retain(edited)
	if len(edited.read) != 1 {
		t.Fatal("empty overlay inspected source lines")
	}
}

func TestOverlayOrdersErrorsAndGroupsItems(t *testing.T) {
	rep := diag.Report{Source: []byte("abcdefgh"), Items: []diag.Diagnostic{
		{Severity: diag.SeverityWarning, Span: span(1, 1, 7), Labels: []diag.Label{{Span: span(1, 7, 9)}}},
		errorAt(span(1, 3, 5)),
	}}
	overlay := diag.NewOverlay(rep)
	ranges := overlay.Ranges(0)
	if len(ranges) != 3 || ranges[2].Severity != diag.SeverityError || ranges[2].Item != 1 {
		t.Fatalf("errors must paint last: %+v", ranges)
	}
	if items := overlay.At(diag.Cell{Col: 2}); !slices.Equal(items, []int{1, 0}) {
		t.Fatalf("items = %v", items)
	}
	if items := overlay.At(diag.Cell{Col: 7}); !slices.Equal(items, []int{0, 1}) {
		t.Fatalf("cursor priority = %v", items)
	}
}

func TestOverlayMarksEmptyAndEOFLocations(t *testing.T) {
	rep := diag.Report{Source: []byte("abc\n"), Items: []diag.Diagnostic{
		errorAt(span(2, 1, 1)),
		{Severity: diag.SeverityWarning, Span: diag.Span{Start: diag.Pos{Line: 9, Col: 1}}},
	}}
	overlay := diag.NewOverlay(rep)
	if _, ok := overlay.Mark(0); ok {
		t.Fatal("unaffected line marked")
	}
	if severity, ok := overlay.Mark(1); !ok || severity != diag.SeverityError {
		t.Fatal("empty and EOF locations must share the worst mark")
	}
	if len(overlay.At(diag.Cell{Line: 1})) != 2 {
		t.Fatal("empty-line findings inaccessible")
	}
	next, ok := overlay.Next(diag.Cell{}, false)
	if !ok || next != (diag.Cell{Line: 1}) {
		t.Fatalf("next = %+v", next)
	}
	if prev, _ := overlay.Next(next, true); prev != next {
		t.Fatal("both findings must share one navigation stop")
	}
}

func TestOverlayNavigationWraps(t *testing.T) {
	overlay := diag.NewOverlay(diag.Report{
		Source: []byte("ab\ncd\nef"),
		Items:  []diag.Diagnostic{errorAt(span(3, 2, 3)), errorAt(span(1, 1, 2))},
	})
	first, second := diag.Cell{}, diag.Cell{Line: 2, Col: 1}
	for _, tt := range []struct {
		from      diag.Cell
		backwards bool
		want      diag.Cell
	}{
		{first, false, second},
		{second, false, first},
		{first, true, second},
		{second, true, first},
		{diag.Cell{Line: 1}, false, second},
		{diag.Cell{Line: 1}, true, first},
	} {
		if got, ok := overlay.Next(tt.from, tt.backwards); !ok || got != tt.want {
			t.Fatalf("Next(%+v, %t) = %+v, want %+v", tt.from, tt.backwards, got, tt.want)
		}
	}
	if _, ok := overlay.Retain(newLineSource("ab\ncd\nef")).Next(first, false); ok {
		t.Fatal("a retained overlay must not navigate")
	}
}

// Compare against a whole-line reference filter, including repeated edits.
func FuzzOverlayRetain(f *testing.F) {
	for _, source := range []string{"", "# @mock path=/x", "\u3000# @mock\r\n\n", "a\xffb\nc"} {
		f.Add(source, source)
		f.Add(source, "prefix\n"+source)
	}
	f.Fuzz(func(t *testing.T, source, edited string) {
		if len(source)+len(edited) > 16384 {
			t.Skip()
		}
		rep := diag.Report{Source: []byte(source)}
		for _, line := range []int{1, 2, 4} {
			for _, end := range []int{1, 3, 9} {
				rep.Items = append(rep.Items, errorAt(span(line, 1, end)))
			}
		}
		got := diag.NewOverlay(rep)
		want := referenceOf(got)
		lines := strings.Split(source, "\n")
		for _, text := range []string{edited, source} {
			got = got.Retain(newLineSource(text))
			want = want.retain(lines, text)
			for line := range 5 {
				if !slices.Equal(got.Ranges(line), want.lines[line]) {
					t.Fatalf("line %d differs for source=%q edited=%q", line, source, text)
				}
				mark, ok := got.Mark(line)
				if wantMark, wantOK := want.marks[line]; ok != wantOK || mark != wantMark {
					t.Fatalf("mark %d differs for source=%q edited=%q", line, source, text)
				}
			}
		}
	})
}

type reference struct {
	lines map[int][]diag.Range
	marks map[int]diag.Severity
}

func referenceOf(o *diag.Overlay) reference {
	r := reference{lines: make(map[int][]diag.Range), marks: make(map[int]diag.Severity)}
	for line := range 5 {
		r.lines[line] = o.Ranges(line)
		if severity, ok := o.Mark(line); ok {
			r.marks[line] = severity
		}
	}
	return r
}

func (r reference) retain(source []string, edited string) reference {
	after := strings.Split(edited, "\n")
	next := reference{lines: make(map[int][]diag.Range), marks: make(map[int]diag.Severity)}
	for line, ranges := range r.lines {
		if line >= len(source) || line >= len(after) {
			continue
		}
		before := []rune(strings.TrimSuffix(source[line], "\r"))
		now := []rune(strings.TrimSuffix(after[line], "\r"))
		whole := slices.Equal(before, now)
		for _, rg := range ranges {
			aligned := whole
			if rg.Start != rg.End {
				aligned = rg.End <= len(before) && rg.End <= len(now) && slices.Equal(before[:rg.End], now[:rg.End])
			}
			if aligned {
				next.lines[line] = append(next.lines[line], rg)
			}
		}
		if severity, ok := r.marks[line]; ok && whole {
			next.marks[line] = severity
		}
	}
	return next
}

func BenchmarkOverlayRetain(b *testing.B) {
	for _, tt := range []struct {
		name           string
		lines, first   int
		stride, suffix int
	}{
		{"small", 50, 2, 0, 0},
		{"large clean", 10000, -1, 0, 0},
		{"large early", 10000, 2, 0, 0},
		{"large late", 10000, 9998, 0, 0},
		{"large sparse", 10000, 2, 1000, 0},
		{"large dense", 10000, 0, 1, 0},
		{"long unicode", 10, 2, 0, 32000},
	} {
		b.Run(tt.name, func(b *testing.B) {
			lines := make([]string, tt.lines)
			for i := range lines {
				lines[i] = "# ordinary comment in a request file"
			}
			var items []diag.Diagnostic
			for i := tt.first; i >= 0 && i < len(lines); i += tt.stride {
				lines[i] = "# @mock path=/x" + strings.Repeat("漢", tt.suffix)
				items = append(items, errorAt(span(i+1, 3, 8)))
				if tt.stride == 0 {
					break
				}
			}
			source := strings.Join(lines, "\n")
			overlay := diag.NewOverlay(diag.Report{Source: []byte(source), Items: items})
			edited := newLineSource(source + "x")
			b.ReportAllocs()
			for b.Loop() {
				overlay.Retain(edited)
			}
		})
	}
}
