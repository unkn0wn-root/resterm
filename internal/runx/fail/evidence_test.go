package runfail

import (
	"errors"
	"testing"
	"time"
)

func TestFromEvidenceChecksInOrder(t *testing.T) {
	e := Evidence{
		Canceled:       true,
		Err:            errors.New("dial tcp: connection refused"),
		ScriptErr:      errors.New("x is not defined"),
		StreamErr:      errors.New("stream closed"),
		Tests:          []TestFailureFields{{Name: "ok", Passed: true}, {Name: "status", Message: "want 200"}},
		Breaches:       []TraceBudgetBreachFields{{Kind: "total", Over: time.Millisecond}},
		ProfileFailure: New(CodeTimeout, "slow", "profile"),
		StepFailure:    New(CodeNetwork, "down", "step"),
		MarkedFailed:   true,
		Summary:        "expected 200",
		StatusText:     "500 Internal Server Error",
	}
	if got, _ := FromEvidence(Evidence{Skipped: true, Canceled: true}); got.Code != "" {
		t.Fatalf("skipped evidence gave %+v", got)
	}
	steps := []struct {
		drop   func(*Evidence)
		source string
		origin Origin
	}{
		{func(*Evidence) {}, "canceled", OriginSelf},
		{func(e *Evidence) { e.Canceled = false }, "error", OriginSelf},
		{func(e *Evidence) { e.Err = nil }, "scriptError", OriginSelf},
		{func(e *Evidence) { e.ScriptErr = nil }, "stream", OriginSelf},
		{func(e *Evidence) { e.StreamErr = nil }, "tests", OriginSelf},
		{func(e *Evidence) { e.Tests = nil }, "trace", OriginSelf},
		{func(e *Evidence) { e.Breaches = nil }, "profile", OriginProfile},
		{func(e *Evidence) { e.ProfileFailure = Failure{} }, "step", OriginStep},
		{func(e *Evidence) { e.StepFailure = Failure{} }, "status", OriginSelf},
	}
	for _, s := range steps {
		s.drop(&e)
		if got, origin := FromEvidence(e); got.Source != s.source || origin != s.origin {
			t.Fatalf("source = %q origin = %d, want %q and %d", got.Source, origin, s.source, s.origin)
		}
	}
	if got, _ := FromEvidence(e); got.Message != "expected 200" {
		t.Fatalf("status message = %q, want the summary", got.Message)
	}
	e.Summary = " "
	if got, _ := FromEvidence(e); got.Message != "500 Internal Server Error" {
		t.Fatalf("status message = %q, want the status text", got.Message)
	}
	e.MarkedFailed = false
	if got, _ := FromEvidence(e); got.Code != "" {
		t.Fatalf("evidence without failure gave %+v", got)
	}
}
