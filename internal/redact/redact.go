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
	}
	return set
}

type span struct{ start, end int }

// Text masks secrets raw or under up to 4 layers of JSON, Go, URL, HTML or base64.
func (s Set) Text(v string) string {
	if len(s.needles) == 0 {
		return v
	}
	spans := s.search(v, nil, nil)
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

func (s Set) search(text string, layers []layer, spans []span) []span {
	add := func(a, b int) {
		for k := len(layers) - 1; k >= 0; k-- {
			a, _ = layers[k].at(a)
			_, b = layers[k].at(b - 1)
		}
		spans = append(spans, span{a, b})
	}
	for _, n := range s.needles {
		for i := 0; ; i++ {
			j := strings.Index(text[i:], n)
			if j < 0 {
				break
			}
			i += j
			add(i, i+len(n))
		}
	}

	// Binary data, such as a WebSocket frame or a Basic credential, arrives as base64.
	long := base64.RawStdEncoding.EncodedLen(s.short)
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
			l := layers[len(layers)-1]
			if k := sort.Search(len(l), func(k int) bool { return l[k].d+l[k].dn > i }); k == len(l) || l[k].d >= end {
				i = end
				continue
			}
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
			if d := string(buf[:n]); s.Text(d) != d {
				add(i, end)
				break
			}
		}
		i = end
	}

	if len(layers) == maxDepth {
		return spans
	}
	// Form encoding writes a space as "+", which other escaping keeps literal.
	if strings.IndexByte(text, '+') >= 0 {
		form, l := unescape(text, allKinds|plus)
		spans = s.search(form, append(layers[:len(layers):len(layers)], l), spans)
	}
	all, la := unescape(text, allKinds)
	if la == nil {
		return spans
	}
	spans = s.search(all, append(layers[:len(layers):len(layers)], la), spans)
	// Decoding all kinds at once can also decode escape-like text left literal.
	if !slices.ContainsFunc(la, func(p piece) bool { return text[p.s] != text[la[0].s] }) {
		return spans
	}
	for _, k := range [...]int{backslash, percent, reference} {
		if next, l := unescape(text, k); l != nil && next != all {
			spans = s.search(next, append(layers[:len(layers):len(layers)], l), spans)
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
	// Unmarshal turns invalid UTF-8 into U+FFFD, which would hide a secret.
	if !utf8.Valid(b) || !json.Valid(b) {
		if m := s.Text(src); m != src {
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
		if m := s.Text(v); m != v {
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
