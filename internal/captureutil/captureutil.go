package captureutil

import (
	"strings"
)

var strictKeys = []string{
	"capture.strict",
	"capture-strict",
	"capture_strict",
}

type strictAliasState struct {
	set      bool
	val      bool
	conflict bool
}

func IsLegacyTemplate(ex string) bool {
	s := strings.TrimSpace(ex)
	if s == "" {
		return false
	}
	var q byte
	esc := false
	for i := 0; i+1 < len(s); i++ {
		ch := s[i]
		if q != 0 {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
				continue
			}
			if ch == q {
				q = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			q = ch
			continue
		}
		if ch != '{' || s[i+1] != '{' {
			continue
		}
		if !strings.Contains(s[i+2:], "}}") {
			return false
		}
		return true
	}
	return false
}

func StrictEnabled(ss ...map[string]string) bool {
	v, ok := strictValue(ss...)
	return ok && v
}

func strictValue(ss ...map[string]string) (bool, bool) {
	set := false
	val := false
	for _, s := range ss {
		v, ok := strictFromMap(s)
		if !ok {
			continue
		}
		set = true
		val = v
	}
	return val, set
}

func strictFromMap(s map[string]string) (bool, bool) {
	if len(s) == 0 {
		return false, false
	}
	states := [3]strictAliasState{}
	for k, raw := range s {
		idx := strictKeyIdx(k)
		if idx < 0 {
			continue
		}
		b, ok := parseBool(raw)
		if !ok {
			continue
		}
		state := &states[idx]
		if !state.set {
			state.set = true
			state.val = b
			continue
		}
		if state.val != b {
			state.conflict = true
			state.val = false
		}
	}
	for i := range strictKeys {
		state := states[i]
		if !state.set {
			continue
		}
		if state.conflict {
			// Conflicting canonicalized declarations resolve to safe default.
			return false, true
		}
		return state.val, true
	}
	return false, false
}

func strictKeyIdx(k string) int {
	nk := strings.ToLower(strings.TrimSpace(k))
	for i := range strictKeys {
		if nk == strictKeys[i] {
			return i
		}
	}
	return -1
}

func parseBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "t", "1", "yes", "on":
		return true, true
	case "false", "f", "0", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func SuspiciousJSONDoubleDot(ex string) bool {
	s := strings.TrimSpace(ex)
	if s == "" {
		return false
	}
	var q byte
	esc := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if q != 0 {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
				continue
			}
			if ch == q {
				q = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			q = ch
			continue
		}
		if hasJSONDoubleDotPrefix(s, i) {
			return true
		}
	}
	return false
}

func hasJSONDoubleDotPrefix(s string, i int) bool {
	ps := []string{"response.json..", "last.json.."}
	for _, p := range ps {
		if !prefixFold(s, i, p) {
			continue
		}
		if i > 0 {
			c := s[i-1]
			if ident(c) || c == '.' {
				continue
			}
		}
		return true
	}
	return false
}

func prefixFold(s string, i int, p string) bool {
	n := len(p)
	if i+n > len(s) {
		return false
	}
	return strings.EqualFold(s[i:i+n], p)
}

func ident(b byte) bool {
	if b >= 'a' && b <= 'z' {
		return true
	}
	if b >= 'A' && b <= 'Z' {
		return true
	}
	if b >= '0' && b <= '9' {
		return true
	}
	return b == '_'
}
