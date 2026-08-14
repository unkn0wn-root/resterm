package ui

import "strings"

const (
	noResponseWordmarkRow    = 1
	noResponseWordmarkHeight = 3
)

var (
	noResponseLogo          = strings.Split(noResponseMessage, "\n")
	noResponseWordmarkWidth = visibleWidth(noResponseLogo[noResponseWordmarkRow])
)

// logoPlaceholder wraps and centers the empty-response logo.
func logoPlaceholder(width, height int) string {
	if noResponseMessage == "" {
		return ""
	}
	content := noResponseMessage
	if width > 0 {
		content = wrapToWidth(content, width)
	}
	return centerLogoContent(content, width, height)
}

// centerLogoContent centers the wordmark while preserving its decorations.
func centerLogoContent(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	wide := 0
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
		wide = max(wide, visibleWidth(lines[i]))
	}

	// Wrapped art no longer has stable wordmark rows.
	row, rows := noResponseWordmarkRow, noResponseWordmarkHeight
	if len(lines) != len(noResponseLogo) {
		row, rows = 0, len(lines)
	}

	// Keep the widest row inside the pane.
	left := max(min((width-noResponseWordmarkWidth)/2, width-wide), 0)
	if left > 0 {
		pad := strings.Repeat(" ", left)
		for i, line := range lines {
			lines[i] = pad + line
		}
	}

	if top := max((height-rows)/2-row, 0); top > 0 {
		lines = append(make([]string, top, top+len(lines)), lines...)
	}

	return strings.Join(lines, "\n")
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
