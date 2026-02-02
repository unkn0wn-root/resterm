package ui

import "strings"

// we need this so we can return the centered logo without passing through the
// response wrapping pipeline. This preserves the leading padding that centers
// the logo, which can otherwise be stripped during structured wrapping.
func logoPlaceholder(width, height int) string {
	if noResponseMessage == "" {
		return ""
	}
	content := noResponseMessage
	if width > 0 {
		// wrap before centering so the centering padding isn't stripped by wrapping.
		content = wrapToWidth(content, width)
	}
	return centerContent(content, width, height)
}

func logoPlaceholderCache(width, height int) cachedWrap {
	content := logoPlaceholder(width, height)
	spans, rev := mapNoWrapLines(content)
	return cachedWrap{
		width:   width,
		content: content,
		valid:   true,
		spans:   spans,
		rev:     rev,
	}
}

// mapNoWrapLines builds a 1:1 line mapping for content that is already wrapped.
func mapNoWrapLines(content string) ([]lineSpan, []int) {
	if content == "" {
		return nil, nil
	}
	lines := strings.Split(content, "\n")
	spans := make([]lineSpan, len(lines))
	rev := make([]int, len(lines))
	for i := range lines {
		spans[i] = lineSpan{start: i, end: i}
		rev[i] = i
	}
	return spans, rev
}
