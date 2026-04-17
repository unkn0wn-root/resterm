package util

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func Trim(s string) string {
	return strings.TrimSpace(s)
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

func FirstTrimmed(values ...string) string {
	for _, value := range values {
		if value = Trim(value); value != "" {
			return value
		}
	}
	return ""
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
