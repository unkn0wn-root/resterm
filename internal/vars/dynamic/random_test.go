package dynamic

import (
	"math"
	"testing"
)

func TestRandUint64StaysBelowN(t *testing.T) {
	t.Parallel()

	sizes := []uint64{1, 2, 6, 62, 1 << 32, 1<<62 + 1, math.MaxUint64/3 + 1, math.MaxUint64}
	for _, n := range sizes {
		for range 100 {
			if got := randUint64(n); got >= n {
				t.Fatalf("randUint64(%d) = %d, want below %d", n, got, got)
			}
		}
	}
}

// A range 2^64 does not divide evenly is where plain modulo skews low.
func TestRandUint64IsUnbiasedOnWideRanges(t *testing.T) {
	t.Parallel()

	const (
		n     = math.MaxUint64/3*2 + 1 // one wrap covers this range once and a bit
		draws = 4000
	)
	low := 0
	for range draws {
		if randUint64(n) < n/2 {
			low++
		}
	}
	// Plain modulo puts two thirds of the draws in the low half. Sampling
	// noise is about 0.8%, so a 5% band cannot flake.
	if low < draws*45/100 || low > draws*55/100 {
		t.Fatalf("%d of %d draws landed in the low half, want about half", low, draws)
	}
}

func TestRandomDigitsUsesEveryDigit(t *testing.T) {
	t.Parallel()

	seen := make(map[rune]bool, 10)
	for range 500 {
		for _, r := range randomDigits(4) {
			if r < '0' || r > '9' {
				t.Fatalf("randomDigits returned %q", r)
			}
			seen[r] = true
		}
	}
	if len(seen) != 10 {
		t.Fatalf("randomDigits produced %d distinct digits, want 10", len(seen))
	}
}
