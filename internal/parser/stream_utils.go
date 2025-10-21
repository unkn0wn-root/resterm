package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func parsePositiveInt(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, errors.New("empty value")
	}

	n, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("value must be non-negative: %d", n)
	}
	return n, nil
}

func parseByteSize(value string) (int64, error) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return 0, errors.New("empty value")
	}

	multipliers := map[string]int64{
		"":    1,
		"b":   1,
		"kb":  1024,
		"kib": 1024,
		"mb":  1024 * 1024,
		"mib": 1024 * 1024,
		"gb":  1024 * 1024 * 1024,
		"gib": 1024 * 1024 * 1024,
	}

	var numberPart string
	var suffix string
	for i := len(trimmed); i >= 0; i-- {
		prefix := trimmed[:i]
		if _, err := strconv.ParseFloat(prefix, 64); err == nil {
			numberPart = prefix
			suffix = strings.TrimSpace(trimmed[i:])
			break
		}
	}
	if numberPart == "" {
		return 0, fmt.Errorf("invalid size %q", value)
	}

	suffix = strings.TrimSpace(suffix)
	multiplier, ok := multipliers[suffix]
	if !ok {
		return 0, fmt.Errorf("unknown size suffix %q", suffix)
	}

	parsed, err := strconv.ParseFloat(numberPart, 64)
	if err != nil {
		return 0, err
	}
	if parsed < 0 {
		return 0, fmt.Errorf("value must be non-negative: %f", parsed)
	}
	return int64(parsed * float64(multiplier)), nil
}
