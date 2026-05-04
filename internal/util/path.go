package util

import "path/filepath"

// SamePath reports whether two non-empty paths resolve to the same lexical path.
//
// It does not stat files or resolve symlinks; use os.SameFile with os.Stat
// results when physical file identity is required.
func SamePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}

	if a == b {
		return true
	}

	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	return absA == absB
}

// SamePathOrBothEmpty reports whether a and b are both empty or name the same path.
func SamePathOrBothEmpty(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	return SamePath(a, b)
}
