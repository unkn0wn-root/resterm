package exec

import (
	"net/http"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRetryBackoffCapsExponentialAndJitter(t *testing.T) {
	t.Parallel()
	spec := restfile.ExponentialBackoffSpec{Initial: 100 * time.Millisecond, Max: 250 * time.Millisecond}
	if got := retryBackoff(spec, 1); got != 100*time.Millisecond {
		t.Fatalf("first backoff = %s", got)
	}
	if got := retryBackoff(spec, 2); got != 200*time.Millisecond {
		t.Fatalf("second backoff = %s", got)
	}
	if got := retryBackoff(spec, 3); got != 250*time.Millisecond {
		t.Fatalf("capped backoff = %s", got)
	}
	for range 100 {
		jittered := retryBackoff(restfile.ExponentialBackoffSpec{
			Initial:       250 * time.Millisecond,
			Max:           250 * time.Millisecond,
			JitterPercent: 100,
		}, 1)
		if jittered < 0 || jittered > 250*time.Millisecond {
			t.Fatalf("jittered backoff = %s, outside hard cap", jittered)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		want  time.Duration
		state retryAfterState
	}{
		{name: "seconds", value: "12", want: 12 * time.Second, state: retryAfterValid},
		{
			name:  "date",
			value: now.Add(3 * time.Second).Format(http.TimeFormat),
			want:  3 * time.Second,
			state: retryAfterValid,
		},
		{
			name:  "past date",
			value: now.Add(-time.Second).Format(http.TimeFormat),
			state: retryAfterValid,
		},
		{
			name:  "too large for a duration",
			value: "18446744073709551615",
			want:  time.Duration(maxRetryAfterSeconds) * time.Second,
			state: retryAfterValid,
		},
		{name: "beyond uint64", value: "99999999999999999999999", state: retryAfterInvalid},
		{name: "negative seconds", value: "-1", state: retryAfterInvalid},
		{name: "unreadable", value: "later", state: retryAfterInvalid},
		{name: "missing", state: retryAfterAbsent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			header := make(http.Header)
			if test.value != "" {
				header.Set("Retry-After", test.value)
			}
			got, state := retryAfter(header, now)
			if got != test.want || state != test.state {
				t.Fatalf("retryAfter() = %s, %d; want %s, %d", got, state, test.want, test.state)
			}
		})
	}
}
