package util

import "slices"

func SortedKeys[M ~map[K]V, K ~string, V any](m M) []K {
	if len(m) == 0 {
		return nil
	}
	ks := make([]K, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}
