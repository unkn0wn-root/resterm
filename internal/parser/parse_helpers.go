package parser

import "strings"

func trim(s string) string {
	return strings.TrimSpace(s)
}

func normKey(s string) string {
	return strings.ToLower(trim(s))
}

func popOpt(opts map[string]string, key string) string {
	if len(opts) == 0 {
		return ""
	}
	val, ok := opts[key]
	if !ok {
		return ""
	}
	delete(opts, key)
	return trim(val)
}

func popOptAny(opts map[string]string, keys ...string) string {
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
			out = trim(val)
		}
		delete(opts, key)
	}
	return out
}

func splitCSV(s string) []string {
	s = trim(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = trim(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isOffToken(s string) bool {
	switch normKey(s) {
	case "0", "false", "off", "disable", "disabled":
		return true
	default:
		return false
	}
}

func cloneSlice[T any](in []T) []T {
	return append([]T(nil), in...)
}
