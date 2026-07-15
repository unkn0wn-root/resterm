package ui

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func prettifyBody(body []byte, contentType string) string {
	return bodyfmt.Prettify(
		body,
		contentType,
		bodyfmt.PrettyOptions{Color: termcolor.TrueColor()},
	)
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if limit < 4 || len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit-3])) + "..."
}
