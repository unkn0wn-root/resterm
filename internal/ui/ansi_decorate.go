package ui

import (
	"strings"
	"unicode/utf8"
)

// applyANSIAwareWrap wraps segment in prefix/suffix, re-applying prefix after
// inner SGR sequences so embedded styles cannot cancel the outer decoration.
// restore is appended after suffix to resume the style active after segment.
func applyANSIAwareWrap(segment, prefix, suffix, restore string) string {
	if prefix == "" {
		return segment
	}

	trailing := suffix + restore
	if segment == "" {
		return prefix + trailing
	}
	if !ansiSequenceRegex.MatchString(segment) {
		return prefix + segment + trailing
	}

	var builder strings.Builder
	builder.Grow(len(segment) + len(prefix)*2 + len(trailing))
	builder.WriteString(prefix)
	for i := 0; i < len(segment); {
		if segment[i] == '\x1b' {
			if seq, size := ansiSequenceAt(segment, i); size > 0 {
				builder.WriteString(seq)
				if isSGR(seq) {
					builder.WriteString(prefix)
				}
				i += size
				continue
			}
		}

		_, size := utf8.DecodeRuneInString(segment[i:])
		if size <= 0 {
			size = 1
		}
		builder.WriteString(segment[i : i+size])
		i += size
	}
	builder.WriteString(trailing)
	return builder.String()
}

func ansiSequenceAt(content string, index int) (string, int) {
	if index < 0 || index >= len(content) || content[index] != '\x1b' {
		return "", 0
	}
	loc := ansiSequenceRegex.FindStringIndex(content[index:])
	if loc == nil || loc[0] != 0 {
		return "", 0
	}
	return content[index : index+loc[1]], loc[1]
}
