package runner

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

func resultFailure(res Result) runfail.Failure {
	if res.Failure.Code != "" {
		return res.Failure
	}
	if res.Skipped {
		return runfail.Failure{}
	}
	switch {
	case res.Canceled:
		return runfail.Canceled("canceled", "canceled")
	case res.Err != nil:
		return runfail.FromErrorSource(res.Err, "error")
	case res.ScriptErr != nil:
		return runfail.Script(res.ScriptErr.Error(), "scriptError")
	case anyScriptTestFailed(res.Tests):
		return runfail.Assertion(scriptTestFailureMessage(res.Tests), "tests")
	case traceFailed(res.Trace):
		return runfail.TraceBudget(traceBreachMessage(res.Trace))
	}
	if res.Profile != nil && len(res.Profile.Failures) > 0 {
		if f := res.Profile.Failures[0].Failure; f.Code != "" {
			return f
		}
	}
	if f := firstStepFailure(res.Steps); f.Code != "" {
		return f
	}
	if res.Passed {
		return runfail.Failure{}
	}
	msg := str.FirstTrimmed(res.Summary, protocolStatusText(res.Response, res.GRPC))
	return runfail.Assertion(msg, "status")
}

func stepFailure(step StepResult) runfail.Failure {
	if step.Failure.Code != "" {
		return step.Failure
	}
	if step.Skipped {
		return runfail.Failure{}
	}
	switch {
	case step.Canceled:
		return runfail.Canceled("canceled", "canceled")
	case step.Err != nil:
		return runfail.FromErrorSource(step.Err, "error")
	case step.ScriptErr != nil:
		return runfail.Script(step.ScriptErr.Error(), "scriptError")
	case anyScriptTestFailed(step.Tests):
		return runfail.Assertion(scriptTestFailureMessage(step.Tests), "tests")
	case traceFailed(step.Trace):
		return runfail.TraceBudget(traceBreachMessage(step.Trace))
	case step.Passed:
		return runfail.Failure{}
	default:
		msg := str.FirstTrimmed(step.Summary, protocolStatusText(step.Response, step.GRPC))
		return runfail.Assertion(msg, "status")
	}
}

func firstStepFailure(steps []StepResult) runfail.Failure {
	for _, step := range steps {
		if f := stepFailure(step); f.Code != "" {
			return f
		}
	}
	return runfail.Failure{}
}

func anyScriptTestFailed(tests []scripts.TestResult) bool {
	for _, test := range tests {
		if !test.Passed {
			return true
		}
	}
	return false
}

func scriptTestFailureMessage(tests []scripts.TestResult) string {
	return runfail.FirstTestFailureMessage(
		tests,
		func(test scripts.TestResult) runfail.TestFailureFields {
			return runfail.TestFailureFields{
				Name:    str.Trim(test.Name),
				Message: str.Trim(test.Message),
				Passed:  test.Passed,
			}
		},
	)
}

func traceBreachMessage(info *TraceInfo) string {
	if info == nil || info.Summary == nil {
		return "trace budget breached"
	}
	return runfail.FirstTraceBudgetBreachMessage(
		info.Summary.Breaches,
		func(breach history.TraceBreach) runfail.TraceBudgetBreachFields {
			return runfail.TraceBudgetBreachFields{
				Kind:   str.Trim(breach.Kind),
				Limit:  breach.Limit,
				Actual: breach.Actual,
				Over:   breach.Over,
			}
		},
	)
}

func protocolStatusText(http *httpclient.Response, grpc *grpcclient.Response) string {
	switch {
	case http != nil:
		return str.Trim(http.Status)
	case grpc != nil:
		code := grpc.StatusCode.String()
		msg := str.Trim(grpc.StatusMessage)
		if code != "" && msg != "" && !strings.EqualFold(msg, code) {
			return code + " (" + msg + ")"
		}
		return code
	default:
		return ""
	}
}
