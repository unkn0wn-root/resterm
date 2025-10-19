package ui

import (
	"strings"
	"unicode/utf8"
)

const wrapContinuationUnit = "    "

func wrapStructuredSegments(line string, width int, prefix string, prefixWidth int) []string {
	if width <= 0 {
		return []string{line}
	}
	if line == "" {
		return []string{""}
	}
	if visibleWidth(line) <= width && prefix == "" {
		return []string{line}
	}

	tokens := tokenizeLine(line)
	if len(tokens) == 0 {
		return []string{""}
	}

	var current strings.Builder
	segments := make([]string, 0, len(tokens))
	currentWidth := 0
	lineHasNonSpace := false
	segmentHasContent := false
	pendingANSIPrefix := ""

	appendSegment := func(segment string) {
		if segment == "" {
			return
		}
		trimmed := strings.TrimRight(segment, " ")
		if trimmed != "" {
			segment = trimmed
		}
		segments = append(segments, segment)
	}

	emit := func(startContinuation bool) {
		if segmentHasContent {
			appendSegment(current.String())
		}
		current.Reset()
		currentWidth = 0
		segmentHasContent = false
		if startContinuation && prefix != "" {
			current.WriteString(prefix)
			currentWidth = prefixWidth
		}
	}

	for _, tok := range tokens {
		text := tok.text
		if pendingANSIPrefix != "" {
			text = pendingANSIPrefix + text
			pendingANSIPrefix = ""
		}
		if tok.isSpace {
			clean, suffix := detachTrailingANSIPrefix(text)
			if suffix != "" {
				text = clean
				pendingANSIPrefix = suffix
			}
		}
		tokWidth := tok.width
		if text == "" {
			continue
		}
		tokenANSIPrefix := ansiPrefix(text)

		if tokWidth == 0 {
			current.WriteString(text)
			if text != "" {
				segmentHasContent = true
			}
			continue
		}

		if !segmentHasContent && tok.isSpace && lineHasNonSpace {
			continue
		}

		for {
			available := width - currentWidth
			if available <= 0 {
				emit(true)
				available = width - currentWidth
			}
			if available <= 0 {
				appendSegment(text)
				current.Reset()
				currentWidth = 0
				segmentHasContent = false
				break
			}

			if tokWidth <= available {
				current.WriteString(text)
				segmentHasContent = true
				currentWidth += tokWidth
				if !tok.isSpace {
					lineHasNonSpace = true
				}
				break
			}

			segment, rest := splitSegment(text, available)
			if segment == "" {
				segment = text
				rest = ""
			}
			current.WriteString(segment)
			segmentHasContent = true
			currentWidth += visibleWidth(segment)
			if !tok.isSpace {
				lineHasNonSpace = true
			}
			emit(true)
			if rest == "" {
				break
			}
			noProgress := rest == text
			rest = ensureANSIPrefix(rest, tokenANSIPrefix)
			if noProgress {
				current.WriteString(rest)
				segmentHasContent = true
				emit(true)
				break
			}
			text = rest
			tokenANSIPrefix = ansiPrefix(text)
			tokWidth = visibleWidth(text)
			if tokWidth == 0 {
				break
			}
		}
	}

	if segmentHasContent {
		emit(false)
	}

	if len(segments) == 0 {
		return []string{""}
	}
	return segments
}

func wrapStructuredContent(content string, width int) string {
	if width <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		segments := wrapStructuredLine(line, width)
		wrapped = append(wrapped, segments...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapStructuredLine(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	prefix, prefixWidth := structuredContinuationPrefix(line, width)
	return wrapStructuredSegments(line, width, prefix, prefixWidth)
}

func structuredContinuationPrefix(line string, width int) (string, int) {
	indent := leadingWhitespaceWithANSI(line)
	indentWidth := visibleWidth(indent)

	unit := wrapContinuationUnit
	unitWidth := visibleWidth(unit)
	if unitWidth == 0 {
		unitWidth = 4
	}

	prefix := indent + unit
	prefixWidth := indentWidth + unitWidth

	if prefixWidth >= width {
		prefix = indent
		prefixWidth = indentWidth
		if prefixWidth >= width {
			prefix = ""
			prefixWidth = 0
		}
	}

	if prefix == "" && unitWidth < width {
		prefix = unit
		prefixWidth = unitWidth
		if prefixWidth >= width {
			prefix = ""
			prefixWidth = 0
		}
	}

	return prefix, prefixWidth
}

func leadingWhitespaceWithANSI(line string) string {
	if line == "" {
		return ""
	}
	var builder strings.Builder
	index := 0
	for index < len(line) {
		if loc := ansiSequenceRegex.FindStringIndex(line[index:]); loc != nil && loc[0] == 0 {
			builder.WriteString(line[index : index+loc[1]])
			index += loc[1]
			continue
		}
		r, size := utf8.DecodeRuneInString(line[index:])
		if size <= 0 {
			size = 1
		}
		if r == ' ' || r == '\t' {
			builder.WriteString(line[index : index+size])
			index += size
			continue
		}
		break
	}
	clean, _ := detachTrailingANSIPrefix(builder.String())
	return clean
}

func ansiPrefix(text string) string {
	if text == "" {
		return ""
	}
	var builder strings.Builder
	index := 0
	for index < len(text) {
		if loc := ansiSequenceRegex.FindStringIndex(text[index:]); loc != nil && loc[0] == 0 {
			builder.WriteString(text[index : index+loc[1]])
			index += loc[1]
			continue
		}
		break
	}
	return builder.String()
}

func ensureANSIPrefix(text, prefix string) string {
	if prefix == "" || text == "" {
		return text
	}
	if strings.HasPrefix(text, prefix) {
		return text
	}
	return prefix + text
}

func detachTrailingANSIPrefix(text string) (string, string) {
	if text == "" {
		return "", ""
	}
	remaining := text
	prefix := ""
	for {
		indices := ansiSequenceRegex.FindAllStringIndex(remaining, -1)
		if len(indices) == 0 {
			break
		}
		last := indices[len(indices)-1]
		if last[1] != len(remaining) {
			break
		}
		code := remaining[last[0]:last[1]]
		prefix = code + prefix
		remaining = remaining[:last[0]]
	}
	return remaining, prefix
}
