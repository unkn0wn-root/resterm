package runfmt

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/runx/fail"
)

func TestJSONRoundTrip(t *testing.T) {
	ms := time.Millisecond
	fail := &Failure{
		Code:     runfail.CodeTimeout,
		Category: runfail.CategoryTimeout,
		ExitCode: runfail.ExitTimeout,
		Message:  "slow",
		Source:   "error",
		Chain: []FailureChain{
			{Code: "timeout", Kind: "cause", Message: "slow", Children: []FailureChain{{Message: "dial"}}},
		},
		Frames: []FailureFrame{{Name: "check", Pos: FailurePos{Path: "api.http", Line: 3, Col: 5}}, {Name: "anon"}},
	}
	rep := Report{
		SchemaVersion:        "1",
		Version:              "v1",
		FilePath:             "api.http",
		EnvName:              "dev",
		EnvironmentSelection: map[string]string{"api": "dev"},
		StartedAt:            time.Date(2026, 10, 9, 8, 0, 0, 123000000, time.UTC),
		EndedAt:              time.Date(2026, 10, 9, 8, 0, 3, 0, time.UTC),
		Duration:             3 * time.Second,
		Total:                3,
		Passed:               2,
		Failed:               1,
		StopReason:           "fail_fast",
		Warnings:             []string{"api.http:3: unknown setting"},
		Results: []Result{
			{
				Kind:            "request",
				Name:            "ok",
				Method:          "GET",
				Target:          "https://example.com/{{id}}",
				EffectiveTarget: "https://example.com/7",
				Environment:     "dev",
				Status:          StatusPass,
				Duration:        25 * ms,
				HTTP:            &HTTP{Status: "200 OK", StatusCode: 200, Protocol: "HTTP/1.1"},
				Tests:           []Test{{Name: "status", Passed: true, Elapsed: 2 * ms}},
				Stream:          &Stream{Kind: "sse", EventCount: 2, Summary: map[string]any{"wait": 5 * ms, "n": 2}},
				Response: &Response{
					Headers:       http.Header{"Content-Type": {"application/octet-stream"}},
					Body:          []byte{0, 0xff, 'a'},
					BodySize:      4,
					BodyTruncated: true,
					Captures: []Capture{
						{Scope: "request", Name: "tok", Value: "•••", Secret: true},
						{Scope: "file", Name: "id", Value: "42"},
					},
				},
				Trace: &Trace{
					Duration: 25 * ms,
					Budget: &TraceBudget{
						Total:     50 * ms,
						Tolerance: ms,
						Phases:    map[string]time.Duration{"dns": ms},
					},
					Breaches:     []TraceBreach{{Kind: "dns", Limit: ms, Actual: 2 * ms, Over: ms}},
					ArtifactPath: "trace.json",
				},
			},
			{
				Kind:        "workflow",
				Name:        "wf",
				Method:      "WORKFLOW",
				Status:      StatusFail,
				Summary:     "1 step failed",
				Error:       "boom",
				ScriptError: "x is not defined",
				Failure:     fail,
				Steps: []Step{{
					Name:      "a",
					Branch:    "main",
					Iteration: 1,
					Total:     2,
					Status:    StatusFail,
					Failure:   fail,
					Response:  &Response{Body: []byte("ok"), BodySize: 2},
					GRPC: &GRPC{
						Code:          "Unavailable",
						StatusCode:    14,
						StatusMessage: "down",
						StatusDetails: []string{"retry"},
					},
				}, {
					Name:     "empty",
					Status:   StatusPass,
					Response: &Response{Body: []byte{}},
				}, {
					Name:     "left out",
					Status:   StatusPass,
					Response: &Response{BodySize: 7},
				}},
			},
			{
				Kind:    "profile",
				Name:    "p",
				Status:  StatusPass,
				Compare: &Compare{Baseline: "dev", Group: "api"},
				Profile: &Profile{
					Count:          3,
					Warmup:         1,
					Delay:          ms,
					TotalRuns:      3,
					WarmupRuns:     1,
					SuccessfulRuns: 2,
					FailedRuns:     1,
					Latency:        &Latency{Count: 2, Min: ms, Max: 3 * ms, Mean: 2 * ms, Median: 2 * ms, StdDev: ms},
					Percentiles:    []Percentile{{Percentile: 50, Value: 2 * ms}, {Percentile: 90, Value: 3 * ms}},
					Histogram:      []HistBin{{From: ms, To: 3 * ms, Count: 2}},
					Failures: []ProfileFailure{
						{Iteration: 2, Reason: "slow", Status: "500", StatusCode: 500, Duration: 4 * ms, Failure: fail},
					},
				},
			},
		},
	}
	first, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Report
	if err := json.Unmarshal(first, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	second, err := json.Marshal(back)
	if err != nil {
		t.Fatalf("marshal again: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("round trip changed the JSON\nfirst:  %s\nsecond: %s", first, second)
	}
	if got := back.Results[0].Response; got == nil || !bytes.Equal(got.Body, []byte{0, 0xff, 'a'}) {
		t.Fatalf("decoded response = %+v, want the binary body back", got)
	}
	if steps := back.Results[1].Steps; steps[1].Response.Body == nil || steps[2].Response.Body != nil {
		t.Fatalf("decoded bodies = %#v and %#v, want empty and nil", steps[1].Response.Body, steps[2].Response.Body)
	}
}

func TestUnmarshalNullKeepsValue(t *testing.T) {
	test := Test{Name: "status", Passed: true}
	if err := json.Unmarshal([]byte("null"), &test); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if test.Name != "status" || !test.Passed {
		t.Fatalf("null changed the value to %+v", test)
	}
}

func TestUnmarshalResponseBody(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []byte
		err  bool
	}{
		{name: "missing", in: `{"bodySize":7}`},
		{name: "null", in: `{"body":null,"bodySize":7}`},
		{name: "empty", in: `{"body":"","bodySize":0}`, want: []byte{}},
		{name: "text", in: `{"body":"ok","bodySize":2}`, want: []byte("ok")},
		{name: "base64", in: `{"body":"AP9h","bodyEncoding":"base64","bodySize":3}`, want: []byte{0, 0xff, 'a'}},
		{name: "empty base64", in: `{"body":"","bodyEncoding":"base64","bodySize":0}`, want: []byte{}},
		{name: "bad base64", in: `{"body":"not base64!","bodyEncoding":"base64","bodySize":3}`, err: true},
		{name: "unknown bodyEncoding", in: `{"body":"eJw=","bodyEncoding":"zlib","bodySize":2}`, err: true},
		{name: "unknown bodyEncoding without body", in: `{"bodyEncoding":"zlib","bodySize":3}`, err: true},
		{name: "bodyEncoding without body", in: `{"bodyEncoding":"base64","bodySize":3}`, err: true},
		{name: "bodyEncoding with a null body", in: `{"body":null,"bodyEncoding":"base64","bodySize":3}`, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp Response
			err := json.Unmarshal([]byte(tt.in), &resp)
			if (err != nil) != tt.err {
				t.Fatalf("unmarshal error = %v, want error %t", err, tt.err)
			}
			if (resp.Body == nil) != (tt.want == nil) || !bytes.Equal(resp.Body, tt.want) {
				t.Fatalf("Body = %#v, want %#v", resp.Body, tt.want)
			}
		})
	}
}
