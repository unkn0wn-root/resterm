package directive

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// The second result is false when raw is not one of Resterm's boolean spellings.
func ParseBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "t", "1", "yes", "on":
		return true, true
	case "false", "f", "0", "no", "off":
		return false, true
	default:
		return false, false
	}
}

// Feature directives also accept "disable" and "disabled", which are not
// general boolean values. Every false spelling counts as off.
func IsOff(raw string) bool {
	if val, ok := ParseBool(raw); ok {
		return !val
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "disable", "disabled":
		return true
	default:
		return false
	}
}

// Zero is valid because this parser is used for limits where zero means disabled.
func ParseNonNegativeInt(raw string) (int, error) {
	tr := strings.TrimSpace(raw)
	if tr == "" {
		return 0, errors.New("empty value")
	}
	n, err := strconv.Atoi(tr)
	if err != nil {
		return 0, fmt.Errorf("value must be a number: %q", tr)
	}
	if n < 0 {
		return 0, fmt.Errorf("value must be non-negative: %d", n)
	}
	return n, nil
}

func SplitCSV(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func TrimQuotes(value string) string {
	if len(value) < 2 {
		return value
	}
	first := value[0]
	last := value[len(value)-1]
	if first == '"' && last == '"' || first == '\'' && last == '\'' {
		return value[1 : len(value)-1]
	}
	return value
}

// Identifiers use an ASCII letter or underscore first, then letters, digits, or underscores.
func IsIdent(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if !isIdentStart(r) {
				return false
			}
			continue
		}
		if !IsIdentRune(r) {
			return false
		}
	}
	return true
}

func IsIdentRune(r rune) bool {
	return isIdentStart(r) || isDigit(r)
}

// Option, setting, and variable names also allow the separators that
// identifiers reject.
func IsKeyRune(r rune) bool {
	return IsIdentRune(r) || r == '-' || r == '.'
}

func isIdentStart(r rune) bool {
	return r == '_' || isAlpha(r)
}

func isAlpha(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}
