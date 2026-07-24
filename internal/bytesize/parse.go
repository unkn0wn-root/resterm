package bytesize

import (
	"errors"
	"fmt"
	"math/big"
	"math/bits"
	"strings"
)

const (
	kibibyte int64 = 1 << 10
	mebibyte int64 = 1 << 20
	gibibyte int64 = 1 << 30

	maxInt64Digits = 19
)

// Sizes can be plain bytes or use KB, KiB, MB, MiB, GB, and GiB.
// Every suffix uses a power-of-two multiplier, and fractions must resolve to a whole byte.
func Parse(raw string) (int64, error) {
	input := strings.TrimSpace(raw)
	if input == "" {
		return 0, errors.New("empty value")
	}

	number, suffix, err := splitValue(input)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", raw, err)
	}

	scale, ok := scaleForSuffix(suffix)
	if !ok {
		return 0, fmt.Errorf("invalid size %q: unknown suffix %q", raw, suffix)
	}

	size, err := scaleDecimal(number, scale)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", raw, err)
	}
	return size, nil
}

func splitValue(input string) (string, string, error) {
	if input[0] == '-' {
		return "", "", errors.New("value must be non-negative")
	}

	numberEnd := 0
	if input[0] == '+' {
		numberEnd++
	}

	digitSeen := false
	for numberEnd < len(input) && isDigit(input[numberEnd]) {
		digitSeen = true
		numberEnd++
	}
	if numberEnd < len(input) && input[numberEnd] == '.' {
		numberEnd++
		for numberEnd < len(input) && isDigit(input[numberEnd]) {
			digitSeen = true
			numberEnd++
		}
	}
	if !digitSeen {
		return "", "", errors.New("expected a decimal number")
	}

	suffix := strings.TrimSpace(input[numberEnd:])
	if !isASCIILetters(suffix) {
		return "", "", errors.New("suffix must contain only letters")
	}
	return input[:numberEnd], suffix, nil
}

func scaleForSuffix(suffix string) (int64, bool) {
	switch strings.ToLower(suffix) {
	case "", "b":
		return 1, true
	case "kb", "kib":
		return kibibyte, true
	case "mb", "mib":
		return mebibyte, true
	case "gb", "gib":
		return gibibyte, true
	default:
		return 0, false
	}
}

func scaleDecimal(number string, scale int64) (int64, error) {
	number = strings.TrimPrefix(number, "+")
	whole, fraction, _ := strings.Cut(number, ".")
	whole = strings.TrimLeft(whole, "0")
	fraction = strings.TrimRight(fraction, "0")

	if whole == "" {
		whole = "0"
	}
	if len(whole) > maxInt64Digits {
		return 0, errors.New("value exceeds the maximum byte size")
	}

	// Multiplying by 2^n cannot make a decimal with more than n meaningful
	// fractional digits whole. There is no need to build a large rational for it.
	maxFractionDigits := bits.TrailingZeros64(uint64(scale))
	if len(fraction) > maxFractionDigits {
		return 0, errors.New("value resolves to a fractional byte")
	}

	canonical := whole
	if fraction != "" {
		canonical += "." + fraction
	}
	// Exact arithmetic keeps large values from being rounded across a byte or int64 boundary.
	value, ok := new(big.Rat).SetString(canonical)
	if !ok {
		return 0, errors.New("expected a decimal number")
	}
	value.Mul(value, new(big.Rat).SetInt64(scale))

	if !value.IsInt() {
		return 0, errors.New("value resolves to a fractional byte")
	}
	if !value.Num().IsInt64() {
		return 0, errors.New("value exceeds the maximum byte size")
	}
	return value.Num().Int64(), nil
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isASCIILetters(value string) bool {
	for i := range len(value) {
		ch := value[i]
		isLower := ch >= 'a' && ch <= 'z'
		isUpper := ch >= 'A' && ch <= 'Z'
		if !isLower && !isUpper {
			return false
		}
	}
	return true
}
