package wrap

import (
	"bytes"
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

type Mode uint8

const (
	Plain Mode = iota
	Structured
	Pre
)

const ContinuationUnit = "  "

const (
	// SGR extended color selectors.
	sgrExtForeground = 38 // foreground color
	sgrExtBackground = 48 // background color
	sgrExtUnderline  = 58 // underline color

	// SGR extended color modes (after 38/48/58).
	sgrExtPalette = 5 // 256-color palette: 38/48/58;5;N
	sgrExtRGB     = 2 // truecolor: 38/48/58;2;R;G;B
)

// Units scanned between context checks. Err takes a lock on a cancellable
// context, so checking it on every byte costs more than it saves.
const ctxSample = 512

var contUnitB = []byte(ContinuationUnit)

type Span struct {
	S int
	E int
}

type Res struct {
	S  string
	Sp []Span
	Rv []int
}

// Wrap is the core wrapper for full content blocks. It streams line by line and
// writes rows straight into a buffer rather than building large intermediate
// slices. When mp is true it also records how wrapped rows map back to the
// original logical lines (Sp and Rv). The UI needs that mapping to move a cursor
// and a selection, so it stays optional and the extra work is skipped when no
// caller wants it.
func Wrap(ctx context.Context, s string, w int, m Mode, mp bool) (Res, bool) {
	if done(ctx) {
		return Res{}, false
	}
	if w <= 0 {
		if !mp {
			return Res{S: s}, true
		}
		return mapNoWrap(s), true
	}

	var out bytes.Buffer
	out.Grow(len(s) + len(s)/8)

	var sp []Span
	var rv []int
	if mp {
		sp = make([]Span, 0, 64)
		rv = make([]int, 0, 128)
	}

	// Rows are counted, not derived from the buffer length. An empty row adds no
	// bytes, so a length test would skip its separator and leave Rv describing
	// more rows than the output actually has.
	rows := 0
	ln := 0
	rest := []byte(s)
	for {
		line := rest
		last := true
		if i := bytes.IndexByte(rest, '\n'); i >= 0 {
			line, rest, last = rest[:i], rest[i+1:], false
		}

		start := rows
		ok := wrapLine(ctx, prefixes(line, w, m), w, func(seg []byte) {
			if rows > 0 {
				out.WriteByte('\n')
			}
			out.Write(seg)
			rows++
			if mp {
				rv = append(rv, ln)
			}
		})
		if !ok {
			return Res{}, false
		}
		if mp {
			sp = append(sp, Span{S: start, E: rows - 1})
		}
		ln++
		if last {
			break
		}
	}

	res := Res{S: out.String()}
	if mp {
		res.Sp = sp
		res.Rv = rv
	}
	return res, true
}

func Line(ctx context.Context, s string, w int, m Mode) ([]string, bool) {
	if done(ctx) {
		return nil, false
	}
	if w <= 0 {
		return []string{s}, true
	}

	b := []byte(s)
	segs := make([]string, 0, 4)
	ok := wrapLine(ctx, prefixes(b, w, m), w, func(seg []byte) {
		segs = append(segs, string(seg))
	})
	if !ok {
		return nil, false
	}
	return segs, true
}

func mapNoWrap(s string) Res {
	n := strings.Count(s, "\n") + 1
	sp := make([]Span, n)
	rv := make([]int, n)
	for i := range sp {
		sp[i] = Span{S: i, E: i}
		rv[i] = i
	}
	return Res{S: s, Sp: sp, Rv: rv}
}

// pfx describes the leading run that every row of a logical line repeats. p0
// applies to the first row and p1 to continuations. prefixes keeps both widths
// below w, so a row always has at least one cell left for content.
type pfx struct {
	body []byte
	p0   []byte
	w0   int
	p1   []byte
	w1   int
}

func prefixes(line []byte, w int, m Mode) pfx {
	p := pfx{body: line}
	switch m {
	case Structured:
		p.p1, p.w1 = structPref(line, w)
	case Pre:
		ind := leadIndent(line)
		if len(ind) == 0 {
			break
		}
		iw := visW(ind)
		if iw >= w {
			break
		}
		p.p0, p.w0 = ind, iw
		p.p1, p.w1 = ind, iw
		p.body = line[len(ind):]
	}
	return p
}

// lw builds one row at a time. buf is the row under construction and prev is the
// row already finished but not yet handed to out. Holding one row back lets a
// trailing zero-width byte join the row it belongs to. That matters for an ANSI
// reset arriving after the last visible cell, which would otherwise be dropped
// along with an invisible continuation row.
type lw struct {
	w    int    // row width in cells
	p0   []byte // prefix for the first row
	w0   int    // cells p0 occupies
	p1   []byte // prefix repeated on continuation rows
	w1   int    // cells p1 occupies
	ap   []byte // active SGR state, replayed onto continuations
	ansi bool   // input may contain escape sequences
	pref bool   // a prefix is in play, so rows are narrower than w
	out  func([]byte)
	spl  func(b []byte, lim int) (seg, rest []byte, w int)

	buf  []byte // row under construction
	base int    // leading bytes of buf that are prefix and replayed state
	prev []byte // finished row awaiting out
	held bool   // prev holds a row
	cw   int    // cells used in buf
	has  bool   // buf gained a visible cell since start
	hns  bool   // a non-space token was placed on this logical line
	segs int    // rows emitted so far
}

// start begins a new row. On a continuation it writes the continuation prefix
// (p1) and the active ANSI state (ap) so styling carries across the wrap. That
// state comes only from bytes already emitted, which keeps the coloring
// deterministic even when a long token is split across rows.
func (l *lw) start(cont bool) {
	l.buf = l.buf[:0]
	l.cw = 0
	l.has = false
	switch {
	case cont:
		if len(l.p1) > 0 {
			l.buf = append(l.buf, l.p1...)
			l.cw = l.w1
		}
		// Keep prefix styling local to the indent. Merging p1 into the active
		// state would let continuation text inherit indent-only colors.
		l.buf = append(l.buf, l.ap...)
	case len(l.p0) > 0:
		l.buf = append(l.buf, l.p0...)
		l.cw = l.w0
		if l.ansi {
			l.ap = updState(l.ap, l.p0)
		}
	}
	l.base = len(l.buf)
}

func (l *lw) emitSeg() {
	l.buf = trimSp(l.buf)
	l.segs++
	l.release()
	l.buf, l.prev = l.prev, l.buf
	l.held = true
}

func (l *lw) release() {
	if !l.held {
		return
	}
	l.out(l.prev)
	l.held = false
}

func (l *lw) emitWrap() {
	l.emitSeg()
	l.start(true)
}

// finish closes the last row. A row that never gained a visible cell holds
// nothing but its prefix and replayed state, so it is dropped. Anything appended
// past that point is zero width and belongs to the row before it, which release
// has not handed out yet.
func (l *lw) finish() {
	switch {
	case l.segs == 0 || l.has:
		l.emitSeg()
	case len(l.buf) > l.base:
		l.prev = append(l.prev, l.buf[l.base:]...)
	}
	l.release()
}

func (l *lw) write(b []byte, w int, sp bool) {
	l.buf = append(l.buf, b...)
	if l.ansi {
		l.ap = updState(l.ap, b)
	}
	if w == 0 {
		return
	}
	l.cw += w
	l.has = true
	if !sp {
		l.hns = true
	}
}

// dropPref strips the indent from a row that holds nothing else, giving the full
// width to content that could not fit beside it. The replayed ANSI state stays.
// Only the prefix loses its cells and its styling, and that styling described an
// indent the row no longer has. Staying within w matters more than the indent,
// because a row wider than the pane gets wrapped again by the terminal, and that
// desyncs the row mapping the UI navigates by.
func (l *lw) dropPref() {
	l.buf = append(l.buf[:0], l.ap...)
	l.base = len(l.buf)
	l.cw = 0
}

func (l *lw) split(b []byte) ([]byte, []byte, int) {
	rem := l.w - l.cw
	if rem <= 0 {
		return nil, b, 0
	}
	return l.spl(b, rem)
}

// addTok places a token in the current row, splitting it when it does not fit.
// A token wider than the row fills the space left on the current row first and
// then continues across new rows, so no row ends up holding only an indent or an
// ANSI prefix. A split never lands inside a grapheme cluster, and a cluster too
// wide for the space left moves down whole rather than pushing the row past w.
func (l *lw) addTok(g *guard, tb []byte, tw int, sp bool) bool {
	if len(tb) == 0 {
		return true
	}
	if sp && l.cw == 0 && l.hns {
		return true
	}
	if tw == 0 || l.cw+tw <= l.w {
		l.write(tb, tw, sp)
		return true
	}
	// A token that fits a row of its own moves down whole. A prefix makes every
	// row narrower than w, so when there is one we fill the current row first
	// instead of stranding its tail behind the indent.
	if tw <= l.w && (!l.pref || sp || l.cw == 0) {
		if l.cw > 0 {
			l.emitWrap()
		}
		if sp && l.hns {
			return true
		}
		if l.cw+tw <= l.w {
			l.write(tb, tw, sp)
			return true
		}
		// The continuation prefix took the room this token needed. Fall through
		// and let the splitter choose between splitting and dropping the indent.
	}
	for len(tb) > 0 {
		if g.done() {
			return false
		}
		seg, rest, sw := l.split(tb)
		if len(seg) == 0 {
			switch {
			case l.has:
				l.emitWrap()
				continue
			case l.cw > 0:
				// The row holds nothing but its prefix, so this cluster is too
				// wide for the indented area rather than for the row itself.
				l.dropPref()
				continue
			}
			// Wider than a whole row, so place it instead of wrapping forever.
			seg, rest, sw = forceUnit(tb)
		}
		l.write(seg, sw, sp)
		tb = rest
		if len(tb) > 0 {
			l.emitWrap()
		}
	}
	return true
}

// wrapLine wraps a single logical line. It sets up lw with the prefixes and a
// splitting strategy, then takes the ASCII fast path where it can. That path
// skips grapheme segmentation and ANSI scanning, which is a large win on ordinary
// JSON and text. It always emits at least one row, so callers can count rows from
// out. w must be positive.
func wrapLine(ctx context.Context, p pfx, w int, out func([]byte)) bool {
	if done(ctx) {
		return false
	}
	if len(p.body) == 0 {
		out(p.p0)
		return true
	}

	l := lw{
		w:    w,
		p0:   p.p0,
		w0:   p.w0,
		p1:   p.p1,
		w1:   p.w1,
		pref: len(p.p0) > 0 || len(p.p1) > 0,
		out:  out,
		buf:  make([]byte, 0, len(p.body)+len(p.p0)+len(p.p1)),
	}
	g := guard{ctx: ctx}

	if isASCII(p.body) {
		l.spl = cutASCII
		return wrapASCII(&g, p.body, &l)
	}
	l.ansi = true
	l.spl = cutCells
	return wrapCells(&g, p.body, &l)
}

// wrapASCII treats every byte as one cell and breaks only on spaces and tabs. It
// is much faster than the grapheme and ANSI path, and it is safe because the
// caller only picks it when the line holds no ESC bytes and no non-ASCII.
func wrapASCII(g *guard, b []byte, l *lw) bool {
	l.start(false)
	for i := 0; i < len(b); {
		if g.done() {
			return false
		}
		// The two scans are written out rather than shared so each compiles to a
		// tight compare against a constant. This is the hottest loop in the package.
		sp := isBlank(b[i])
		j := i + 1
		if sp {
			for j < len(b) && isBlank(b[j]) {
				j++
			}
		} else {
			for j < len(b) && !isBlank(b[j]) {
				j++
			}
		}
		if !l.addTok(g, b[i:j], j-i, sp) {
			return false
		}
		i = j
	}
	l.finish()
	return true
}

// wrapCells is the grapheme and ANSI path. It preserves escape sequences and
// measures tokens in cells. Tokens are runs of whitespace or non-whitespace, and
// an escape sequence may sit inside a token. The ANSI state is never derived from
// the whole input line. It is updated only as bytes are emitted, which keeps
// continuation rows correctly colored when a long token is split.
//
// Escapes arriving while a whitespace token is open are parked in pp rather than
// folded into it. Whitespace is dropped at a wrap boundary, and dropping a color
// change with it would leak styling into the rest of the view.
func wrapCells(g *guard, b []byte, l *lw) bool {
	l.start(false)
	sc := newScan(b)
	var tb []byte
	tw := 0
	ts := byte(0)
	pp := make([]byte, 0, 16)

	flush := func() bool {
		if len(tb) == 0 {
			return true
		}
		if !l.addTok(g, tb, tw, ts == 1) {
			return false
		}
		tb = tb[:0]
		tw = 0
		ts = 0
		return true
	}

	for {
		u, ok := sc.next()
		if !ok {
			break
		}
		if g.done() {
			return false
		}
		if u.esc {
			if ts == 2 {
				tb = append(tb, u.b...)
			} else {
				pp = append(pp, u.b...)
			}
			continue
		}
		st := byte(2)
		if unicode.IsSpace(u.r) {
			st = 1
		}
		if ts != st {
			if ts != 0 && !flush() {
				return false
			}
			if len(pp) > 0 {
				tb = append(tb, pp...)
				pp = pp[:0]
			}
			ts = st
		}
		tb = append(tb, u.b...)
		tw += u.w
	}

	if !flush() {
		return false
	}
	if len(pp) > 0 && !l.addTok(g, pp, 0, false) {
		return false
	}
	l.finish()
	return true
}

// scan walks a byte slice one display unit at a time. A unit is either a
// complete ANSI escape sequence, which costs no cells, or one grapheme cluster.
// Working in clusters is what keeps a combining accent attached to its letter and
// a multi-rune emoji in one piece.
type scan struct {
	b  []byte
	i  int
	st int // uniseg grapheme boundary state
}

type unit struct {
	b   []byte
	w   int  // display cells
	r   rune // first rune of the cluster, unset for escapes
	esc bool
}

func newScan(b []byte) scan {
	return scan{b: b, st: -1}
}

// skip advances over bytes the segmenter never saw and drops its boundary state.
// That state is only meaningful for the exact slice FirstGraphemeCluster returned,
// because it carries the property of that slice's first rune. Stepping over
// anything leaves it describing the wrong rune, and the next cluster would then be
// judged by a stale property and torn apart.
func (s *scan) skip(n int) []byte {
	b := s.b[s.i : s.i+n]
	s.i += n
	s.st = -1
	return b
}

func (s *scan) next() (unit, bool) {
	if s.i >= len(s.b) {
		return unit{}, false
	}
	if n := scanEsc(s.b, s.i); n > 0 {
		return unit{b: s.skip(n), esc: true}, true
	}
	// Printable ASCII followed by another ASCII byte is always a cluster on its
	// own, because nothing that joins a cluster encodes below 0x80. Skipping the
	// segmenter keeps colored ASCII near byte speed, and that is by far the most
	// common input.
	if c := s.b[s.i]; c > 0x1f && c < 0x7f && (s.i+1 == len(s.b) || s.b[s.i+1] < 0x80) {
		return unit{b: s.skip(1), w: 1, r: rune(c)}, true
	}
	cl, _, _, st := uniseg.FirstGraphemeCluster(s.b[s.i:], s.st)
	s.st = st
	r, _ := utf8.DecodeRune(cl)
	s.i += len(cl)
	return unit{b: cl, w: clusterW(cl), r: r}, true
}

// clusterW measures one grapheme cluster the way runewidth.StringWidth does, as
// the width of its first non-zero-width rune. Combining marks, variation selectors
// and zero-width joiners cost nothing, so an accented letter or an emoji sequence
// takes a single slot. Control bytes measure zero under runewidth while the ASCII
// path spends a cell on them, so they are charged one cell here and the two paths
// stay in agreement.
func clusterW(b []byte) int {
	for i := 0; i < len(b); {
		r, sz := utf8.DecodeRune(b[i:])
		if w := runewidth.RuneWidth(r); w > 0 {
			return w
		}
		if unicode.IsControl(r) {
			return 1
		}
		i += sz
	}
	return 0
}

// guard samples ctx instead of checking it on every unit. Err takes a lock on a
// cancellable context, and paying that on every byte of a large response costs
// more than the responsiveness is worth.
type guard struct {
	ctx context.Context
	n   int
}

func (g *guard) done() bool {
	g.n++
	if g.n < ctxSample {
		return false
	}
	g.n = 0
	return done(g.ctx)
}

// updState updates the active ANSI SGR state from the bytes that were actually
// emitted. Mirroring the real output stream is the most reliable way to carry
// styling across a wrap. Only SGR sequences (CSI ... m) matter here, and every
// other ANSI code is ignored for state tracking.
func updState(ap []byte, b []byte) []byte {
	if len(b) == 0 {
		return ap
	}
	if bytes.IndexByte(b, 0x1b) == -1 {
		return ap
	}
	i := 0
	for i < len(b) {
		if n := scanEsc(b, i); n > 0 {
			seq := b[i : i+n]
			if isSGR(seq) {
				ap = updSGR(ap, seq)
			}
			i += n
			continue
		}
		_, sz := utf8.DecodeRune(b[i:])
		if sz <= 0 {
			sz = 1
		}
		i += sz
	}
	return ap
}

// isSGR reports whether b is a CSI ending in 'm', for example ESC[31m. Those
// carry the color and style changes worth preserving across a wrap boundary.
func isSGR(b []byte) bool {
	if len(b) < 3 {
		return false
	}
	if b[0] != 0x1b || b[1] != '[' {
		return false
	}
	return b[len(b)-1] == 'm'
}

// updSGR keeps a running prefix of the active SGR codes. A reset (ESC[0m or
// ESC[m) clears it and anything else is appended. This stays deliberately simple
// and favors correctness and determinism over deduplication.
func updSGR(ap []byte, seq []byte) []byte {
	if len(seq) < 3 {
		return ap
	}
	rst, oth := sgrFlags(seq)
	if rst && !oth {
		return ap[:0]
	}
	if rst {
		ap = ap[:0]
	}
	ap = append(ap, seq...)
	return ap
}

// sgrFlags counts param 0 or an empty param as a reset only when it stands on
// its own. Values inside an extended color sequence (38/48/58;5;n or
// 38/48/58;2;r;g;b) must not clear the active state, since ESC[38;5;0m is a valid
// color rather than a reset.
func sgrFlags(b []byte) (bool, bool) {
	if len(b) < 3 || b[len(b)-1] != 'm' {
		return false, false
	}
	if len(b) == 3 {
		return true, false
	}
	params := b[2 : len(b)-1]
	if len(params) == 0 {
		return true, false
	}

	vals := make([]int, 0, 8)
	num := -1
	for i := range params {
		c := params[i]
		switch {
		case c == ';':
			if num < 0 {
				vals = append(vals, 0)
			} else {
				vals = append(vals, num)
				num = -1
			}
		case c >= '0' && c <= '9':
			if num < 0 {
				num = int(c - '0')
			} else {
				num = num*10 + int(c-'0')
			}
		default:
			if num >= 0 {
				vals = append(vals, num)
				num = -1
			}
			vals = append(vals, -1)
		}
	}
	if num < 0 {
		if params[len(params)-1] == ';' {
			vals = append(vals, 0)
		}
	} else {
		vals = append(vals, num)
	}

	reset := false
	other := false
	for i := 0; i < len(vals); {
		p := vals[i]
		// Handle extended color sequences: 38/48/58;5;N or 38/48/58;2;R;G;B.
		if p == sgrExtForeground || p == sgrExtBackground || p == sgrExtUnderline {
			if i+1 < len(vals) {
				switch vals[i+1] {
				case sgrExtPalette:
					other = true
					i += 3
					continue
				case sgrExtRGB:
					other = true
					i += 5
					continue
				}
			}
			other = true
			i++
			continue
		}
		switch p {
		case 0:
			reset = true
		default:
			other = true
		}
		i++
	}
	return reset, other
}

func cutASCII(b []byte, lim int) ([]byte, []byte, int) {
	if lim <= 0 {
		return nil, b, 0
	}
	if len(b) <= lim {
		return b, nil, len(b)
	}
	return b[:lim], b[lim:], lim
}

// cutCells splits b so the leading part occupies at most lim cells and never
// lands inside a grapheme cluster. Escape sequences cost no cells and travel
// with the leading part. A zero-length leading part means not even one cluster fits,
// which tells the caller to wrap before it can make progress.
func cutCells(b []byte, lim int) ([]byte, []byte, int) {
	sc := newScan(b)
	end := 0
	w := 0
	for {
		u, ok := sc.next()
		if !ok {
			return b, nil, w // trailing escapes stay with the text they close
		}
		if u.esc {
			continue
		}
		if w+u.w > lim {
			return b[:end], b[end:], w
		}
		w += u.w
		end = sc.i
	}
}

// forceUnit takes any leading escapes plus exactly one grapheme cluster. It is
// the last resort for a cluster wider than a whole row, where wrapping again
// frees no space and refusing to place the cluster would loop forever.
func forceUnit(b []byte) ([]byte, []byte, int) {
	sc := newScan(b)
	w := 0
	for {
		u, ok := sc.next()
		if !ok {
			break
		}
		if !u.esc {
			w = u.w
			break
		}
	}
	return b[:sc.i], b[sc.i:], w
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c == 0x1b || c >= 0x80 {
			return false
		}
	}
	return true
}

func isBlank(c byte) bool {
	return c == ' ' || c == '\t'
}

func leadIndent(b []byte) []byte {
	i := 0
	for i < len(b) && isBlank(b[i]) {
		i++
	}
	return b[:i]
}

func structPref(b []byte, w int) ([]byte, int) {
	ind := leadWSANSI(b)
	iw := visW(ind)
	if iw >= w {
		return nil, 0
	}
	uw := visW(contUnitB)
	if iw+uw < w {
		p := append(append([]byte(nil), ind...), contUnitB...)
		return p, iw + uw
	}
	return ind, iw
}

func leadWSANSI(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	i := 0
	out := make([]byte, 0, 16)
	for i < len(b) {
		if n := scanEsc(b, i); n > 0 {
			out = append(out, b[i:i+n]...)
			i += n
			continue
		}
		if !isBlank(b[i]) {
			break
		}
		out = append(out, b[i])
		i++
	}
	if len(out) == 0 {
		return nil
	}
	clean, suf := trimANSISuffix(out)
	// Keep an explicit reset when the trailing ANSI held one, so a continuation
	// prefix does not inherit an outer style such as bold from its parent. SGR 0
	// clears every graphic attribute, which stops prefix styling from bleeding
	// into the content tokens that follow.
	if hasResetSGR(suf) {
		const resetAll = "\x1b[0m"
		clean = append(append([]byte(nil), clean...), resetAll...)
	}
	return clean
}

func visW(b []byte) int {
	sc := newScan(b)
	w := 0
	for {
		u, ok := sc.next()
		if !ok {
			return w
		}
		w += u.w
	}
}

func trimANSISuffix(b []byte) ([]byte, []byte) {
	if len(b) == 0 {
		return b, nil
	}
	type rng struct{ s, e int }
	var rs []rng
	i := 0
	for i < len(b) {
		if n := scanEsc(b, i); n > 0 {
			rs = append(rs, rng{s: i, e: i + n})
			i += n
			continue
		}
		_, sz := utf8.DecodeRune(b[i:])
		if sz <= 0 {
			sz = 1
		}
		i += sz
	}
	if len(rs) == 0 {
		return b, nil
	}
	end := len(b)
	for len(rs) > 0 {
		last := rs[len(rs)-1]
		if last.e != end {
			break
		}
		end = last.s
		rs = rs[:len(rs)-1]
	}
	if end == len(b) {
		return b, nil
	}
	return b[:end], b[end:]
}

func hasResetSGR(b []byte) bool {
	i := 0
	for i < len(b) {
		n := scanEsc(b, i)
		if n == 0 {
			i++
			continue
		}
		seq := b[i : i+n]
		if isSGR(seq) {
			r, o := sgrFlags(seq)
			if r && !o {
				return true
			}
		}
		i += n
	}
	return false
}

func scanEsc(b []byte, i int) int {
	if i >= len(b) || b[i] != 0x1b || i+1 >= len(b) {
		return 0
	}
	switch b[i+1] {
	case '[':
		j := i + 2
		for j < len(b) {
			c := b[j]
			if (c >= '0' && c <= '9') || c == ';' || c == '?' {
				j++
				continue
			}
			break
		}
		for j < len(b) {
			c := b[j]
			if c >= ' ' && c <= '/' {
				j++
				continue
			}
			break
		}
		if j < len(b) && b[j] >= '@' && b[j] <= '~' {
			return j - i + 1
		}
	case ']':
		j := i + 2
		for j < len(b) {
			if b[j] == 0x07 {
				return j - i + 1
			}
			if b[j] == 0x1b && j+1 < len(b) && b[j+1] == '\\' {
				return j - i + 2
			}
			j++
		}
	}
	return 0
}

func trimSp(b []byte) []byte {
	i := len(b)
	for i > 0 && b[i-1] == ' ' {
		i--
	}
	if i == 0 {
		return b
	}
	return b[:i]
}

func done(ctx context.Context) bool {
	return ctx != nil && ctx.Err() != nil
}
