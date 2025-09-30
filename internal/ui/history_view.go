package ui

import (
	"fmt"
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
	desc := fmt.Sprintf("%s %s [%s]", h.entry.Method, h.entry.URL, dur)
	if h.entry.Environment != "" {
		desc += fmt.Sprintf(" | env:%s", h.entry.Environment)
	}
	return desc
}

func (h historyItem) FilterValue() string {
	return h.entry.URL
}

func makeHistoryItems(entries []history.Entry) []list.Item {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = historyItem{entry: e}
	}
	return items
}
