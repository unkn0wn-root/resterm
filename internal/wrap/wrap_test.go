package wrap

import (
	"context"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

const (
	rd = "\x1b[31m"
	gr = "\x1b[32m"
	rs = "\x1b[0m"

	// Multi-rune clusters written as escapes so the joiners are visible in
	// source. Each renders as one glyph and must never be split.
	family = "\U0001F468\u200d\U0001F469\u200d\U0001F467\u200d\U0001F466" // ZWJ sequence, 2 cells
	heart  = "\u2764\ufe0f"                                               // emoji presentation selector
	thumb  = "\U0001F44D\U0001F3FD"                                       // skin tone modifier
	flagPL = "\U0001F1F5\U0001F1F1"                                       // regional indicator pair
	acute  = "e\u0301"                                                    // decomposed e-acute, 1 cell
	zwsp   = "\u200b"                                                     // zero-width space
)

// rows splits a result into rows the way the UI does, on newlines.
func rows(s string) []string {
	return strings.Split(s, "\n")
}

// stripANSI drops CSI sequences. It is a separate implementation from the one
// under test so the assertions cannot inherit its bugs.
func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < '@' || s[j] > '~') {
				j++
			}
			i = j + 1
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r':
			return -1
		}
		return r
	}, s)
}

func widestCluster(s string) int {
	widest := 0
	g := uniseg.NewGraphemes(stripANSI(s))
	for g.Next() {
		widest = max(widest, runewidth.StringWidth(g.Str()))
	}
	return widest
}

func TestMappingCoversEveryRow(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"leading blank", "\nabc"},
		{"trailing blank", "abc\n"},
		{"only blanks", "\n\n\n"},
		{"interior blanks", "a\n\n\nb"},
		{"empty", ""},
		{"single", "abc"},
		{"wrapping", strings.Repeat("x", 25) + "\n\n" + strings.Repeat("y", 13)},
		{"blank then wrap", "\n" + strings.Repeat("z", 30)},
		{"ansi blank", "\n" + rd + "abc" + rs},
	}

	for _, tc := range cases {
		for _, m := range []Mode{Plain, Structured, Pre} {
			res, ok := Wrap(context.Background(), tc.in, 10, m, true)
			if !ok {
				t.Fatalf("%s/%d: wrap failed", tc.name, m)
			}
			n := len(rows(res.S))
			if n != len(res.Rv) {
				t.Fatalf("%s/%d: %d rows but Rv has %d entries: %q %v",
					tc.name, m, n, len(res.Rv), res.S, res.Rv)
			}
			if got, want := len(res.Sp), strings.Count(tc.in, "\n")+1; got != want {
				t.Fatalf("%s/%d: %d spans for %d logical lines", tc.name, m, got, want)
			}
			next := 0
			for i, sp := range res.Sp {
				if sp.S != next || sp.E < sp.S {
					t.Fatalf("%s/%d: span %d = %+v, expected to start at %d", tc.name, m, i, sp, next)
				}
				for r := sp.S; r <= sp.E; r++ {
					if res.Rv[r] != i {
						t.Fatalf("%s/%d: row %d maps to line %d, want %d", tc.name, m, r, res.Rv[r], i)
					}
				}
				next = sp.E + 1
			}
			if next != len(res.Rv) {
				t.Fatalf("%s/%d: spans cover %d rows, Rv has %d", tc.name, m, next, len(res.Rv))
			}
		}
	}
}

func TestMapNoWrapMatchesInputLines(t *testing.T) {
	for _, in := range []string{"", "a", "a\nb", "\n", "\n\na\n"} {
		res, ok := Wrap(context.Background(), in, 0, Plain, true)
		if !ok {
			t.Fatalf("%q: wrap failed", in)
		}
		if res.S != in {
			t.Fatalf("%q: content changed to %q", in, res.S)
		}
		n := len(rows(res.S))
		if len(res.Rv) != n || len(res.Sp) != n {
			t.Fatalf("%q: %d rows, Rv=%d Sp=%d", in, n, len(res.Rv), len(res.Sp))
		}
		for i, sp := range res.Sp {
			if sp.S != i || sp.E != i || res.Rv[i] != i {
				t.Fatalf("%q: row %d maps to %+v/%d", in, i, sp, res.Rv[i])
			}
		}
	}
}

func TestGraphemeClustersStayWhole(t *testing.T) {
	cases := []struct {
		name string
		in   string
		w    int
	}{
		{"combining accent", acute + "x", 2},
		{"family emoji", family + "x", 3},
		{"variation selector", heart + "x", 2},
		{"flag", flagPL, 1},
		{"skin tone", thumb + "x", 3},
	}

	for _, tc := range cases {
		segs, ok := Line(context.Background(), tc.in, tc.w, Plain)
		if !ok {
			t.Fatalf("%s: wrap failed", tc.name)
		}
		if len(segs) != 1 {
			t.Errorf("%s: width %d fits %d cells but split into %d rows: %q",
				tc.name, runewidth.StringWidth(tc.in), tc.w, len(segs), segs)
		}
		if segs[0] != tc.in {
			t.Errorf("%s: content changed to %q", tc.name, segs[0])
		}
	}
}

func TestGraphemeSplitLandsOnClusterBoundary(t *testing.T) {
	in := strings.Repeat(acute, 6)
	segs, ok := Line(context.Background(), in, 4, Plain)
	if !ok {
		t.Fatalf("wrap failed")
	}
	for _, s := range segs {
		for _, r := range s {
			if r == 0x0301 {
				break
			}
			if r != 'e' {
				t.Fatalf("unexpected rune %U in %q", r, s)
			}
		}
		if strings.HasPrefix(s, "́") {
			t.Fatalf("row %q starts with an orphaned combining mark, segs=%q", s, segs)
		}
	}
	if got := strings.Join(segs, ""); got != in {
		t.Fatalf("content changed: %q", got)
	}
}

// The segmenter's boundary state describes the slice it last returned, so
// stepping over an escape without dropping that state judges the next cluster by
// a stale property and tears it apart.
func TestANSIBetweenUnicodeRunsKeepsClustersWhole(t *testing.T) {
	cases := []struct {
		name string
		in   string
		w    int
	}{
		{"before family", acute + rd + family + "x", 4},
		{"inside run", family + rd + family, 4},
		{"between accents", acute + rd + acute, 2},
		{"before flag", "a" + rd + flagPL, 2},
		{"before skin tone", "a" + rd + thumb, 3},
		{"reset between", family + rs + rd + family, 4},
		{"ascii then cluster", "ab" + rd + family, 4},
	}

	for _, tc := range cases {
		segs, ok := Line(context.Background(), tc.in, tc.w, Plain)
		if !ok {
			t.Fatalf("%s: wrap failed", tc.name)
		}
		if w := runewidth.StringWidth(stripANSI(tc.in)); w > tc.w {
			t.Fatalf("%s: test input is %d cells, wider than the %d it is given", tc.name, w, tc.w)
		}
		if len(segs) != 1 {
			t.Errorf("%s: visible content fits %d cells but wrapped into %d rows: %q",
				tc.name, tc.w, len(segs), segs)
		}
		if segs[0] != tc.in {
			t.Errorf("%s: content changed to %q", tc.name, segs[0])
		}
	}
}

func TestZeroWidthRunesCostNoCells(t *testing.T) {
	segs, ok := Line(context.Background(), "ab"+zwsp+"cd", 4, Plain)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if len(segs) != 1 {
		t.Fatalf("zero-width space should not consume a cell, got %q", segs)
	}
}

func TestWideRuneNeverOverflowsRow(t *testing.T) {
	cases := []struct {
		in string
		w  int
	}{
		{"a 界界", 3},
		{"ab 界", 3},
		{"界界界", 4},
		{"a界b界c", 3},
		{strings.Repeat("界", 8), 5},
	}

	for _, tc := range cases {
		segs, ok := Line(context.Background(), tc.in, tc.w, Plain)
		if !ok {
			t.Fatalf("%q: wrap failed", tc.in)
		}
		for _, s := range segs {
			if w := runewidth.StringWidth(s); w > tc.w {
				t.Fatalf("%q at width %d: row %q is %d cells, segs=%q", tc.in, tc.w, s, w, segs)
			}
		}
		if got := stripSpace(strings.Join(segs, "")); got != stripSpace(tc.in) {
			t.Fatalf("%q: content changed to %q", tc.in, got)
		}
	}
}

// A cluster wider than the row itself has nowhere to wrap to, so it is placed
// anyway. It must still land on a row of its own.
func TestClusterWiderThanRowIsPlacedAlone(t *testing.T) {
	segs, ok := Line(context.Background(), "a界b", 1, Plain)
	if !ok {
		t.Fatalf("wrap failed")
	}
	want := []string{"a", "界", "b"}
	if len(segs) != len(want) {
		t.Fatalf("got %q, want %q", segs, want)
	}
	for i := range want {
		if segs[i] != want[i] {
			t.Fatalf("got %q, want %q", segs, want)
		}
	}
}

// A prefix narrows every row, so a grapheme can fit the row yet not the indented
// area left over. The indent is dropped for that row rather than pushed past the
// limit, because the terminal re-wraps an over-wide row and that desyncs the row
// mapping the UI navigates by.
func TestPrefixNeverPushesWideGraphemesPastWidth(t *testing.T) {
	cases := []struct {
		name string
		in   string
		w    int
		m    Mode
	}{
		{"structured wide", "界界", 3, Structured},
		{"pre wide", "  界界", 3, Pre},
		{"structured wider than row", "界界", 2, Structured},
		{"pre unicode space run", "  \u2003\u2003abc", 3, Pre},
		{"structured family", family + family, 3, Structured},
		{"pre family", "  " + family + family, 4, Pre},
		{"structured tight", "界", 1, Structured},
	}

	for _, tc := range cases {
		segs, ok := Line(context.Background(), tc.in, tc.w, tc.m)
		if !ok {
			t.Fatalf("%s: wrap failed", tc.name)
		}
		widest := widestCluster(tc.in)
		for _, s := range segs {
			w := runewidth.StringWidth(stripANSI(s))
			// An over-wide cluster may sit alone on a row, but nothing may be
			// indented beside it.
			if w > tc.w && w > widest {
				t.Errorf("%s: row %q is %d cells, limit %d (widest cluster %d): %q",
					tc.name, s, w, tc.w, widest, segs)
			}
		}
		if got, want := stripSpace(strings.Join(segs, "")), stripSpace(tc.in); got != want {
			t.Errorf("%s: content changed to %q, want %q", tc.name, got, want)
		}
	}
}

func TestPrefixModesKeepRowsWithinWidth(t *testing.T) {
	cases := []struct {
		name string
		in   string
		w    int
		m    Mode
	}{
		{"structured json", `  "key": "` + strings.Repeat("v", 60) + `"`, 20, Structured},
		{"structured wide", `  "k": "` + strings.Repeat("界", 20) + `"`, 16, Structured},
		{"pre indent", "    " + strings.Repeat("x", 40), 12, Pre},
		{"pre wide", "  " + strings.Repeat("界", 20), 11, Pre},
		{"pre tabs", "\t\tdeep " + strings.Repeat("y", 30), 12, Pre},
	}

	for _, tc := range cases {
		res, ok := Wrap(context.Background(), tc.in, tc.w, tc.m, false)
		if !ok {
			t.Fatalf("%s: wrap failed", tc.name)
		}
		for i, r := range rows(res.S) {
			if w := runewidth.StringWidth(stripANSI(r)); w > tc.w {
				t.Fatalf("%s: row %d %q is %d cells, limit %d", tc.name, i, r, w, tc.w)
			}
		}
	}
}

func TestWrapPreDoesNotEmitIndentOnlyLine(t *testing.T) {
	res, ok := Wrap(context.Background(), "    abc", 6, Pre, false)
	if !ok {
		t.Fatalf("wrap failed")
	}
	lines := rows(res.S)
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %d line(s): %q", len(lines), res.S)
	}
	if strings.TrimSpace(lines[0]) == "" {
		t.Fatalf("expected first line to include content, got %q", lines[0])
	}
}

func TestTrailingResetSurvivesWrap(t *testing.T) {
	cases := []struct {
		name string
		in   string
		w    int
	}{
		{"reset after dropped space", rd + "foo " + rs, 3},
		{"reset after wrapped token", rd + "foobar" + rs, 3},
		{"reset alone at end", rd + "foo bar " + rs, 4},
		{"reset after wide rune", rd + "界界 " + rs, 2},
	}

	for _, tc := range cases {
		res, ok := Wrap(context.Background(), tc.in, tc.w, Plain, false)
		if !ok {
			t.Fatalf("%s: wrap failed", tc.name)
		}
		if !strings.Contains(res.S, rs) {
			t.Fatalf("%s: reset dropped, styling leaks past the block: %q", tc.name, res.S)
		}
		last := rows(res.S)
		if !strings.HasSuffix(last[len(last)-1], rs) {
			t.Fatalf("%s: reset is not on the final row: %q", tc.name, res.S)
		}
	}
}

func TestANSISequencesArePreservedInOrder(t *testing.T) {
	in := rd + "alpha " + gr + "beta" + rs + " gamma"
	res, ok := Wrap(context.Background(), in, 6, Plain, false)
	if !ok {
		t.Fatalf("wrap failed")
	}
	want := []string{rd, gr, rs}
	pos := 0
	for _, seq := range want {
		i := strings.Index(res.S[pos:], seq)
		if i < 0 {
			t.Fatalf("sequence %q missing or out of order in %q", seq, res.S)
		}
		pos += i + len(seq)
	}
	if got := stripSpace(stripANSI(res.S)); got != stripSpace(stripANSI(in)) {
		t.Fatalf("visible content changed: %q", got)
	}
}

func TestWrapCarriesResetColorSequence(t *testing.T) {
	line := "\x1b[0;31m" + strings.Repeat("X", 12) + rs
	res, ok := Wrap(context.Background(), line, 5, Plain, false)
	if !ok {
		t.Fatalf("wrap failed")
	}
	lines := rows(res.S)
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %d line(s): %q", len(lines), res.S)
	}
	if !strings.HasPrefix(lines[1], "\x1b[0;31m") {
		t.Fatalf("expected continuation to keep 0;31m prefix, got %q", lines[1])
	}
}

func TestWrapExtendedColorDoesNotClearOtherStyles(t *testing.T) {
	line := "\x1b[1m\x1b[38;5;0m" + strings.Repeat("Y", 12)
	res, ok := Wrap(context.Background(), line, 5, Plain, false)
	if !ok {
		t.Fatalf("wrap failed")
	}
	lines := rows(res.S)
	if len(lines) < 2 {
		t.Fatalf("expected wrapping, got %d line(s): %q", len(lines), res.S)
	}
	wantPrefix := "\x1b[1m\x1b[38;5;0m"
	if !strings.HasPrefix(lines[1], wantPrefix) {
		t.Fatalf("expected continuation to keep %q, got %q", wantPrefix, lines[1])
	}
}

// Structured mode hangs an indent under the original one, and that indent
// carries its own ANSI. These four pin how prefix styling and token styling stay
// separated on a continuation row.
func TestWrapStructuredContinuationAvoidsPrefixColorLeak(t *testing.T) {
	ln := rd + "    " + rs + gr + strings.Repeat("A", 36) + rs

	sg, ok := Line(context.Background(), ln, 16, Structured)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if len(sg) < 2 {
		t.Fatalf("expected wrapped continuation, got %d segment(s)", len(sg))
	}

	c := sg[1]
	if !strings.HasPrefix(c, rd) {
		t.Fatalf("expected continuation to keep prefix ANSI, got %q", c)
	}
	_, after, found := strings.Cut(c, gr)
	if !found {
		t.Fatalf("expected continuation to include token color, got %q", c)
	}
	if strings.Contains(after, rd) {
		t.Fatalf("expected no leaked prefix color after token color, got %q", c)
	}
}

func TestWrapStructuredContinuationKeepsTokenColorAfterPrefixReset(t *testing.T) {
	ln := rd + rs + "    " + gr + strings.Repeat("B", 36) + rs

	sg, ok := Line(context.Background(), ln, 16, Structured)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if len(sg) < 2 {
		t.Fatalf("expected wrapped continuation, got %d segment(s)", len(sg))
	}

	c := sg[1]
	if !strings.Contains(c, gr) {
		t.Fatalf("expected continuation to restore token color after prefix reset, got %q", c)
	}
	_, after, _ := strings.Cut(c, gr)
	if strings.Contains(after, rd) {
		t.Fatalf("expected no prefix color after token color, got %q", c)
	}
}

func TestWrapStructuredContinuationKeepsResetFromPrefix(t *testing.T) {
	cl := "\x1b[38;2;230;219;116m"
	ln := "  " + rs + cl + "\"Root=1-6998c40f-78b385122e209bd52563 3827\""

	sg, ok := Line(context.Background(), ln, 40, Structured)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if len(sg) < 2 {
		t.Fatalf("expected wrapped continuation, got %d segment(s)", len(sg))
	}

	c := sg[1]
	if !strings.Contains(c, rs) {
		t.Fatalf("expected continuation to retain reset from prefix, got %q", c)
	}
	i := strings.Index(c, rs)
	j := strings.Index(c, cl)
	if i == -1 || j == -1 || i > j {
		t.Fatalf("expected reset before color in continuation, got %q", c)
	}
}

func TestWrapStructuredContinuationNoSyntheticResetWithoutPrefixReset(t *testing.T) {
	ln := "  " + gr + "\"" + strings.Repeat("x", 32) + "\""

	sg, ok := Line(context.Background(), ln, 20, Structured)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if len(sg) < 2 {
		t.Fatalf("expected wrapped continuation, got %d segment(s)", len(sg))
	}

	c := sg[1]
	if strings.Contains(c, rs) {
		t.Fatalf("expected no synthetic reset on continuation, got %q", c)
	}
}

func TestCancellationStopsWrapping(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	big := strings.Repeat("token ", 4000)
	if _, ok := Wrap(ctx, big, 20, Plain, true); ok {
		t.Fatal("Wrap ignored a cancelled context")
	}
	if _, ok := Wrap(ctx, big, 0, Plain, true); ok {
		t.Fatal("Wrap ignored cancellation on the unwrapped path")
	}
	if _, ok := Line(ctx, big, 20, Structured); ok {
		t.Fatal("Line ignored a cancelled context")
	}
}

// lateCancel reports cancellation only after Err has been polled a set number of
// times. Polling is how the wrapper observes a context, so this drops the
// cancellation at a chosen point inside the run instead of at an entry check.
type lateCancel struct {
	context.Context
	left  int
	polls int
}

func (c *lateCancel) Err() error {
	c.polls++
	if c.left > 0 {
		c.left--
		return nil
	}
	return context.Canceled
}

// Wrap and wrapLine each poll once on entry, so anything beyond that is the guard.
const entryPolls = 2

func TestCancellationIsNoticedMidWrap(t *testing.T) {
	cases := []struct {
		name string
		in   string
		w    int
	}{
		// Many tokens, so the guard fires from the ASCII tokenizer loop.
		{"ascii tokens", strings.Repeat("token ", 4000), 20},
		// One huge token, so it can only fire from the splitter loop in addTok.
		{"single long token", strings.Repeat("x", 200000), 40},
		// Non-ASCII, so it fires from the grapheme tokenizer loop.
		{"unicode", strings.Repeat("界", 200000), 40},
	}

	for _, tc := range cases {
		ctx := &lateCancel{Context: context.Background(), left: entryPolls}
		if _, ok := Wrap(ctx, tc.in, tc.w, Plain, false); ok {
			t.Fatalf("%s: Wrap ignored cancellation raised mid-run", tc.name)
		}
		if ctx.polls <= entryPolls {
			t.Fatalf("%s: only %d polls, so the entry check aborted and the guard never ran",
				tc.name, ctx.polls)
		}
	}
}

// Sampling is why a mid-run cancellation needs a long input to be noticed, so
// pin the sampling rate rather than leaving it implied.
func TestGuardSamplesRatherThanPollingEveryUnit(t *testing.T) {
	ctx := &lateCancel{Context: context.Background(), left: 1 << 30}
	in := strings.Repeat("token ", 4000)
	if _, ok := Wrap(ctx, in, 20, Plain, false); !ok {
		t.Fatal("wrap failed")
	}
	if ctx.polls == 0 {
		t.Fatal("guard never polled the context on a long input")
	}
	if units := len(in); ctx.polls > units/ctxSample+8 {
		t.Fatalf("guard polled %d times for %d bytes, expected roughly one per %d",
			ctx.polls, units, ctxSample)
	}
}

// Whitespace may be dropped at a wrap boundary, but nothing visible may be lost
// or reordered.
func TestVisibleContentIsPreserved(t *testing.T) {
	inputs := []string{
		"the quick brown fox jumps over the lazy dog",
		rd + "colored " + gr + "words " + rs + "here",
		strings.Repeat("nowhitespaceatallinthisverylongtoken", 3),
		"tab\tseparated\tfields with spaces",
		"界面 mixed 宽度 text",
		acute + strings.Repeat("\u00e1", 20),
		"trailing spaces    ",
		"    leading spaces",
		"\U0001F600 \U0001F601 \U0001F602 emoji run",
		"a\x1b[38;2;1;2;3mb" + rs + "c",
	}

	for _, in := range inputs {
		for _, w := range []int{1, 2, 3, 5, 8, 13, 40} {
			for _, m := range []Mode{Plain, Structured, Pre} {
				res, ok := Wrap(context.Background(), in, w, m, false)
				if !ok {
					t.Fatalf("%q/%d/%d: wrap failed", in, w, m)
				}
				want := stripSpace(stripANSI(in))
				got := stripSpace(stripANSI(strings.ReplaceAll(res.S, "\n", "")))
				if got != want {
					t.Fatalf("%q at width %d mode %d: visible content became %q, want %q",
						in, w, m, got, want)
				}
			}
		}
	}
}

func TestPlainRowsStayWithinWidth(t *testing.T) {
	inputs := []string{
		"the quick brown fox jumps over the lazy dog",
		"界面 mixed 宽度 text with 汉字 everywhere",
		rd + "colored " + gr + "words " + rs + "here",
		strings.Repeat("longtoken", 5),
		acute + "x combining",
	}

	for _, in := range inputs {
		widest := widestCluster(in)
		for _, w := range []int{1, 2, 3, 5, 8, 13, 40} {
			res, ok := Wrap(context.Background(), in, w, Plain, false)
			if !ok {
				t.Fatalf("%q/%d: wrap failed", in, w)
			}
			lim := max(w, widest)
			for i, r := range rows(res.S) {
				if got := runewidth.StringWidth(stripANSI(r)); got > lim {
					t.Fatalf("%q at width %d: row %d %q is %d cells, limit %d",
						in, w, i, r, got, lim)
				}
			}
		}
	}
}

// Bare CR and other control bytes have no agreed width. They are charged one
// cell so both wrapping paths agree, and they must survive intact.
func TestCRLFAndControlBytesDoNotBreakWidth(t *testing.T) {
	in := "abc\rdef\aghi"
	res, ok := Wrap(context.Background(), in, 4, Plain, false)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if got := stripANSI(res.S); strings.ReplaceAll(got, "\n", "") != in {
		t.Fatalf("content changed: %q", got)
	}
}

func TestInvalidUTF8IsNotDropped(t *testing.T) {
	in := "ab\xffcd\xfe\xfeef"
	res, ok := Wrap(context.Background(), in, 3, Plain, false)
	if !ok {
		t.Fatalf("wrap failed")
	}
	if got := strings.ReplaceAll(res.S, "\n", ""); got != in {
		t.Fatalf("content changed: %q, want %q", got, in)
	}
}

func BenchmarkWrapASCII(b *testing.B) {
	in := strings.Repeat("the quick brown fox jumps over the lazy dog ", 2000)
	b.ReportAllocs()
	b.SetBytes(int64(len(in)))
	for b.Loop() {
		Wrap(context.Background(), in, 80, Plain, false)
	}
}

func BenchmarkWrapANSI(b *testing.B) {
	in := strings.Repeat(rd+"\"key\""+rs+": "+gr+"\"value\""+rs+",\n  ", 2000)
	b.ReportAllocs()
	b.SetBytes(int64(len(in)))
	for b.Loop() {
		Wrap(context.Background(), in, 80, Structured, true)
	}
}

func BenchmarkWrapUnicode(b *testing.B) {
	in := strings.Repeat("界面 mixed 宽度 text "+acute+" \U0001F600 ", 2000)
	b.ReportAllocs()
	b.SetBytes(int64(len(in)))
	for b.Loop() {
		Wrap(context.Background(), in, 80, Plain, false)
	}
}
