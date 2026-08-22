package bytesize

import "strings"

type Budget struct {
	limit int64
	set   bool
}

func Of(limit int64) Budget { return Budget{limit: limit, set: true} }

func Unlimited() Budget { return Budget{set: true} }

func (b Budget) Set() bool { return b.set }

func (b Budget) Or(def int64) int64 {
	if !b.set {
		return def
	}
	return b.limit
}

func ParseBudget(raw string) (Budget, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "none", "off", "unlimited":
		return Unlimited(), nil
	}

	size, err := Parse(raw)
	if err != nil {
		return Budget{}, err
	}
	return Of(size), nil
}
