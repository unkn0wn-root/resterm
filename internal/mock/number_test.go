package mock

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

func TestParseNumberAcceptsAndRejects(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"0", true},
		{"-0", true},
		{"12", true},
		{"+12", true},
		{"007", true},
		{"1.5", true},
		{"-1.50", true},
		{"1e2", true},
		{"1E+2", true},
		{"1e-2", true},
		{"0.0e0", true},
		{"1e999999999999999999999999", true},

		{"", false},
		{" 1", false},
		{"1 ", false},
		{".5", false},
		{"5.", false},
		{"1.2.3", false},
		{"1e", false},
		{"1e+", false},
		{"1e1.5", false},
		{"0x10", false},
		{"1_000", false},
		{"none", false},
		{"Inf", false},
		{"NaN", false},
	}
	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			if _, ok := parseNumber(test.text); ok != test.want {
				t.Fatalf("parseNumber(%q) ok = %t, want %t", test.text, ok, test.want)
			}
		})
	}
}

func TestNumberCmpIsExact(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"zero forms", "0", "-0", 0},
		{"zero against padded zero", "0.000", "0e100", 0},
		{"negative zero is not below zero", "-0.0", "0", 0},
		{"exponent forms", "100", "1e2", 0},
		{"trailing zeros", "1.5", "1.500", 0},
		{"leading zeros", "007", "7", 0},
		{"adjacent large integers", "9007199254740993", "9007199254740992", 1},
		{"large integer against its exponent form", "9007199254740993", "9.007199254740993e15", 0},
		{"negatives order the other way", "-2", "-1", -1},
		{"negative below positive", "-1e100", "1e-100", -1},
		{"zero below a tiny positive", "0", "1e-999999999", -1},
		{"tiny positive above zero", "1e-999999999", "0", 1},
		{"tiny positives stay ordered", "1e-999999999", "2e-999999999", -1},
		{"tiny positive against a smaller one", "1e-999999999", "1e-1000000000", 1},
		{"huge exponents stay ordered", "1e999999999", "2e999999999", -1},
		{"huge exponent against a bigger one", "1e1000000000", "1e999999999", 1},
		{"same value at different exponents", "1e100", "10e99", 0},
		{"exponents just under the limit", "1e999999999999999997", "1e999999999999999996", 1},
		{"the shift is applied before the limit", "0.001e1000000000000000000", "1e999999999999999997", 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, ok := parseNumber(test.a)
			if !ok {
				t.Fatalf("parseNumber(%q) failed", test.a)
			}
			b, ok := parseNumber(test.b)
			if !ok {
				t.Fatalf("parseNumber(%q) failed", test.b)
			}
			if got := a.cmp(b); got != test.want {
				t.Fatalf("%s.cmp(%s) = %d, want %d", test.a, test.b, got, test.want)
			}
			if got := b.cmp(a); got != -test.want {
				t.Fatalf("%s.cmp(%s) = %d, want %d", test.b, test.a, got, -test.want)
			}
		})
	}
}

func TestNumberExponentSaturates(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"huge exponents collapse together", "1e99999999999999999999", "1e99999999999999999998", 0},
		{"digits stop counting once huge", "1e10000000000000000000", "9e10000000000000000000", 0},
		{"tiny exponents collapse together", "1e-99999999999999999999", "1e-99999999999999999998", 0},
		{"huge stays above the largest finite", "1e10000000000000000000", "1e999999999999999997", 1},
		{"tiny stays below the smallest finite", "1e-10000000000000000000", "1e-999999999999999997", -1},
		{"tiny stays above zero", "1e-10000000000000000000", "0", 1},
		{"huge negatives sort below huge positives", "-1e10000000000000000000", "1e10000000000000000000", -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, aok := parseNumber(test.a)
			b, bok := parseNumber(test.b)
			if !aok || !bok {
				t.Fatalf("parseNumber(%q) = %t, parseNumber(%q) = %t", test.a, aok, test.b, bok)
			}
			if got := a.cmp(b); got != test.want {
				t.Fatalf("%s.cmp(%s) = %d, want %d", test.a, test.b, got, test.want)
			}
			if got := b.cmp(a); got != -test.want {
				t.Fatalf("%s.cmp(%s) = %d, want %d", test.b, test.a, got, -test.want)
			}
		})
	}
}

func TestParseNumberDoesNotAllocate(t *testing.T) {
	huge := "1e" + strings.Repeat("9", 1<<20)
	if allocs := testing.AllocsPerRun(10, func() { parseNumber(huge) }); allocs != 0 {
		t.Fatalf("parseNumber allocated %v times per run, want 0", allocs)
	}
}

func TestNumbersStayDistinctPastAnyFixedPrecision(t *testing.T) {
	for _, digits := range []int{78, 4096} {
		high := strings.Repeat("9", digits)
		low := high[:digits-1] + "8"

		a, ok := parseNumber(high)
		if !ok {
			t.Fatalf("%d digits did not parse", digits)
		}
		b, ok := parseNumber(low)
		if !ok {
			t.Fatalf("%d digits did not parse", digits)
		}
		if a.cmp(b) != 1 {
			t.Fatalf("%d digits: the larger value did not compare greater", digits)
		}
	}
}

func TestEqualJSONNumbers(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1", "1", true},
		{"100", "1e2", true},
		{"1.5", "1.50", true},
		{"0", "-0", true},
		{"9007199254740993", "9.007199254740993e15", true},
		{"9007199254740993", "9007199254740992", false},
		{"9007199254740993", "9007199254740992.0", false},
		{"1e100", "10e99", true},
		{"1", "2", false},
		{"1", "1e999999999", false},
		{"1e999999999", "1e999999999", true},
		{"1e999999999", "2e999999999", false},
		{"1e-999999999", "1e-1000000000", false},
		{"1e-999999999", "0", false},
	}
	for _, test := range tests {
		t.Run(test.a+"_"+test.b, func(t *testing.T) {
			if got := equalJSONNumbers(json.Number(test.a), json.Number(test.b)); got != test.want {
				t.Fatalf("equalJSONNumbers(%q, %q) = %t, want %t", test.a, test.b, got, test.want)
			}
		})
	}
}

func FuzzNumberCmpMatchesBigRat(f *testing.F) {
	seeds := []string{"0", "-0", "1", "1.0", "1e2", "100", "-2.5", "007", "0.1", "1e-3"}
	for _, a := range seeds {
		for _, b := range seeds {
			f.Add(a, b)
		}
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		x, okx := parseNumber(a)
		y, oky := parseNumber(b)
		if okx && oky && x.cmp(y) != -y.cmp(x) {
			t.Fatalf("%q.cmp(%q) = %d, %q.cmp(%q) = %d", a, b, x.cmp(y), b, a, y.cmp(x))
		}
		rx, okrx := boundedRat(a)
		ry, okry := boundedRat(b)
		if !okx || !oky || !okrx || !okry {
			return
		}
		if got, want := x.cmp(y), rx.Cmp(ry); got != want {
			t.Fatalf("%q.cmp(%q) = %d, want %d", a, b, got, want)
		}
	})
}

func boundedRat(s string) (*big.Rat, bool) {
	n, ok := parseNumber(s)
	if !ok || len(s) > 40 || strings.ContainsAny(s, "/pP") {
		return nil, false
	}
	if n.digits != "" && (n.exp > 100 || n.exp < -100) {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(s)
	return r, ok
}
