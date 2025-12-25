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

func jsonGet(v any, path string) (any, bool) {
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
			if buf.Len() > 0 {
				out = append(out, jseg{key: buf.String()})
				buf.Reset()
			}
		case '[':
			if buf.Len() > 0 {
				out = append(out, jseg{key: buf.String()})
				buf.Reset()
			}
			j := i + 1
			for j < len(p) && p[j] != ']' {
				j++
			}
			if j >= len(p) {
				return out
			}
			idx, err := strconv.Atoi(p[i+1 : j])
			if err == nil {
				out = append(out, jseg{idx: idx, isI: true})
			}
			i = j
		default:
			buf.WriteByte(ch)
		}
	}
	if buf.Len() > 0 {
		out = append(out, jseg{key: buf.String()})
	}
	return out
}
