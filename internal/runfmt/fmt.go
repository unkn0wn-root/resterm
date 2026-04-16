package runfmt

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func reportFileLabel(path string) string {
	if base := filepath.Base(path); base != "" {
		return base
	}
	return path
}

func reportEnvLabel(name string) string {
	if name != "" {
		return name
	}
	return "<default>"
}

func reportTargetLabel(rep *Report) string {
	for _, res := range rep.Results {
		if res.Kind != "request" {
			return "target(s)"
		}
	}
	return "request(s)"
}

func resultLabel(res Result) string {
	switch res.Status {
	case StatusSkip:
		return "SKIP"
	case StatusFail:
		return "FAIL"
	default:
		return "PASS"
	}
}

func stepLabel(step Step) string {
	switch {
	case step.Canceled:
		return "CANCELED"
	case step.Status == StatusSkip:
		return "SKIP"
	case step.Status == StatusFail:
		return "FAIL"
	default:
		return "PASS"
	}
}

func requestMethodValue(method string) string {
	method = strings.ToUpper(method)
	if method == "" {
		return "REQ"
	}
	return method
}

func resultName(res Result) string {
	if res.Name != "" {
		return res.Name
	}
	target := res.Target
	if target == "" {
		return "<unnamed>"
	}
	if len(target) > 80 {
		return target[:77] + "..."
	}
	return target
}

func stepName(step Step) string {
	if step.Name != "" {
		return step.Name
	}
	if step.Environment != "" {
		return step.Environment
	}
	if step.Target != "" {
		return step.Target
	}
	return "<step>"
}

func protocolStatus(http *HTTP, grpc *GRPC) string {
	switch {
	case http != nil:
		return http.Status
	case grpc != nil:
		code := grpc.Code
		msg := grpc.StatusMessage
		if code != "" && msg != "" && !strings.EqualFold(msg, code) {
			return code + " (" + msg + ")"
		}
		return code
	default:
		return ""
	}
}

func resultStatus(res Result) string {
	return protocolStatus(res.HTTP, res.GRPC)
}

func stepStatus(step Step) string {
	return protocolStatus(step.HTTP, step.GRPC)
}

func resultLine(res Result) string {
	switch res.Kind {
	case "workflow", "for-each":
		return workflowLine(res)
	case "compare":
		return compareLine(res)
	case "profile":
		return profileLine(res)
	}
	base := fmt.Sprintf("%s %s", requestMethodValue(res.Method), resultName(res))
	switch {
	case res.Status == StatusSkip:
		if reason := res.SkipReason; reason != "" {
			return fmt.Sprintf("%s [%s]", base, reason)
		}
		return base
	case res.Error != "":
		return fmt.Sprintf("%s [%s]", base, res.Error)
	case res.ScriptError != "":
		return fmt.Sprintf("%s [%s]", base, res.ScriptError)
	}

	if n := failedTestCount(res.Tests); n > 0 {
		return fmt.Sprintf("%s [%d test(s) failed]", base, n)
	}
	if msg := traceFailureText(res.Trace); msg != "" {
		return fmt.Sprintf("%s [%s]", base, msg)
	}

	status := resultStatus(res)
	dur := res.Duration
	if status == "" && dur <= 0 {
		return base
	}
	if dur <= 0 {
		return fmt.Sprintf("%s [%s]", base, status)
	}
	if status == "" {
		return fmt.Sprintf("%s [%s]", base, dur)
	}
	return fmt.Sprintf("%s [%s in %s]", base, status, dur)
}

func workflowLine(res Result) string {
	base := fmt.Sprintf("%s %s", requestMethodValue(res.Method), resultName(res))
	pass, fail, skip := stepCounts(res.Steps)
	detail := fmt.Sprintf("%d passed, %d failed, %d skipped", pass, fail, skip)
	if res.Canceled {
		detail += ", canceled"
	}
	if dur := res.Duration; dur > 0 {
		detail = fmt.Sprintf("%s in %s", detail, dur)
	}
	return fmt.Sprintf("%s [%s]", base, detail)
}

func compareLine(res Result) string {
	base := fmt.Sprintf("%s %s", requestMethodValue(res.Method), resultName(res))
	pass, fail, skip := stepCounts(res.Steps)
	detail := fmt.Sprintf("%d passed, %d failed, %d skipped", pass, fail, skip)
	if res.Compare != nil && res.Compare.Baseline != "" {
		detail = fmt.Sprintf("baseline: %s, %s", res.Compare.Baseline, detail)
	}
	if res.Canceled {
		detail += ", canceled"
	}
	if dur := res.Duration; dur > 0 {
		detail = fmt.Sprintf("%s in %s", detail, dur)
	}
	return fmt.Sprintf("%s [%s]", base, detail)
}

func profileLine(res Result) string {
	base := fmt.Sprintf("%s %s", requestMethodValue(res.Method), resultName(res))
	prof := res.Profile
	if prof == nil ||
		(prof.TotalRuns == 0 && prof.WarmupRuns == 0 && prof.SuccessfulRuns == 0 && prof.FailedRuns == 0) {
		if summary := res.Summary; summary != "" {
			return fmt.Sprintf("%s [%s]", base, summary)
		}
		return base
	}

	detail := fmt.Sprintf(
		"%d total, %d success, %d failure",
		prof.TotalRuns,
		prof.SuccessfulRuns,
		prof.FailedRuns,
	)

	if prof.WarmupRuns > 0 {
		detail = fmt.Sprintf("%s, %d warmup", detail, prof.WarmupRuns)
	}
	if res.Canceled {
		detail += ", canceled"
	}
	if dur := res.Duration; dur > 0 {
		detail = fmt.Sprintf("%s in %s", detail, dur)
	}
	return fmt.Sprintf("%s [%s]", base, detail)
}

func stepCounts(steps []Step) (pass, fail, skip int) {
	for _, step := range steps {
		switch step.Status {
		case StatusSkip:
			skip++
		case StatusFail:
			fail++
		default:
			pass++
		}
	}
	return pass, fail, skip
}

func stepLine(step Step) string {
	base := stepName(step)
	switch {
	case step.Canceled:
		if msg := step.Summary; msg != "" {
			return fmt.Sprintf("%s [%s]", base, msg)
		}
		return base
	case step.Status == StatusSkip:
		if msg := step.SkipReason; msg != "" {
			return fmt.Sprintf("%s [%s]", base, msg)
		}
		if msg := step.Summary; msg != "" {
			return fmt.Sprintf("%s [%s]", base, msg)
		}
		return base
	case step.Error != "":
		return fmt.Sprintf("%s [%s]", base, step.Error)
	case step.ScriptError != "":
		return fmt.Sprintf("%s [%s]", base, step.ScriptError)
	}

	if n := failedTestCount(step.Tests); n > 0 {
		return fmt.Sprintf("%s [%d test(s) failed]", base, n)
	}
	if msg := traceFailureText(step.Trace); msg != "" {
		return fmt.Sprintf("%s [%s]", base, msg)
	}
	if step.Status == StatusFail {
		if msg := step.Summary; msg != "" {
			return fmt.Sprintf("%s [%s]", base, msg)
		}
	}

	status := stepStatus(step)
	dur := step.Duration
	if status == "" && dur <= 0 {
		return base
	}
	if dur <= 0 {
		return fmt.Sprintf("%s [%s]", base, status)
	}
	if status == "" {
		return fmt.Sprintf("%s [%s]", base, dur)
	}
	return fmt.Sprintf("%s [%s in %s]", base, status, dur)
}

func failedTestCount(tests []Test) int {
	n := 0
	for _, test := range tests {
		if !test.Passed {
			n++
		}
	}
	return n
}

func failedTests(tests []Test) []Test {
	out := make([]Test, 0, len(tests))
	for _, test := range tests {
		if !test.Passed {
			out = append(out, test)
		}
	}
	return out
}

func traceFailureText(info *Trace) string {
	if !traceFailed(info) {
		return ""
	}
	breach := info.Breaches[0]
	label := breach.Kind
	if label == "" {
		label = "trace"
	}
	if breach.Over > 0 {
		return fmt.Sprintf("trace budget breach %s (+%s)", label, breach.Over)
	}
	if breach.Limit > 0 && breach.Actual > 0 {
		return fmt.Sprintf("trace budget breach %s (%s > %s)", label, breach.Actual, breach.Limit)
	}
	return fmt.Sprintf("trace budget breach %s", label)
}

func suiteName(res Result) string {
	return requestMethodValue(res.Method) + " " + resultName(res)
}

func resultFailureMessage(res Result) string {
	switch {
	case res.Error != "":
		return res.Error
	case res.ScriptError != "":
		return res.ScriptError
	}
	if failed := failedTests(res.Tests); len(failed) > 0 {
		return testFailureMessage(failed)
	}
	if msg := traceFailureText(res.Trace); msg != "" && res.Status == StatusFail {
		return msg
	}
	if msg := res.Summary; msg != "" && res.Status == StatusFail {
		return msg
	}
	if status := resultStatus(res); status != "" && res.Status == StatusFail {
		return status
	}
	return ""
}

func stepFailureMessage(step Step) string {
	switch {
	case step.Error != "":
		return step.Error
	case step.ScriptError != "":
		return step.ScriptError
	case step.Canceled:
		if msg := step.Summary; msg != "" {
			return msg
		}
		return "canceled"
	}
	if failed := failedTests(step.Tests); len(failed) > 0 {
		return testFailureMessage(failed)
	}
	if msg := traceFailureText(step.Trace); msg != "" && step.Status == StatusFail {
		return msg
	}
	if msg := step.Summary; msg != "" && step.Status == StatusFail {
		return msg
	}
	if status := stepStatus(step); status != "" && step.Status == StatusFail {
		return status
	}
	return ""
}

func testFailureMessage(tests []Test) string {
	if len(tests) == 0 {
		return ""
	}
	first := tests[0]
	switch {
	case first.Name != "" && first.Message != "":
		return first.Name + ": " + first.Message
	case first.Name != "":
		return first.Name
	case first.Message != "":
		return first.Message
	default:
		return "test failed"
	}
}

func skipMessage(parts ...string) string {
	for _, part := range parts {
		if part != "" {
			return part
		}
	}
	return "skipped"
}

func junitTime(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}

func durMS(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return d.Milliseconds()
}

func jsonAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, val := range src {
		out[key] = jsonAnyValue(val)
	}
	return out
}

func jsonAnyValue(v any) any {
	switch x := v.(type) {
	case time.Duration:
		return durMS(x)
	case map[string]any:
		return jsonAnyMap(x)
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, jsonAnyValue(item))
		}
		return out
	default:
		return x
	}
}
