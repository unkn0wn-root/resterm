package nettrace

import (
	"sort"
	"time"
)

type PhaseKind string

const (
	PhaseDNS      PhaseKind = "dns"
	PhaseConnect  PhaseKind = "connect"
	PhaseTLS      PhaseKind = "tls"
	PhaseReqHdrs  PhaseKind = "request_headers"
	PhaseReqBody  PhaseKind = "request_body"
	PhaseTTFB     PhaseKind = "ttfb"
	PhaseTransfer PhaseKind = "transfer"
	PhaseTotal    PhaseKind = "total"
)

type PhaseMeta struct {
	Addr   string
	Reused bool
	Cached bool
}

type Phase struct {
	Kind     PhaseKind
	Start    time.Time
	End      time.Time
	Duration time.Duration
	Err      string
	Meta     PhaseMeta
}

type Timeline struct {
	Started   time.Time
	Completed time.Time
	Duration  time.Duration
	Err       string
	Phases    []Phase
	Details   *TraceDetails
}

func (tl *Timeline) Clone() *Timeline {
	if tl == nil {
		return nil
	}

	ph := make([]Phase, len(tl.Phases))
	copy(ph, tl.Phases)
	return &Timeline{
		Started:   tl.Started,
		Completed: tl.Completed,
		Duration:  tl.Duration,
		Err:       tl.Err,
		Phases:    ph,
		Details:   tl.Details.Clone(),
	}
}

func normalizePhases(phases []Phase) []Phase {
	if len(phases) <= 1 {
		return phases
	}

	sorted := make([]Phase, len(phases))
	copy(sorted, phases)
	sort.SliceStable(sorted, func(i, j int) bool {
		si := sorted[i]
		sj := sorted[j]
		if si.Start.Equal(sj.Start) {
			return si.End.Before(sj.End)
		}
		return si.Start.Before(sj.Start)
	})
	return sorted
}

type Budget struct {
	Total     time.Duration
	Tolerance time.Duration
	Phases    map[PhaseKind]time.Duration
}

// Clone produces a deep copy of the budget to prevent shared mutations.
func (b Budget) Clone() Budget {
	clone := Budget{Total: b.Total, Tolerance: b.Tolerance}
	if len(b.Phases) > 0 {
		clone.Phases = make(map[PhaseKind]time.Duration, len(b.Phases))
		for kind, dur := range b.Phases {
			clone.Phases[kind] = dur
		}
	}
	return clone
}

type BudgetBreach struct {
	Kind   PhaseKind
	Limit  time.Duration
	Actual time.Duration
	Over   time.Duration
}

type BudgetReport struct {
	Breaches []BudgetBreach
}

func (r BudgetReport) WithinLimit() bool {
	return len(r.Breaches) == 0
}

func EvaluateBudget(tl *Timeline, b Budget) BudgetReport {
	if tl == nil {
		return BudgetReport{}
	}

	var breaches []BudgetBreach
	tol := b.Tolerance
	durations := aggregateDurations(tl)

	if b.Total > 0 {
		allowed := b.Total + tol
		if tl.Duration > allowed {
			over := tl.Duration - allowed
			breaches = append(breaches, BudgetBreach{
				Kind:   PhaseTotal,
				Limit:  b.Total,
				Actual: tl.Duration,
				Over:   over,
			})
		}
	}

	for kind, limit := range b.Phases {
		if limit <= 0 {
			continue
		}
		actual := durations[kind]
		allowed := limit + tol
		if actual > allowed {
			over := actual - allowed
			breaches = append(breaches, BudgetBreach{
				Kind:   kind,
				Limit:  limit,
				Actual: actual,
				Over:   over,
			})
		}
	}

	return BudgetReport{Breaches: breaches}
}

// Multiple phases can have the same kind (e.g. multiple DNS lookups)
// so we sum them up for budget checking.
func aggregateDurations(tl *Timeline) map[PhaseKind]time.Duration {
	out := make(map[PhaseKind]time.Duration, len(tl.Phases)+1)
	for _, phase := range tl.Phases {
		if phase.Duration <= 0 {
			continue
		}
		out[phase.Kind] += phase.Duration
	}
	return out
}
