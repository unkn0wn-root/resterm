package hint

import "strings"

// Hint represents a single autocomplete option.
type Hint struct {
	Label      string
	Aliases    []string
	Summary    string
	Insert     string
	CursorBack int
}

// Filter returns hints matching the query (prefix match on label or aliases).
func Filter(opts []Hint, q string) []Hint {
	if len(opts) == 0 {
		return nil
	}
	if q == "" {
		return clone(opts)
	}
	lq := strings.ToLower(q)
	var out []Hint
	for _, opt := range opts {
		if match(opt, lq) {
			out = append(out, opt)
		}
	}
	return out
}

func match(h Hint, q string) bool {
	if q == "" {
		return true
	}
	if hasPrefix(h.Label, q) {
		return true
	}
	for _, a := range h.Aliases {
		if hasPrefix(a, q) {
			return true
		}
	}
	return false
}

func hasPrefix(label, q string) bool {
	trimmed := strings.TrimPrefix(label, "@")
	return strings.HasPrefix(strings.ToLower(trimmed), q)
}

func clone(opts []Hint) []Hint {
	if len(opts) == 0 {
		return nil
	}
	out := make([]Hint, len(opts))
	copy(out, opts)
	return out
}
