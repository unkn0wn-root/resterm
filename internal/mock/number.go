package mock

import (
	"cmp"
	"encoding/json"
	"strings"
)

// number stores a decimal as 0.<digits> * 10^exp without rounding. Leading and
// trailing zeros are removed from digits. Zero uses an empty digit string.
type number struct {
	neg    bool
	digits string
	exp    exponent
}

// Exponents come from request bodies and may contain millions of digits. The
// parser caps them so it stays linear without allocating. The cap is far beyond
// any shift caused by a 4 MiB number, so a capped exponent cannot return to the
// finite range after the decimal point is accounted for.
type exponent int64

const expLimit exponent = 1e18

func (e exponent) finite() bool { return -expLimit < e && e < expLimit }

func (e exponent) shift(digits int) exponent {
	return min(max(e+exponent(digits), -expLimit), expLimit)
}

func parseExponent(s string) (exponent, bool) {
	const scanMax = 4 * expLimit

	if s == "" {
		return 0, true
	}
	if s[0] != 'e' && s[0] != 'E' {
		return 0, false
	}

	s = s[1:]
	neg := strings.HasPrefix(s, "-")
	if neg || strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	digits, rest := cutDigits(s)
	if digits == "" || rest != "" {
		return 0, false
	}

	var e exponent
	digits = strings.TrimLeft(digits, "0")
	for i := range digits {
		if e > scanMax/10 {
			e = scanMax
			break
		}
		e = e*10 + exponent(digits[i]-'0')
	}
	e = min(e, scanMax)
	if neg {
		return -e, true
	}
	return e, true
}

type numberRelation uint8

const (
	relGT numberRelation = iota + 1
	relGTE
	relLT
	relLTE
)

func (rel numberRelation) holds(c int) bool {
	switch rel {
	case relGT:
		return c > 0
	case relGTE:
		return c >= 0
	case relLT:
		return c < 0
	case relLTE:
		return c <= 0
	default:
		return false
	}
}

func parseNumber(s string) (number, bool) {
	neg := false
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}

	whole, s := cutDigits(s)
	if whole == "" {
		return number{}, false
	}
	var frac string
	if rest, ok := strings.CutPrefix(s, "."); ok {
		if frac, s = cutDigits(rest); frac == "" {
			return number{}, false
		}
	}

	exp, ok := parseExponent(s)
	if !ok {
		return number{}, false
	}

	digits := whole
	if frac != "" {
		digits += frac
	}
	lead := len(digits) - len(strings.TrimLeft(digits, "0"))
	digits = strings.TrimRight(digits[lead:], "0")
	if digits == "" {
		return number{}, true
	}
	// Account for the decimal point and any removed leading zeros.
	return number{neg: neg, digits: digits, exp: exp.shift(len(whole) - lead)}, true
}

func cutDigits(s string) (digits, rest string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i], s[i:]
}

func (n number) cmp(o number) int {
	switch {
	case n.neg != o.neg:
		if n.neg {
			return -1
		}
		return 1
	case n.digits == "" && o.digits == "":
		return 0
	case n.digits == "":
		return -1
	case o.digits == "":
		return 1
	}

	c := cmp.Compare(n.exp, o.exp)
	if c == 0 && n.exp.finite() {
		c = strings.Compare(n.digits, o.digits)
	}
	if n.neg {
		return -c
	}
	return c
}

func equalJSONNumbers(want, got json.Number) bool {
	if want == got {
		return true
	}
	x, ok := parseNumber(string(want))
	if !ok {
		return false
	}
	y, ok := parseNumber(string(got))
	return ok && x.cmp(y) == 0
}
