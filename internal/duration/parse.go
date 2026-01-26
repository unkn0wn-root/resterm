package duration

import (
	"math"
	"strconv"
	"strings"
	"time"
)

const maxDurationFloat = float64(^uint64(0) >> 1)

var unitTable = map[string]time.Duration{
	"ns": time.Nanosecond,
	"us": time.Microsecond,
	"ms": time.Millisecond,
	"s":  time.Second,
	"m":  time.Minute,
	"h":  time.Hour,
	"d":  24 * time.Hour,
	"w":  7 * 24 * time.Hour,
}

// Parses a duration string with support for d (days) and w (weeks).
// Accepts the same units as time.ParseDuration plus d and w, and also
// allows mixed units such as "1h30m" or "2d3h".
func Parse(value string) (time.Duration, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(trimmed); err == nil {
		return d, true
	}
	return parseExtended(trimmed)
}

func parseExtended(input string) (time.Duration, bool) {
	s := strings.TrimSpace(input)
	if s == "" {
		return 0, false
	}

	sign := 1.0
	if s[0] == '+' || s[0] == '-' {
		if s[0] == '-' {
			sign = -1.0
		}
		s = strings.TrimSpace(s[1:])
	}
	if s == "" {
		return 0, false
	}

	var total float64
	parsed := false
	for len(s) > 0 {
		s = strings.TrimSpace(s)
		if s == "" {
			break
		}
		numEnd := scanNumber(s)
		if numEnd == 0 {
			return 0, false
		}
		numStr := s[:numEnd]
		n, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			return 0, false
		}

		unitEnd := scanUnit(s[numEnd:])
		if unitEnd == 0 {
			return 0, false
		}
		unit := strings.ToLower(s[numEnd : numEnd+unitEnd])
		scale, ok := unitTable[unit]
		if !ok {
			return 0, false
		}

		part := n * float64(scale)
		if math.IsNaN(part) || math.IsInf(part, 0) {
			return 0, false
		}

		total += part
		if math.Abs(total) > maxDurationFloat {
			return 0, false
		}
		parsed = true
		s = s[numEnd+unitEnd:]
	}

	if !parsed {
		return 0, false
	}

	total *= sign
	if math.Abs(total) > maxDurationFloat {
		return 0, false
	}

	return time.Duration(math.Round(total)), true
}

func scanNumber(s string) int {
	dotSeen := false
	digitSeen := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= '0' && ch <= '9':
			digitSeen = true
		case ch == '.' && !dotSeen:
			dotSeen = true
		default:
			if !digitSeen {
				return 0
			}
			return i
		}
	}
	if !digitSeen {
		return 0
	}
	return len(s)
}

// scanUnit returns the length of the contiguous alphabetic unit suffix.
// Example: "h30m" -> 1 (unit "h").
func scanUnit(s string) int {
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') {
			return i
		}
	}
	return len(s)
}
