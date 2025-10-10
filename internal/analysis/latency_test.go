package analysis

import (
	"testing"
	"time"
)

func TestComputeLatencyStats(t *testing.T) {
	durations := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
	}

	stats := ComputeLatencyStats(durations, []int{50, 90, 95, 99}, 5)

	if stats.Count != len(durations) {
		t.Fatalf("expected count %d, got %d", len(durations), stats.Count)
	}
	if stats.Min != 100*time.Millisecond {
		t.Fatalf("expected min 100ms, got %s", stats.Min)
	}
	if stats.Max != 500*time.Millisecond {
		t.Fatalf("expected max 500ms, got %s", stats.Max)
	}
	if stats.Median != 300*time.Millisecond {
		t.Fatalf("expected median 300ms, got %s", stats.Median)
	}
	if stats.Mean != 300*time.Millisecond {
		t.Fatalf("expected mean 300ms, got %s", stats.Mean)
	}

	if p50 := stats.Percentiles[50]; p50 != 300*time.Millisecond {
		t.Fatalf("expected P50 300ms, got %s", p50)
	}
	if p90 := stats.Percentiles[90]; p90 != 500*time.Millisecond {
		t.Fatalf("expected P90 500ms, got %s", p90)
	}

	if len(stats.Histogram) != 5 {
		t.Fatalf("expected 5 histogram buckets, got %d", len(stats.Histogram))
	}
	count := 0
	for _, bucket := range stats.Histogram {
		count += bucket.Count
	}
	if count != len(durations) {
		t.Fatalf("expected histogram counts sum to %d, got %d", len(durations), count)
	}
}
