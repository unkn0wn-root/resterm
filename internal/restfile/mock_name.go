package restfile

import (
	"fmt"
	"regexp"
	"strings"
)

const MaxMockNameLen = 56

type Names map[string]struct{}

func (n Names) Add(name string) { n[strings.ToLower(name)] = struct{}{} }

func (n Names) Has(name string) bool {
	_, ok := n[strings.ToLower(name)]
	return ok
}

var mockNameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func ValidMockName(name string) bool {
	return mockNameRE.MatchString(name)
}

// MockNameSlug reduces a label to the characters a mock name accepts, joining
// every run of rejected ones with a single dash. A label with nothing usable in
// it slugs to the empty string, which no mock can be named.
func MockNameSlug(raw string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-._")
}

func TruncateMockName(base string) string {
	if len(base) <= MaxMockNameLen {
		return base
	}
	return strings.Trim(base[:MaxMockNameLen], "-._")
}

// UniqueMockName returns base, or the first free base-N when base is taken, and
// records the result in used. Callers slug their label first, so an empty base
// is the only one it has to name itself.
func UniqueMockName(base string, used Names) string {
	base = strings.Trim(strings.TrimSpace(base), "-")
	if base == "" {
		base = "scenario"
	}
	if !used.Has(base) {
		used.Add(base)
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if used.Has(candidate) {
			continue
		}
		used.Add(candidate)
		return candidate
	}
}
