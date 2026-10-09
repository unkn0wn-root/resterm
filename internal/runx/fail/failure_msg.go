package runfail

import "time"

type TestFailureFields struct {
	Name    string
	Message string
	Passed  bool
}

func TestFailureMessage(name, message string) string {
	switch {
	case name != "" && message != "":
		return name + ": " + message
	case name != "":
		return name
	case message != "":
		return message
	default:
		return "test failed"
	}
}

type TraceBudgetBreachFields struct {
	Kind   string
	Limit  time.Duration
	Actual time.Duration
	Over   time.Duration
}

func TraceBudgetBreachMessage(kind string, limit, actual, over time.Duration) string {
	l := kind
	if l == "" {
		l = "trace"
	}
	if over > 0 {
		return "trace budget breach " + l + " (+" + over.String() + ")"
	}
	if limit > 0 && actual > 0 {
		return "trace budget breach " + l + " (" + actual.String() + " > " + limit.String() + ")"
	}
	return "trace budget breach " + l
}
