package history

import "testing"

func TestCompareEntryBaselineResult(t *testing.T) {
	grouped := []CompareResult{
		{Environment: "api=dev, auth=ci", Profile: "dev"},
		{Environment: "api=prod, auth=ci", Profile: "prod"},
	}
	flat := []CompareResult{
		{Environment: "dev"},
		{Environment: "prod"},
	}
	tests := []struct {
		name     string
		baseline string
		results  []CompareResult
		want     string
	}{
		{
			name:     "flat environment",
			baseline: "prod",
			results:  flat,
			want:     "prod",
		},
		{
			name:     "grouped profile ignoring case",
			baseline: "PROD",
			results:  grouped,
			want:     "api=prod, auth=ci",
		},
		{
			name:     "grouped full label",
			baseline: "api=prod, auth=ci",
			results:  grouped,
			want:     "api=prod, auth=ci",
		},
		{
			name:    "blank baseline",
			results: grouped,
			want:    "api=dev, auth=ci",
		},
		{
			name:     "unknown baseline",
			baseline: "missing",
			results:  grouped,
			want:     "api=dev, auth=ci",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &CompareEntry{Baseline: tt.baseline, Results: tt.results}
			res := entry.BaselineResult()
			if res == nil {
				t.Fatal("expected baseline result")
			}
			if res.Environment != tt.want {
				t.Fatalf("environment = %q, want %q", res.Environment, tt.want)
			}
		})
	}
}

func TestCompareEntryBaselineResultEmpty(t *testing.T) {
	var entry *CompareEntry
	if res := entry.BaselineResult(); res != nil {
		t.Fatalf("nil entry: got %+v", res)
	}
	if res := (&CompareEntry{Baseline: "prod"}).BaselineResult(); res != nil {
		t.Fatalf("empty results: got %+v", res)
	}
}
