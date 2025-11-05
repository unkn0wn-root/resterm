package scripts

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/traceutil"
)

// TraceInput carries timeline and budget information into the scripting runtime.
type TraceInput struct {
	Timeline *nettrace.Timeline
	Budgets  TraceBudget
}

// TraceBudget represents optional latency budgets configured for a request.
type TraceBudget struct {
	Total     time.Duration
	Tolerance time.Duration
	Phases    map[string]time.Duration
}

type traceBinding struct {
	timeline   *nettrace.Timeline
	budgets    TraceBudget
	report     nettrace.BudgetReport
	aggregates map[string]*traceAggregate
	segments   []traceSegment
	phaseOrder []string
}

type traceAggregate struct {
	Name     string
	Duration time.Duration
	Count    int
	Segments []traceSegment
}

type traceSegment struct {
	Name     string
	Start    time.Time
	End      time.Time
	Duration time.Duration
	Err      string
	Meta     nettrace.PhaseMeta
}

// NewTraceInput converts a timeline/spec pair into a scripting input, cloning data to keep it immutable.
func NewTraceInput(tl *nettrace.Timeline, spec *restfile.TraceSpec) *TraceInput {
	if tl == nil {
		return nil
	}

	clone := tl.Clone()
	budget := TraceBudget{}
	if spec != nil {
		budget.Total = spec.Budgets.Total
		budget.Tolerance = spec.Budgets.Tolerance
		if len(spec.Budgets.Phases) > 0 {
			budget.Phases = make(map[string]time.Duration, len(spec.Budgets.Phases))
			for name, dur := range spec.Budgets.Phases {
				budget.Phases[name] = dur
			}
		}
	}
	return &TraceInput{Timeline: clone, Budgets: budget}
}

// newTraceBinding clones the supplied trace input and prepares aggregates and
// budget evaluation used by the scripting interface.
func newTraceBinding(input *TraceInput) *traceBinding {
	if input == nil || input.Timeline == nil {
		return &traceBinding{}
	}

	timeline := input.Timeline.Clone()
	binding := &traceBinding{
		timeline:   timeline,
		budgets:    cloneTraceBudget(input.Budgets),
		aggregates: make(map[string]*traceAggregate),
	}

	for _, phase := range timeline.Phases {
		segment := traceSegment{
			Name:     string(phase.Kind),
			Start:    phase.Start,
			End:      phase.End,
			Duration: phase.Duration,
			Err:      phase.Err,
			Meta:     phase.Meta,
		}
		binding.segments = append(binding.segments, segment)

		key := strings.ToLower(segment.Name)
		agg, ok := binding.aggregates[key]
		if !ok {
			agg = &traceAggregate{Name: segment.Name}
			binding.aggregates[key] = agg
			binding.phaseOrder = append(binding.phaseOrder, segment.Name)
		}
		agg.Duration += segment.Duration
		agg.Count++
		agg.Segments = append(agg.Segments, segment)
	}

	binding.report = evaluateTraceBudget(timeline, binding.budgets)
	return binding
}

// cloneTraceBudget copies the duration values so scripts cannot mutate shared
// state.
func cloneTraceBudget(b TraceBudget) TraceBudget {
	clone := TraceBudget{Total: b.Total, Tolerance: b.Tolerance}
	if len(b.Phases) > 0 {
		clone.Phases = make(map[string]time.Duration, len(b.Phases))
		for name, dur := range b.Phases {
			clone.Phases[name] = dur
		}
	}
	return clone
}

// evaluateTraceBudget converts the scripting budget format into a nettrace
// budget report by reusing the shared traceutil conversion path.
func evaluateTraceBudget(tl *nettrace.Timeline, budget TraceBudget) nettrace.BudgetReport {
	if tl == nil {
		return nettrace.BudgetReport{}
	}

	raw := restfile.TraceBudget{
		Total:     budget.Total,
		Tolerance: budget.Tolerance,
	}
	if len(budget.Phases) > 0 {
		raw.Phases = make(map[string]time.Duration, len(budget.Phases))
		for name, dur := range budget.Phases {
			raw.Phases[name] = dur
		}
	}
	converted := traceutil.BudgetFromTraceBudget(raw)
	return nettrace.EvaluateBudget(tl, converted)
}

// object exposes the bindings consumed by the scripting runtime, exporting
// functions that lazily compute each property.
func (tb *traceBinding) object() map[string]interface{} {
	if tb == nil {
		tb = &traceBinding{}
	}

	return map[string]interface{}{
		"enabled":         func() bool { return tb.timeline != nil },
		"durationMs":      func() float64 { return tb.durationMillis() },
		"durationSeconds": func() float64 { return tb.durationSeconds() },
		"durationString":  func() string { return tb.durationString() },
		"error":           func() string { return tb.errorString() },
		"started":         func() string { return tb.startedAt() },
		"completed":       func() string { return tb.completedAt() },
		"phases":          func() []map[string]interface{} { return tb.exportPhases() },
		"getPhase":        func(name string) map[string]interface{} { return tb.getPhase(name) },
		"phaseNames":      func() []string { return tb.phaseNames() },
		"budgets":         func() map[string]interface{} { return tb.exportBudgets() },
		"breaches":        func() []map[string]interface{} { return tb.exportBreaches() },
		"withinBudget":    func() bool { return len(tb.report.Breaches) == 0 },
		"hasBudgets":      func() bool { return tb.hasBudgets() },
	}
}

func (tb *traceBinding) durationMillis() float64 {
	if tb.timeline == nil {
		return 0
	}
	return float64(tb.timeline.Duration) / float64(time.Millisecond)
}

func (tb *traceBinding) durationSeconds() float64 {
	if tb.timeline == nil {
		return 0
	}
	return tb.timeline.Duration.Seconds()
}

func (tb *traceBinding) durationString() string {
	if tb.timeline == nil {
		return ""
	}
	return tb.timeline.Duration.String()
}

func (tb *traceBinding) errorString() string {
	if tb.timeline == nil {
		return ""
	}
	return tb.timeline.Err
}

func (tb *traceBinding) startedAt() string {
	if tb.timeline == nil || tb.timeline.Started.IsZero() {
		return ""
	}
	return tb.timeline.Started.Format(time.RFC3339Nano)
}

func (tb *traceBinding) completedAt() string {
	if tb.timeline == nil || tb.timeline.Completed.IsZero() {
		return ""
	}
	return tb.timeline.Completed.Format(time.RFC3339Nano)
}

func (tb *traceBinding) exportPhases() []map[string]interface{} {
	if len(tb.segments) == 0 {
		return []map[string]interface{}{}
	}

	out := make([]map[string]interface{}, 0, len(tb.segments))
	for _, seg := range tb.segments {
		out = append(out, exportSegment(seg))
	}
	return out
}

// getPhase returns aggregate information for a named phase including the
// original segments that contributed to it.
func (tb *traceBinding) getPhase(name string) map[string]interface{} {
	if tb.timeline == nil {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return nil
	}

	agg, ok := tb.aggregates[key]
	if !ok {
		return nil
	}

	segments := make([]map[string]interface{}, 0, len(agg.Segments))
	for _, seg := range agg.Segments {
		segments = append(segments, exportSegment(seg))
	}

	return map[string]interface{}{
		"name":            agg.Name,
		"count":           agg.Count,
		"durationMs":      float64(agg.Duration) / float64(time.Millisecond),
		"durationSeconds": agg.Duration.Seconds(),
		"durationString":  agg.Duration.String(),
		"segments":        segments,
	}
}

func (tb *traceBinding) phaseNames() []string {
	if len(tb.phaseOrder) == 0 {
		return []string{}
	}
	out := make([]string, len(tb.phaseOrder))
	copy(out, tb.phaseOrder)
	return out
}

func (tb *traceBinding) hasBudgets() bool {
	if tb == nil {
		return false
	}
	if tb.budgets.Total > 0 || tb.budgets.Tolerance > 0 {
		return true
	}
	return len(tb.budgets.Phases) > 0
}

// exportBudgets normalizes the internal budget representation to a JSON ready
// structure that is easy for scripts to consume.
func (tb *traceBinding) exportBudgets() map[string]interface{} {
	if !tb.hasBudgets() {
		return map[string]interface{}{"enabled": false}
	}

	phases := make(map[string]float64, len(tb.budgets.Phases))
	for name, dur := range tb.budgets.Phases {
		phases[name] = float64(dur) / float64(time.Millisecond)
	}

	return map[string]interface{}{
		"enabled":          true,
		"totalMs":          float64(tb.budgets.Total) / float64(time.Millisecond),
		"totalSeconds":     tb.budgets.Total.Seconds(),
		"toleranceMs":      float64(tb.budgets.Tolerance) / float64(time.Millisecond),
		"toleranceSeconds": tb.budgets.Tolerance.Seconds(),
		"phases":           phases,
	}
}

// exportBreaches lists each breach in a script friendly structure with values
// exposed in both milliseconds and seconds.
func (tb *traceBinding) exportBreaches() []map[string]interface{} {
	if len(tb.report.Breaches) == 0 {
		return []map[string]interface{}{}
	}

	out := make([]map[string]interface{}, 0, len(tb.report.Breaches))
	for _, breach := range tb.report.Breaches {
		out = append(out, map[string]interface{}{
			"name":          string(breach.Kind),
			"limitMs":       float64(breach.Limit) / float64(time.Millisecond),
			"limitSeconds":  breach.Limit.Seconds(),
			"actualMs":      float64(breach.Actual) / float64(time.Millisecond),
			"actualSeconds": breach.Actual.Seconds(),
			"overMs":        float64(breach.Over) / float64(time.Millisecond),
			"overSeconds":   breach.Over.Seconds(),
		})
	}
	return out
}

// exportSegment converts an individual segment into the structure surfaced in
// the scripting runtime.
func exportSegment(seg traceSegment) map[string]interface{} {
	meta := map[string]interface{}{
		"addr":   seg.Meta.Addr,
		"reused": seg.Meta.Reused,
		"cached": seg.Meta.Cached,
	}

	result := map[string]interface{}{
		"name":            seg.Name,
		"durationMs":      float64(seg.Duration) / float64(time.Millisecond),
		"durationSeconds": seg.Duration.Seconds(),
		"durationString":  seg.Duration.String(),
		"error":           seg.Err,
		"meta":            meta,
	}

	if !seg.Start.IsZero() {
		result["start"] = seg.Start.Format(time.RFC3339Nano)
	}
	if !seg.End.IsZero() {
		result["end"] = seg.End.Format(time.RFC3339Nano)
	}
	return result
}
