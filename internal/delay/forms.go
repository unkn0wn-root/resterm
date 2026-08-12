package delay

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/duration"
)

type kind uint8

const (
	fixed kind = iota
	random
	normal
	jitter
)

type form struct {
	name     string // empty for a bare duration, which is not a call
	params   [2]string
	summary  string
	usage    string
	relative bool // the deviation may be a percentage of base
	check    func(Spec) error
	sample   func(Spec, source) time.Duration
}

// Every call form takes a base value and a deviation from it. The table is
// indexed by kind, so a new distribution is one constant above, one entry
// here, and a sampler.
var forms = [...]form{
	fixed: {sample: sampleFixed},
	random: {
		name:    "random",
		params:  [2]string{"min", "max"},
		summary: "Uniform wait between two bounds",
		usage:   "random(100ms,500ms)",
		check:   checkRandom,
		sample:  sampleRandom,
	},
	normal: {
		name:     "normal",
		params:   [2]string{"mean", "stddev"},
		summary:  "Normally distributed wait around a mean",
		usage:    "normal(250ms,50ms)",
		relative: true,
		sample:   sampleNormal,
	},
	jitter: {
		name:     "jitter",
		params:   [2]string{"base", "spread"},
		summary:  "Uniform wait within a spread of a base value",
		usage:    "jitter(200ms,20%)",
		relative: true,
		sample:   sampleJitter,
	},
}

func lookup(name string) (kind, form, bool) {
	for i, f := range forms {
		if f.name != "" && strings.EqualFold(f.name, name) {
			return kind(i), f, true
		}
	}
	return fixed, form{}, false
}

func (f form) duration(i int, raw string) (time.Duration, error) {
	d, ok := duration.Parse(raw)
	switch {
	case !ok:
		return 0, fmt.Errorf("%s %s %q is not a duration", f.name, f.params[i], raw)
	case d < 0:
		return 0, fmt.Errorf("%s %s cannot be negative", f.name, f.params[i])
	}
	return d, nil
}

// A value without the percent sign belongs to duration, so the only failure
// here is a percentage written where the form does not take one.
func (f form) percent(i int, raw string) (float64, bool, error) {
	text, ok := strings.CutSuffix(raw, "%")
	if !ok {
		return 0, false, nil
	}
	if !f.relative {
		return 0, false, fmt.Errorf("%s %s must be a duration, not a percentage", f.name, f.params[i])
	}
	pct, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	switch {
	case err != nil || math.IsNaN(pct) || math.IsInf(pct, 0):
		return 0, false, fmt.Errorf("%s %s %q is not a percentage", f.name, f.params[i], raw)
	case pct < 0:
		return 0, false, fmt.Errorf("%s %s cannot be negative", f.name, f.params[i])
	}
	return pct, true, nil
}

func checkRandom(s Spec) error {
	if s.spread < s.base {
		return errors.New("random max cannot be shorter than min")
	}
	return nil
}

func sampleFixed(s Spec, _ source) time.Duration {
	return max(s.base, 0)
}

func sampleRandom(s Spec, r source) time.Duration {
	lo, hi := s.base, s.spread
	return clamp(float64(lo) + float64(hi-lo)*r.Float64())
}

func sampleNormal(s Spec, r source) time.Duration {
	mean, stddev := s.base, s.deviation()
	return clamp(float64(mean) + float64(stddev)*r.NormFloat64())
}

func sampleJitter(s Spec, r source) time.Duration {
	center, spread := s.base, s.deviation()
	return clamp(float64(center) + float64(spread)*(2*r.Float64()-1))
}

// Tests seed their own source instead of the global one below.
type source interface {
	Float64() float64
	NormFloat64() float64
}

// The top level math/rand/v2 functions are safe for concurrent use, which the
// mock server needs. A wait is not security sensitive.
type globalSource struct{}

func (globalSource) Float64() float64 { return rand.Float64() }

func (globalSource) NormFloat64() float64 { return rand.NormFloat64() }

const maxDurationFloat = float64(math.MaxInt64)

// clamp keeps a draw inside the duration range; below zero nothing is left to wait for.
func clamp(v float64) time.Duration {
	switch {
	case v <= 0:
		return 0
	case v >= maxDurationFloat:
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(v)
}
