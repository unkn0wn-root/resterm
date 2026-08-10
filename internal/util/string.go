package util

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func Trim(s string) string {
	return strings.TrimSpace(s)
}

func Lower(s string) string {
	return strings.ToLower(s)
}

func TrimLeft(s string) string {
	return strings.TrimLeftFunc(s, unicode.IsSpace)
}

func TrimRight(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}

func UpperTrim(s string) string {
	return strings.ToUpper(Trim(s))
}

func LowerTrim(s string) string {
	return strings.ToLower(Trim(s))
}

func Contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func HasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

func FirstTrimmed(values ...string) string {
	for _, value := range values {
		if value = Trim(value); value != "" {
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

func TrimLeadingOnce(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	if unicode.IsSpace(r) {
		return s[size:]
	}
	return s
}
