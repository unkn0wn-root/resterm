package wrap

import (
	"bytes"
	"context"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

type Mode uint8

const (
	Plain Mode = iota
	Structured
	Pre
)

const contUnit = "  "

var contUnitB = []byte(contUnit)

type Span struct {
	S int
	E int
}

type Res struct {
	S  string
	Sp []Span
	Rv []int
}

/*
This is the core wrapper for full content blocks. It streams line-by-line and
emits wrapped segments directly into a buffer to avoid building large
intermediate slices. When mp is true, it also records how wrapped rows map
back to the original logical lines (Sp + Rv). That mapping is required for
cursor/selection navigation in the UI, so we keep it optional to avoid extra
work when it is not needed.
*/
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

	b := []byte(s)
	ls := 0
	ln := 0
	for i := 0; i <= len(b); i++ {
		if i != len(b) && b[i] != '\n' {
			continue
		}
		line := b[ls:i]
		ls = i + 1

		body := line
		var p0, p1 []byte
		var w0, w1 int

		switch m {
		case Structured:
			p1, w1 = structPref(line, w)
		case Pre:
			ind := leadIndent(line)
			if len(ind) > 0 {
				iw := visW(ind)
				if iw < w {
					p0, w0 = ind, iw
					p1, w1 = ind, iw
					body = line[len(ind):]
				}
			}
		}

		start := len(rv)
		n, ok := wrapLine(ctx, body, w, p0, w0, p1, w1, false, func(seg []byte) {
			if out.Len() > 0 {
				out.WriteByte('\n')
			}
			out.Write(seg)
			if mp {
				rv = append(rv, ln)
			}
		})
		if !ok {
			return Res{}, false
		}
		if mp {
			if n == 0 {
				rv = append(rv, ln)
			}
			sp = append(sp, Span{S: start, E: len(rv) - 1})
		}
		ln++
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
	body := b
	var p0, p1 []byte
	var w0, w1 int

	switch m {
	case Structured:
		p1, w1 = structPref(b, w)
	case Pre:
		ind := leadIndent(b)
		if len(ind) > 0 {
			iw := visW(ind)
			if iw < w {
				p0, w0 = ind, iw
				p1, w1 = ind, iw
				body = b[len(ind):]
			}
		}
	}

	segs := make([]string, 0, 4)
	n, ok := wrapLine(ctx, body, w, p0, w0, p1, w1, false, func(seg []byte) {
		segs = append(segs, string(seg))
	})
	if !ok {
		return nil, false
	}
	if n == 0 {
		segs = append(segs, "")
	}
	return segs, true
}

func mapNoWrap(s string) Res {
	b := []byte(s)
	sp := make([]Span, 0, 64)
	rv := make([]int, 0, 128)
	ln := 0
	for i := 0; i <= len(b); i++ {
		if i != len(b) && b[i] != '\n' {
			continue
		}
		idx := len(rv)
		sp = append(sp, Span{S: idx, E: idx})
		rv = append(rv, ln)
		ln++
	}
	return Res{S: s, Sp: sp, Rv: rv}
}

type lw struct {
	w    int
	p0   []byte
	w0   int
	p1   []byte
	w1   int
	ap   []byte
	ansi bool
	keep bool
	out  func([]byte)
	buf  []byte
	cw   int
	hns  bool
	has  bool
	segs int
	spl  func(context.Context, []byte, int) ([]byte, []byte, int, bool)
}

/*
start() initializes a new output segment. If this is a continuation, it
applies the continuation prefix (p1) and the currently active ANSI state (ap)
so visual styling stays consistent across wrapped lines. The ANSI prefix is
derived only from bytes that have already been emitted, which makes the
coloring deterministic even when long tokens are split mid-line.
*/
func (l *lw) start(cont bool) {
	l.buf = l.buf[:0]
	l.cw = 0
	l.has = false
	if cont {
		if len(l.p1) > 0 && l.w1 < l.w {
			l.buf = append(l.buf, l.p1...)
			l.cw = l.w1
			if l.ansi {
				l.ap = updState(l.ap, l.p1)
			}
		}
		if len(l.ap) > 0 {
			l.buf = append(l.buf, l.ap...)
		}
		return
	}
	if len(l.p0) > 0 && l.w0 < l.w {
		l.buf = append(l.buf, l.p0...)
		l.cw = l.w0
		if l.ansi {
			l.ap = updState(l.ap, l.p0)
		}
	}
}

func (l *lw) emitSeg() {
	seg := trimSp(l.buf)
	l.out(seg)
	l.segs++
}

func (l *lw) emitWrap() {
	l.emitSeg()
	l.start(true)
}

/*
addTok() appends a token into the current segment or splits it as needed.
The tricky part is long tokens (tw > line width). For those, we fill any
remaining space on the current line first, then split the rest across new
lines. This avoids emitting a line that only contains indentation or ANSI
prefixes, which was both visually confusing and wasteful.
*/
func (l *lw) addTok(ctx context.Context, tb []byte, tw int, sp bool) bool {
	if len(tb) == 0 {
		return true
	}
	if sp && l.cw == 0 && l.hns && !l.keep {
		return true
	}
	if tw == 0 {
		l.buf = append(l.buf, tb...)
		if l.ansi {
			l.ap = updState(l.ap, tb)
		}
		return true
	}
	if l.cw+tw <= l.w {
		l.buf = append(l.buf, tb...)
		l.cw += tw
		if l.ansi {
			l.ap = updState(l.ap, tb)
		}
		l.has = true
		if !sp {
			l.hns = true
		}
		return true
	}
	if tw > l.w {
		if l.cw > 0 {
			rem := l.w - l.cw
			if rem > 0 {
				seg, rest, sw, ok := l.spl(ctx, tb, rem)
				if !ok {
					return false
				}
				if len(seg) == 0 && len(rest) == len(tb) {
					seg = tb[:1]
					rest = tb[1:]
					sw = 1
				}
				l.buf = append(l.buf, seg...)
				if sw > 0 {
					l.cw += sw
					if l.ansi {
						l.ap = updState(l.ap, seg)
					}
					l.has = true
					if !sp {
						l.hns = true
					}
				}
				tb = rest
				if len(tb) == 0 {
					return true
				}
				if sw > 0 {
					l.emitWrap()
				}
			} else {
				l.emitWrap()
			}
		}
		rest := tb
		for len(rest) > 0 {
			if done(ctx) {
				return false
			}
			lim := l.w - l.cw
			if lim <= 0 {
				l.emitWrap()
				lim = l.w - l.cw
				if lim <= 0 {
					lim = l.w
				}
			}
			seg, rem, sw, ok := l.spl(ctx, rest, lim)
			if !ok {
				return false
			}
			if len(seg) == 0 && len(rem) == len(rest) {
				seg = rest[:1]
				rem = rest[1:]
				sw = 1
			}
			l.buf = append(l.buf, seg...)
			if sw > 0 {
				l.cw += sw
				if l.ansi {
					l.ap = updState(l.ap, seg)
				}
				l.has = true
				if !sp {
					l.hns = true
				}
			}
			rest = rem
			if len(rest) > 0 && sw > 0 {
				l.emitWrap()
			}
		}
		return true
	}
	if l.cw > 0 {
		l.emitWrap()
	}
	if sp && l.hns && !l.keep {
		return true
	}
	l.buf = append(l.buf, tb...)
	l.cw += tw
	if l.ansi {
		l.ap = updState(l.ap, tb)
	}
	l.has = true
	if !sp {
		l.hns = true
	}
	return true
}

/*
wrapLine is a low-level wrapper for a single logical line. It configures
the line wrapper (lw) with prefixes and a splitting strategy, then chooses
the ASCII fast path when possible. The ASCII path avoids rune width checks
and ANSI scanning, which is a major performance win for typical JSON/text.
*/
func wrapLine(
	ctx context.Context,
	b []byte,
	w int,
	p0 []byte,
	w0 int,
	p1 []byte,
	w1 int,
	keep bool,
	out func([]byte),
) (int, bool) {
	if done(ctx) {
		return 0, false
	}
	if w <= 0 {
		seg := append(append([]byte(nil), p0...), b...)
		out(seg)
		return 1, true
	}
	if w0 >= w {
		p0 = nil
		w0 = 0
	}
	if w1 >= w {
		p1 = nil
		w1 = 0
	}
	if len(b) == 0 {
		if len(p0) > 0 {
			out(p0)
		} else {
			out(nil)
		}
		return 1, true
	}

	l := lw{
		w:    w,
		p0:   p0,
		w0:   w0,
		p1:   p1,
		w1:   w1,
		keep: keep,
		out:  out,
		buf:  make([]byte, 0, len(b)+len(p0)+len(p1)),
	}

	if isASCII(b) {
		l.ansi = false
		l.spl = cutASCII
		return wrapASCII(ctx, b, &l)
	}
	l.ansi = true
	l.spl = cutUTF
	return wrapUTF(ctx, b, &l)
}

/*
The ASCII path treats each byte as width 1 and only splits on spaces/tabs.
This is significantly faster than UTF/ANSI handling and is safe because we
only take this path when the input contains no ESC bytes and no non-ASCII.
*/
func wrapASCII(ctx context.Context, b []byte, l *lw) (int, bool) {
	l.start(false)
	i := 0
	for i < len(b) {
		if done(ctx) {
			return 0, false
		}
		if b[i] == ' ' || b[i] == '\t' {
			j := i + 1
			for j < len(b) && (b[j] == ' ' || b[j] == '\t') {
				j++
			}
			if !l.addTok(ctx, b[i:j], j-i, true) {
				return 0, false
			}
			i = j
			continue
		}
		j := i + 1
		for j < len(b) && b[j] != ' ' && b[j] != '\t' {
			j++
		}
		if !l.addTok(ctx, b[i:j], j-i, false) {
			return 0, false
		}
		i = j
	}
	if l.segs == 0 || l.has {
		l.emitSeg()
	}
	return l.segs, true
}

/*
The UTF/ANSI path must preserve escape sequences and account for rune widths.
We build tokens as runs of either whitespace or non-whitespace, while allowing
ANSI escape sequences to appear inside tokens. We do not update ANSI state
here based on the full input line; instead, we update it only when bytes are
emitted, which keeps continuation lines correctly colored when long tokens
are split.
*/
func wrapUTF(ctx context.Context, b []byte, l *lw) (int, bool) {
	l.start(false)
	var tb []byte
	tw := 0
	ts := byte(0)
	pp := make([]byte, 0, 16)

	flush := func() bool {
		if len(tb) == 0 {
			return true
		}
		sp := ts == 1
		if !l.addTok(ctx, tb, tw, sp) {
			return false
		}
		tb = tb[:0]
		tw = 0
		ts = 0
		return true
	}

	i := 0
	for i < len(b) {
		if done(ctx) {
			return 0, false
		}
		if n := scanEsc(b, i); n > 0 {
			if ts == 0 || ts == 1 {
				pp = append(pp, b[i:i+n]...)
			} else {
				tb = append(tb, b[i:i+n]...)
			}
			i += n
			continue
		}
		r, sz := utf8.DecodeRune(b[i:])
		if sz <= 0 {
			sz = 1
			r = rune(b[i])
		}
		sp := unicode.IsSpace(r)
		if ts == 0 {
			if len(pp) > 0 {
				tb = append(tb, pp...)
				pp = pp[:0]
			}
			tb = append(tb, b[i:i+sz]...)
			tw += rw(r)
			if sp {
				ts = 1
			} else {
				ts = 2
			}
			i += sz
			continue
		}
		if (ts == 1 && !sp) || (ts == 2 && sp) {
			if !flush() {
				return 0, false
			}
			if len(pp) > 0 {
				tb = append(tb, pp...)
				pp = pp[:0]
			}
			tb = append(tb, b[i:i+sz]...)
			tw += rw(r)
			if sp {
				ts = 1
			} else {
				ts = 2
			}
			i += sz
			continue
		}
		tb = append(tb, b[i:i+sz]...)
		tw += rw(r)
		i += sz
	}

	if !flush() {
		return 0, false
	}
	if len(pp) > 0 {
		if !l.addTok(ctx, pp, 0, false) {
			return 0, false
		}
	}
	if l.segs == 0 || l.has {
		l.emitSeg()
	}
	return l.segs, true
}

/*
updState updates the active ANSI SGR state by scanning the bytes that were
actually emitted. This is the most robust way to carry styles across wraps
because it mirrors the real output stream. We only care about SGR (CSI ... m)
sequences; other ANSI codes are ignored for state tracking.
*/
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

/*
An SGR sequence is a CSI ending in 'm' (e.g., ESC[31m). We use this to
identify color/style changes so we can preserve them across wrap boundaries.
*/
func isSGR(b []byte) bool {
	if len(b) < 3 {
		return false
	}
	if b[0] != 0x1b || b[1] != '[' {
		return false
	}
	return b[len(b)-1] == 'm'
}

/*
We keep a simple "active prefix" of SGR codes. A reset (ESC[0m or ESC[m)
clears it, otherwise we append the SGR sequence. This is intentionally simple:
it prioritizes correctness and determinism over deduplication.
*/
func updSGR(ap []byte, seq []byte) []byte {
	if len(seq) < 3 {
		return ap
	}
	if isSGRReset(seq) {
		return ap[:0]
	}
	ap = append(ap, seq...)
	return ap
}

/*
We only treat plain resets as resets: ESC[m and ESC[0m. Other sequences like
ESC[38;5;0m are valid colors and must *not* clear the active state.
*/
func isSGRReset(b []byte) bool {
	if len(b) < 3 || b[len(b)-1] != 'm' {
		return false
	}
	if len(b) == 3 {
		return true
	}
	params := b[2 : len(b)-1]
	if len(params) == 0 {
		return true
	}
	num := -1
	for i := range params {
		if params[i] == ';' {
			break
		}
		if params[i] < '0' || params[i] > '9' {
			return false
		}
		if num < 0 {
			num = 0
		}
		num = num*10 + int(params[i]-'0')
	}
	if num < 0 {
		return true
	}
	return num == 0
}

func cutASCII(_ context.Context, b []byte, lim int) ([]byte, []byte, int, bool) {
	if lim <= 0 {
		lim = 1
	}
	if len(b) <= lim {
		return b, nil, len(b), true
	}
	return b[:lim], b[lim:], lim, true
}

/*
cutUTF splits a byte slice so the leading segment fits within lim cells.
It skips ANSI sequences and measures display width using rune width. This
is used by the token splitter when a long token must be broken across lines.
*/
func cutUTF(ctx context.Context, b []byte, lim int) ([]byte, []byte, int, bool) {
	if done(ctx) {
		return nil, nil, 0, false
	}
	if lim <= 0 {
		lim = 1
	}
	i := 0
	w := 0
	for i < len(b) {
		if done(ctx) {
			return nil, nil, 0, false
		}
		if n := scanEsc(b, i); n > 0 {
			i += n
			continue
		}
		r, sz := utf8.DecodeRune(b[i:])
		if sz <= 0 {
			sz = 1
			r = rune(b[i])
		}
		rw := rw(r)
		if w+rw > lim {
			break
		}
		i += sz
		w += rw
	}
	if i == 0 {
		if n := scanEsc(b, 0); n > 0 {
			i = n
		} else {
			r, sz := utf8.DecodeRune(b)
			if sz <= 0 {
				sz = 1
				r = rune(b[0])
			}
			i = sz
			w = rw(r)
		}
	}
	return b[:i], b[i:], w, true
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c == 0x1b || c >= 0x80 {
			return false
		}
	}
	return true
}

func rw(r rune) int {
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		return 1
	}
	return w
}

func leadIndent(b []byte) []byte {
	i := 0
	for i < len(b) {
		r, sz := utf8.DecodeRune(b[i:])
		if sz <= 0 {
			sz = 1
			r = rune(b[i])
		}
		if r == ' ' || r == '\t' {
			i += sz
			continue
		}
		break
	}
	if i == 0 {
		return nil
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
		r, sz := utf8.DecodeRune(b[i:])
		if sz <= 0 {
			sz = 1
			r = rune(b[i])
		}
		if r == ' ' || r == '\t' {
			out = append(out, b[i:i+sz]...)
			i += sz
			continue
		}
		break
	}
	if len(out) == 0 {
		return nil
	}
	clean, _ := trimANSISuffix(out)
	return clean
}

func visW(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	w := 0
	i := 0
	for i < len(b) {
		if n := scanEsc(b, i); n > 0 {
			i += n
			continue
		}
		r, sz := utf8.DecodeRune(b[i:])
		if sz <= 0 {
			sz = 1
			r = rune(b[i])
		}
		rw := runewidth.RuneWidth(r)
		if rw > 0 {
			w += rw
		}
		i += sz
	}
	return w
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
