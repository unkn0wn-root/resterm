package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/mock"
)

func TestMockLogLineIncludesSequenceProgress(t *testing.T) {
	line := mockLogLine(mock.Event{
		Time:          time.Date(2026, 7, 17, 12, 30, 0, 0, time.UTC),
		Method:        "GET",
		Target:        "/payments/1",
		Status:        200,
		Scenario:      "polling",
		SequenceStep:  2,
		SequenceTotal: 3,
	})
	if !strings.Contains(line, "polling 2/3") {
		t.Fatalf("mock log line = %q", line)
	}
}
