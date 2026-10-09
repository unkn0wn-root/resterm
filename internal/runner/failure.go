package runner

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func resultFailure(res Result) (runfail.Failure, runfail.Origin) {
	if res.Failure.Code != "" {
		return res.Failure, res.failureFrom
	}
	var prof runfail.Failure
	if f, ok := res.Profile.measuredFailure(); ok {
		prof = f.Failure
	}
	_, step := firstFailedStep(res.Steps)
	return runfail.FromEvidence(runfail.Evidence{
		Skipped:        res.Skipped,
		Canceled:       res.Canceled,
		Err:            res.Err,
		ScriptErr:      res.ScriptErr,
		StreamErr:      streamErr(res.Stream),
		Tests:          testFields(res.Tests),
		Breaches:       breachFields(res.Trace),
		ProfileFailure: prof,
		StepFailure:    step,
		MarkedFailed:   !res.Passed,
		Summary:        res.Summary,
		StatusText:     protocolStatusText(res.Response, res.GRPC),
	})
}

func stepFailure(step StepResult) runfail.Failure {
	if step.Failure.Code != "" {
		return step.Failure
	}
	f, _ := runfail.FromEvidence(runfail.Evidence{
		Skipped:      step.Skipped,
		Canceled:     step.Canceled,
		Err:          step.Err,
		ScriptErr:    step.ScriptErr,
		StreamErr:    streamErr(step.Stream),
		Tests:        testFields(step.Tests),
		Breaches:     breachFields(step.Trace),
		MarkedFailed: !step.Passed,
		Summary:      step.Summary,
		StatusText:   protocolStatusText(step.Response, step.GRPC),
	})
	return f
}

func firstFailedStep(steps []StepResult) (StepResult, runfail.Failure) {
	for _, step := range steps {
		if f := stepFailure(step); f.Code != "" {
			return step, f
		}
	}
	return StepResult{}, runfail.Failure{}
}

func streamErr(info *StreamInfo) error {
	if info == nil {
		return nil
	}
	return info.Err
}

func testFields(tests []scripts.TestResult) []runfail.TestFailureFields {
	out := make([]runfail.TestFailureFields, 0, len(tests))
	for _, test := range tests {
		out = append(out, runfail.TestFailureFields{
			Name:    strings.TrimSpace(test.Name),
			Message: strings.TrimSpace(test.Message),
			Passed:  test.Passed,
		})
	}
	return out
}

func breachFields(info *TraceInfo) []runfail.TraceBudgetBreachFields {
	if info == nil || info.Summary == nil {
		return nil
	}
	out := make([]runfail.TraceBudgetBreachFields, 0, len(info.Summary.Breaches))
	for _, breach := range info.Summary.Breaches {
		out = append(out, runfail.TraceBudgetBreachFields{
			Kind:   strings.TrimSpace(breach.Kind),
			Limit:  breach.Limit,
			Actual: breach.Actual,
			Over:   breach.Over,
		})
	}
	return out
}

func protocolStatusText(http *httpx.Response, grpc *grpcx.Response) string {
	switch {
	case http != nil:
		return strings.TrimSpace(http.Status)
	case grpc != nil:
		return grpc.StatusText()
	default:
		return ""
	}
}
