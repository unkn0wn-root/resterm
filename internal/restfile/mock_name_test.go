package restfile

import (
	"strings"
	"testing"
)

var slugSeeds = []string{
	"",
	"   ",
	"---",
	"!!!",
	"...",
	"___",
	"-leading",
	"trailing-",
	".dotted.",
	"Payment accepted",
	"application/json",
	"200 OK",
	"v1.2.3",
	"a--b",
	"a/b?c=d&e",
	"Ünïcødé näme",
	"emoji \U0001F389 here",
	"tabs\tand\nnewlines",
	strings.Repeat("x", 300),
}

// A slug is fed straight back as a mock name, so anything it emits has to pass
// the check the parser and the compiler apply to a written one.
func FuzzMockNameSlugStaysValid(f *testing.F) {
	for _, seed := range slugSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		slug := MockNameSlug(raw)
		if slug != "" && !ValidMockName(slug) {
			t.Fatalf("MockNameSlug(%q) = %q, which ValidMockName rejects", raw, slug)
		}
	})
}

func TestUniqueMockNameStaysValidAndUnique(t *testing.T) {
	used := make(map[string]struct{})
	seen := make(map[string]bool)
	for _, label := range slugSeeds {
		name := UniqueMockName(MockNameSlug(label), used)
		if !ValidMockName(name) {
			t.Errorf("UniqueMockName for %q = %q, which ValidMockName rejects", label, name)
		}
		if seen[name] {
			t.Errorf("UniqueMockName for %q reused %q", label, name)
		}
		seen[name] = true
	}
}
