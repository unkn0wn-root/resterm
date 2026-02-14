package parser

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

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

func firstOpt(opts map[string]string, keys ...string) (string, bool) {
	_, value, ok := firstOptWithKey(opts, keys...)
	return value, ok
}

func firstOptWithKey(opts map[string]string, keys ...string) (string, string, bool) {
	for _, key := range keys {
		value := strings.TrimSpace(opts[key])
		if value != "" {
			return key, value, true
		}
	}
	return "", "", false
}

func setOptBool(opt *restfile.Opt[bool], opts map[string]string, keys ...string) {
	for _, key := range keys {
		raw, ok := opts[key]
		if !ok {
			continue
		}
		opt.Set = true
		value := true
		if raw != "" {
			if parsed, ok := parseBool(raw); ok {
				value = parsed
			}
		}
		opt.Val = value
		return
	}
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
