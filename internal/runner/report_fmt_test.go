package runner

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/runx/report"
	"github.com/unkn0wn-root/resterm/internal/scriptapi"
	"google.golang.org/grpc/codes"
)

func TestReportModelTrimsStrings(t *testing.T) {
	rep := &Report{
		Version: "  v1  ",
		EnvName: "  dev  ",
		Results: []Result{{
			Kind:            ResultKindCompare,
			Name:            "  cmp  ",
			Method:          "  compare  ",
			Target:          "  https://example.com  ",
			EffectiveTarget: "  https://api.example.com  ",
			Environment:     "  dev  ",
			Summary:         "  ok  ",
			Passed:          true,
			SkipReason:      "  skipped  ",
			Response: &httpx.Response{
				Status: "  200 OK  ",
				Proto:  "  HTTP/1.1  ",
			},
			Tests: []scriptapi.TestResult{{
				Name:    "  status  ",
				Message: "  ok  ",
				Passed:  true,
			}},
			Compare: &CompareInfo{Baseline: "  stage  "},
			Profile: &ProfileInfo{
				Failures: []ProfileFailure{{
					Reason: "  boom  ",
					Status: "  500  ",
				}},
			},
			Stream: &StreamInfo{
				Kind:           "  ws  ",
				TranscriptPath: "  stream.log  ",
				Summary:        map[string]any{"ok": true},
			},
			Trace: &TraceInfo{
				Summary: &history.TraceSummary{
					Error: "  slow  ",
					Breaches: []history.TraceBreach{{
						Kind: "  total  ",
					}},
				},
				ArtifactPath: "  trace.json  ",
			},
			Steps: []StepResult{{
				Name:            "  step  ",
				Method:          "  grpc  ",
				Target:          "  /svc.Method  ",
				EffectiveTarget: "  api.example.com:443  ",
				Environment:     "  dev  ",
				Branch:          "  if-true  ",
				Summary:         "  ok  ",
				Passed:          true,
				SkipReason:      "  skipped  ",
				GRPC: &grpcx.Response{
					StatusCode:    codes.OK,
					StatusMessage: "  ok  ",
				},
				Tests: []scriptapi.TestResult{{
					Name:    "  pass  ",
					Message: "  ok  ",
					Passed:  true,
				}},
				Stream: &StreamInfo{
					Kind:           "  sse  ",
					TranscriptPath: "  step.log  ",
				},
				Trace: &TraceInfo{
					Summary: &history.TraceSummary{
						Error: "  later  ",
						Breaches: []history.TraceBreach{{
							Kind: "  phase  ",
						}},
					},
					ArtifactPath: "  step-trace.json  ",
				},
			}},
		}},
	}

	got := ReportModel(rep)
	if got.Version != "v1" || got.EnvName != "dev" {
		t.Fatalf("expected top-level strings trimmed, got %+v", got)
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected one result, got %+v", got.Results)
	}

	res := got.Results[0]
	if res.Name != "cmp" || res.Method != "compare" || res.Target != "https://example.com" ||
		res.EffectiveTarget != "https://api.example.com" ||
		res.Environment != "dev" || res.Summary != "ok" || res.SkipReason != "skipped" {
		t.Fatalf("expected result strings trimmed, got %+v", res)
	}
	if res.HTTP == nil || res.HTTP.Status != "200 OK" || res.HTTP.Protocol != "HTTP/1.1" {
		t.Fatalf("expected http strings trimmed, got %+v", res.HTTP)
	}
	if len(res.Tests) != 1 || res.Tests[0].Name != "status" || res.Tests[0].Message != "ok" {
		t.Fatalf("expected test strings trimmed, got %+v", res.Tests)
	}
	if res.Compare == nil || res.Compare.Baseline != "stage" {
		t.Fatalf("expected compare strings trimmed, got %+v", res.Compare)
	}
	if res.Profile == nil || len(res.Profile.Failures) != 1 ||
		res.Profile.Failures[0].Reason != "boom" || res.Profile.Failures[0].Status != "500" {
		t.Fatalf("expected profile failure strings trimmed, got %+v", res.Profile)
	}
	if res.Stream == nil || res.Stream.Kind != "ws" || res.Stream.TranscriptPath != "stream.log" {
		t.Fatalf("expected stream strings trimmed, got %+v", res.Stream)
	}
	if res.Trace == nil || res.Trace.Error != "slow" || res.Trace.ArtifactPath != "trace.json" ||
		len(res.Trace.Breaches) != 1 || res.Trace.Breaches[0].Kind != "total" {
		t.Fatalf("expected trace strings trimmed, got %+v", res.Trace)
	}
	if len(res.Steps) != 1 {
		t.Fatalf("expected one step, got %+v", res.Steps)
	}

	step := res.Steps[0]
	if step.Name != "step" || step.Method != "grpc" || step.Target != "/svc.Method" ||
		step.EffectiveTarget != "api.example.com:443" ||
		step.Environment != "dev" || step.Branch != "if-true" || step.Summary != "ok" ||
		step.SkipReason != "skipped" {
		t.Fatalf("expected step strings trimmed, got %+v", step)
	}
	if step.GRPC == nil || step.GRPC.StatusMessage != "ok" {
		t.Fatalf("expected grpc strings trimmed, got %+v", step.GRPC)
	}
	if len(step.Tests) != 1 || step.Tests[0].Name != "pass" || step.Tests[0].Message != "ok" {
		t.Fatalf("expected step test strings trimmed, got %+v", step.Tests)
	}
	if step.Stream == nil || step.Stream.Kind != "sse" || step.Stream.TranscriptPath != "step.log" {
		t.Fatalf("expected step stream strings trimmed, got %+v", step.Stream)
	}
	if step.Trace == nil || step.Trace.Error != "later" ||
		step.Trace.ArtifactPath != "step-trace.json" ||
		len(step.Trace.Breaches) != 1 ||
		step.Trace.Breaches[0].Kind != "phase" {
		t.Fatalf("expected step trace strings trimmed, got %+v", step.Trace)
	}
}

func TestReportModelUsesStructuredProfileFailure(t *testing.T) {
	rep := &Report{
		Results: []Result{{
			Kind:   ResultKindProfile,
			Name:   "prof",
			Passed: false,
			Profile: &ProfileInfo{
				Failures: []ProfileFailure{{
					Reason:     "HTTP 500",
					StatusCode: 500,
					Failure: runfail.New(
						runfail.CodeTimeout,
						"context deadline exceeded",
						"profile",
					),
				}},
			},
		}},
	}

	got := ReportModel(rep)
	if len(got.Results) != 1 || got.Results[0].Profile == nil ||
		len(got.Results[0].Profile.Failures) != 1 {
		t.Fatalf("unexpected formatted profile: %+v", got.Results)
	}
	failure := got.Results[0].Profile.Failures[0].Failure
	if failure == nil || failure.Code != runfail.CodeTimeout ||
		failure.ExitCode != runfail.ExitTimeout ||
		failure.Message != "context deadline exceeded" {
		t.Fatalf("unexpected profile failure: %+v", failure)
	}
	if got.Results[0].Failure == nil ||
		got.Results[0].Failure.Code != runfail.CodeTimeout {
		t.Fatalf(
			"expected result failure to use structured profile failure, got %+v",
			got.Results[0].Failure,
		)
	}
}

func TestReportModelIncludesErrorDetails(t *testing.T) {
	reqErr := diag.Wrap(
		&url.Error{
			Op:  "Get",
			URL: "https://api.local",
			Err: &net.DNSError{Err: "no such host", Name: "api.local"},
		},
		"perform request",
		diag.WithComponent(diag.ComponentHTTP),
	)
	scriptErr := errors.New("script boom")
	rep := &Report{
		Results: []Result{{
			Kind:      ResultKindRequest,
			Name:      "lookup",
			Err:       reqErr,
			ScriptErr: scriptErr,
			Profile: &ProfileInfo{
				Failures: []ProfileFailure{{
					Reason:  "request failed",
					Err:     reqErr,
					Failure: runfail.FromErrorSource(reqErr, "profile"),
				}},
			},
			Steps: []StepResult{{
				Name: "step",
				Err:  reqErr,
			}},
		}},
	}

	got := ReportModel(rep)
	res := got.Results[0]
	if res.ErrorDetail == nil ||
		!strings.Contains(res.ErrorDetail.Rendered, "\nperform request\n") ||
		res.ScriptErrorDetail == nil ||
		!strings.Contains(res.ScriptErrorDetail.Rendered, "script boom") {
		t.Fatalf("expected result error details, got %+v", res)
	}
	if len(res.Steps) != 1 || res.Steps[0].ErrorDetail == nil ||
		!strings.Contains(res.Steps[0].ErrorDetail.Rendered, "lookup api.local") {
		t.Fatalf("expected step error detail, got %+v", res.Steps)
	}
	if res.Profile == nil || len(res.Profile.Failures) != 1 ||
		res.Profile.Failures[0].Failure == nil ||
		len(res.Profile.Failures[0].Failure.Chain) == 0 {
		t.Fatalf("expected profile failure chain, got %+v", res.Profile)
	}
}

func TestReportModelKeepsJoinedErrorText(t *testing.T) {
	err := fmt.Errorf("perform request: %w", errors.Join(errors.New("dial a"), errors.New("close b")))
	res := ReportModel(&Report{Results: []Result{{Name: "r", Err: err}}}).Results[0]
	const want = "perform request: dial a\nclose b"
	if res.Error != want || res.Failure.Message != want {
		t.Fatalf("Error = %q, Failure.Message = %q, want %q", res.Error, res.Failure.Message, want)
	}
	const head = "error[unknown]: perform request: dial a; close b\n"
	if got := res.ErrorDetail.Rendered; !strings.HasPrefix(got, head) {
		t.Fatalf("Rendered = %q, want it to start with %q", got, head)
	}
}

func TestReportModelUsesResponseDuration(t *testing.T) {
	rep := &Report{
		Results: []Result{{
			Passed: true,
			Response: &httpx.Response{
				Duration: 25 * time.Millisecond,
			},
		}},
	}

	got := ReportModel(rep)
	if len(got.Results) != 1 || got.Results[0].Duration != 25*time.Millisecond {
		t.Fatalf("expected response duration fallback, got %+v", got.Results)
	}
}

func TestFormatStreamCarriesTheFailure(t *testing.T) {
	got := formatStream(&StreamInfo{Kind: "sse", Err: errors.New("sse line exceeds 16 bytes")}, redact.Set{})
	if got.Error != "sse line exceeds 16 bytes" {
		t.Fatalf("Error = %q, want the stream failure", got.Error)
	}
	if plain := formatStream(&StreamInfo{Kind: "sse"}, redact.Set{}); plain.Error != "" {
		t.Fatalf("Error = %q, want a complete transcript to report none", plain.Error)
	}
}

// A stream carries its failure as text, so the class it recorded is what sets
// the reported failure and the exit code.
func TestStreamFailureKeepsItsExitCode(t *testing.T) {
	tests := []struct {
		name string
		sum  httpx.WebSocketSummary
		code runfail.Code
		exit int
	}{
		{
			name: "unreadable payload file",
			sum: httpx.WebSocketSummary{
				ClosedBy:    "error",
				CloseReason: "websocket step 2:send_file: open payload.bin: no such file",
				ErrorClass:  diag.ClassFilesystem,
			},
			code: runfail.CodeFilesystem,
			exit: runfail.ExitFilesystem,
		},
		{
			name: "broken frame",
			sum: httpx.WebSocketSummary{
				ClosedBy:    "error",
				CloseReason: "read websocket message: bad frame",
				ErrorClass:  diag.ClassProtocol,
			},
			code: runfail.CodeProtocol,
			exit: runfail.ExitProtocol,
		},
		{
			name: "connection dropped",
			sum: httpx.WebSocketSummary{
				ClosedBy:    "error",
				CloseReason: "read websocket message: connection reset by peer",
				ErrorClass:  diag.ClassNetwork,
			},
			code: runfail.CodeNetwork,
			exit: runfail.ExitNetwork,
		},
		{
			name: "a transcript with no class",
			sum:  httpx.WebSocketSummary{ClosedBy: "error", CloseReason: "something broke"},
			code: runfail.CodeProtocol,
			exit: runfail.ExitProtocol,
		},
		{
			name: "a deadline the caller set",
			sum: httpx.WebSocketSummary{
				ClosedBy:    "timeout",
				CloseReason: "context deadline exceeded",
				ErrorClass:  diag.ClassTimeout,
			},
			code: runfail.CodeTimeout,
			exit: runfail.ExitTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Result{Stream: &StreamInfo{Kind: "websocket", Err: tt.sum.Err()}}

			got, _ := resultFailure(res)
			if got.Code != tt.code || got.ExitCode != tt.exit {
				t.Fatalf("failure = %s/%d, want %s/%d", got.Code, got.ExitCode, tt.code, tt.exit)
			}
			if got.Source != "stream" {
				t.Fatalf("source = %q, want %q", got.Source, "stream")
			}
		})
	}
}

func TestWebSocketHandshakeTimeoutExitCode(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()

	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "ws://" + ln.Addr().String() + "/ws",
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{HandshakeTimeout: 200 * time.Millisecond},
		},
	}
	_, _, err = httpx.NewClient(nil).StartWebSocket(t.Context(), req, nil, httpx.Options{})

	got, _ := resultFailure(Result{Err: err})
	if got.Code != runfail.CodeTimeout || got.ExitCode != runfail.ExitTimeout {
		t.Fatalf("failure = %s/%d, want %s/%d",
			got.Code, got.ExitCode, runfail.CodeTimeout, runfail.ExitTimeout)
	}
}

func TestReachedStreamLimitIsNotAFailure(t *testing.T) {
	sums := []httpx.WebSocketSummary{
		{ClosedBy: "timeout", CloseReason: "idle timeout after 5s"},
		{ClosedBy: "server", CloseReason: "bye"},
		{ClosedBy: "client"},
	}

	for _, sum := range sums {
		if err := sum.Err(); err != nil {
			t.Fatalf("closedBy %q ended as a failure: %v", sum.ClosedBy, err)
		}
	}
}

func TestReportModelIgnoresWarmupProfileFailures(t *testing.T) {
	warm := ProfileFailure{Iteration: 1, Warmup: true, Failure: runfail.Assertion("HTTP 503", "profile")}
	measured := ProfileFailure{Iteration: 2, Failure: runfail.Assertion("HTTP 500", "profile")}
	tests := []struct {
		name     string
		failures []ProfileFailure
		passed   bool
		wantExit int
	}{
		{name: "warmup only", failures: []ProfileFailure{warm}, passed: true, wantExit: runfail.ExitPass},
		{name: "measured", failures: []ProfileFailure{warm, measured}, wantExit: runfail.ExitFailure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := Result{Kind: ResultKindProfile, Name: "prof", Passed: test.passed}
			res.Profile = &ProfileInfo{Failures: test.failures}
			res.Failure, res.failureFrom = resultFailure(res)
			if got := resultFailed(res); got == test.passed {
				t.Fatalf("resultFailed() = %t, want %t", got, !test.passed)
			}
			got := ReportModel(&Report{Results: []Result{res}})
			if code := got.ExitCode(runfail.ExitDetailed); code != test.wantExit {
				t.Fatalf("ExitCode() = %d, want %d", code, test.wantExit)
			}
			if !test.passed && got.Results[0].Failure.Message != "HTTP 500" {
				t.Fatalf("result failure = %+v, want the measured one", got.Results[0].Failure)
			}
		})
	}
}

func TestReportModelKeepsInheritedFailureDetails(t *testing.T) {
	stepErr := diag.Wrap(diag.New(diag.ClassScript, "assert failed"), "run check")
	profErr := diag.Wrap(diag.New(diag.ClassTimeout, "too slow"), "send request")
	results := []Result{
		{
			Kind:  ResultKindWorkflow,
			Steps: []StepResult{{Name: "ok", Passed: true}, {Name: "check", Err: stepErr}},
		},
		{
			Kind: ResultKindProfile,
			Profile: &ProfileInfo{Failures: []ProfileFailure{
				{Iteration: 1, Err: profErr, Failure: runfail.FromError(profErr)},
			}},
		},
	}
	for _, stored := range []bool{false, true} {
		in := append([]Result(nil), results...)
		if stored {
			for i := range in {
				in[i].Failure, in[i].failureFrom = resultFailure(in[i])
			}
		}
		m := ReportModel(&Report{Results: in})
		children := []*runfmt.Failure{m.Results[0].Steps[1].Failure, m.Results[1].Profile.Failures[0].Failure}
		for i, child := range children {
			got := m.Results[i].Failure
			if len(child.Chain) == 0 || !reflect.DeepEqual(got, child) {
				t.Fatalf("stored=%v result %d failure = %+v, want the child's %+v", stored, i, got, child)
			}
		}
	}
}

func TestReportModelMasksEveryField(t *testing.T) {
	for typ, want := range map[reflect.Type]int{
		reflect.TypeFor[Result]():         32,
		reflect.TypeFor[StepResult]():     27,
		reflect.TypeFor[ProfileFailure](): 8,
		reflect.TypeFor[runfmt.Result]():  27,
		reflect.TypeFor[runfmt.Step]():    25,
		reflect.TypeFor[runfmt.Report]():  15,
	} {
		if got := typ.NumField(); got != want {
			t.Fatalf("%v has %d fields, want %d: give the new field a secret below and mask it", typ, got, want)
		}
	}

	const sec = `tök/s3c"7`
	esc := func(s string) string { return strings.Trim(strconv.Quote(s), `"`) }
	q := url.QueryEscape(sec)
	srcErr := diag.FromReport(diag.Report{
		Path:   "mask.http",
		Source: []byte("GET https://h/x\n# @assert vars.get(\"t\") == \"" + sec + "\"\n"),
		Items: []diag.Diagnostic{{
			Class:    diag.ClassScript,
			Severity: diag.SeverityError,
			Message:  "assert failed for " + sec + " p1a1n",
			Span:     diag.Span{Start: diag.Pos{Line: 2, Col: 3}},
			Chain:    []diag.ChainEntry{{Message: "cause " + q}},
			Frames:   []diag.StackFrame{{Name: "fn " + sec}},
		}},
	}, errors.New("assert failed"))
	step := func(name string) StepResult {
		return StepResult{
			Name:            name + " " + sec,
			Method:          "GET " + sec,
			Target:          "https://h/s?k=" + q + "&p=p1a1n",
			EffectiveTarget: "https://h/s?k=" + strings.ToLower(q),
			Environment:     "dev",
			Branch:          "when " + esc(sec),
			Summary:         "summary " + sec + " p1a1n",
			SkipReason:      "skip " + sec,
			Response:        &httpx.Response{Status: "401 " + sec + " p1a1n", StatusCode: 401, Proto: "HTTP/1.1"},
			Err:             srcErr,
			ScriptErr:       errors.New("script " + strconv.Quote(sec)),
			Tests:           []scriptapi.TestResult{{Name: "t " + sec, Message: "m " + sec + " p1a1n"}},
			Stream: &StreamInfo{
				Kind:    "websocket",
				Summary: map[string]any{"closeReason": "denied " + esc(sec)},
			},
			Trace: &TraceInfo{Summary: &history.TraceSummary{Error: "Get https://h/?k=" + q}},
		}
	}
	grpcResult := Result{
		Kind: ResultKindRequest,
		Name: "rpc",
		GRPC: &grpcx.Response{
			StatusCode:    codes.Unauthenticated,
			StatusMessage: "denied " + sec,
			StatusDetails: []string{`{"reason":"` + esc(sec) + `","plain":"p1a1n"}`},
		},
		Stream: &StreamInfo{Kind: "grpc", Err: errors.New("stream " + sec)},
	}
	request := Result{
		Kind:            ResultKindRequest,
		Name:            "login " + sec + " p1a1n",
		Method:          "GET",
		Target:          "https://h/x?k=" + q + "&p=p1a1n",
		EffectiveTarget: "https://h/final?k=" + esc(sec),
		Environment:     "dev",
		Summary:         "summary " + sec,
		Response:        &httpx.Response{Status: "401 " + sec, StatusCode: 401, Proto: "HTTP/1.1"},
		Err:             srcErr,
		ScriptErr:       errors.New("script " + sec),
		Tests:           []scriptapi.TestResult{{Name: "t " + sec, Message: "m " + sec}},
		Warnings:        []string{"warn " + sec + " p1a1n"},
		Stream:          &StreamInfo{Kind: "sse", Summary: map[string]any{"lastEvent": sec, "eventCount": 2}},
		Trace:           &TraceInfo{Summary: &history.TraceSummary{Error: "Get https://h/?k=" + q}},
	}
	workflow := Result{
		Kind:    ResultKindWorkflow,
		Name:    "flow " + sec,
		Method:  "WORKFLOW",
		Summary: "flow failed at " + sec,
		Steps:   []StepResult{step("one"), step("two")},
	}
	forEach := workflow
	forEach.Kind, forEach.Name = ResultKindForEach, "GET https://h/each?k="+q
	compare := Result{
		Kind:    ResultKindCompare,
		Name:    "cmp " + sec,
		Target:  "https://h/c?k=" + q,
		Compare: &CompareInfo{Baseline: "dev", Group: "region"},
		Steps:   []StepResult{step("dev"), step("stage")},
	}
	profile := Result{
		Kind:   ResultKindProfile,
		Name:   "prof " + sec,
		Target: "https://h/p?k=" + q,
		Profile: &ProfileInfo{Count: 1, Failures: []ProfileFailure{{
			Iteration: 1,
			Reason:    "HTTP 401 " + sec,
			Status:    "401 " + sec,
			Err:       srcErr,
			Failure:   runfail.New(runfail.CodeAssertion, "HTTP 401 "+sec, "profile "+sec),
		}}},
	}
	skipped := Result{
		Kind:       ResultKindRequest,
		Name:       "skipped " + sec,
		Target:     "https://h/k?k=" + q,
		Skipped:    true,
		SkipReason: "skipped after --fail-fast " + sec,
	}
	rep := &Report{Warnings: []string{"mask.http:3: @capture scope \"" + sec + "\""}, secrets: redact.New(sec)}
	for _, res := range []Result{request, grpcResult, workflow, forEach, compare, profile, skipped} {
		rep.add(res)
	}

	var shown []string
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.String:
			shown = append(shown, v.String())
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Struct:
			for i := range v.NumField() {
				walk(v.Field(i))
			}
		case reflect.Slice:
			if v.Type().Elem().Kind() == reflect.Uint8 {
				shown = append(shown, string(v.Bytes()))
				return
			}
			for i := range v.Len() {
				walk(v.Index(i))
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				walk(k)
				walk(v.MapIndex(k))
			}
		}
	}
	walk(reflect.ValueOf(ReportModel(rep)))
	var outs strings.Builder
	for _, write := range []func(io.Writer) error{rep.WriteJSON, rep.WriteJUnit, rep.WriteText} {
		if err := write(&outs); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	shown = append(shown, outs.String())

	plain := 0
	for _, s := range shown {
		for _, form := range []string{sec, q, strings.ToLower(q), esc(sec), "s3c"} {
			if strings.Contains(s, form) {
				t.Errorf("report shows %q in %q", form, s)
			}
		}
		if strings.Contains(s, "p1a1n") {
			plain++
		}
	}
	if plain < 10 {
		t.Errorf("only %d report strings keep p1a1n, want the text around each secret kept", plain)
	}
	if rep.Results[0].Name != request.Name || rep.Results[0].Err != srcErr {
		t.Errorf("ReportModel changed the report: %q %v", rep.Results[0].Name, rep.Results[0].Err)
	}
}

func BenchmarkReportModel(b *testing.B) {
	var secs []string
	for i := range 20 {
		secs = append(secs, fmt.Sprintf("s3cr3t-%02d-%x", i, i*7919))
	}
	rep := &Report{secrets: redact.New(secs...)}
	for i := range 1000 {
		rep.add(Result{
			Kind:            ResultKindRequest,
			Name:            fmt.Sprintf("request %d", i),
			Method:          "GET",
			Target:          fmt.Sprintf("https://api.example.test/v1/items/%d?key={{key}}", i),
			EffectiveTarget: fmt.Sprintf("https://api.example.test/v1/items/%d?key=%s", i, secs[i%20]),
			Response:        &httpx.Response{Status: "200 OK", StatusCode: 200, Proto: "HTTP/1.1"},
			Tests:           []scriptapi.TestResult{{Name: "status is 200", Passed: true}},
		})
	}
	for b.Loop() {
		ReportModel(rep)
	}
}
