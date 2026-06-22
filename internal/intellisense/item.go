package intellisense

import (
	"slices"
	"strings"
)

type Item struct {
	Label      string
	Aliases    []string
	Summary    string
	Insert     string
	CursorBack int
}

func filter(opts []Item, q string) []Item {
	if len(opts) == 0 {
		return nil
	}
	if q == "" {
		return slices.Clone(opts)
	}
	lq := strings.ToLower(q)
	var out []Item
	for _, opt := range opts {
		if match(opt, lq) {
			out = append(out, opt)
		}
	}
	return out
}

func match(it Item, q string) bool {
	if hasPrefix(it.Label, q) {
		return true
	}
	for _, a := range it.Aliases {
		if hasPrefix(a, q) {
			return true
		}
	}
	return false
}

func hasPrefix(label, q string) bool {
	label = strings.TrimPrefix(label, "@")
	return strings.HasPrefix(strings.ToLower(label), q)
}
