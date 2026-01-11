package util

// DedupeNonEmptyStrings returns a copy of values without empty strings or duplicates, preserving order.
func DedupeNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// DedupeSortedStrings removes consecutive duplicates from a sorted slice.
func DedupeSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:1]
	last := values[0]
	for _, v := range values[1:] {
		if v == last {
			continue
		}
		out = append(out, v)
		last = v
	}
	return out
}
