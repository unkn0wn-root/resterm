package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
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

func TestCompareSummaryMarksGroupedBaseline(t *testing.T) {
	entry := history.Entry{Compare: &history.CompareEntry{
		Baseline: "prod",
		Group:    "api",
		Results: []history.CompareResult{
			{Environment: "api=dev, auth=ci", Profile: "dev", Status: "200 OK"},
			{Environment: "api=prod, auth=ci", Profile: "prod", Status: "200 OK"},
		},
	}}

	got := compareSummary(entry)
	if !strings.Contains(got, "api=prod, auth=ci*:200 OK") {
		t.Fatalf("grouped baseline is not marked: %q", got)
	}
	if strings.Contains(got, "api=dev, auth=ci*:200 OK") {
		t.Fatalf("non-baseline row is marked: %q", got)
	}
}
