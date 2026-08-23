package exec

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/delay"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type retryAfterState uint8

const (
	retryAfterAbsent retryAfterState = iota
	retryAfterInvalid
	retryAfterValid
)

const maxRetryAfterSeconds = uint64(math.MaxInt64 / int64(time.Second))

func retriable(err error) bool {
	switch diag.ClassOf(err) {
	case diag.ClassNetwork, diag.ClassTimeout:
		return true
	default:
		return false
	}
}

func retryBackoff(spec restfile.ExponentialBackoffSpec, retryNumber int) time.Duration {
	base := spec.Initial
	for range max(retryNumber-1, 0) {
		// Avoid overflow when doubling.
		if base > spec.Max/2 {
			base = spec.Max
			break
		}
		base *= 2
	}
	return min(delay.Jitter(base, spec.JitterPercent).Sample(), spec.Max)
}

func retryAfter(headers http.Header, now time.Time) (time.Duration, retryAfterState) {
	raw := strings.TrimSpace(headers.Get("Retry-After"))
	if raw == "" {
		return 0, retryAfterAbsent
	}
	if seconds, err := strconv.ParseUint(raw, 10, 64); err == nil {
		// Clamp before conversion so the duration does not overflow.
		return time.Duration(min(seconds, maxRetryAfterSeconds)) * time.Second, retryAfterValid
	}
	when, err := http.ParseTime(raw)
	if err != nil {
		return 0, retryAfterInvalid
	}
	return max(when.Sub(now), 0), retryAfterValid
}
