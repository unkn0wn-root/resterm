package runclass

import "time"

type TestFailureFields struct {
	Name    string
	Message string
	Passed  bool
}

func FirstTestFailureMessage[T any](tests []T, fields func(T) TestFailureFields) string {
	for _, t := range tests {
		g := fields(t)
		if g.Passed {
			continue
		}
		return TestFailureMessage(g.Name, g.Message)
	}
	return "test failed"
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

func FirstTraceBudgetBreachMessage[T any](breaches []T, fields func(T) TraceBudgetBreachFields) string {
	if len(breaches) == 0 {
		return "trace budget breached"
	}
	f := fields(breaches[0])
	return TraceBudgetBreachMessage(f.Kind, f.Limit, f.Actual, f.Over)
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
