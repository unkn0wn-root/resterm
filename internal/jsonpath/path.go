package jsonpath

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type segment struct {
	key   string
	index int
	isIdx bool
}

type segmentResult struct {
	segment segment
	next    int
	ok      bool
	stop    bool
}

type indexResult struct {
	index int
	next  int
	ok    bool
	stop  bool
}

// Get resolves path against v.
func Get(v any, path string) (any, bool) {
	p := strings.TrimSpace(path)
	if p == "" {
		return v, true
	}
	if after, ok := strings.CutPrefix(p, "$"); ok {
		p = strings.TrimPrefix(after, ".")
	}

	cur := v
	for _, s := range split(p) {
		if s.isIdx {
			items, ok := cur.([]any)
			if !ok || s.index < 0 || s.index >= len(items) {
				return nil, false
			}
			cur = items[s.index]
			continue
		}

		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		var found bool
		cur, found = obj[s.key]
		if !found {
			return nil, false
		}
	}
	return cur, true
}

// Valid reports whether path uses the supported syntax.
func Valid(path string) bool {
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
			next, ok := validBracket(p, i)
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

func validBracket(path string, start int) (int, bool) {
	if start+1 >= len(path) {
		return 0, false
	}
	i := start + 1
	if path[i] == '"' || path[i] == '\'' {
		_, end, ok := readQuoted(path, i)
		return end + 1, ok
	}
	res := readIndex(path, i)
	return res.next + 1, res.ok && res.index >= 0
}

func split(path string) []segment {
	var out []segment
	var buf strings.Builder
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '.':
			out = add(out, &buf)
		case '[':
			out = add(out, &buf)
			res := readSegment(path, i)
			if res.stop {
				return out
			}
			if res.ok {
				out = append(out, res.segment)
			}
			i = res.next
		default:
			buf.WriteByte(path[i])
		}
	}
	return add(out, &buf)
}

func add(out []segment, buf *strings.Builder) []segment {
	if buf.Len() == 0 {
		return out
	}
	out = append(out, segment{key: buf.String()})
	buf.Reset()
	return out
}

func readSegment(path string, i int) segmentResult {
	if i+1 >= len(path) {
		return segmentResult{stop: true}
	}
	i++
	if q := path[i]; q == '"' || q == '\'' {
		key, next, ok := readQuoted(path, i)
		if !ok {
			return segmentResult{stop: true}
		}
		return segmentResult{segment: segment{key: key}, next: next, ok: true}
	}
	res := readIndex(path, i)
	if res.stop {
		return segmentResult{stop: true}
	}
	if res.ok {
		return segmentResult{
			segment: segment{index: res.index, isIdx: true},
			next:    res.next,
			ok:      true,
		}
	}
	return segmentResult{next: res.next}
}

func readIndex(path string, i int) indexResult {
	j := i
	for j < len(path) && path[j] != ']' {
		j++
	}
	if j >= len(path) {
		return indexResult{stop: true}
	}
	idx, err := strconv.Atoi(strings.TrimSpace(path[i:j]))
	if err != nil {
		return indexResult{next: j}
	}
	return indexResult{index: idx, next: j, ok: true}
}

func readQuoted(path string, i int) (string, int, bool) {
	quote := path[i]
	i++
	var buf strings.Builder
	for i < len(path) {
		ch := path[i]
		if ch == '\\' {
			if i+1 >= len(path) {
				return "", 0, false
			}
			i++
			buf.WriteByte(unescape(path[i]))
			i++
			continue
		}
		if ch == quote {
			i = skipSpace(path, i+1)
			if i >= len(path) || path[i] != ']' {
				return "", 0, false
			}
			return buf.String(), i, true
		}
		buf.WriteByte(ch)
		i++
	}
	return "", 0, false
}

func unescape(b byte) byte {
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

func skipSpace(path string, i int) int {
	for i < len(path) {
		switch path[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}
