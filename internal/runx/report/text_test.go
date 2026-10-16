package runfmt

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestWriteTextStyledColorPreservesPlainText(t *testing.T) {
	rep := &Report{
		FilePath: "workflow.http",
		EnvName:  "dev",
		Results: []Result{{
			Kind:     "workflow",
			Name:     "sample-order",
			Method:   "WORKFLOW",
			Status:   StatusPass,
			Duration: 125 * time.Millisecond,
			Steps: []Step{
				{
					Name:     "Login",
					Status:   StatusPass,
					Duration: 25 * time.Millisecond,
					HTTP:     &HTTP{Status: "200 OK", StatusCode: 200},
				},
				{
					Name:     "Checkout",
					Status:   StatusFail,
					Summary:  "unexpected status code 500",
					Duration: 50 * time.Millisecond,
				},
			},
		}},
		Total:   1,
		Passed:  0,
		Failed:  1,
		Skipped: 0,
	}

	var plain strings.Builder
	if err := WriteText(&plain, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}

	var colored strings.Builder
	if err := WriteTextStyled(&colored, rep, ansiTextPainter()); err != nil {
		t.Fatalf("WriteTextStyled(...): %v", err)
	}

	out := colored.String()
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ansi output, got %q", out)
	}
	if got := stripANSI(out); got != plain.String() {
		t.Fatalf(
			"expected stripped output to match plain text\nwant:\n%s\n\ngot:\n%s",
			plain.String(),
			got,
		)
	}
}

func TestWriteTextIncludesTargetDetailsWhenDifferent(t *testing.T) {
	rep := &Report{
		FilePath: "reports.http",
		Results: []Result{{
			Kind:            "request",
			Name:            "reports",
			Method:          "GET",
			Target:          "{{services.api.base}}/reports",
			EffectiveTarget: "https://httpbin.org/anything/api/reports",
			Status:          StatusPass,
			Duration:        463 * time.Millisecond,
			HTTP:            &HTTP{Status: "200 OK", StatusCode: 200},
			Steps: []Step{{
				Name:            "dev",
				Method:          "GET",
				Target:          "{{services.api.base}}/reports",
				EffectiveTarget: "https://dev.httpbin.org/anything/api/reports",
				Status:          StatusPass,
				Duration:        250 * time.Millisecond,
				HTTP:            &HTTP{Status: "200 OK", StatusCode: 200},
			}},
		}},
		Total:   1,
		Passed:  1,
		Failed:  0,
		Skipped: 0,
	}

	var out strings.Builder
	if err := WriteText(&out, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"Source Target: {{services.api.base}}/reports",
		"Effective Target: https://httpbin.org/anything/api/reports",
		"Effective Target: https://dev.httpbin.org/anything/api/reports",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in output, got %q", want, text)
		}
	}
}

func TestWriteTextIncludesProfileDetails(t *testing.T) {
	rep := &Report{
		FilePath: "profile.http",
		Results: []Result{{
			Kind:     "profile",
			Name:     "prof",
			Method:   "PROFILE",
			Status:   StatusFail,
			Duration: 1500 * time.Millisecond,
			Profile: &Profile{
				Count:          3,
				Warmup:         1,
				Delay:          25 * time.Millisecond,
				TotalRuns:      4,
				WarmupRuns:     1,
				SuccessfulRuns: 2,
				FailedRuns:     1,
				Latency: &Latency{
					Count:  2,
					Min:    10 * time.Millisecond,
					Max:    40 * time.Millisecond,
					Mean:   25 * time.Millisecond,
					Median: 20 * time.Millisecond,
					StdDev: 15 * time.Millisecond,
				},
				Percentiles: []Percentile{
					{Percentile: 50, Value: 20 * time.Millisecond},
					{Percentile: 90, Value: 38 * time.Millisecond},
					{Percentile: 95, Value: 39 * time.Millisecond},
					{Percentile: 99, Value: 40 * time.Millisecond},
				},
				Histogram: []HistBin{
					{From: 10 * time.Millisecond, To: 20 * time.Millisecond, Count: 1},
					{From: 20 * time.Millisecond, To: 40 * time.Millisecond, Count: 2},
				},
				Failures: []ProfileFailure{
					{
						Iteration:  1,
						Warmup:     true,
						Reason:     "dial tcp timeout",
						Duration:   5 * time.Millisecond,
						StatusCode: 0,
					},
					{
						Iteration:  4,
						Reason:     "HTTP 500",
						Status:     "500 Internal Server Error",
						StatusCode: 500,
						Duration:   40 * time.Millisecond,
					},
				},
			},
		}},
		Total:   1,
		Passed:  0,
		Failed:  1,
		Skipped: 0,
	}

	var out strings.Builder
	if err := WriteText(&out, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}

	text := out.String()
	for _, want := range []string{
		"Profile:",
		"Plan: 3 measured | 1 warmup",
		"Runs: 4 total | 2 success | 1 failure | 1 warmup",
		"Success: 67% (2/3)",
		"Delay: 25ms between runs",
		"Latency: 2 samples | min 10ms | p50 20ms | p90 38ms | p95 39ms | p99 40ms | max 40ms",
		"Stats: mean 25ms | median 20ms | stddev 15ms",
		"Histogram:",
		"10ms - 20ms | ###########........... (1, 33.3%)",
		"20ms - 40ms | ###################### (2, 66.7%)",
		"Failures:",
		"- Warmup 1: dial tcp timeout [5ms]",
		"- Run 4: HTTP 500 [500 Internal Server Error | 40ms]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in output, got %q", want, text)
		}
	}
}

func TestWriteTextProfileLatencyUsesMedianWhenP50Missing(t *testing.T) {
	rep := &Report{
		FilePath: "profile.http",
		Results: []Result{{
			Kind:   "profile",
			Name:   "prof",
			Method: "PROFILE",
			Status: StatusPass,
			Profile: &Profile{
				TotalRuns:      2,
				SuccessfulRuns: 2,
				Latency: &Latency{
					Count:  2,
					Min:    10 * time.Millisecond,
					Max:    40 * time.Millisecond,
					Mean:   25 * time.Millisecond,
					Median: 25 * time.Millisecond,
					StdDev: 15 * time.Millisecond,
				},
			},
		}},
		Total:  1,
		Passed: 1,
	}

	var out strings.Builder
	if err := WriteText(&out, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}

	text := out.String()
	if !strings.Contains(text, "Latency: 2 samples | min 10ms | median 25ms | max 40ms") {
		t.Fatalf("expected median fallback in latency row, got %q", text)
	}
	if strings.Contains(text, "p50 25ms") {
		t.Fatalf("did not expect p50 fallback label, got %q", text)
	}
}

func TestWriteTextStyledProfileHistogramUsesAnsiColors(t *testing.T) {
	rep := &Report{
		FilePath: "profile.http",
		Results: []Result{{
			Kind:   "profile",
			Name:   "prof",
			Method: "PROFILE",
			Status: StatusPass,
			Profile: &Profile{
				TotalRuns:      3,
				SuccessfulRuns: 3,
				Latency: &Latency{
					Count:  3,
					Min:    10 * time.Millisecond,
					Max:    70 * time.Millisecond,
					Mean:   40 * time.Millisecond,
					Median: 40 * time.Millisecond,
					StdDev: 25 * time.Millisecond,
				},
				Percentiles: []Percentile{
					{Percentile: 50, Value: 30 * time.Millisecond},
					{Percentile: 90, Value: 60 * time.Millisecond},
				},
				Histogram: []HistBin{
					{From: 10 * time.Millisecond, To: 20 * time.Millisecond, Count: 1},
					{From: 30 * time.Millisecond, To: 50 * time.Millisecond, Count: 1},
					{From: 60 * time.Millisecond, To: 70 * time.Millisecond, Count: 1},
				},
			},
		}},
		Total:  1,
		Passed: 1,
	}

	var plain strings.Builder
	if err := WriteText(&plain, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}

	var colored strings.Builder
	if err := WriteTextStyled(&colored, rep, ansiTextPainter()); err != nil {
		t.Fatalf("WriteTextStyled(...): %v", err)
	}

	out := colored.String()
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("expected ansi output, got %q", out)
	}
	if got := stripANSI(out); got != plain.String() {
		t.Fatalf(
			"expected stripped profile output to match plain text\nwant:\n%s\n\ngot:\n%s",
			plain.String(),
			got,
		)
	}
}

func ansiTextPainter() TextPainter {
	return TextPaintFunc(func(text, _ string, _ bool) string {
		if text == "" {
			return text
		}
		return "\x1b[31m" + text + "\x1b[0m"
	})
}

var ansiSeq = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func stripANSI(s string) string {
	return ansiSeq.ReplaceAllString(s, "")
}

func TestWriteTextIncludesResultWarnings(t *testing.T) {
	rep := &Report{
		FilePath: "a.http",
		Results: []Result{{
			Kind:     "request",
			Name:     "one",
			Method:   "GET",
			Target:   "http://example.test",
			Status:   StatusPass,
			Warnings: []string{"Script sends {{token}} in header X-Id as written."},
		}},
		Total:  1,
		Passed: 1,
	}
	var out strings.Builder
	if err := WriteText(&out, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}
	want := "PASS GET one\n  WARN Script sends {{token}} in header X-Id as written.\n"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("expected %q in output, got %q", want, out.String())
	}
}

func TestErrorRowsStayOnOneLine(t *testing.T) {
	two := "join.http:2:15: boom\n\n  test script 1: Error: bad  "
	rep := &Report{
		Results: []Result{
			{Kind: "request", Name: "script", Method: "GET", Status: StatusFail, ScriptError: two},
			{Kind: "request", Name: "request", Method: "GET", Status: StatusFail, Error: two},
			{Kind: "workflow", Name: "flow", Method: "WORKFLOW", Status: StatusFail, Steps: []Step{
				{Name: "step-script", Status: StatusFail, ScriptError: two},
				{Name: "step-request", Status: StatusFail, Error: two},
			}},
		},
	}
	var text, junit strings.Builder
	if err := WriteText(&text, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}
	if err := WriteJUnit(&junit, rep); err != nil {
		t.Fatalf("WriteJUnit(...): %v", err)
	}
	for _, name := range []string{"GET script", "GET request", "step-script", "step-request"} {
		row := name + " [join.http:2:15: boom; test script 1: Error: bad]"
		if !strings.Contains(text.String(), row+"\n") {
			t.Fatalf("text output lacks row %q:\n%s", row, text.String())
		}
		if out := "<system-out>" + row + "</system-out>"; !strings.Contains(junit.String(), out) {
			t.Fatalf("junit output lacks %q:\n%s", out, junit.String())
		}
	}
}

func TestWriteTextLeavesBlankLinesEmpty(t *testing.T) {
	rendered := "error[script]: a\n   1 | fail(\"a\") \n\nerror[script]: b"
	rep := &Report{
		Results: []Result{{
			Kind:              "request",
			Name:              "two",
			Method:            "GET",
			Status:            StatusFail,
			ScriptErrorDetail: &ErrorDetail{Rendered: rendered},
		}},
	}
	var out strings.Builder
	if err := WriteText(&out, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}
	want := "  Script Error:\n" +
		"    error[script]: a\n" +
		"       1 | fail(\"a\") \n" +
		"\n" +
		"    error[script]: b\n"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("output =\n%q\nwant it to contain\n%q", out.String(), want)
	}
}

func TestProfileFailureReasonStaysOnOneLine(t *testing.T) {
	rep := &Report{
		Results: []Result{{
			Kind:   "profile",
			Name:   "prof",
			Method: "PROFILE",
			Status: StatusFail,
			Profile: &Profile{Failures: []ProfileFailure{
				{Iteration: 1, Reason: "a\nb", Duration: 5 * time.Millisecond},
				{Iteration: 2, Reason: "plain", Duration: 5 * time.Millisecond},
			}},
		}},
	}
	var out strings.Builder
	if err := WriteText(&out, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}
	for _, want := range []string{"- Run 1: a; b [5ms]\n", "- Run 2: plain [5ms]\n"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("expected %q in output, got %q", want, out.String())
		}
	}
}

func TestTextRowsEscapeControlCharacters(t *testing.T) {
	msg := "a.http:2:1: bad \x1b]0;pwn\a body\r\nnext"
	rep := &Report{
		Results: []Result{
			{Kind: "request", Name: "script", Method: "GET", Status: StatusFail, ScriptError: msg},
			{Kind: "workflow", Name: "flow", Method: "WORKFLOW", Status: StatusFail, Steps: []Step{
				{Name: "step", Status: StatusFail, ScriptError: msg},
			}},
			{Kind: "profile", Name: "prof", Method: "PROFILE", Status: StatusFail, Profile: &Profile{
				Failures: []ProfileFailure{{Iteration: 1, Reason: msg}},
			}},
		},
	}
	var text, junit strings.Builder
	if err := WriteText(&text, rep); err != nil {
		t.Fatalf("WriteText(...): %v", err)
	}
	if err := WriteJUnit(&junit, rep); err != nil {
		t.Fatalf("WriteJUnit(...): %v", err)
	}
	out := text.String()
	esc := `a.http:2:1: bad \u001b]0;pwn\u0007 body; next`
	for _, want := range []string{"GET script [" + esc + "]\n", "step [" + esc + "]\n", "- Run 1: " + esc + "\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got %q", want, out)
		}
	}
	if strings.ContainsAny(out, "\x1b\a\r") {
		t.Fatalf("text output keeps raw control characters: %q", out)
	}
	if strings.Contains(junit.String(), `\u001b`) {
		t.Fatalf("junit output escapes for the terminal: %q", junit.String())
	}
}
