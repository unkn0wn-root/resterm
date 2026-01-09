package ui

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
)

type historyFilter struct {
	method string
	dates  []historyDateRange
	tokens []string
}

type historyDateRange struct {
	start time.Time
	end   time.Time
}

func filterHistoryEntries(entries []history.Entry, query string) []history.Entry {
	if len(entries) == 0 {
		return entries
	}
	filter := parseHistoryFilter(query)
	if filter.empty() {
		return entries
	}
	out := make([]history.Entry, 0, len(entries))
	for _, entry := range entries {
		if historyEntryMatchesFilter(entry, filter) {
			out = append(out, entry)
		}
	}
	return out
}

func parseHistoryFilter(query string) historyFilter {
	filter, _ := parseHistoryFilterAt(query, time.Now())
	return filter
}

func parseHistoryFilterAt(query string, now time.Time) (historyFilter, []string) {
	filter := historyFilter{}
	var invalidDates []string
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return filter, nil
	}
	var textParts []string
	for i := 0; i < len(fields); i++ {
		token := fields[i]
		key, val, ok := splitHistoryFilterToken(token)
		if !ok {
			textParts = append(textParts, token)
			continue
		}
		consumedNext := false
		val = strings.TrimSpace(val)
		if val == "" && i+1 < len(fields) {
			i++
			consumedNext = true
			val = strings.TrimSpace(fields[i])
		}
		if val == "" {
			textParts = append(textParts, token)
			if consumedNext {
				textParts = append(textParts, val)
			}
			continue
		}
		switch key {
		case "method":
			filter.method = strings.ToUpper(val)
		case "date":
			if ranges, ok := parseHistoryDateRanges(val, now); ok {
				filter.dates = appendHistoryDateRanges(filter.dates, ranges)
			} else {
				if val != "" {
					invalidDates = append(invalidDates, val)
				}
				textParts = append(textParts, token)
				if consumedNext {
					textParts = append(textParts, val)
				}
			}
		default:
			textParts = append(textParts, token)
			if consumedNext {
				textParts = append(textParts, val)
			}
		}
	}
	filter.tokens = historyFilterTokens(strings.Join(textParts, " "))
	return filter, invalidDates
}

func splitHistoryFilterToken(token string) (string, string, bool) {
	lowered := strings.ToLower(token)
	switch {
	case strings.HasPrefix(lowered, "method:"):
		return "method", token[len("method:"):], true
	case strings.HasPrefix(lowered, "date:"):
		return "date", token[len("date:"):], true
	default:
		return "", "", false
	}
}

func (f historyFilter) empty() bool {
	return f.method == "" && len(f.dates) == 0 && len(f.tokens) == 0
}

func historyFilterTokens(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return strings.Fields(strings.ToLower(text))
}

func historyEntryMatchesFilter(entry history.Entry, filter historyFilter) bool {
	if filter.method != "" && !historyMethodMatchesFilter(entry.Method, filter.method) {
		return false
	}
	if len(filter.dates) > 0 {
		matched := false
		for _, rng := range filter.dates {
			if rng.contains(entry.ExecutedAt) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(filter.tokens) == 0 {
		return true
	}
	search := historyEntrySearchText(entry)
	for _, token := range filter.tokens {
		if !strings.Contains(search, token) {
			return false
		}
	}
	return true
}

func historyMethodMatchesFilter(method string, filter string) bool {
	m := strings.ToUpper(strings.TrimSpace(method))
	f := strings.ToUpper(strings.TrimSpace(filter))
	if f == "" {
		return true
	}
	if m == "" {
		return false
	}
	return strings.HasPrefix(m, f)
}

func historyEntrySearchText(entry history.Entry) string {
	parts := []string{
		entry.RequestName,
		entry.URL,
		entry.Description,
		strings.Join(entry.Tags, " "),
		entry.Environment,
	}
	if entry.Compare != nil {
		for _, res := range entry.Compare.Results {
			parts = append(parts, res.Environment, res.Status)
		}
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func parseHistoryDateRanges(value string, now time.Time) ([]historyDateRange, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, false
	}
	lowered := strings.ToLower(value)
	switch lowered {
	case "today":
		return []historyDateRange{dateRangeForDay(now)}, true
	case "yesterday":
		return []historyDateRange{dateRangeForDay(now.AddDate(0, 0, -1))}, true
	default:
		ranges := make([]historyDateRange, 0, 4)
		layouts := []string{
			"02-01-2006",
			"01-02-2006",
			"02-Jan-2006",
			"2-Jan-2006",
			"02-January-2006",
			"2-January-2006",
		}
		for _, layout := range layouts {
			if rng, ok := parseHistoryDateLayout(layout, value, now); ok {
				ranges = appendHistoryDateRanges(ranges, []historyDateRange{rng})
			}
		}
		if len(ranges) == 0 {
			return nil, false
		}
		return ranges, true
	}
}

func parseHistoryDateLayout(layout, value string, now time.Time) (historyDateRange, bool) {
	loc := now.Location()
	parsed, err := time.ParseInLocation(layout, value, loc)
	if err != nil {
		return historyDateRange{}, false
	}
	return dateRangeForDay(parsed), true
}

func appendHistoryDateRanges(
	dst []historyDateRange,
	ranges []historyDateRange,
) []historyDateRange {
	if len(ranges) == 0 {
		return dst
	}
	if len(dst) == 0 {
		return append(dst, ranges...)
	}
	seen := make(map[time.Time]struct{}, len(dst)+len(ranges))
	for _, r := range dst {
		seen[r.start] = struct{}{}
	}
	for _, r := range ranges {
		if _, ok := seen[r.start]; ok {
			continue
		}
		seen[r.start] = struct{}{}
		dst = append(dst, r)
	}
	return dst
}

func dateRangeForDay(day time.Time) historyDateRange {
	loc := day.Location()
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	return historyDateRange{
		start: start,
		end:   start.AddDate(0, 0, 1),
	}
}

func (r historyDateRange) contains(ts time.Time) bool {
	if ts.IsZero() {
		return false
	}
	return !ts.Before(r.start) && ts.Before(r.end)
}
