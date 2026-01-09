package ui

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func TestParseHistoryFilterMethodWithSpace(t *testing.T) {
	now := time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC)
	filter, invalid := parseHistoryFilterAt("method: GET users", now)
	if len(invalid) != 0 {
		t.Fatalf("expected no invalid dates, got %+v", invalid)
	}
	if filter.method != "GET" {
		t.Fatalf("expected method GET, got %q", filter.method)
	}
	if len(filter.tokens) != 1 || filter.tokens[0] != "users" {
		t.Fatalf("expected tokens [users], got %+v", filter.tokens)
	}
}

func TestParseHistoryDateDDMMYYYY(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	ranges, ok := parseHistoryDateRanges("10-01-2024", now)
	if !ok {
		t.Fatalf("expected date to parse")
	}
	match := time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC)
	matched := false
	for _, rng := range ranges {
		if rng.contains(match) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("expected date range to include %v", match)
	}
	next := time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC)
	for _, rng := range ranges {
		if rng.contains(next) {
			t.Fatalf("did not expect date range to include %v", next)
		}
	}
}

func TestParseHistoryDateMMDDYYYY(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	ranges, ok := parseHistoryDateRanges("01-10-2024", now)
	if !ok {
		t.Fatalf("expected date to parse")
	}
	match := time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC)
	matched := false
	for _, rng := range ranges {
		if rng.contains(match) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("expected date range to include %v", match)
	}
}

func TestParseHistoryDateMonthName(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	ranges, ok := parseHistoryDateRanges("05-Jun-2024", now)
	if !ok {
		t.Fatalf("expected date to parse")
	}
	match := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
	matched := false
	for _, rng := range ranges {
		if rng.contains(match) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("expected date range to include %v", match)
	}
}

func TestParseHistoryDateAmbiguous(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	ranges, ok := parseHistoryDateRanges("05-06-2024", now)
	if !ok {
		t.Fatalf("expected date to parse")
	}
	if len(ranges) != 2 {
		t.Fatalf("expected 2 date ranges, got %d", len(ranges))
	}
	may := time.Date(2024, 5, 6, 12, 0, 0, 0, time.UTC)
	jun := time.Date(2024, 6, 5, 12, 0, 0, 0, time.UTC)
	matchMay := false
	matchJun := false
	for _, rng := range ranges {
		if rng.contains(may) {
			matchMay = true
		}
		if rng.contains(jun) {
			matchJun = true
		}
	}
	if !matchMay || !matchJun {
		t.Fatalf("expected ambiguous date to match both May 6 and Jun 5")
	}
}

func TestHistoryEntryMatchesFilter(t *testing.T) {
	now := time.Date(2024, 1, 10, 8, 30, 0, 0, time.UTC)
	entry := history.Entry{
		Method:      "GET",
		ExecutedAt:  time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC),
		RequestName: "List Users",
		URL:         "https://api.example.com/users",
	}
	filter, invalid := parseHistoryFilterAt("method:get date:10-01-2024 users", now)
	if len(invalid) != 0 {
		t.Fatalf("expected no invalid dates, got %+v", invalid)
	}
	if !historyEntryMatchesFilter(entry, filter) {
		t.Fatalf("expected entry to match filter")
	}
}

func TestHistoryEntryMatchesPartialMethod(t *testing.T) {
	now := time.Date(2024, 1, 10, 8, 30, 0, 0, time.UTC)
	entry := history.Entry{
		Method:      "GET",
		ExecutedAt:  time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC),
		RequestName: "List Users",
		URL:         "https://api.example.com/users",
	}
	filter, invalid := parseHistoryFilterAt("method:GE users", now)
	if len(invalid) != 0 {
		t.Fatalf("expected no invalid dates, got %+v", invalid)
	}
	if !historyEntryMatchesFilter(entry, filter) {
		t.Fatalf("expected entry to match partial method filter")
	}
}
