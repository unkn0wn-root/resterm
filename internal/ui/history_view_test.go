package ui

import (
	"testing"
	"time"
)

func TestHistoryTimestampLabelToday(t *testing.T) {
	loc := time.FixedZone("UTC", 0)
	now := time.Date(2025, time.January, 2, 18, 0, 0, 0, loc)
	at := time.Date(2025, time.January, 2, 9, 30, 0, 0, loc)

	got := historyTimestampLabel(at, now)
	want := "09:30:00"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestHistoryTimestampLabelPastDay(t *testing.T) {
	loc := time.FixedZone("UTC", 0)
	now := time.Date(2025, time.January, 3, 1, 0, 0, 0, loc)
	at := time.Date(2025, time.January, 2, 9, 30, 0, 0, loc)

	got := historyTimestampLabel(at, now)
	want := "02-01-2025 09:30:00"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
