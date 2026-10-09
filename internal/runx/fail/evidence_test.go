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
	if got := FromEvidence(Evidence{Skipped: true, Canceled: true}); got.Code != "" {
		t.Fatalf("skipped evidence gave %+v", got)
	}
	steps := []struct {
		drop   func(*Evidence)
		source string
	}{
		{func(*Evidence) {}, "canceled"},
		{func(e *Evidence) { e.Canceled = false }, "error"},
		{func(e *Evidence) { e.Err = nil }, "scriptError"},
		{func(e *Evidence) { e.ScriptErr = nil }, "stream"},
		{func(e *Evidence) { e.StreamErr = nil }, "tests"},
		{func(e *Evidence) { e.Tests = nil }, "trace"},
		{func(e *Evidence) { e.Breaches = nil }, "profile"},
		{func(e *Evidence) { e.ProfileFailure = Failure{} }, "step"},
		{func(e *Evidence) { e.StepFailure = Failure{} }, "status"},
	}
	for _, s := range steps {
		s.drop(&e)
		if got := FromEvidence(e).Source; got != s.source {
			t.Fatalf("source = %q, want %q", got, s.source)
		}
	}
	if got := FromEvidence(e).Message; got != "expected 200" {
		t.Fatalf("status message = %q, want the summary", got)
	}
	e.Summary = " "
	if got := FromEvidence(e).Message; got != "500 Internal Server Error" {
		t.Fatalf("status message = %q, want the status text", got)
	}
	e.MarkedFailed = false
	if got := FromEvidence(e); got.Code != "" {
		t.Fatalf("evidence without failure gave %+v", got)
	}
}
