package headless

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestStatusValid(t *testing.T) {
	cases := []struct {
		st   Status
		want bool
	}{
		{st: StatusPass, want: true},
		{st: StatusFail, want: true},
		{st: StatusSkip, want: true},
		{st: Status("canceled"), want: false},
		{st: Status(""), want: false},
	}
	for _, tc := range cases {
		if got := tc.st.Valid(); got != tc.want {
			t.Fatalf("status %q valid = %v, want %v", tc.st, got, tc.want)
		}
	}
}

func TestIsUsageError(t *testing.T) {
	base := errors.New("bad flag")
	err := ErrUsage{err: base}
	if !IsUsageError(err) {
		t.Fatal("expected ErrUsage to be classified as usage error")
	}
	if !IsUsageError(errors.Join(err, errors.New("extra"))) {
		t.Fatal("expected joined usage error to be classified as usage error")
	}
	if IsUsageError(base) {
		t.Fatal("expected plain error to not be classified as usage error")
	}
}

func TestUsageErrorZero(t *testing.T) {
	if got := (ErrUsage{}).Error(); got != "usage error" {
		t.Fatalf("zero ErrUsage error = %q, want %q", got, "usage error")
	}
}

func TestZeroValues(t *testing.T) {
	var opt Opt
	if opt.FilePath != "" || opt.EnvName != "" || opt.Profile {
		t.Fatalf("unexpected opt zero value: %+v", opt)
	}
	if opt.HTTP.Follow != nil || opt.GRPC.Plaintext != nil {
		t.Fatalf("unexpected opt zero pointer values: %+v", opt)
	}

	var rep Report
	if rep.Total != 0 || rep.Passed != 0 || rep.Failed != 0 || rep.Skipped != 0 {
		t.Fatalf("unexpected report zero counts: %+v", rep)
	}
	if rep.Results != nil {
		t.Fatalf("expected nil zero-value results slice, got %+v", rep.Results)
	}

	var item Result
	if item.Status.Valid() {
		t.Fatalf("expected zero-value status to be invalid, got %q", item.Status)
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
		{typ: reflect.TypeFor[Opt](), name: "FilePath", tag: "filePath,omitempty"},
		{typ: reflect.TypeFor[Opt](), name: "CompareTargets", tag: "compareTargets,omitempty"},
		{typ: reflect.TypeFor[Report](), name: "FilePath", tag: "filePath"},
		{typ: reflect.TypeFor[Report](), name: "EnvName", tag: "envName,omitempty"},
		{typ: reflect.TypeFor[Result](), name: "Status", tag: "status"},
		{typ: reflect.TypeFor[Result](), name: "ScriptError", tag: "scriptError,omitempty"},
		{typ: reflect.TypeFor[Step](), name: "SkipReason", tag: "skipReason,omitempty"},
		{typ: reflect.TypeFor[HTTP](), name: "StatusCode", tag: "statusCode,omitempty"},
		{typ: reflect.TypeFor[GRPC](), name: "StatusMessage", tag: "statusMessage,omitempty"},
		{typ: reflect.TypeFor[Test](), name: "Elapsed", tag: "elapsed,omitempty"},
		{typ: reflect.TypeFor[Profile](), name: "TotalRuns", tag: "totalRuns,omitempty"},
		{typ: reflect.TypeFor[ProfileFail](), name: "StatusCode", tag: "statusCode,omitempty"},
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
				Failures: []ProfileFail{{
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

func jsonTag(t reflect.Type, name string) string {
	f, ok := t.FieldByName(name)
	if !ok {
		return ""
	}
	return f.Tag.Get("json")
}
