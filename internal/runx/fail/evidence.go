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

type Origin int

const (
	OriginSelf Origin = iota
	OriginProfile
	OriginStep
)

// FromEvidence is the failure rule the runner and the headless package share.
func FromEvidence(e Evidence) (Failure, Origin) {
	if e.Skipped {
		return Failure{}, OriginSelf
	}
	switch {
	case e.Canceled:
		return Canceled("canceled", "canceled"), OriginSelf
	case e.Err != nil:
		return FromErrorSource(e.Err, "error"), OriginSelf
	case e.ScriptErr != nil:
		return Script(e.ScriptErr.Error(), "scriptError"), OriginSelf
	case e.StreamErr != nil:
		return FromErrorSource(e.StreamErr, "stream"), OriginSelf
	}
	for _, t := range e.Tests {
		if !t.Passed {
			return Assertion(TestFailureMessage(t.Name, t.Message), "tests"), OriginSelf
		}
	}
	if len(e.Breaches) > 0 {
		b := e.Breaches[0]
		return TraceBudget(TraceBudgetBreachMessage(b.Kind, b.Limit, b.Actual, b.Over)), OriginSelf
	}
	switch {
	case e.ProfileFailure.Code != "":
		return e.ProfileFailure, OriginProfile
	case e.StepFailure.Code != "":
		return e.StepFailure, OriginStep
	case !e.MarkedFailed:
		return Failure{}, OriginSelf
	default:
		return Assertion(str.FirstTrimmed(e.Summary, e.StatusText), "status"), OriginSelf
	}
}
