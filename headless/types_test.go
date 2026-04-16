package headless

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestIsUsageError(t *testing.T) {
	base := errors.New("bad flag")
	err := UsageError{err: base}
	if !IsUsageError(err) {
		t.Fatal("expected UsageError to be classified as usage error")
	}
	if !IsUsageError(errors.Join(err, errors.New("extra"))) {
		t.Fatal("expected joined usage error to be classified as usage error")
	}
	if IsUsageError(base) {
		t.Fatal("expected plain error to not be classified as usage error")
	}
}

func TestUsageErrorZero(t *testing.T) {
	if got := (UsageError{}).Error(); got != "usage error" {
		t.Fatalf("zero UsageError error = %q, want %q", got, "usage error")
	}
}

func TestZeroValues(t *testing.T) {
	var opt Options
	if opt.FilePath != "" || opt.Environment.Name != "" || opt.Profile {
		t.Fatalf("unexpected options zero value: %+v", opt)
	}
	if opt.HTTP.FollowRedirects != nil || opt.GRPC.Plaintext != nil {
		t.Fatalf("unexpected options zero pointer values: %+v", opt)
	}

	var rep Report
	if rep.Total != 0 || rep.Passed != 0 || rep.Failed != 0 || rep.Skipped != 0 {
		t.Fatalf("unexpected report zero counts: %+v", rep)
	}
	if rep.Results != nil {
		t.Fatalf("expected nil zero-value results slice, got %+v", rep.Results)
	}
	if (&rep).HasFailures() {
		t.Fatalf("expected zero-value report to have no failures: %+v", rep)
	}

	var tr Trace
	if tr.Budget != nil || tr.Breaches != nil {
		t.Fatalf("unexpected zero trace value: %+v", tr)
	}
}

func TestJSONTags(t *testing.T) {
	cases := []struct {
		typ  reflect.Type
		name string
		tag  string
	}{
		{typ: reflect.TypeFor[Options](), name: "FilePath", tag: "filePath,omitempty"},
		{typ: reflect.TypeFor[Options](), name: "Selection", tag: "selection,omitempty"},
		{typ: reflect.TypeFor[StateOptions](), name: "ArtifactDir", tag: "artifactDir,omitempty"},
		{typ: reflect.TypeFor[EnvironmentOptions](), name: "FilePath", tag: "filePath,omitempty"},
		{typ: reflect.TypeFor[CompareOptions](), name: "Targets", tag: "targets,omitempty"},
		{
			typ:  reflect.TypeFor[HTTPOptions](),
			name: "FollowRedirects",
			tag:  "followRedirects,omitempty",
		},
		{typ: reflect.TypeFor[GRPCOptions](), name: "Plaintext", tag: "plaintext,omitempty"},
		{typ: reflect.TypeFor[Report](), name: "FilePath", tag: ""},
		{typ: reflect.TypeFor[Report](), name: "EnvName", tag: ""},
		{typ: reflect.TypeFor[Result](), name: "Status", tag: ""},
		{typ: reflect.TypeFor[Result](), name: "ScriptError", tag: ""},
		{typ: reflect.TypeFor[Step](), name: "SkipReason", tag: ""},
		{typ: reflect.TypeFor[HTTP](), name: "StatusCode", tag: "statusCode,omitempty"},
		{typ: reflect.TypeFor[GRPC](), name: "StatusMessage", tag: "statusMessage,omitempty"},
		{typ: reflect.TypeFor[Test](), name: "Elapsed", tag: "elapsed,omitempty"},
		{typ: reflect.TypeFor[Profile](), name: "TotalRuns", tag: "totalRuns,omitempty"},
		{typ: reflect.TypeFor[ProfileFailure](), name: "StatusCode", tag: "statusCode,omitempty"},
		{typ: reflect.TypeFor[Stream](), name: "TranscriptPath", tag: "transcriptPath,omitempty"},
		{typ: reflect.TypeFor[Trace](), name: "ArtifactPath", tag: "artifactPath,omitempty"},
		{typ: reflect.TypeFor[TraceBudget](), name: "Phases", tag: "phases,omitempty"},
	}
	for _, tc := range cases {
		if got := jsonTag(tc.typ, tc.name); got != tc.tag {
			t.Fatalf("%s.%s tag = %q, want %q", tc.typ.Name(), tc.name, got, tc.tag)
		}
	}
}

func TestPublicTypesHoldStableValues(t *testing.T) {
	rep := Report{
		Version:   "v1",
		FilePath:  "api.http",
		EnvName:   "dev",
		StartedAt: time.Unix(10, 0),
		EndedAt:   time.Unix(12, 0),
		Duration:  2 * time.Second,
		Results: []Result{{
			Kind:        KindCompare,
			Name:        "cmp",
			Method:      "GET",
			Target:      "https://example.com",
			Environment: "dev",
			Status:      StatusPass,
			Summary:     "ok",
			Duration:    time.Second,
			HTTP: &HTTP{
				Status:     "200 OK",
				StatusCode: 200,
				Protocol:   "HTTP/1.1",
			},
			Tests: []Test{{
				Name:    "status",
				Passed:  true,
				Elapsed: time.Millisecond,
			}},
			Compare: &Compare{Baseline: "stage"},
			Profile: &Profile{
				Count:          3,
				Warmup:         1,
				Delay:          time.Second,
				TotalRuns:      3,
				WarmupRuns:     1,
				SuccessfulRuns: 2,
				FailedRuns:     1,
				Latency: &Latency{
					Count:  2,
					Min:    time.Millisecond,
					Max:    2 * time.Millisecond,
					Mean:   1500 * time.Microsecond,
					Median: 1500 * time.Microsecond,
					StdDev: 500 * time.Microsecond,
				},
				Percentiles: []Percentile{{Percentile: 95, Value: 2 * time.Millisecond}},
				Histogram: []HistBin{
					{From: time.Millisecond, To: 2 * time.Millisecond, Count: 2},
				},
				Failures: []ProfileFailure{{
					Iteration:  2,
					Status:     "500",
					StatusCode: 500,
					Duration:   3 * time.Millisecond,
				}},
			},
			Trace: &Trace{
				Duration:     time.Second,
				Error:        "boom",
				ArtifactPath: "trace.json",
				Budget: &TraceBudget{
					Total:     2 * time.Second,
					Tolerance: 10 * time.Millisecond,
					Phases:    map[string]time.Duration{"dns": time.Millisecond},
				},
				Breaches: []TraceBreach{{
					Kind:   "dns",
					Limit:  time.Millisecond,
					Actual: 2 * time.Millisecond,
					Over:   time.Millisecond,
				}},
			},
			Stream: &Stream{
				Kind:           "sse",
				EventCount:     1,
				Summary:        map[string]any{"lastEventID": "1"},
				TranscriptPath: "stream.log",
			},
			Steps: []Step{{
				Name:   "step",
				Status: StatusPass,
			}},
		}},
		Total:   1,
		Passed:  1,
		Failed:  0,
		Skipped: 0,
	}
	if rep.Results[0].Kind != KindCompare || rep.Results[0].Status != StatusPass {
		t.Fatalf("unexpected stable values: %+v", rep.Results[0])
	}
	if rep.Results[0].Profile == nil || rep.Results[0].Trace == nil ||
		rep.Results[0].Stream == nil {
		t.Fatalf("expected nested public values to be retained: %+v", rep.Results[0])
	}
}

func TestReportHasFailures(t *testing.T) {
	if (*Report)(nil).HasFailures() {
		t.Fatal("expected nil report to have no failures")
	}

	if (&Report{Failed: 1}).HasFailures() == false {
		t.Fatal("expected failed count to report failures")
	}

	rep := &Report{
		Results: []Result{
			{Status: StatusPass},
			{Status: StatusFail},
		},
	}
	if !rep.HasFailures() {
		t.Fatalf("expected failed result to be detected: %+v", rep)
	}

	if (&Report{Results: []Result{{Error: "boom"}}}).HasFailures() == false {
		t.Fatal("expected result error to report failures")
	}

	if (&Report{Results: []Result{{Status: StatusPass}}}).HasFailures() {
		t.Fatal("expected passing results to report no failures")
	}
}

func TestResultFailedUsesEffectiveStatus(t *testing.T) {
	res := Result{
		Status:      StatusPass,
		Canceled:    true,
		Error:       "boom",
		ScriptError: "script boom",
		Tests:       []Test{{Passed: false}},
		Trace:       &Trace{Breaches: []TraceBreach{{Kind: "total"}}},
	}
	if !res.Failed() {
		t.Fatalf("expected failure evidence to report failure: %+v", res)
	}
	res.Status = StatusSkip
	if res.Failed() {
		t.Fatalf("expected skip status to suppress failure reporting: %+v", res)
	}
}

func TestStepFailedUsesEffectiveStatus(t *testing.T) {
	step := Step{
		Status:      StatusPass,
		Canceled:    true,
		Error:       "boom",
		ScriptError: "script boom",
		Tests:       []Test{{Passed: false}},
		Trace:       &Trace{Breaches: []TraceBreach{{Kind: "total"}}},
	}
	if !step.Failed() {
		t.Fatalf("expected failure evidence to report failure: %+v", step)
	}
	step.Status = StatusSkip
	if step.Failed() {
		t.Fatalf("expected skip status to suppress failure reporting: %+v", step)
	}
}

func jsonTag(t reflect.Type, name string) string {
	f, ok := t.FieldByName(name)
	if !ok {
		return ""
	}
	return f.Tag.Get("json")
}
