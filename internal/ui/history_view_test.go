package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"

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

func TestHistoryStatusStyleUsesProfileStatus(t *testing.T) {
	base := lipgloss.NewStyle()
	tests := []struct {
		name  string
		entry history.Entry
		want  lipgloss.TerminalColor
	}{
		{
			name:  "pass",
			entry: history.Entry{Status: "PASS 2/2", ProfileResults: &history.ProfileResults{Status: "pass"}},
			want:  statsSuccessStyle.GetForeground(),
		},
		{
			name:  "fail",
			entry: history.Entry{Status: "FAIL 1/2", ProfileResults: &history.ProfileResults{Status: "fail"}},
			want:  statsWarnStyle.GetForeground(),
		},
		{
			name:  "canceled",
			entry: history.Entry{Status: "CANCELED 1/2", ProfileResults: &history.ProfileResults{Status: "canceled"}},
			want:  statsCautionStyle.GetForeground(),
		},
		{
			name:  "older entry",
			entry: history.Entry{Status: "profile completed", ProfileResults: &history.ProfileResults{}},
			want:  base.GetForeground(),
		},
	}
	for _, test := range tests {
		if got := historyStatusStyle(base, test.entry).GetForeground(); got != test.want {
			t.Fatalf("%s: color = %v, want %v", test.name, got, test.want)
		}
	}
}
