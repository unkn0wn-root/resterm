package dynamic

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const samples = 200

func TestRandomChoicePicksAnOption(t *testing.T) {
	t.Parallel()

	options := map[string]int{"alpha": 0, "beta": 0, "gamma": 0}
	for range samples {
		out, err := Resolve(`$randomChoice("alpha", 'beta', gamma)`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := options[out]; !ok {
			t.Fatalf("got %q, want one of the options", out)
		}
		options[out]++
	}
	for name, hits := range options {
		if hits == 0 {
			t.Fatalf("option %q was never picked in %d draws", name, samples)
		}
	}
}

func TestRandomString(t *testing.T) {
	t.Parallel()

	out, err := Resolve("$randomString")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != defaultStringLen {
		t.Fatalf("$randomString = %q, want %d characters", out, defaultStringLen)
	}

	sized, err := Resolve("$randomString(64)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sized) != 64 {
		t.Fatalf("$randomString(64) = %q, want 64 characters", sized)
	}
	if strings.ContainsFunc(sized, func(r rune) bool { return !strings.ContainsRune(alphanum, r) }) {
		t.Fatalf("$randomString(64) = %q, want alphanumeric characters only", sized)
	}
	if sized == strings.Repeat(sized[:1], len(sized)) {
		t.Fatalf("$randomString(64) = %q, want varied characters", sized)
	}
}

func TestRandomIntRanges(t *testing.T) {
	t.Parallel()

	seen := make(map[int64]bool)
	for range samples {
		n := resolveInt(t, "$randomInt(1, 6)")
		if n < 1 || n > 6 {
			t.Fatalf("$randomInt(1, 6) = %d, want 1 to 6", n)
		}
		seen[n] = true
	}
	if len(seen) < 2 {
		t.Fatalf("$randomInt(1, 6) never varied across %d draws", samples)
	}
	if n := resolveInt(t, "$randomInt(3, 3)"); n != 3 {
		t.Fatalf("$randomInt(3, 3) = %d, want 3", n)
	}
	if n := resolveInt(t, "$randomInt(-5, -5)"); n != -5 {
		t.Fatalf("$randomInt(-5, -5) = %d, want -5", n)
	}
	if n := resolveInt(t, "$randomInt(0)"); n != 0 {
		t.Fatalf("$randomInt(0) = %d, want 0", n)
	}
	if n := resolveInt(t, "$randomInt"); n < 0 {
		t.Fatalf("$randomInt = %d, want a non-negative value", n)
	}
}

func TestFakeHelpersLookReal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref     string
		pattern string
	}{
		{ref: "$randomName", pattern: `^[A-Z][a-z]+ [A-Z][a-z]+$`},
		{ref: "$fake.person", pattern: `^[A-Z][a-z]+ [A-Z][a-z]+$`},
		{ref: "$fake.firstName", pattern: `^[A-Z][a-z]+$`},
		{ref: "$fake.lastName", pattern: `^[A-Z][a-z]+$`},
		{ref: "$randomEmail", pattern: `^[a-z]+\.[a-z]+[0-9]{2}@example\.(com|net|org)$`},
		{ref: "$fake.email", pattern: `^[a-z]+\.[a-z]+[0-9]{2}@example\.(com|net|org)$`},
		{ref: "$fake.username", pattern: `^[a-z]+[0-9]{2}$`},
		{ref: "$fake.company", pattern: `^[A-Z][a-z]+ [A-Z][a-z]+$`},
		{ref: "$fake.domain", pattern: `^[a-z]+\.example\.com$`},
		{ref: "$fake.phone", pattern: `^\+1-555-01[0-9]{2}$`},
		{ref: "$fake.word", pattern: `^[a-z]+$`},
		{ref: "$fake.sentence", pattern: `^[A-Z][a-z]+( [a-z]+){3,7}\.$`},
		{ref: "$fake.city", pattern: `^[A-Z][a-z]+$`},
		{ref: "$fake.country", pattern: `^[A-Z][a-z]+$`},
	}
	for _, test := range tests {
		t.Run(test.ref, func(t *testing.T) {
			t.Parallel()

			re := regexp.MustCompile(test.pattern)
			values := make(map[string]bool)
			for range samples {
				out, err := Resolve(test.ref)
				if err != nil {
					t.Fatalf("Resolve(%q): %v", test.ref, err)
				}
				if !re.MatchString(out) {
					t.Fatalf("Resolve(%q) = %q, want it to match %s", test.ref, out, test.pattern)
				}
				values[out] = true
			}
			if len(values) < 2 {
				t.Fatalf("Resolve(%q) returned the same value %d times", test.ref, samples)
			}
		})
	}
}

func TestUUIDIsUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool, samples)
	for range samples {
		out, err := Resolve("$guid")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen[out] {
			t.Fatalf("$guid repeated %q", out)
		}
		seen[out] = true
	}
}

func resolveInt(t *testing.T, ref string) int64 {
	t.Helper()

	out, err := Resolve(ref)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", ref, err)
	}
	n, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		t.Fatalf("Resolve(%q) = %q, want a number", ref, out)
	}
	return n
}
