package runfail

import str "github.com/unkn0wn-root/resterm/internal/util"

type Evidence struct {
	Skipped        bool
	Canceled       bool
	Err            error
	ScriptErr      error
	StreamErr      error
	Tests          []TestFailureFields
	Breaches       []TraceBudgetBreachFields
	ProfileFailure Failure
	StepFailure    Failure
	MarkedFailed   bool
	Summary        string
	StatusText     string
}

// FromEvidence is the failure rule the runner and the headless package share.
func FromEvidence(e Evidence) Failure {
	if e.Skipped {
		return Failure{}
	}
	switch {
	case e.Canceled:
		return Canceled("canceled", "canceled")
	case e.Err != nil:
		return FromErrorSource(e.Err, "error")
	case e.ScriptErr != nil:
		return Script(e.ScriptErr.Error(), "scriptError")
	case e.StreamErr != nil:
		return FromErrorSource(e.StreamErr, "stream")
	}
	for _, t := range e.Tests {
		if !t.Passed {
			return Assertion(TestFailureMessage(t.Name, t.Message), "tests")
		}
	}
	if len(e.Breaches) > 0 {
		b := e.Breaches[0]
		return TraceBudget(TraceBudgetBreachMessage(b.Kind, b.Limit, b.Actual, b.Over))
	}
	switch {
	case e.ProfileFailure.Code != "":
		return e.ProfileFailure
	case e.StepFailure.Code != "":
		return e.StepFailure
	case !e.MarkedFailed:
		return Failure{}
	default:
		return Assertion(str.FirstTrimmed(e.Summary, e.StatusText), "status")
	}
}
