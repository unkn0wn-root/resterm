package runfmt

import (
	"fmt"
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

func TestWriteTextListsFailedTests(t *testing.T) {
	orders := "api.http:10: order 1: total -5 is negative\napi.http:10: order 3: no items"
	tests := []struct {
		name string
		rep  *Report
		want string
	}{
		{
			name: "results",
			rep: &Report{
				FilePath: "users.http",
				Results: []Result{
					{Kind: "request", Name: "one", Method: "GET", Status: StatusPass, Tests: []Test{
						{Name: "status is 200", Passed: true},
					}},
					{Kind: "request", Name: "two", Method: "GET", Status: StatusFail, Tests: []Test{
						{Name: "status is 200", Passed: true},
						{Name: "response.statusCode == 200"},
						{Name: "user has an id", Message: "two.http:9: no id"},
					}},
					{
						Kind:              "request",
						Name:              "three",
						Method:            "GET",
						Status:            StatusFail,
						ScriptError:       "three.http:4: boom",
						ScriptErrorDetail: &ErrorDetail{Rendered: "error[script]: boom\n --> three.http:4"},
						Tests:             []Test{{Name: "body has items"}},
					},
				},
				Total:  3,
				Passed: 1,
				Failed: 2,
			},
			want: "Running 3 request(s) from users.http with env <default>\n" +
				"PASS GET one\n" +
				"FAIL GET two [2 test(s) failed]\n" +
				"  Failed Tests:\n" +
				"    response.statusCode == 200\n" +
				"    user has an id - two.http:9: no id\n" +
				"FAIL GET three [three.http:4: boom]\n" +
				"  Script Error:\n" +
				"    error[script]: boom\n" +
				"     --> three.http:4\n" +
				"  Failed Tests:\n" +
				"    body has items\n" +
				"Summary: total=3 passed=1 failed=2 skipped=0\n",
		},
		{
			name: "for-each",
			rep: &Report{
				FilePath: "users.http",
				EnvName:  "dev",
				Results: []Result{{
					Kind:   "for-each",
					Name:   "POST users",
					Method: "FOR-EACH",
					Status: StatusFail,
					Steps: []Step{
						{
							Name:            "POST users (1/2)",
							Target:          "users",
							EffectiveTarget: "http://127.0.0.1:18765/v1/users",
							Status:          StatusPass,
							HTTP:            &HTTP{Status: "201 Created", StatusCode: 201},
							Tests:           []Test{{Name: "user has an id", Passed: true}},
						},
						{
							Name:            "POST users (2/2)",
							Target:          "users",
							EffectiveTarget: "http://127.0.0.1:18765/v1/users",
							Status:          StatusFail,
							HTTP:            &HTTP{Status: "201 Created", StatusCode: 201},
							Tests:           []Test{{Name: "user has an id", Message: "users.http:10: no id for tom"}},
						},
					},
				}},
				Total:  1,
				Failed: 1,
			},
			want: "Running 1 target(s) from users.http with env dev\n" +
				"FAIL FOR-EACH POST users [1 passed, 1 failed, 0 skipped]\n" +
				"  1. PASS POST users (1/2) [201 Created]\n" +
				"    Source Target: users\n" +
				"    Effective Target: http://127.0.0.1:18765/v1/users\n" +
				"  2. FAIL POST users (2/2) [1 test(s) failed]\n" +
				"    Source Target: users\n" +
				"    Effective Target: http://127.0.0.1:18765/v1/users\n" +
				"    Failed Tests:\n" +
				"      user has an id - users.http:10: no id for tom\n" +
				"Summary: total=1 passed=0 failed=1 skipped=0\n",
		},
		{
			name: "workflow",
			rep: &Report{
				FilePath: "api.http",
				Results: []Result{{
					Kind:   "workflow",
					Name:   "orders",
					Method: "WORKFLOW",
					Status: StatusFail,
					Steps: []Step{{
						Name:   "check",
						Status: StatusFail,
						Tests:  []Test{{Name: "orders are valid", Message: orders}},
					}},
				}},
				Total:  1,
				Failed: 1,
			},
			want: "Running 1 target(s) from api.http with env <default>\n" +
				"FAIL WORKFLOW orders [0 passed, 1 failed, 0 skipped]\n" +
				"  1. FAIL check [1 test(s) failed]\n" +
				"    Failed Tests:\n" +
				"      orders are valid - api.http:10: order 1: total -5 is negative\n" +
				"                         api.http:10: order 3: no items\n" +
				"Summary: total=1 passed=0 failed=1 skipped=0\n",
		},
		{
			name: "compare",
			rep: &Report{
				FilePath: "users.http",
				Results: []Result{{
					Kind:    "compare",
					Name:    "users",
					Method:  "COMPARE",
					Status:  StatusFail,
					Compare: &Compare{Baseline: "dev"},
					Steps: []Step{
						{
							Name:   "dev",
							Status: StatusPass,
							HTTP:   &HTTP{Status: "200 OK", StatusCode: 200},
							Tests:  []Test{{Name: "status is 200", Passed: true}},
						},
						{
							Name:   "prod",
							Status: StatusFail,
							HTTP:   &HTTP{Status: "500 Internal Server Error", StatusCode: 500},
							Tests:  []Test{{Name: "status is 200", Message: "users.http:5: got 500"}},
						},
					},
				}},
				Total:  1,
				Failed: 1,
			},
			want: "Running 1 target(s) from users.http with env <default>\n" +
				"FAIL COMPARE users [baseline: dev, 1 passed, 1 failed, 0 skipped]\n" +
				"  1. PASS dev [200 OK]\n" +
				"  2. FAIL prod [1 test(s) failed]\n" +
				"    Failed Tests:\n" +
				"      status is 200 - users.http:5: got 500\n" +
				"Summary: total=1 passed=0 failed=1 skipped=0\n",
		},
		{
			name: "escape",
			rep: &Report{
				FilePath: "esc.http",
				Results: []Result{{
					Kind:   "request",
					Name:   "esc",
					Method: "GET",
					Status: StatusFail,
					Tests:  []Test{{Name: "café\x1b[2J", Message: "x\r\ny\a"}},
				}},
				Total:  1,
				Failed: 1,
			},
			want: "Running 1 request(s) from esc.http with env <default>\n" +
				"FAIL GET esc [1 test(s) failed]\n" +
				"  Failed Tests:\n" +
				`    café\u001b[2J - x` + "\n" +
				`                    y\u0007` + "\n" +
				"Summary: total=1 passed=0 failed=1 skipped=0\n",
		},
		{
			name: "blank line",
			rep: &Report{
				FilePath: "api.http",
				Results: []Result{{
					Kind:   "request",
					Name:   "blank",
					Method: "GET",
					Status: StatusFail,
					Tests:  []Test{{Name: "t", Message: "a\n\nb"}},
				}},
				Total:  1,
				Failed: 1,
			},
			want: "Running 1 request(s) from api.http with env <default>\n" +
				"FAIL GET blank [1 test(s) failed]\n" +
				"  Failed Tests:\n" +
				"    t - a\n" +
				"\n" +
				"        b\n" +
				"Summary: total=1 passed=0 failed=1 skipped=0\n",
		},
		{
			name: "wide",
			rep: &Report{
				FilePath: "api.http",
				Results: []Result{{
					Kind:   "request",
					Name:   "wide",
					Method: "GET",
					Status: StatusFail,
					Tests:  []Test{{Name: "注文 🚀", Message: "a\nb"}},
				}},
				Total:  1,
				Failed: 1,
			},
			want: "Running 1 request(s) from api.http with env <default>\n" +
				"FAIL GET wide [1 test(s) failed]\n" +
				"  Failed Tests:\n" +
				"    注文 🚀 - a\n" +
				"              b\n" +
				"Summary: total=1 passed=0 failed=1 skipped=0\n",
		},
		{
			name: "no name",
			rep: &Report{
				FilePath: "api.http",
				Results: []Result{{
					Kind:   "request",
					Name:   "anon",
					Method: "GET",
					Status: StatusFail,
					Tests:  []Test{{Message: "a\nb"}, {}},
				}},
				Total:  1,
				Failed: 1,
			},
			want: "Running 1 request(s) from api.http with env <default>\n" +
				"FAIL GET anon [2 test(s) failed]\n" +
				"  Failed Tests:\n" +
				"    a\n" +
				"    b\n" +
				"    test failed\n" +
				"Summary: total=1 passed=0 failed=1 skipped=0\n",
		},
		{
			name: "passing",
			rep: &Report{
				FilePath: "api.http",
				Results: []Result{{
					Kind:   "workflow",
					Name:   "orders",
					Method: "WORKFLOW",
					Status: StatusPass,
					Steps: []Step{{
						Name:   "check",
						Status: StatusPass,
						HTTP:   &HTTP{Status: "200 OK", StatusCode: 200},
						Tests:  []Test{{Name: "orders are valid", Passed: true}},
					}},
				}},
				Total:  1,
				Passed: 1,
			},
			want: "Running 1 target(s) from api.http with env <default>\n" +
				"PASS WORKFLOW orders [1 passed, 0 failed, 0 skipped]\n" +
				"  1. PASS check [200 OK]\n" +
				"Summary: total=1 passed=1 failed=0 skipped=0\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			if err := WriteText(&out, tt.rep); err != nil {
				t.Fatalf("WriteText(...): %v", err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("output =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestWriteTextStyledFailedTests(t *testing.T) {
	rep := &Report{
		FilePath: "api.http",
		Results: []Result{{
			Kind:   "workflow",
			Name:   "orders",
			Method: "WORKFLOW",
			Status: StatusFail,
			Steps: []Step{{
				Name:   "check",
				Status: StatusFail,
				Tests: []Test{{
					Name:    "orders are valid",
					Message: "api.http:10: order 1: total -5 is negative\napi.http:10: order 3: no items",
				}},
			}},
		}},
		Total:  1,
		Failed: 1,
	}
	paint := TextPaintFunc(func(s, fg string, bold bool) string {
		return fmt.Sprintf("<%s %t>%s</>", fg, bold, s)
	})
	var out strings.Builder
	if err := WriteTextStyled(&out, rep, paint); err != nil {
		t.Fatalf("WriteTextStyled(...): %v", err)
	}
	want := "    <#A6A1BB false>Failed Tests:</>\n" +
		"      <#E8E9F0 false>orders are valid - api.http:10: order 1: total -5 is negative</>\n" +
		"      <#E8E9F0 false>                   api.http:10: order 3: no items</>\n"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("output =\n%s\nwant it to contain\n%s", out.String(), want)
	}
}
