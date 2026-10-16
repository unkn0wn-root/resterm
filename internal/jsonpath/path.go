package jsonpath

import (
	"errors"
	"fmt"
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

// A nil or empty Path refers to the root.
type Path []segment

// Miss.At includes the segment where lookup failed.
type Miss struct {
	At     Path
	Reason string
}

func (m *Miss) Error() string {
	return m.At.Format("$") + ": " + m.Reason
}

func Parse(path string) (Path, error) {
	s := strings.TrimSpace(path)
	var p Path
	var err error
	if rest, ok := strings.CutPrefix(s, "$"); ok {
		s = rest
	} else if s != "" && s[0] != '[' {
		var key string
		if key, s, err = cutKey(s); err != nil {
			return nil, err
		}
		p = append(p, segment{key: key})
	}
	for s != "" {
		var seg segment
		switch {
		case strings.HasPrefix(s, ".["):
			s = s[1:]
			continue
		case s[0] == '.':
			seg.key, s, err = cutKey(s[1:])
		case s[0] == '[':
			seg, s, err = cutBracket(s[1:])
		default:
			r, _ := utf8.DecodeRuneInString(s)
			return nil, fmt.Errorf("expected '.' or '[', got %q", r)
		}
		if err != nil {
			return nil, err
		}
		p = append(p, seg)
	}
	return p, nil
}

func cutKey(s string) (string, string, error) {
	n := strings.IndexAny(s, ".[]")
	if n < 0 {
		n = len(s)
	}
	key := s[:n]
	switch {
	case key == "":
		return "", "", errors.New("empty path segment")
	case strings.ContainsFunc(key, unicode.IsSpace):
		return "", "", fmt.Errorf("unquoted key %q has a space. Use %s", key, Key("", key))
	case strings.ContainsFunc(key, unicode.IsControl):
		return "", "", fmt.Errorf("unquoted key %q has a control character. Use a quoted key", key)
	}
	return key, s[n:], nil
}

var (
	keyQuoter = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	// Other escapes drop the backslash, so \q becomes q.
	keyEscapes = map[byte]byte{'n': '\n', 'r': '\r', 't': '\t'}
)

func cutBracket(s string) (segment, string, error) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	if s == "" || s[0] != '"' && s[0] != '\'' {
		idx, rest, ok := strings.Cut(s, "]")
		if !ok {
			return segment{}, "", errors.New("missing closing bracket")
		}
		idx = strings.TrimSpace(idx)
		if idx == "" {
			return segment{}, "", errors.New("empty array index")
		}
		i, err := strconv.Atoi(idx)
		if err != nil {
			return segment{}, "", fmt.Errorf("invalid array index %q", idx)
		}
		return segment{index: i, isIdx: true}, rest, nil
	}

	q := s[0]
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c == q:
			rest, ok := strings.CutPrefix(strings.TrimLeftFunc(s[i+1:], unicode.IsSpace), "]")
			if !ok {
				return segment{}, "", errors.New("missing closing bracket")
			}
			return segment{key: b.String()}, rest, nil
		case c == '\\' && i+1 < len(s):
			i++
			c = s[i]
			if e, ok := keyEscapes[c]; ok {
				c = e
			}
		}
		b.WriteByte(c)
	}
	return segment{}, "", errors.New("missing closing quote")
}

func (p Path) Format(root string) string {
	s := root
	for _, seg := range p {
		if seg.isIdx {
			s += "[" + strconv.Itoa(seg.index) + "]"
		} else {
			s = Key(s, seg.key)
		}
	}
	return s
}

func Get(v any, path string) (any, error) {
	p, err := Parse(path)
	if err != nil {
		return nil, err
	}
	return p.Get(v)
}

func (p Path) Get(v any) (any, error) {
	return Walk(v, p, func(v any) (map[string]any, bool) {
		m, ok := v.(map[string]any)
		return m, ok
	}, func(v any) ([]any, bool) {
		l, ok := v.([]any)
		return l, ok
	})
}

func Walk[T any](v T, p Path, dict func(T) (map[string]T, bool), list func(T) ([]T, bool)) (T, error) {
	var zero T
	for i, seg := range p {
		m, isDict := dict(v)
		l, isList := list(v)
		var reason string
		switch {
		case isDict && !seg.isIdx:
			if it, ok := m[seg.key]; ok {
				v = it
				continue
			}
			reason = "key not found"
		case isList && seg.isIdx:
			n := seg.index
			if n < 0 {
				n += len(l)
			}
			if 0 <= n && n < len(l) {
				v = l[n]
				continue
			}
			reason = fmt.Sprintf("index %d out of range", seg.index)
		case isList:
			reason = "cannot access object key on array"
		case isDict:
			reason = "cannot index an object"
		default:
			reason = "cannot traverse non-container value"
		}
		return zero, &Miss{At: p[:i+1], Reason: reason}
	}
	return v, nil
}

func Key(path, key string) string {
	plain := key != ""
	for i := 0; plain && i < len(key); i++ {
		c := key[i]
		letter := c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
		digit := '0' <= c && c <= '9'
		plain = letter || i > 0 && digit
	}
	if plain {
		return path + "." + key
	}
	return path + `["` + keyQuoter.Replace(key) + `"]`
}
