package str

import (
	"strings"
	"unicode"
)

func TrimLeft(s string) string {
	return strings.TrimLeftFunc(s, unicode.IsSpace)
}

func TrimRight(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}

func UpperTrim(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func LowerTrim(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func FirstTrimmed(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

// FoldLines trims each line, drops blank lines, and joins the rest with spaces.
// A string without line breaks is returned unchanged.
func FoldLines(s string) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	var out []string
	for ln := range strings.SplitSeq(s, "\n") {
		if ln = strings.TrimSpace(ln); ln != "" {
			out = append(out, ln)
		}
	}
	return strings.Join(out, " ")
}

func AllBlank(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return false
		}
	}
	return true
}
