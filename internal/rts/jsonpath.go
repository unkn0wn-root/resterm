package rts

import (
	"strconv"
	"strings"
)

type jseg struct {
	key string
	idx int
	isI bool
}

func jsonPathGet(v any, path string) (any, bool) {
	p := strings.TrimSpace(path)
	if p == "" {
		return v, true
	}
	if strings.HasPrefix(p, "$") {
		p = strings.TrimPrefix(p, "$")
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
			seg, ni, ok, stop := readSeg(p, i)
			if stop {
				return out
			}
			if ok {
				out = append(out, seg)
			}
			i = ni
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

func readSeg(p string, i int) (jseg, int, bool, bool) {
	if i+1 >= len(p) {
		return jseg{}, 0, false, true
	}
	i++
	if q := p[i]; q == '"' || q == '\'' {
		key, ni, ok := readQ(p, i)
		if !ok {
			return jseg{}, 0, false, true
		}
		return jseg{key: key}, ni, true, false
	}
	idx, ni, ok, stop := readIdx(p, i)
	if stop {
		return jseg{}, 0, false, true
	}
	if ok {
		return jseg{idx: idx, isI: true}, ni, true, false
	}
	return jseg{}, ni, false, false
}

func readIdx(p string, i int) (int, int, bool, bool) {
	j := i
	for j < len(p) && p[j] != ']' {
		j++
	}
	if j >= len(p) {
		return 0, 0, false, true
	}
	idx, err := strconv.Atoi(strings.TrimSpace(p[i:j]))
	if err != nil {
		return 0, j, false, false
	}
	return idx, j, true, false
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
