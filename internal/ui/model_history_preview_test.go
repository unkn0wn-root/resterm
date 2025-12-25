package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/history"
)

func TestOpenHistoryPreviewResetsState(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	entry := history.Entry{
		RequestName: "Sample",
		Method:      "GET",
		URL:         "https://example.com",
		ExecutedAt:  time.Now(),
		Status:      "200 OK",
		ProfileResults: &history.ProfileResults{
			TotalRuns: 3,
		},
	}

	model.openHistoryPreview(entry)
	if !model.showHistoryPreview {
		t.Fatalf("expected preview modal to be shown")
	}
	if model.historyPreviewTitle != "Sample" {
		t.Fatalf("expected title %q, got %q", "Sample", model.historyPreviewTitle)
	}
	if model.historyPreviewViewport == nil {
		t.Fatalf("expected preview viewport to be initialised")
	}

	_ = model.renderHistoryPreviewModal()
	if model.historyPreviewViewport.Width == 0 || model.historyPreviewViewport.Height == 0 {
		t.Fatalf(
			"expected viewport dimensions to be set, got %dx%d",
			model.historyPreviewViewport.Width,
			model.historyPreviewViewport.Height,
		)
	}
	if model.historyPreviewViewport.YOffset != 0 {
		t.Fatalf(
			"expected viewport offset reset to 0, got %d",
			model.historyPreviewViewport.YOffset,
		)
	}
}

func TestHistoryPreviewScrolling(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 100
	model.height = 30
	entry := history.Entry{RequestName: "Scroll"}
	entry.ProfileResults = &history.ProfileResults{
		Percentiles: make([]history.ProfilePercentile, 0, 50),
	}
	for i := 0; i < 50; i++ {
		entry.ProfileResults.Percentiles = append(
			entry.ProfileResults.Percentiles,
			history.ProfilePercentile{Percentile: i, Value: time.Duration(i)},
		)
	}
	model.openHistoryPreview(entry)
	_ = model.renderHistoryPreviewModal()

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	if _, cmd := model.Update(msg); cmd != nil {
		t.Fatalf("expected no command from scroll, got %v", cmd)
	}
	if model.historyPreviewViewport.YOffset == 0 {
		t.Fatalf("expected viewport to scroll down")
	}

	upMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	model.Update(upMsg)
	if model.historyPreviewViewport.YOffset != 0 {
		t.Fatalf("expected viewport to return to top, got %d", model.historyPreviewViewport.YOffset)
	}
}
