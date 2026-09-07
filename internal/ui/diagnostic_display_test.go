package ui

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

type diagnosticTestSource struct {
	lines [][]rune
	read  []int
}

func newDiagnosticTestSource(source string) *diagnosticTestSource {
	s := &diagnosticTestSource{}
	for line := range strings.SplitSeq(source, "\n") {
		s.lines = append(s.lines, []rune(line))
	}
	return s
}

func (s *diagnosticTestSource) LineCount() int { return len(s.lines) }

func (s *diagnosticTestSource) LineRunes(line int) []rune {
	s.read = append(s.read, line)
	return s.lines[line]
}

func TestDiagnosticDisplayRetainsOnlyMatchingPrefixes(t *testing.T) {
	const source = "\u3000# @mock path=/x\r\nsecond"
	snapshot := newDiagnosticSnapshot(diagnosticKey{}, diag.Report{
		Source: []byte(source),
		Items: []diag.Diagnostic{{
			Severity: diag.SeverityError,
			Span: diag.Span{
				Start: diag.Pos{Line: 1, Col: 6},
				End:   diag.Pos{Line: 1, Col: 11},
			},
		}},
	})
	display := snapshot.display
	for _, tt := range []struct {
		name, source string
		keep         bool
	}{
		{"unchanged", source, true},
		{"argument edited", "\u3000# @mock path=/漢\nsecond", true},
		{"argument removed", "\u3000# @mock\nsecond", true},
		{"other line edited", "\u3000# @mock path=/x\nchanged", true},
		{"prefix edited", " # @mock path=/x\nsecond", false},
		{"token edited", "\u3000# @name path=/x\nsecond", false},
		{"shortened", "\u3000# @moc", false},
		{"line shifted", "first\n" + source, false},
		{"line removed", "second", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			afterEdit := display.afterEdit(newDiagnosticTestSource(tt.source))
			if got := len(afterEdit.lines[0]) == 1; got != tt.keep {
				t.Fatalf("decoration retained = %t, want %t", got, tt.keep)
			}
			if afterEdit.errors != 1 || strings.Join(afterEdit.sourceLines, "\n") != source ||
				string(snapshot.report.Source) != source {
				t.Fatal("editing changed the last completed result")
			}
			if tt.keep {
				afterEdit.lines[0][0].start = 99
			} else if restored := afterEdit.afterEdit(newDiagnosticTestSource(source)); len(restored.lines) != 0 {
				t.Fatal("dropped decorations reappeared without a fresh parse")
			}
			if len(display.lines[0]) != 1 || display.lines[0][0].start != 3 {
				t.Fatal("filtering mutated the original snapshot")
			}
		})
	}
}

func TestDiagnosticDisplayEmptyRangesRequireMatchingLine(t *testing.T) {
	display := newDiagnosticSnapshot(diagnosticKey{}, diag.Report{
		Source: []byte("abc\n\r\n"),
		Items: []diag.Diagnostic{{
			Severity: diag.SeverityError,
			Span: diag.Span{
				Start: diag.Pos{Line: 2, Col: 1},
				End:   diag.Pos{Line: 2, Col: 1},
			},
		}},
	}).display
	for _, tt := range []struct {
		name, source string
		keep         bool
	}{
		{"unchanged", "abc\n\n", true},
		{"changed", "abc\nx\n", false},
		{"removed", "abc", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			afterEdit := display.afterEdit(newDiagnosticTestSource(tt.source))
			if _, ok := afterEdit.marks[1]; ok != tt.keep {
				t.Fatalf("gutter mark retained = %t, want %t", ok, tt.keep)
			}
			if got := len(afterEdit.lines[1]) == 1; got != tt.keep {
				t.Fatalf("empty range retained = %t, want %t", got, tt.keep)
			}
			delete(afterEdit.marks, 1)
			if len(display.marks) != 1 || len(display.lines[1]) != 1 {
				t.Fatal("filtering mutated the original snapshot")
			}
		})
	}
}

func TestDiagnosticDisplayReadsOnlyMarkedLines(t *testing.T) {
	source := strings.Repeat("# @mock path=/x\n", 1000)
	display := newDiagnosticSnapshot(diagnosticKey{}, diag.Report{
		Source: []byte(source),
		Items: []diag.Diagnostic{{
			Severity: diag.SeverityError,
			Span:     diag.Span{Start: diag.Pos{Line: 999, Col: 3}, End: diag.Pos{Line: 999, Col: 8}},
		}},
	}).display
	edited := newDiagnosticTestSource(source + "x")
	before := string(edited.lines[998])
	retained := display.afterEdit(edited)
	if !slices.Equal(edited.read, []int{998}) || string(edited.lines[998]) != before {
		t.Fatal("filtering read unrelated lines or modified the editor")
	}
	edited.lines[998][3] = '!'
	if retained.sourceLines[998] != before || display.sourceLines[998] != before {
		t.Fatal("display retained mutable editor storage")
	}
	shortened := newDiagnosticTestSource("# @mock path=/x")
	retained = retained.afterEdit(shortened)
	if len(shortened.read) != 0 || len(retained.lines) != 0 {
		t.Fatal("filtering read a removed line or retained its decoration")
	}
	retained.afterEdit(edited)
	if len(edited.read) != 1 {
		t.Fatal("empty display inspected source lines")
	}
}

// Compare against the original string-based filter, including repeated edits.
func FuzzDiagnosticDisplayAfterEdit(f *testing.F) {
	for _, source := range []string{"", "# @mock path=/x", "\u3000# @mock\r\n\n", "a\xffb\nc"} {
		f.Add(source, source)
		f.Add(source, "prefix\n"+source)
	}
	f.Fuzz(func(t *testing.T, source, edited string) {
		if len(source)+len(edited) > 16384 {
			t.Skip()
		}
		report := diag.Report{Source: []byte(source)}
		for _, line := range []int{1, 2, 4} {
			for _, end := range []int{1, 3, 9} {
				report.Items = append(report.Items, diag.Diagnostic{
					Severity: diag.SeverityError,
					Span:     diag.Span{Start: diag.Pos{Line: line, Col: 1}, End: diag.Pos{Line: line, Col: end}},
				})
			}
		}
		got := newDiagnosticSnapshot(diagnosticKey{}, report).display
		want := got
		for _, text := range []string{edited, source} {
			got = got.afterEdit(newDiagnosticTestSource(text))
			want = diagnosticDisplayReference(want, text)
			if !maps.EqualFunc(got.lines, want.lines, slices.Equal) || !maps.Equal(got.marks, want.marks) ||
				got.errors != want.errors || got.warnings != want.warnings {
				t.Fatalf("display differs for source=%q edited=%q", source, text)
			}
		}
	})
}

func diagnosticDisplayReference(d *diagnosticDisplay, source string) *diagnosticDisplay {
	edited := strings.Split(source, "\n")
	next := &diagnosticDisplay{
		path: d.path, sourceLines: d.sourceLines, errors: d.errors, warnings: d.warnings,
		lines: make(map[int][]diagnosticRange), marks: make(map[int]diag.Severity),
	}
	for line, ranges := range d.lines {
		if line >= len(d.sourceLines) || line >= len(edited) {
			continue
		}
		before := []rune(strings.TrimSuffix(d.sourceLines[line], "\r"))
		after := []rune(strings.TrimSuffix(edited[line], "\r"))
		for _, r := range ranges {
			aligned := slices.Equal(before, after)
			if r.start != r.end {
				aligned = r.end <= len(before) && r.end <= len(after) && slices.Equal(before[:r.end], after[:r.end])
			}
			if aligned {
				next.lines[line] = append(next.lines[line], r)
			}
		}
		if severity, marked := d.marks[line]; marked && slices.Equal(before, after) {
			next.marks[line] = severity
		}
	}
	return next
}
