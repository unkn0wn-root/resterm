package runner

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
)

func TestReportWriteJUnitIncludesCompareAndProfile(t *testing.T) {
	rep := &Report{
		Duration: 3 * time.Second,
		Results: []Result{
			{
				Kind:   ResultKindCompare,
				Name:   "cmp",
				Method: "COMPARE",
				Compare: &CompareInfo{
					Baseline: "dev",
				},
				Steps: []StepResult{
					{
						Name:        "dev",
						Environment: "dev",
						Passed:      true,
						Response:    &httpclient.Response{Status: "200 OK", StatusCode: 200},
					},
					{
						Name:        "stage",
						Environment: "stage",
						Passed:      false,
						Err:         errors.New("stage failed"),
					},
				},
			},
			{
				Kind:     ResultKindProfile,
				Name:     "prof",
				Method:   "PROFILE",
				Passed:   true,
				Duration: 2 * time.Second,
				Profile: &ProfileInfo{
					Results: &history.ProfileResults{
						TotalRuns:      4,
						WarmupRuns:     1,
						SuccessfulRuns: 3,
						FailedRuns:     0,
					},
				},
			},
		},
	}

	var out strings.Builder
	if err := rep.WriteJUnit(&out); err != nil {
		t.Fatalf("WriteJUnit: %v", err)
	}
	xml := out.String()
	if !strings.Contains(xml, `<testsuite name="COMPARE cmp"`) {
		t.Fatalf("expected compare suite in junit output, got %q", xml)
	}
	if !strings.Contains(xml, `<testcase name="stage" classname="COMPARE cmp">`) {
		t.Fatalf("expected compare testcase in junit output, got %q", xml)
	}
	if !strings.Contains(xml, `failure message="stage failed"`) {
		t.Fatalf("expected compare failure in junit output, got %q", xml)
	}
	if !strings.Contains(xml, `<testsuite name="PROFILE prof"`) {
		t.Fatalf("expected profile suite in junit output, got %q", xml)
	}
	if !strings.Contains(xml, `<testcase name="prof" classname="PROFILE prof" time="2.000">`) {
		t.Fatalf("expected profile testcase timing in junit output, got %q", xml)
	}
}
