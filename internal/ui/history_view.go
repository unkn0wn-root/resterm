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
	scope historyScope
}

const (
	historyDateFormat   = "02-01-2006"
	historyTimeFormat   = "15:04:05"
	historyUnnamedLabel = "unnamed"
)

type historyTitleParts struct {
	line   string
	prefix string
	code   string
	suffix string
}

func buildHistoryTitleParts(item historyItem) historyTitleParts {
	return buildHistoryTitlePartsAt(item, time.Now())
}

func buildHistoryTitlePartsAt(item historyItem, now time.Time) historyTitleParts {
	entry := item.entry
	ts := historyTimestampLabel(entry.ExecutedAt, now)
	if entry.Method == restfile.HistoryMethodCompare && entry.Compare != nil {
		count := len(entry.Compare.Results)
		return historyTitleParts{line: fmt.Sprintf("%s Compare (%d env)", ts, count)}
	}
	label := historyTitleLabel(item)
	return historyTitleParts{
		prefix: fmt.Sprintf("%s %s (", ts, label),
		code:   fmt.Sprintf("%d", entry.StatusCode),
		suffix: ")",
	}
}

func historyTitleLabel(item historyItem) string {
	entry := item.entry
	scope := item.scope
	status := strings.TrimSpace(entry.Status)
	// Treat workflow entries as workflow scope even in Global/File lists.
	if entry.Method == restfile.HistoryMethodWorkflow {
		scope = historyScopeWorkflow
	}
	switch scope {
	case historyScopeRequest, historyScopeWorkflow:
		return status
	}
	name := strings.TrimSpace(entry.RequestName)
	url := strings.TrimSpace(entry.URL)
	if name != "" && (url == "" || !strings.EqualFold(name, url)) {
		return name
	}
	if status == "" {
		return historyUnnamedLabel
	}
	return fmt.Sprintf("%s (%s)", status, historyUnnamedLabel)
}

func historyTitleText(item historyItem) string {
	parts := buildHistoryTitleParts(item)
	if parts.line != "" {
		return parts.line
	}
	return parts.prefix + parts.code + parts.suffix
}

func historyTimestampLabel(at, now time.Time) string {
	if at.IsZero() {
		return ""
	}
	if now.IsZero() {
		now = time.Now()
	}
	loc := now.Location()
	at = at.In(loc)
	now = now.In(loc)
	if sameDay(at, now) {
		return at.Format(historyTimeFormat)
	}
	return fmt.Sprintf("%s %s", at.Format(historyDateFormat), at.Format(historyTimeFormat))
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func historyBaseLine(entry history.Entry) string {
	dur := entry.Duration.Truncate(time.Millisecond)
	base := fmt.Sprintf("%s %s [%s]", entry.Method, entry.URL, dur)
	if env := strings.TrimSpace(entry.Environment); env != "" {
		base = fmt.Sprintf("%s | env:%s", base, env)
	}
	if entry.Method == restfile.HistoryMethodCompare && entry.Compare != nil {
		base = fmt.Sprintf("%s | %s", base, compareSummary(entry))
	}
	return base
}

func historyDescriptionLines(entry history.Entry) []string {
	lines := []string{historyBaseLine(entry)}
	if desc := strings.TrimSpace(entry.Description); desc != "" {
		lines = append(lines, condense(desc, 80))
	}
	if tags := joinTags(entry.Tags, 5); tags != "" {
		lines = append(lines, tags)
	}
	return lines
}

func (h historyItem) Title() string {
	return historyTitleText(h)
}

func (h historyItem) Description() string {
	return strings.Join(historyDescriptionLines(h.entry), "\n")
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

func makeHistoryItems(entries []history.Entry, scope historyScope) []list.Item {
	items := make([]list.Item, len(entries))
	for i, e := range entries {
		items[i] = historyItem{entry: e, scope: scope}
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
