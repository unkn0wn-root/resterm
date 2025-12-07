package hint

import "strings"

func Anchor(runes []rune, caret int) int {
	for i := caret - 1; i >= 0; i-- {
		switch runes[i] {
		case '@':
			return i
		case '\n':
			return -1
		}
	}
	return -1
}

func InDirectiveContext(runes []rune, anchor int) bool {
	if anchor <= 0 {
		return false
	}
	lineStart := anchor
	for lineStart > 0 && runes[lineStart-1] != '\n' {
		lineStart--
	}
	prefix := strings.TrimSpace(string(runes[lineStart:anchor]))
	if prefix == "" {
		return false
	}
	if strings.HasSuffix(prefix, "#") || strings.HasSuffix(prefix, "*") {
		return true
	}
	if strings.HasSuffix(prefix, "//") || strings.HasSuffix(prefix, "/*") {
		return true
	}
	return false
}
