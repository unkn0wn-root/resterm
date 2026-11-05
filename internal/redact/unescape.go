package redact

import (
	"html"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Decoded bytes [d, d+dn) came from the escape at source bytes [s, s+sn).
type piece struct{ d, dn, s, sn int }

// A layer decodes the cluster that starts at off in the text above.
type layer struct {
	pieces []piece
	off    int
}

func (l layer) at(d int) (int, int) {
	k := sort.Search(len(l.pieces), func(k int) bool { return l.pieces[k].d > d }) - 1
	if k < 0 {
		return l.off + d, l.off + d + 1
	}
	p := l.pieces[k]
	if d < p.d+p.dn {
		return l.off + p.s, l.off + p.s + p.sn
	}
	s := l.off + p.s + p.sn + d - p.d - p.dn
	return s, s + 1
}

const (
	backslash = 1 << iota
	percent
	reference
	plus
	allKinds = backslash | percent | reference
)

const (
	shortNames = `"\/'bfnrtav`
	shortBytes = "\"\\/'\b\f\n\r\t\a\v"
	maxRef     = len("&CounterClockwiseContourIntegral;")
	refChars   = "#ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// Nil pieces mean v holds no escape of the given kinds.
func unescape(v string, kinds int) (string, []piece) {
	var out strings.Builder
	var l []piece
	last := 0
	// A "+" is too common to stop at when it is not decoded.
	chars := `\%&`
	if kinds&plus != 0 {
		chars = `\%&+`
	}
	for i := 0; i < len(v); {
		k := strings.IndexAny(v[i:], chars)
		if k < 0 {
			break
		}
		i += k
		dec, n := "", 0
		switch v[i] {
		case '+':
			dec, n = " ", 1
		case '%':
			switch {
			case kinds&percent == 0:
			case strings.HasPrefix(v[i:], "%u"):
				if r, m := utf16Escape(v[i:], "%u"); m > 0 {
					dec, n = string(r), m
				}
			default:
				if c, ok := hexRune(v[i+1:], 2); ok {
					dec, n = string([]byte{byte(c)}), 3
				}
			}
		case '&':
			if kinds&reference == 0 {
				break
			}
			// A reference decodes only whole, or escapes inside it would decode a layer late.
			end := strings.IndexByte(v[i:min(len(v), i+maxRef)], ';')
			if end > 1 && strings.Trim(v[i+1:i+end], refChars) == "" {
				ref := v[i : i+end+1]
				if d := html.UnescapeString(ref); d != ref {
					dec, n = d, len(ref)
				}
			}
		case '\\':
			if kinds&backslash == 0 || i+1 == len(v) {
				break
			}
			if j := strings.IndexByte(shortNames, v[i+1]); j >= 0 {
				dec, n = shortBytes[j:j+1], 2
				break
			}
			switch v[i+1] {
			case 'x':
				if c, ok := hexRune(v[i+2:], 2); ok {
					dec, n = string([]byte{byte(c)}), 4
				}
			case 'u':
				if r, m := utf16Escape(v[i:], `\u`); m > 0 {
					dec, n = string(r), m
				}
			case 'U':
				if r, ok := hexRune(v[i+2:], 8); ok && utf8.ValidRune(r) {
					dec, n = string(r), 10
				}
			}
		}
		if n == 0 {
			i++
			continue
		}
		if l == nil {
			out.Grow(len(v))
		}
		out.WriteString(v[last:i])
		l = append(l, piece{d: out.Len(), dn: len(dec), s: i, sn: n})
		out.WriteString(dec)
		i += n
		last = i
	}
	if l == nil {
		return v, nil
	}
	out.WriteString(v[last:])
	return out.String(), l
}

// A surrogate pair is joined into one rune.
func utf16Escape(v, prefix string) (rune, int) {
	r, ok := hexRune(v[2:], 4)
	switch {
	case !ok:
	case !utf16.IsSurrogate(r):
		return r, 6
	case strings.HasPrefix(v[6:], prefix):
		if lo, ok := hexRune(v[8:], 4); ok {
			if r := utf16.DecodeRune(r, lo); r != utf8.RuneError {
				return r, 12
			}
		}
	}
	return 0, 0
}

// Not strconv, whose error for each non-hex byte allocates.
func hexRune(v string, n int) (rune, bool) {
	if len(v) < n {
		return 0, false
	}
	var r rune
	for i := range n {
		c := v[i]
		switch {
		case '0' <= c && c <= '9':
			c -= '0'
		case 'a' <= c && c <= 'f':
			c -= 'a' - 10
		case 'A' <= c && c <= 'F':
			c -= 'A' - 10
		default:
			return 0, false
		}
		r = r<<4 | rune(c)
	}
	return r, true
}
