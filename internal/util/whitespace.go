package util

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func TrimLeftSpace(s string) string {
	return strings.TrimLeftFunc(s, unicode.IsSpace)
}

func TrimRightSpace(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}

func TrimLeadingSpaceOnce(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	if unicode.IsSpace(r) {
		return s[size:]
	}
	return s
}
