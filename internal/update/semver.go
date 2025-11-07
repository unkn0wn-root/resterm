package update

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidSemver = errors.New("invalid semantic version")

type semver struct {
	maj   int
	min   int
	patch int
	pre   string
}

func parseSemver(v string) (semver, error) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return semver{}, ErrInvalidSemver
	}

	var pre string
	parts := strings.SplitN(v, "-", 2)
	if len(parts) == 2 {
		pre = parts[1]
	}
	core := parts[0]

	segs := strings.Split(core, ".")
	if len(segs) < 1 || len(segs) > 3 {
		return semver{}, fmt.Errorf("%w: %s", ErrInvalidSemver, v)
	}
	vals := make([]int, 3)
	for i := 0; i < 3; i++ {
		if i < len(segs) {
			n, err := strconv.Atoi(segs[i])
			if err != nil {
				return semver{}, fmt.Errorf("%w: %s", ErrInvalidSemver, v)
			}
			vals[i] = n
		}
	}

	return semver{maj: vals[0], min: vals[1], patch: vals[2], pre: pre}, nil
}

// Stable releases sort higher than prereleases with the same major.minor.patch.
// So 1.0.0 > 1.0.0-beta even though lexicographically that looks backwards.
func (a semver) lt(b semver) bool {
	if a.maj != b.maj {
		return a.maj < b.maj
	}
	if a.min != b.min {
		return a.min < b.min
	}
	if a.patch != b.patch {
		return a.patch < b.patch
	}
	if a.pre == b.pre {
		return false
	}
	if a.pre == "" {
		return false
	}
	if b.pre == "" {
		return true
	}
	return a.pre < b.pre
}

func compareSemver(a, b string) (int, error) {
	la, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	lb, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	switch {
	case la.lt(lb):
		return -1, nil
	case lb.lt(la):
		return 1, nil
	default:
		return 0, nil
	}
}
