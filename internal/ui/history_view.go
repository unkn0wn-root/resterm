package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"

	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type historyItem struct {
	entry history.Entry
}

func (h historyItem) Title() string {
	ts := h.entry.ExecutedAt.Format("15:04:05")
	if h.entry.Method == restfile.HistoryMethodCompare && h.entry.Compare != nil {
		count := len(h.entry.Compare.Results)
		return fmt.Sprintf("%s Compare (%d env)", ts, count)
	}
	return fmt.Sprintf("%s %s (%d)", ts, h.entry.Status, h.entry.StatusCode)
}

func (h historyItem) Description() string {
	dur := h.entry.Duration.Truncate(time.Millisecond)
	base := fmt.Sprintf("%s %s [%s]", h.entry.Method, h.entry.URL, dur)
	if env := strings.TrimSpace(h.entry.Environment); env != "" {
		base = fmt.Sprintf("%s | env:%s", base, env)
	}
	if h.entry.Method == restfile.HistoryMethodCompare && h.entry.Compare != nil {
		base = fmt.Sprintf("%s | %s", base, compareSummary(h.entry))
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
	if h.entry.Compare != nil {
		for _, res := range h.entry.Compare.Results {
			parts = append(parts, res.Environment, res.Status)
		}
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

func compareSummary(entry history.Entry) string {
	if entry.Compare == nil || len(entry.Compare.Results) == 0 {
		return "compare: none"
	}
	segments := make([]string, 0, len(entry.Compare.Results))
	for _, res := range entry.Compare.Results {
		label := res.Environment
		if entry.Compare.Baseline != "" &&
			strings.EqualFold(entry.Compare.Baseline, res.Environment) {
			label += "*"
		}
		status := strings.TrimSpace(res.Status)
		if status == "" && res.StatusCode > 0 {
			status = fmt.Sprintf("%d", res.StatusCode)
		}
		if status == "" {
			status = "pending"
		}
		segments = append(segments, fmt.Sprintf("%s:%s", label, status))
	}
	return "compare " + strings.Join(segments, " ")
}
