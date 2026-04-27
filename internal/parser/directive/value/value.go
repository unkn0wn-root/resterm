package value

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func ParseBool(raw string) (bool, bool) {
	val := strings.TrimSpace(strings.ToLower(raw))
	switch val {
	case "true", "t", "1", "yes", "on":
		return true, true
	case "false", "f", "0", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func IsOffToken(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "0", "false", "off", "disable", "disabled":
		return true
	default:
		return false
	}
}

func ParsePositiveInt(raw string) (int, error) {
	tr := strings.TrimSpace(raw)
	if tr == "" {
		return 0, errors.New("empty value")
	}

	n, err := strconv.Atoi(tr)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("value must be non-negative: %d", n)
	}
	return n, nil
}

func ParseByteSize(raw string) (int64, error) {
	tr := strings.TrimSpace(strings.ToLower(raw))
	if tr == "" {
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

	var nPart string
	var sx string
	for i := len(tr); i >= 0; i-- {
		px := tr[:i]
		if _, err := strconv.ParseFloat(px, 64); err == nil {
			nPart = px
			sx = strings.TrimSpace(tr[i:])
			break
		}
	}
	if nPart == "" {
		return 0, fmt.Errorf("invalid size %q", raw)
	}

	sx = strings.TrimSpace(sx)
	mlp, ok := multipliers[sx]
	if !ok {
		return 0, fmt.Errorf("unknown size suffix %q", sx)
	}

	pr, err := strconv.ParseFloat(nPart, 64)
	if err != nil {
		return 0, err
	}
	if pr < 0 {
		return 0, fmt.Errorf("value must be non-negative: %f", pr)
	}
	return int64(pr * float64(mlp)), nil
}
