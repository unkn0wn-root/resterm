package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"

	"github.com/unkn0wn-root/resterm/internal/history"
)

type historyItem struct {
	entry history.Entry
}

func (h historyItem) Title() string {
	ts := h.entry.ExecutedAt.Format("15:04:05")
	return fmt.Sprintf("%s %s (%d)", ts, h.entry.Status, h.entry.StatusCode)
}

func (h historyItem) Description() string {
	dur := h.entry.Duration.Truncate(time.Millisecond)
	base := fmt.Sprintf("%s %s [%s]", h.entry.Method, h.entry.URL, dur)
	if env := strings.TrimSpace(h.entry.Environment); env != "" {
		base = fmt.Sprintf("%s | env:%s", base, env)
	}
	var lines []string
	if desc := strings.TrimSpace(h.entry.Description); desc != "" {
		lines = append(lines, condense(desc, 80))
	}
	if tags := joinTags(h.entry.Tags, 5); tags != "" {
		lines = append(lines, tags)
	}
	lines = append(lines, base)
	return strings.Join(lines, "\n")
}

func (h historyItem) FilterValue() string {
	parts := []string{
		h.entry.URL,
		h.entry.Method,
		h.entry.Description,
		strings.Join(h.entry.Tags, " "),
		h.entry.Environment,
	}
	return strings.Join(parts, " ")
}

func makeHistoryItems(entries []history.Entry) []list.Item {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = historyItem{entry: e}
	}
	return items
}
