package mock

import (
	"encoding/json"
	"math/big"
	"strings"
)

// number stores a decimal as 0.<digits> * 10^exp without rounding. Leading and
// trailing zeros are removed from digits. Zero uses an empty digit string.
type number struct {
	neg    bool
	digits string
	exp    *big.Int
}

type numberRelation uint8

const (
	relGT numberRelation = iota + 1
	relGTE
	relLT
	relLTE
)

func (rel numberRelation) holds(cmp int) bool {
	switch rel {
	case relGT:
		return cmp > 0
	case relGTE:
		return cmp >= 0
	case relLT:
		return cmp < 0
	case relLTE:
		return cmp <= 0
	default:
		return false
	}
}

// Query values may include a leading plus or leading zeros, so parseNumber
// accepts both. A big integer exponent prevents overflow and underflow.
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

	exp := new(big.Int)
	switch {
	case len(s) > 1 && (s[0] == 'e' || s[0] == 'E'):
		if _, ok := exp.SetString(s[1:], 10); !ok {
			return number{}, false
		}
	case s != "":
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
	return number{
		neg:    neg,
		digits: digits,
		exp:    exp.Add(exp, big.NewInt(int64(len(whole)-lead))),
	}, true
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

	c := n.exp.Cmp(o.exp)
	if c == 0 {
		// Equal exponents put both digit strings on the same scale.
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
