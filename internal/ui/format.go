package ui

import (
	"context"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/bodyfmt"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/termtext"
)

func prettifyBody(body []byte, contentType string) string {
	return bodyfmt.Prettify(
		context.Background(),
		body,
		contentType,
		bodyfmt.PrettyOptions{Color: termcolor.TrueColor(), Form: bodyfmt.Display},
	)
}

func oneLine(value string) string {
	return termtext.Row(strings.Join(strings.Fields(value), " "))
}

// displayLines escapes terminal controls but preserves line breaks.
func displayLines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if !strings.Contains(value, "\n") {
		return termtext.Row(value)
	}

	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = termtext.Row(line)
	}
	return strings.Join(lines, "\n")
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if limit < 4 || len(runes) <= limit {
		return value
	}
	return strings.TrimSpace(string(runes[:limit-3])) + "..."
}
