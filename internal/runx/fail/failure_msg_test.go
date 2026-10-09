package runfail

import (
	"testing"
	"time"
)

func TestTestFailureMessage(t *testing.T) {
	cases := []struct {
		name    string
		test    string
		message string
		want    string
	}{
		{name: "both", test: "status", message: "got 500", want: "status: got 500"},
		{name: "name only", test: "status", want: "status"},
		{name: "message only", message: "got 500", want: "got 500"},
		{name: "empty", want: "test failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TestFailureMessage(tc.test, tc.message); got != tc.want {
				t.Fatalf("TestFailureMessage() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTraceBudgetBreachMessage(t *testing.T) {
	if got := TraceBudgetBreachMessage("", 0, 0, 0); got != "trace budget breach trace" {
		t.Fatalf("empty breach message = %q", got)
	}
	if got := TraceBudgetBreachMessage(
		"dns",
		0,
		0,
		time.Millisecond,
	); got != "trace budget breach dns (+1ms)" {
		t.Fatalf("over breach message = %q", got)
	}
	if got := TraceBudgetBreachMessage(
		"total",
		time.Second,
		2*time.Second,
		0,
	); got != "trace budget breach total (2s > 1s)" {
		t.Fatalf("limit breach message = %q", got)
	}
}
