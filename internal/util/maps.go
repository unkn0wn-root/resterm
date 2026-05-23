package util

import (
	"maps"
	"sort"
)

func CloneMap[K comparable, V any](src map[K]V) map[K]V {
	if len(src) == 0 {
		return nil
	}
	return maps.Clone(src)
}

func SortedKeys[M ~map[K]V, K ~string, V any](m M) []K {
	if len(m) == 0 {
		return nil
	}
	ks := make([]K, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool { return ks[i] < ks[j] })
	return ks
}
