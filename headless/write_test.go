package headless

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"google.golang.org/grpc/codes"
)

func TestReportWriteTextParity(t *testing.T) {
	src := sampleRunnerReport()

	var want strings.Builder
	if err := src.WriteText(&want); err != nil {
		t.Fatalf("runner WriteText: %v", err)
	}

	gotRep := reportFromRunner(src)
	var got strings.Builder
	if err := gotRep.WriteText(&got); err != nil {
		t.Fatalf("public WriteText: %v", err)
	}

	if got.String() != want.String() {
		t.Fatalf("text mismatch\nwant:\n%s\ngot:\n%s", want.String(), got.String())
	}
}

func TestReportWriteJSONParity(t *testing.T) {
	src := sampleRunnerReport()

	var want strings.Builder
	if err := src.WriteJSON(&want); err != nil {
		t.Fatalf("runner WriteJSON: %v", err)
	}

	gotRep := reportFromRunner(src)
	var got strings.Builder
	if err := gotRep.WriteJSON(&got); err != nil {
		t.Fatalf("public WriteJSON: %v", err)
	}

	if got.String() != want.String() {
		t.Fatalf("json mismatch\nwant:\n%s\ngot:\n%s", want.String(), got.String())
	}
}

func TestReportWriteJUnitParity(t *testing.T) {
	src := sampleRunnerReport()

	var want strings.Builder
	if err := src.WriteJUnit(&want); err != nil {
		t.Fatalf("runner WriteJUnit: %v", err)
	}

	gotRep := reportFromRunner(src)
	var got strings.Builder
	if err := gotRep.WriteJUnit(&got); err != nil {
		t.Fatalf("public WriteJUnit: %v", err)
	}

	if got.String() != want.String() {
		t.Fatalf("junit mismatch\nwant:\n%s\ngot:\n%s", want.String(), got.String())
	}
}

func TestWriteNilWriter(t *testing.T) {
	rep := reportFromRunner(sampleRunnerReport())
	if rep == nil {
		t.Fatal("expected report")
	}

	if err := rep.Encode(nil, JSON); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("Encode(nil, JSON): got %v want %v", err, ErrNilWriter)
	}
	if err := rep.WriteText(nil); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("WriteText(nil): got %v want %v", err, ErrNilWriter)
	}
	if err := rep.WriteJSON(nil); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("WriteJSON(nil): got %v want %v", err, ErrNilWriter)
	}
	if err := rep.WriteJUnit(nil); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("WriteJUnit(nil): got %v want %v", err, ErrNilWriter)
	}
}

func TestWriteNilReport(t *testing.T) {
	var rep *Report

	if err := rep.Encode(&strings.Builder{}, JSON); !errors.Is(err, ErrNilReport) {
		t.Fatalf("Encode(..., JSON): got %v want %v", err, ErrNilReport)
	}
	if err := rep.WriteText(&strings.Builder{}); !errors.Is(err, ErrNilReport) {
		t.Fatalf("WriteText(...): got %v want %v", err, ErrNilReport)
	}
	if err := rep.WriteJSON(&strings.Builder{}); !errors.Is(err, ErrNilReport) {
		t.Fatalf("WriteJSON(...): got %v want %v", err, ErrNilReport)
	}
	if err := rep.WriteJUnit(&strings.Builder{}); !errors.Is(err, ErrNilReport) {
		t.Fatalf("WriteJUnit(...): got %v want %v", err, ErrNilReport)
	}
}

func TestReportEncodeParity(t *testing.T) {
	rep := reportFromRunner(sampleRunnerReport())
	if rep == nil {
		t.Fatal("expected report")
	}

	cases := []struct {
		name   string
		format Format
		write  func(io.Writer) error
	}{
		{name: "json", format: JSON, write: rep.WriteJSON},
		{name: "junit", format: JUnit, write: rep.WriteJUnit},
		{name: "text", format: Text, write: rep.WriteText},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var want strings.Builder
			if err := tc.write(&want); err != nil {
				t.Fatalf("write: %v", err)
			}

			var got strings.Builder
			if err := rep.Encode(&got, tc.format); err != nil {
				t.Fatalf("Encode: %v", err)
			}

			if got.String() != want.String() {
				t.Fatalf(
					"encoded output mismatch\nwant:\n%s\ngot:\n%s",
					want.String(),
					got.String(),
				)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	cases := map[string]Format{
		"json":    JSON,
		"JSON":    JSON,
		" junit ": JUnit,
		"text":    Text,
	}
	for input, want := range cases {
		got, err := ParseFormat(input)
		if err != nil {
			t.Fatalf("ParseFormat(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("ParseFormat(%q) = %v, want %v", input, got, want)
		}
	}

	if _, err := ParseFormat("yaml"); err == nil {
		t.Fatal("expected unknown format error")
	}
}

func TestFormatString(t *testing.T) {
	if JSON.String() != "json" || JUnit.String() != "junit" || Text.String() != "text" {
		t.Fatalf(
			"unexpected format strings: %q %q %q",
			JSON.String(),
			JUnit.String(),
			Text.String(),
		)
	}
	if got := Format(99).String(); got != "format(99)" {
		t.Fatalf("unexpected invalid format string: %q", got)
	}
}

func TestEncodeInvalidFormat(t *testing.T) {
	rep := &Report{FilePath: "api.http"}
	err := rep.Encode(&strings.Builder{}, Format(99))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported format 99") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportWriteJSONUsesCanonicalStatusValues(t *testing.T) {
	rep := &Report{
		FilePath: "api.http",
		Results: []Result{{
			Name:     "wf",
			Method:   "WORKFLOW",
			Status:   StatusFail,
			Canceled: true,
			Steps: []Step{{
				Name:     "step",
				Status:   StatusFail,
				Canceled: true,
			}},
		}},
	}

	var out strings.Builder
	if err := rep.WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got struct {
		Results []struct {
			Status   string `json:"status"`
			Canceled bool   `json:"canceled"`
			Steps    []struct {
				Status   string `json:"status"`
				Canceled bool   `json:"canceled"`
			} `json:"steps"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Status != "fail" || !got.Results[0].Canceled {
		t.Fatalf("unexpected result json: %+v", got.Results)
	}
	if len(got.Results[0].Steps) != 1 || got.Results[0].Steps[0].Status != "fail" ||
		!got.Results[0].Steps[0].Canceled {
		t.Fatalf("unexpected step json: %+v", got.Results[0].Steps)
	}
}

func TestReportWriteJSONUsesEffectiveStatus(t *testing.T) {
	rep := &Report{
		FilePath: "api.http",
		Results: []Result{{
			Name:     "req",
			Status:   StatusPass,
			Canceled: true,
			Error:    "boom",
			Tests: []Test{{
				Name:   "status",
				Passed: false,
			}},
			Trace: &Trace{
				Breaches: []TraceBreach{{Kind: "total"}},
			},
			Steps: []Step{{
				Name:     "step",
				Status:   StatusPass,
				Canceled: true,
				Error:    "boom",
				Tests: []Test{{
					Name:   "status",
					Passed: false,
				}},
				Trace: &Trace{
					Breaches: []TraceBreach{{Kind: "total"}},
				},
			}},
		}},
	}

	var out strings.Builder
	if err := rep.WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got struct {
		Results []struct {
			Status string `json:"status"`
			Steps  []struct {
				Status string `json:"status"`
			} `json:"steps"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Status != "fail" {
		t.Fatalf("unexpected result json: %+v", got.Results)
	}
	if len(got.Results[0].Steps) != 1 || got.Results[0].Steps[0].Status != "fail" {
		t.Fatalf("unexpected step json: %+v", got.Results[0].Steps)
	}
}

func TestReportWriteJSONIncludesFailureMetadata(t *testing.T) {
	rep := &Report{
		FilePath: "api.http",
		Results: []Result{{
			Name:   "slow",
			Status: StatusPass,
			Error:  "context deadline exceeded",
		}},
	}

	var out strings.Builder
	if err := rep.WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got struct {
		SchemaVersion string `json:"schemaVersion"`
		Summary       struct {
			ExitCode     int      `json:"exitCode"`
			FailureCodes []string `json:"failureCodes"`
		} `json:"summary"`
		Results []struct {
			Status  string `json:"status"`
			Failure struct {
				Code     string `json:"code"`
				Category string `json:"category"`
				ExitCode int    `json:"exitCode"`
				Source   string `json:"source"`
			} `json:"failure"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if got.SchemaVersion != "1" {
		t.Fatalf("unexpected schema version %q", got.SchemaVersion)
	}
	if got.Summary.ExitCode != ExitTimeout ||
		len(got.Summary.FailureCodes) != 1 ||
		got.Summary.FailureCodes[0] != string(FailureTimeout) {
		t.Fatalf("unexpected summary failure metadata: %+v", got.Summary)
	}
	if len(got.Results) != 1 || got.Results[0].Status != "fail" ||
		got.Results[0].Failure.Code != string(FailureTimeout) ||
		got.Results[0].Failure.Category != string(CategoryTimeout) ||
		got.Results[0].Failure.ExitCode != ExitTimeout ||
		got.Results[0].Failure.Source != "error" {
		t.Fatalf("unexpected result failure metadata: %+v", got.Results)
	}
	if rep.ExitCode(ExitCodeDetailed) != ExitTimeout {
		t.Fatalf("expected detailed exit code %d, got %d", ExitTimeout, rep.ExitCode(ExitCodeDetailed))
	}
	if rep.ExitCode(ExitCodeSummary) != ExitFailure {
		t.Fatalf("expected summary exit code %d, got %d", ExitFailure, rep.ExitCode(ExitCodeSummary))
	}
}

func TestReportWriteJSONPreservesSkipStatus(t *testing.T) {
	rep := &Report{
		FilePath: "api.http",
		Results: []Result{{
			Name:   "req",
			Status: StatusSkip,
			Error:  "boom",
			Steps: []Step{{
				Name:   "step",
				Status: StatusSkip,
				Error:  "boom",
			}},
		}},
	}

	var out strings.Builder
	if err := rep.WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got struct {
		Results []struct {
			Status string `json:"status"`
			Steps  []struct {
				Status string `json:"status"`
			} `json:"steps"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Status != "skip" {
		t.Fatalf("unexpected result json: %+v", got.Results)
	}
	if len(got.Results[0].Steps) != 1 || got.Results[0].Steps[0].Status != "skip" {
		t.Fatalf("unexpected step json: %+v", got.Results[0].Steps)
	}
}

func TestReportMarshalJSONMatchesWriteJSON(t *testing.T) {
	rep := reportFromRunner(sampleRunnerReport())
	if rep == nil {
		t.Fatal("expected report")
	}

	var want strings.Builder
	if err := rep.WriteJSON(&want); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	got, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var wantDoc any
	if err := json.Unmarshal([]byte(want.String()), &wantDoc); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	var gotDoc any
	if err := json.Unmarshal(got, &gotDoc); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if !reflect.DeepEqual(gotDoc, wantDoc) {
		t.Fatalf("marshal mismatch\nwant:\n%s\ngot:\n%s", want.String(), string(got))
	}
}

func sampleRunnerReport() *runner.Report {
	return &runner.Report{
		Version:   "v1",
		FilePath:  "/tmp/api.http",
		EnvName:   "dev",
		StartedAt: time.Unix(10, 0).UTC(),
		EndedAt:   time.Unix(13, 0).UTC(),
		Duration:  3 * time.Second,
		Total:     5,
		Passed:    2,
		Failed:    2,
		Skipped:   1,
		Results: []runner.Result{
			{
				Kind:        runner.ResultKindRequest,
				Name:        "ok",
				Method:      "GET",
				Target:      "https://example.com/ok",
				Environment: "dev",
				Passed:      true,
				Response: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Proto:      "HTTP/1.1",
					Duration:   25 * time.Millisecond,
				},
				Stream: &runner.StreamInfo{
					Kind:       "sse",
					EventCount: 1,
					Summary: map[string]any{
						"wait": 5 * time.Millisecond,
					},
				},
				Trace: &runner.TraceInfo{
					Summary: &history.TraceSummary{
						Duration: 25 * time.Millisecond,
						Budgets: &history.TraceBudget{
							Total:     50 * time.Millisecond,
							Tolerance: 5 * time.Millisecond,
							Phases: map[string]time.Duration{
								"total": 25 * time.Millisecond,
							},
						},
					},
				},
			},
			{
				Kind:       runner.ResultKindRequest,
				Name:       "skip",
				Method:     "POST",
				Target:     "https://example.com/skip",
				Skipped:    true,
				SkipReason: "not selected",
			},
			{
				Kind:     runner.ResultKindWorkflow,
				Name:     "wf",
				Method:   "WORKFLOW",
				Duration: time.Second,
				Canceled: true,
				Summary:  "canceled",
				Passed:   false,
				Steps: []runner.StepResult{
					{
						Name:     "Login",
						Method:   "GET",
						Target:   "/login",
						Passed:   true,
						Duration: 40 * time.Millisecond,
						Response: &httpclient.Response{
							Status:     "200 OK",
							StatusCode: 200,
							Proto:      "HTTP/1.1",
						},
					},
					{
						Name:       "Guard",
						Skipped:    true,
						SkipReason: "guard",
					},
					{
						Name:     "Use",
						Method:   "GRPC",
						Target:   "/demo.Service/Use",
						Canceled: true,
						Summary:  "stop",
						GRPC: &grpcclient.Response{
							StatusCode:    codes.Canceled,
							StatusMessage: "stop",
							Duration:      10 * time.Millisecond,
						},
					},
				},
			},
			{
				Kind:     runner.ResultKindCompare,
				Name:     "cmp",
				Method:   "COMPARE",
				Duration: 50 * time.Millisecond,
				Summary:  "diffs found",
				Compare: &runner.CompareInfo{
					Baseline: "dev",
				},
				Steps: []runner.StepResult{
					{
						Name:        "dev",
						Environment: "dev",
						Passed:      true,
						Duration:    20 * time.Millisecond,
						Response: &httpclient.Response{
							Status:     "200 OK",
							StatusCode: 200,
							Proto:      "HTTP/1.1",
						},
					},
					{
						Name:        "stage",
						Environment: "stage",
						Duration:    30 * time.Millisecond,
						Err:         errors.New("stage failed"),
					},
				},
			},
			{
				Kind:     runner.ResultKindProfile,
				Name:     "prof",
				Method:   "PROFILE",
				Passed:   true,
				Duration: 2 * time.Second,
				Profile: &runner.ProfileInfo{
					Count:  4,
					Warmup: 1,
					Delay:  time.Second,
					Results: &history.ProfileResults{
						TotalRuns:      4,
						WarmupRuns:     1,
						SuccessfulRuns: 3,
						FailedRuns:     0,
						Latency: &history.ProfileLatency{
							Count:  3,
							Min:    time.Millisecond,
							Max:    3 * time.Millisecond,
							Mean:   2 * time.Millisecond,
							Median: 2 * time.Millisecond,
							StdDev: time.Millisecond,
						},
						Percentiles: []history.ProfilePercentile{
							{Percentile: 99, Value: 3 * time.Millisecond},
							{Percentile: 95, Value: 2 * time.Millisecond},
						},
						Histogram: []history.ProfileHistogramBin{
							{From: time.Millisecond, To: 3 * time.Millisecond, Count: 3},
						},
					},
				},
			},
		},
	}
}
