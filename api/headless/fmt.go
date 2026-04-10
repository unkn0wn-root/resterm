package headless

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
	for _, item := range rep.Results {
		if item.Kind != KindRequest {
			return "target(s)"
		}
	}
	return "request(s)"
}

func resultSkipped(item Result) bool {
	return item.Status == StatusSkip
}

func resultFailed(item Result) bool {
	if resultSkipped(item) {
		return false
	}
	if item.Status == StatusFail || item.Canceled || item.Error != "" || item.ScriptError != "" || traceFailed(item.Trace) {
		return true
	}
	return anyTestFailed(item.Tests)
}

func resultLabel(item Result) string {
	switch {
	case resultSkipped(item):
		return "SKIP"
	case resultFailed(item):
		return "FAIL"
	default:
		return "PASS"
	}
}

func stepSkipped(step Step) bool {
	return step.Status == StatusSkip
}

func stepFailed(step Step) bool {
	if stepSkipped(step) {
		return false
	}
	if step.Canceled || step.Status == StatusFail || step.Error != "" || step.ScriptError != "" || traceFailed(step.Trace) {
		return true
	}
	return anyTestFailed(step.Tests)
}

func anyTestFailed(tests []Test) bool {
	for _, test := range tests {
		if !test.Passed {
			return true
		}
	}
	return false
}

func stepLabel(step Step) string {
	switch {
	case step.Canceled:
		return "CANCELED"
	case stepSkipped(step):
		return "SKIP"
	case stepFailed(step):
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

func resultName(item Result) string {
	name := item.Name
	if name != "" {
		return name
	}
	target := item.Target
	if target == "" {
		return "<unnamed>"
	}
	if len(target) > 80 {
		return target[:77] + "..."
	}
	return target
}

func stepName(step Step) string {
	name := step.Name
	if name != "" {
		return name
	}
	if env := step.Environment; env != "" {
		return env
	}
	if target := step.Target; target != "" {
		return target
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

func resultStatus(item Result) string {
	return protocolStatus(item.HTTP, item.GRPC)
}

func stepStatus(step Step) string {
	return protocolStatus(step.HTTP, step.GRPC)
}

func resultLine(item Result) string {
	switch item.Kind {
	case KindWorkflow, KindForEach:
		return workflowLine(item)
	case KindCompare:
		return compareLine(item)
	case KindProfile:
		return profileLine(item)
	}
	base := fmt.Sprintf("%s %s", requestMethodValue(item.Method), resultName(item))
	switch {
	case resultSkipped(item):
		if reason := item.SkipReason; reason != "" {
			return fmt.Sprintf("%s [%s]", base, reason)
		}
		return base
	case item.Error != "":
		return fmt.Sprintf("%s [%s]", base, item.Error)
	case item.ScriptError != "":
		return fmt.Sprintf("%s [%s]", base, item.ScriptError)
	}

	if n := failedTestCount(item.Tests); n > 0 {
		return fmt.Sprintf("%s [%d test(s) failed]", base, n)
	}
	if msg := traceFailureText(item.Trace); msg != "" {
		return fmt.Sprintf("%s [%s]", base, msg)
	}

	status := resultStatus(item)
	dur := item.Duration
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

func workflowLine(item Result) string {
	base := fmt.Sprintf("%s %s", requestMethodValue(item.Method), resultName(item))
	pass, fail, skip := stepCounts(item.Steps)
	detail := fmt.Sprintf("%d passed, %d failed, %d skipped", pass, fail, skip)
	if item.Canceled {
		detail += ", canceled"
	}
	if dur := item.Duration; dur > 0 {
		detail = fmt.Sprintf("%s in %s", detail, dur)
	}
	return fmt.Sprintf("%s [%s]", base, detail)
}

func compareLine(item Result) string {
	base := fmt.Sprintf("%s %s", requestMethodValue(item.Method), resultName(item))
	pass, fail, skip := stepCounts(item.Steps)
	detail := fmt.Sprintf("%d passed, %d failed, %d skipped", pass, fail, skip)
	if item.Compare != nil && item.Compare.Baseline != "" {
		detail = fmt.Sprintf("baseline: %s, %s", item.Compare.Baseline, detail)
	}
	if item.Canceled {
		detail += ", canceled"
	}
	if dur := item.Duration; dur > 0 {
		detail = fmt.Sprintf("%s in %s", detail, dur)
	}
	return fmt.Sprintf("%s [%s]", base, detail)
}

func profileLine(item Result) string {
	base := fmt.Sprintf("%s %s", requestMethodValue(item.Method), resultName(item))
	prof := item.Profile
	if prof == nil || (prof.TotalRuns == 0 && prof.WarmupRuns == 0 && prof.SuccessfulRuns == 0 && prof.FailedRuns == 0) {
		if summary := item.Summary; summary != "" {
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
	if item.Canceled {
		detail += ", canceled"
	}
	if dur := item.Duration; dur > 0 {
		detail = fmt.Sprintf("%s in %s", detail, dur)
	}
	return fmt.Sprintf("%s [%s]", base, detail)
}

func stepCounts(steps []Step) (pass, fail, skip int) {
	for _, step := range steps {
		switch {
		case stepSkipped(step):
			skip++
		case stepFailed(step):
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
	case stepSkipped(step):
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
	if stepFailed(step) {
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

func traceFailed(info *Trace) bool {
	return info != nil && len(info.Breaches) > 0
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

func suiteName(item Result) string {
	return requestMethodValue(item.Method) + " " + resultName(item)
}

func resultFailureMessage(item Result) string {
	switch {
	case item.Error != "":
		return item.Error
	case item.ScriptError != "":
		return item.ScriptError
	}
	if failed := failedTests(item.Tests); len(failed) > 0 {
		return testFailureMessage(failed)
	}
	if msg := traceFailureText(item.Trace); msg != "" && resultFailed(item) {
		return msg
	}
	if msg := item.Summary; msg != "" && resultFailed(item) {
		return msg
	}
	if status := resultStatus(item); status != "" && resultFailed(item) {
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
	if msg := traceFailureText(step.Trace); msg != "" && stepFailed(step) {
		return msg
	}
	if msg := step.Summary; msg != "" && stepFailed(step) {
		return msg
	}
	if status := stepStatus(step); status != "" && stepFailed(step) {
		return status
	}
	return ""
}

func testFailureMessage(tests []Test) string {
	if len(tests) == 0 {
		return ""
	}
	first := tests[0]
	name := first.Name
	msg := first.Message
	switch {
	case name != "" && msg != "":
		return name + ": " + msg
	case name != "":
		return name
	case msg != "":
		return msg
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
	for key, value := range src {
		out[key] = jsonAnyValue(value)
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
