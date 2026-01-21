package httpver

import "strings"

const Key = "http-version"

type Version int

const (
	Unknown Version = iota
	V10
	V11
	V2
)

func ParseToken(raw string) (Version, bool) {
	return parse(raw, false)
}

func ParseValue(raw string) (Version, bool) {
	return parse(raw, true)
}

func SplitToken(fields []string) ([]string, Version) {
	if len(fields) == 0 {
		return fields, Unknown
	}
	v, ok := ParseToken(fields[len(fields)-1])
	if !ok {
		return fields, Unknown
	}
	return fields[:len(fields)-1], v
}

func Format(v Version) string {
	switch v {
	case V10:
		return "1.0"
	case V11:
		return "1.1"
	case V2:
		return "2"
	default:
		return ""
	}
}

func SetIfMissing(m map[string]string, v Version) map[string]string {
	if v == Unknown {
		return m
	}
	if m == nil {
		m = make(map[string]string)
	}
	if hasKey(m, Key) {
		return m
	}
	if val := Format(v); val != "" {
		m[Key] = val
	}
	return m
}

func parse(raw string, allowBare bool) (Version, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Unknown, false
	}
	s = strings.ToLower(s)
	if strings.HasPrefix(s, "http/") {
		s = strings.TrimPrefix(s, "http/")
	} else if !allowBare {
		return Unknown, false
	}
	switch s {
	case "1.0":
		return V10, true
	case "1.1":
		return V11, true
	case "2", "2.0":
		return V2, true
	default:
		return Unknown, false
	}
}

func hasKey(m map[string]string, key string) bool {
	for k := range m {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}
