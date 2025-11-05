package history

import (
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/traceutil"
)

type TraceSummary struct {
	Started   time.Time     `json:"started,omitempty"`
	Completed time.Time     `json:"completed,omitempty"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error,omitempty"`
	Phases    []TracePhase  `json:"phases,omitempty"`
	Budgets   *TraceBudget  `json:"budgets,omitempty"`
	Breaches  []TraceBreach `json:"breaches,omitempty"`
}

type TracePhase struct {
	Kind     string         `json:"kind"`
	Duration time.Duration  `json:"duration"`
	Error    string         `json:"error,omitempty"`
	Meta     TracePhaseMeta `json:"meta,omitempty"`
}

type TracePhaseMeta struct {
	Addr   string `json:"addr,omitempty"`
	Reused bool   `json:"reused,omitempty"`
	Cached bool   `json:"cached,omitempty"`
}

type TraceBudget struct {
	Total     time.Duration            `json:"total,omitempty"`
	Tolerance time.Duration            `json:"tolerance,omitempty"`
	Phases    map[string]time.Duration `json:"phases,omitempty"`
}

type TraceBreach struct {
	Kind   string        `json:"kind"`
	Limit  time.Duration `json:"limit"`
	Actual time.Duration `json:"actual"`
	Over   time.Duration `json:"over"`
}

// NewTraceSummary captures the essential pieces of a timeline and optional
// report for serialization into history entries.
func NewTraceSummary(tl *nettrace.Timeline, rep *nettrace.Report) *TraceSummary {
	if tl == nil {
		return nil
	}

	summary := &TraceSummary{
		Started:   tl.Started,
		Completed: tl.Completed,
		Duration:  tl.Duration,
		Error:     tl.Err,
	}

	if len(tl.Phases) == 0 {
		return summary
	}

	summary.Phases = make([]TracePhase, len(tl.Phases))
	for i, phase := range tl.Phases {
		summary.Phases[i] = TracePhase{
			Kind:     string(phase.Kind),
			Duration: phase.Duration,
			Error:    phase.Err,
			Meta: TracePhaseMeta{
				Addr:   phase.Meta.Addr,
				Reused: phase.Meta.Reused,
				Cached: phase.Meta.Cached,
			},
		}
	}

	if rep != nil {
		budget := rep.Budget.Clone()
		if traceutil.HasBudget(budget) {
			bud := &TraceBudget{Total: budget.Total, Tolerance: budget.Tolerance}
			if len(budget.Phases) > 0 {
				phases := make(map[string]time.Duration, len(budget.Phases))
				for kind, dur := range budget.Phases {
					if dur <= 0 {
						continue
					}
					phases[string(kind)] = dur
				}
				if len(phases) > 0 {
					bud.Phases = phases
				}
			}
			summary.Budgets = bud
		}

		if len(rep.BudgetReport.Breaches) > 0 {
			breaches := make([]TraceBreach, 0, len(rep.BudgetReport.Breaches))
			for _, br := range rep.BudgetReport.Breaches {
				breaches = append(breaches, TraceBreach{
					Kind:   string(br.Kind),
					Limit:  br.Limit,
					Actual: br.Actual,
					Over:   br.Over,
				})
			}
			summary.Breaches = breaches
		}
	}

	return summary
}

// Timeline reconstructs a nettrace timeline from the summary, inferring start
// and end timestamps when only partial data is available.
func (s *TraceSummary) Timeline() *nettrace.Timeline {
	if s == nil {
		return nil
	}

	tl := &nettrace.Timeline{
		Started:   s.Started,
		Completed: s.Completed,
		Duration:  s.Duration,
		Err:       s.Error,
	}
	if len(s.Phases) == 0 {
		return tl
	}

	phases := make([]nettrace.Phase, len(s.Phases))
	anchor := s.Started
	for i, phase := range s.Phases {
		dur := phase.Duration
		start := anchor
		end := start
		if !start.IsZero() && dur > 0 {
			end = start.Add(dur)
		}
		phases[i] = nettrace.Phase{
			Kind:     nettrace.PhaseKind(phase.Kind),
			Start:    start,
			End:      end,
			Duration: dur,
			Err:      phase.Error,
			Meta: nettrace.PhaseMeta{
				Addr:   phase.Meta.Addr,
				Reused: phase.Meta.Reused,
				Cached: phase.Meta.Cached,
			},
		}
		if !anchor.IsZero() {
			anchor = end
		}
	}

	tl.Phases = phases
	if tl.Duration <= 0 {
		var sum time.Duration
		for _, phase := range phases {
			sum += phase.Duration
		}
		tl.Duration = sum
	}
	if tl.Completed.IsZero() && !tl.Started.IsZero() && tl.Duration > 0 {
		tl.Completed = tl.Started.Add(tl.Duration)
	}
	if tl.Started.IsZero() && !tl.Completed.IsZero() && tl.Duration > 0 {
		tl.Started = tl.Completed.Add(-tl.Duration)
	}
	return tl
}

// Report rebuilds a nettrace report including budget information and recorded
// breaches so callers can reuse the analysis pipeline.
func (s *TraceSummary) Report() *nettrace.Report {
	if s == nil {
		return nil
	}

	tl := s.Timeline()
	if tl == nil {
		return nil
	}

	var budget nettrace.Budget
	if s.Budgets != nil {
		budget.Total = s.Budgets.Total
		budget.Tolerance = s.Budgets.Tolerance
		if len(s.Budgets.Phases) > 0 {
			phases := make(map[nettrace.PhaseKind]time.Duration, len(s.Budgets.Phases))
			for name, dur := range s.Budgets.Phases {
				if dur <= 0 {
					continue
				}
				phases[nettrace.PhaseKind(name)] = dur
			}
			if len(phases) > 0 {
				budget.Phases = phases
			}
		}
	}

	rep := nettrace.NewReport(tl, budget)
	if rep == nil {
		return nil
	}
	if len(s.Breaches) == 0 {
		return rep
	}

	breaches := make([]nettrace.BudgetBreach, len(s.Breaches))
	for i, br := range s.Breaches {
		breaches[i] = nettrace.BudgetBreach{
			Kind:   nettrace.PhaseKind(br.Kind),
			Limit:  br.Limit,
			Actual: br.Actual,
			Over:   br.Over,
		}
	}
	rep.BudgetReport.Breaches = breaches
	return rep
}
