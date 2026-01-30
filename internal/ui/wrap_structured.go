package ui

import (
	"context"
	"strings"
	"unicode/utf8"
)

const wrapContinuationUnit = "  "

func wrapStructuredSegmentsCtx(
	ctx context.Context,
	line string,
	width int,
	prefix string,
	prefixWidth int,
) ([]string, bool) {
	if ctxDone(ctx) {
		return nil, false
	}
	if width <= 0 {
		return []string{line}, true
	}
	if line == "" {
		return []string{""}, true
	}
	if visibleWidth(line) <= width && prefix == "" {
		return []string{line}, true
	}

	tokens, ok := tokenizeLineCtx(ctx, line)
	if !ok {
		return nil, false
	}
	if len(tokens) == 0 {
		return []string{""}, true
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
		if ctxDone(ctx) {
			return nil, false
		}
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
			if ctxDone(ctx) {
				return nil, false
			}
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

			segment, rest, ok := splitSegmentCtx(ctx, text, available)
			if !ok {
				return nil, false
			}
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
		return []string{""}, true
	}
	return segments, true
}

func wrapStructuredContent(content string, width int) string {
	out, _ := wrapStructuredContentCtx(context.Background(), content, width)
	return out
}

func wrapStructuredContentCtx(ctx context.Context, content string, width int) (string, bool) {
	if width <= 0 {
		return content, true
	}
	if ctxDone(ctx) {
		return "", false
	}
	lines := strings.Split(content, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		if ctxDone(ctx) {
			return "", false
		}
		segments, ok := wrapStructuredLineCtx(ctx, line, width)
		if !ok {
			return "", false
		}
		wrapped = append(wrapped, segments...)
	}
	return strings.Join(wrapped, "\n"), true
}

func wrapStructuredLine(line string, width int) []string {
	segments, _ := wrapStructuredLineCtx(context.Background(), line, width)
	return segments
}

func wrapStructuredLineCtx(ctx context.Context, line string, width int) ([]string, bool) {
	if ctxDone(ctx) {
		return nil, false
	}
	if width <= 0 {
		return []string{line}, true
	}
	prefix, prefixWidth := structuredContinuationPrefix(line, width)
	return wrapStructuredSegmentsCtx(ctx, line, width, prefix, prefixWidth)
}

func structuredContinuationPrefix(line string, width int) (string, int) {
	indent := leadingWhitespaceWithANSI(line)
	indentWidth := visibleWidth(indent)

	unit := wrapContinuationUnit
	unitWidth := visibleWidth(unit)
	if unitWidth == 0 {
		unitWidth = 2
	}

	if indentWidth >= width {
		return "", 0
	}

	if indentWidth+unitWidth < width {
		return indent + unit, indentWidth + unitWidth
	}

	return indent, indentWidth
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
