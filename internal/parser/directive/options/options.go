package options

import (
	"regexp"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/parser/directive/lex"
	dvalue "github.com/unkn0wn-root/resterm/internal/parser/directive/value"
)

var nameValueRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+)(?:\s*(?::|=)\s*(.*?)|\s+(\S.*))?$`)

func Parse(input string) map[string]string {
	tr := strings.TrimSpace(input)
	if tr == "" {
		return map[string]string{}
	}
	tos := lex.TokenizeFieldsEscaped(tr)
	if len(tos) == 0 {
		return map[string]string{}
	}
	ops := make(map[string]string, len(tos))
	for _, t := range tos {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		key := t
		val := "true"
		if bef, af, ok := strings.Cut(t, "="); ok {
			key = strings.TrimSpace(bef)
			val = strings.TrimSpace(af)
		}
		if key == "" {
			continue
		}
		ops[strings.ToLower(key)] = lex.TrimQuotes(val)
	}
	return ops
}

func ParseFields(fields []string) map[string]string {
	pairs := map[string]string{}
	for _, f := range fields {
		if f == "" {
			continue
		}
		if bef, af, ok := strings.Cut(f, "="); ok {
			key := strings.TrimSpace(bef)
			val := strings.TrimSpace(af)
			if key == "" {
				continue
			}
			pairs[strings.ToLower(key)] = lex.TrimQuotes(val)
		}
	}
	return pairs
}

func ParseNameValue(input string) (string, string) {
	tr := strings.TrimSpace(input)
	if tr == "" {
		return "", ""
	}
	m := nameValueRe.FindStringSubmatch(tr)
	if m == nil {
		return "", ""
	}
	name := m[1]
	valC := m[2]
	if valC == "" {
		valC = m[3]
	}
	return name, strings.TrimSpace(valC)
}

func IsToken(token string) bool {
	idx := strings.Index(token, "=")
	if idx <= 0 {
		return false
	}
	key := token[:idx]
	for _, r := range key {
		if !isKeyRune(r) {
			return false
		}
	}
	return true
}

func isKeyRune(r rune) bool {
	switch {
	case r == '_', r == '-', r == '.':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= 'a' && r <= 'z':
		return true
	case r >= '0' && r <= '9':
		return true
	default:
		return false
	}
}

func Pop(opts map[string]string, key string) string {
	if len(opts) == 0 {
		return ""
	}
	val, ok := opts[key]
	if !ok {
		return ""
	}
	delete(opts, key)
	return strings.TrimSpace(val)
}

func PopAny(opts map[string]string, keys ...string) string {
	if len(opts) == 0 {
		return ""
	}
	out := ""
	for _, key := range keys {
		val, ok := opts[key]
		if !ok {
			continue
		}
		if out == "" {
			out = strings.TrimSpace(val)
		}
		delete(opts, key)
	}
	return out
}

func First(opts map[string]string, keys ...string) (string, bool) {
	_, value, ok := FirstWithKey(opts, keys...)
	return value, ok
}

func FirstWithKey(opts map[string]string, keys ...string) (string, string, bool) {
	for _, key := range keys {
		value := strings.TrimSpace(opts[key])
		if value != "" {
			return key, value, true
		}
	}
	return "", "", false
}

func Bool(opts map[string]string, keys ...string) (bool, bool) {
	for _, key := range keys {
		raw, ok := opts[key]
		if !ok {
			continue
		}
		if raw == "" {
			return true, true
		}
		if parsed, ok := dvalue.ParseBool(raw); ok {
			return parsed, true
		}
		return true, true
	}
	return false, false
}

func SplitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
