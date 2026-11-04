package redact

import (
	"encoding/base64"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

const Mask = "•••"

// maxDepth bounds the escape layers Text decodes, so hostile nesting stays linear.
const maxDepth = 4

// budget bounds the bytes decoded per input byte. Escapes and base64 it cannot finish searching are masked whole.
const budget = 128

const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/-_"

var urlAlphabet = strings.NewReplacer("+", "-", "/", "_")

var isBase64 = func() (t [256]bool) {
	for i := range len(base64Chars) {
		t[base64Chars[i]] = true
	}
	return t
}()

// The zero Set masks nothing.
type Set struct {
	needles []string
	short   int
	longest int
}

func New(secrets ...string) Set {
	out := make([]string, 0, 2*len(secrets))
	for _, s := range secrets {
		if s == "" {
			continue
		}
		out = append(out, s)
		// Latin-1 forms, such as JavaScript's escape(), write U+0080 to U+00FF as one byte.
		if strings.ContainsFunc(s, func(r rune) bool { return 0x80 <= r && r <= 0xff }) {
			b := make([]byte, 0, len(s))
			for i := 0; i < len(s); {
				r, n := utf8.DecodeRuneInString(s[i:])
				if 0x80 <= r && r <= 0xff {
					b = append(b, byte(r))
				} else {
					b = append(b, s[i:i+n]...)
				}
				i += n
			}
			out = append(out, string(b))
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	set := Set{needles: out}
	if len(out) > 0 {
		set.short = len(slices.MinFunc(out, func(a, b string) int { return len(a) - len(b) }))
		set.longest = len(slices.MaxFunc(out, func(a, b string) int { return len(a) - len(b) }))
	}
	return set
}

type span struct{ start, end int }

type search struct {
	set  Set
	left int
}

// Text masks secrets raw or under up to 4 layers of JSON, Go, URL, HTML or base64.
func (s Set) Text(v string) string {
	if len(s.needles) == 0 {
		return v
	}
	x := search{set: s, left: budget * (len(v) + 1<<10)}
	return x.mask(v)
}

func (s *search) mask(v string) string {
	spans := s.descend(v, nil, nil)
	if spans == nil {
		return v
	}

	slices.SortFunc(spans, func(x, y span) int { return x.start - y.start })
	var out strings.Builder
	out.Grow(len(v))
	last := 0
	for i := 0; i < len(spans); {
		start, end := spans[i].start, spans[i].end
		for i++; i < len(spans) && spans[i].start <= end; i++ {
			end = max(end, spans[i].end)
		}
		out.WriteString(v[last:start])
		out.WriteString(Mask)
		last = end
	}
	out.WriteString(v[last:])
	return out.String()
}

func (s *search) find(text string, layers []layer, spans []span) []span {
	add := func(a, b int) { spans = append(spans, source(layers, a, b)) }
	// A match that holds no byte this layer decoded was found in the layer above.
	top := []piece{{dn: len(text)}}
	if len(layers) > 0 {
		top = layers[len(layers)-1].pieces
	}
	for k := 0; k < len(top); {
		a, b := max(0, top[k].d-s.set.longest+1), top[k].d+top[k].dn+s.set.longest-1
		for k++; k < len(top) && top[k].d-s.set.longest+1 < b; k++ {
			b = top[k].d + top[k].dn + s.set.longest - 1
		}
		w := text[a:min(b, len(text))]
		for _, n := range s.set.needles {
			for i := 0; ; i++ {
				j := strings.Index(w[i:], n)
				if j < 0 {
					break
				}
				i += j
				add(a+i, a+i+len(n))
			}
		}
	}

	// Binary data, such as a WebSocket frame or a Basic credential, arrives as base64.
	long := base64.RawStdEncoding.EncodedLen(s.set.short)
	for i := 0; i+long <= len(text); {
		// A run long enough to hold a secret that starts at or before i+long-1 must cover that byte.
		if c := text[i+long-1]; !isBase64[c] && c != '\n' && c != '\r' && c != ' ' && c != '\t' {
			i += long
			continue
		}
		if !isBase64[text[i]] {
			i++
			continue
		}
		j := i + 1
		for {
			for j < len(text) && isBase64[text[j]] {
				j++
			}
			// MIME, PEM, XML and YAML wrap base64, so one line break and its indent may sit in a run.
			k := j
			if strings.HasPrefix(text[k:], "\r\n") {
				k += 2
			} else if k < len(text) && (text[k] == '\n' || text[k] == '\r') {
				k++
			}
			if k == j {
				break
			}
			k = len(text) - len(strings.TrimLeft(text[k:], " \t"))
			if k == len(text) || !isBase64[text[k]] {
				break
			}
			j = k
		}
		end := j
		for end < len(text) && end-j < 2 && text[end] == '=' {
			end++
		}
		if j-i < long {
			i = end
			continue
		}
		if len(layers) > 0 {
			// A run that holds no byte this layer decoded was tried in the layer above.
			l := layers[len(layers)-1].pieces
			if k := sort.Search(len(l), func(k int) bool { return l[k].d+l[k].dn > i }); k == len(l) || l[k].d >= end {
				i = end
				continue
			}
		}
		if s.left -= 4 * (j - i); s.left < 0 {
			add(i, end)
			i = end
			continue
		}
		run, enc := text[i:j], base64.RawStdEncoding
		if strings.ContainsAny(run, "-_") {
			run, enc = urlAlphabet.Replace(run), base64.RawURLEncoding
		}
		// Dropping line breaks and indentation makes each start offset a base64 character.
		src := slices.DeleteFunc([]byte(run), func(c byte) bool { return !isBase64[c] })
		buf := make([]byte, enc.DecodedLen(len(src)))
		// Text can glue a prefix to the data, so each start offset is tried.
		for off := range min(4, len(src)) {
			// Decode keeps the bytes before a partial last quantum.
			n, _ := enc.Decode(buf, src[off:])
			if d := string(buf[:n]); s.mask(d) != d {
				add(i, end)
				break
			}
		}
		i = end
	}
	return spans
}

func source(layers []layer, a, b int) span {
	for _, l := range slices.Backward(layers) {
		a, _ = l.at(a)
		_, b = l.at(b - 1)
	}
	return span{a, b}
}

func (s *search) descend(text string, layers []layer, spans []span) []span {
	spans = s.find(text, layers, spans)
	if len(layers) == maxDepth {
		return spans
	}
	// A cluster ends past what decodes below can change, plus a needle's margin and the two bytes that cut reads.
	reach := (maxDepth-len(layers))*(maxRef-1) + s.set.longest + 1
	// No base64 run holds the byte before a cut, at any depth below.
	cut := func(p int) bool {
		if p == 0 || p == len(text) {
			return true
		}
		switch c := text[p-1]; {
		case c == ' ' || c == '\t':
			return p == 1 || strings.IndexByte(" \t\r\n", text[p-2]) < 0
		case c == '\n' || c == '\r' || c == '=' || isBase64[c]:
			return false
		}
		return true
	}
	for i := 0; ; {
		k := strings.IndexAny(text[i:], `\%&+`)
		if k < 0 {
			return spans
		}
		a := max(i, i+k-s.set.longest+1)
		for !cut(a) {
			a--
		}
		last, b := i+k, 0
		for {
			b = max(b, min(len(text), last+1+reach))
			for !cut(b) {
				b++
			}
			n := strings.IndexAny(text[last+1:min(len(text), b+s.set.longest-1)], `\%&+`)
			if n < 0 {
				break
			}
			last += n + 1
		}
		spans = s.passes(text[a:b], a, layers, spans)
		if s.left < 0 {
			spans = append(spans, source(layers, a, b))
		}
		i = b
	}
}

func (s *search) passes(text string, off int, layers []layer, spans []span) []span {
	// Form encoding writes a space as "+", which other escaping keeps literal.
	if strings.IndexByte(text, '+') >= 0 {
		if s.left -= len(text); s.left < 0 {
			return spans
		}
		form, l := unescape(text, allKinds|plus)
		spans = s.descend(form, append(layers[:len(layers):len(layers)], layer{l, off}), spans)
	}
	if s.left -= len(text); s.left < 0 {
		return spans
	}
	all, la := unescape(text, allKinds)
	if la == nil {
		return spans
	}
	spans = s.descend(all, append(layers[:len(layers):len(layers)], layer{la, off}), spans)
	// Decoding all kinds at once can also decode escape-like text left literal.
	if !slices.ContainsFunc(la, func(p piece) bool { return text[p.s] != text[la[0].s] }) {
		return spans
	}
	for _, k := range [...]int{backslash, percent, reference} {
		if s.left -= len(text); s.left < 0 {
			return spans
		}
		if next, l := unescape(text, k); l != nil && next != all {
			spans = s.descend(next, append(layers[:len(layers):len(layers)], layer{l, off}), spans)
		}
	}
	return spans
}

// JSON masks each scalar or key holding a secret and keeps every other byte.
func (s Set) JSON(b []byte) []byte {
	if len(s.needles) == 0 {
		return b
	}
	src := string(b)
	x := search{set: s, left: budget * (len(b) + 1<<10)}
	// Unmarshal turns invalid UTF-8 into U+FFFD, which would hide a secret.
	if !utf8.Valid(b) || !json.Valid(b) {
		if m := x.mask(src); m != src {
			return []byte(m)
		}
		return b
	}

	var out []byte
	last := 0
	for i := 0; i < len(src); i++ {
		var v string
		j := i + 1
		switch c := src[i]; {
		case c == '"':
			for ; src[j] != '"'; j++ {
				if src[j] == '\\' {
					j++
				}
			}
			j++
			v = src[i+1 : j-1]
			if strings.IndexByte(v, '\\') >= 0 {
				var d string
				_ = json.Unmarshal(b[i:j], &d)
				v = d
			}
		case c == '-' || '0' <= c && c <= '9' || c == 't' || c == 'f' || c == 'n':
			for j < len(src) && strings.IndexByte(",:]} \t\r\n", src[j]) < 0 {
				j++
			}
			v = src[i:j]
		default:
			continue
		}
		if m := x.mask(v); m != v {
			q, _ := json.Marshal(m)
			out = append(append(out, src[last:i]...), q...)
			last = j
		}
		i = j - 1
	}
	if out == nil {
		return b
	}
	return append(out, src[last:]...)
}
