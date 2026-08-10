package restfile

import (
	"fmt"
	"regexp"
	"strings"
)

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

// UniqueMockName returns base, or the first free base-N when base is taken, and
// records the result in used. Callers slug their label first, so an empty base
// is the only one it has to name itself.
func UniqueMockName(base string, used map[string]struct{}) string {
	base = strings.Trim(strings.TrimSpace(base), "-")
	if base == "" {
		base = "scenario"
	}
	if _, exists := used[base]; !exists {
		used[base] = struct{}{}
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if _, exists := used[candidate]; exists {
			continue
		}
		used[candidate] = struct{}{}
		return candidate
	}
}
