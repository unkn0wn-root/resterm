package rts

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type jseg struct {
	key string
	idx int
	isI bool
}

type segResult struct {
	seg  jseg
	next int
	ok   bool
	stop bool
}

type idxResult struct {
	idx  int
	next int
	ok   bool
	stop bool
}

// JSONPathGet resolves a value using the same lightweight JSON path semantics
// used by the RTS runtime.
func JSONPathGet(v any, path string) (any, bool) {
	p := strings.TrimSpace(path)
	if p == "" {
		return v, true
	}
	if after, ok := strings.CutPrefix(p, "$"); ok {
		p = after
		p = strings.TrimPrefix(p, ".")
	}

	segs := splitPath(p)
	cur := v
	for _, s := range segs {
		if s.isI {
			arr, ok := cur.([]any)
			if !ok {
				return nil, false
			}

			if s.idx < 0 || s.idx >= len(arr) {
				return nil, false
			}

			cur = arr[s.idx]
			continue
		}

		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}

		val, ok := obj[s.key]
		if !ok {
			return nil, false
		}
		cur = val
	}
	return cur, true
}

// ValidJSONPath reports whether path uses the supported JSONPathGet syntax.
func ValidJSONPath(path string) bool {
	p := strings.TrimSpace(path)
	if p == "" {
		return true
	}
	if strings.HasPrefix(p, "$") {
		p = p[1:]
		if p == "" {
			return true
		}
		if p[0] == '.' {
			p = p[1:]
			if p == "" {
				return false
			}
		} else if p[0] != '[' {
			return false
		}
	}

	needKey := true
	for i := 0; i < len(p); {
		switch p[i] {
		case '.':
			if needKey {
				return false
			}
			needKey = true
			i++
		case '[':
			next, ok := validJSONPathBracket(p, i)
			if !ok {
				return false
			}
			needKey = false
			i = next
		default:
			if !needKey {
				return false
			}
			start := i
			for i < len(p) && p[i] != '.' && p[i] != '[' {
				r, size := utf8.DecodeRuneInString(p[i:])
				if r == utf8.RuneError && size == 1 {
					return false
				}
				if r == ']' || unicode.IsSpace(r) || unicode.IsControl(r) {
					return false
				}
				i += size
			}
			if i == start {
				return false
			}
			needKey = false
		}
	}
	return !needKey
}

func validJSONPathBracket(path string, start int) (int, bool) {
	if start+1 >= len(path) {
		return 0, false
	}
	i := start + 1
	if path[i] == '"' || path[i] == '\'' {
		_, end, ok := readQ(path, i)
		return end + 1, ok
	}
	res := readIdx(path, i)
	return res.next + 1, res.ok && res.idx >= 0
}

func splitPath(p string) []jseg {
	var out []jseg
	buf := strings.Builder{}
	for i := 0; i < len(p); i++ {
		ch := p[i]
		switch ch {
		case '.':
			out = addSeg(out, &buf)
		case '[':
			out = addSeg(out, &buf)
			res := readSeg(p, i)
			if res.stop {
				return out
			}
			if res.ok {
				out = append(out, res.seg)
			}
			i = res.next
		default:
			buf.WriteByte(ch)
		}
	}
	out = addSeg(out, &buf)
	return out
}

func addSeg(out []jseg, buf *strings.Builder) []jseg {
	if buf.Len() == 0 {
		return out
	}
	out = append(out, jseg{key: buf.String()})
	buf.Reset()
	return out
}

func readSeg(p string, i int) segResult {
	if i+1 >= len(p) {
		return segResult{stop: true}
	}
	i++
	if q := p[i]; q == '"' || q == '\'' {
		key, ni, ok := readQ(p, i)
		if !ok {
			return segResult{stop: true}
		}
		return segResult{seg: jseg{key: key}, next: ni, ok: true}
	}
	res := readIdx(p, i)
	if res.stop {
		return segResult{stop: true}
	}
	if res.ok {
		return segResult{
			seg:  jseg{idx: res.idx, isI: true},
			next: res.next,
			ok:   true,
		}
	}
	return segResult{next: res.next}
}

func readIdx(p string, i int) idxResult {
	j := i
	for j < len(p) && p[j] != ']' {
		j++
	}
	if j >= len(p) {
		return idxResult{stop: true}
	}
	idx, err := strconv.Atoi(strings.TrimSpace(p[i:j]))
	if err != nil {
		return idxResult{next: j}
	}
	return idxResult{idx: idx, next: j, ok: true}
}

func readQ(p string, i int) (string, int, bool) {
	q := p[i]
	i++
	buf := strings.Builder{}
	for i < len(p) {
		ch := p[i]
		if ch == '\\' {
			if i+1 >= len(p) {
				return "", 0, false
			}
			i++
			buf.WriteByte(esc(p[i]))
			i++
			continue
		}
		if ch == q {
			i++
			i = skipWS(p, i)
			if i >= len(p) || p[i] != ']' {
				return "", 0, false
			}
			return buf.String(), i, true
		}
		buf.WriteByte(ch)
		i++
	}
	return "", 0, false
}

func esc(b byte) byte {
	switch b {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case '\\':
		return '\\'
	case '"':
		return '"'
	case '\'':
		return '\''
	default:
		return b
	}
}

func skipWS(p string, i int) int {
	for i < len(p) {
		switch p[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}
