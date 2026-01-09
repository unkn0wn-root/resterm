package ui

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func TestParseHistoryFilterMethodWithSpace(t *testing.T) {
	now := time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC)
	filter := parseHistoryFilterAt("method: GET users", now)
	if filter.method != "GET" {
		t.Fatalf("expected method GET, got %q", filter.method)
	}
	if len(filter.tokens) != 1 || filter.tokens[0] != "users" {
		t.Fatalf("expected tokens [users], got %+v", filter.tokens)
	}
}

func TestParseHistoryDateDDMMYYYY(t *testing.T) {
	now := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	rng, ok := parseHistoryDate("10-01-2024", now)
	if !ok {
		t.Fatalf("expected date to parse")
	}
	match := time.Date(2024, 1, 10, 12, 0, 0, 0, time.UTC)
	if !rng.contains(match) {
		t.Fatalf("expected date range to include %v", match)
	}
	next := time.Date(2024, 1, 11, 0, 0, 0, 0, time.UTC)
	if rng.contains(next) {
		t.Fatalf("did not expect date range to include %v", next)
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
	filter := parseHistoryFilterAt("method:get date:10-01-2024 users", now)
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
	filter := parseHistoryFilterAt("method:GE users", now)
	if !historyEntryMatchesFilter(entry, filter) {
		t.Fatalf("expected entry to match partial method filter")
	}
}
