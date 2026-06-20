package intellisense

import "strings"

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
		return clone(opts)
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

func clone(opts []Item) []Item {
	out := make([]Item, len(opts))
	copy(out, opts)
	return out
}

func normalizeKey(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "@")
	return strings.ToLower(raw)
}
