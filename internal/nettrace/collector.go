package nettrace

import (
	"errors"
	"sort"
	"sync"
	"time"
)

type phaseState struct {
	kind  PhaseKind
	start time.Time
	meta  PhaseMeta
}

type Collector struct {
	mu        sync.Mutex
	started   time.Time
	finished  time.Time
	err       string
	phases    []Phase
	active    map[PhaseKind]*phaseState
	completed bool
}

func NewCollector() *Collector {
	return &Collector{active: make(map[PhaseKind]*phaseState)}
}

func (c *Collector) Begin(kind PhaseKind, ts time.Time) {
	if kind == "" || kind == PhaseTotal {
		return
	}

	if ts.IsZero() {
		ts = time.Now()
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started.IsZero() || ts.Before(c.started) {
		c.started = ts
	}

	state := &phaseState{kind: kind, start: ts}
	c.active[kind] = state
}

func (c *Collector) End(kind PhaseKind, ts time.Time, err error) {
	if kind == "" || kind == PhaseTotal {
		return
	}

	if ts.IsZero() {
		ts = time.Now()
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	state, ok := c.active[kind]
	if !ok {
		state = &phaseState{kind: kind, start: ts}
	}

	if ts.Before(state.start) {
		ts = state.start
	}

	phase := Phase{
		Kind:     kind,
		Start:    state.start,
		End:      ts,
		Duration: ts.Sub(state.start),
		Meta:     state.meta,
	}
	if err != nil {
		phase.Err = err.Error()
	}

	c.phases = append(c.phases, phase)
	delete(c.active, kind)
	if ts.After(c.finished) {
		c.finished = ts
	}
}

func (c *Collector) UpdateMeta(kind PhaseKind, fn func(*PhaseMeta)) {
	if kind == "" || kind == PhaseTotal || fn == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.active[kind]
	if state == nil {
		return
	}
	fn(&state.meta)
}

func (c *Collector) Fail(err error) {
	if err == nil {
		return
	}
	errMsg := err.Error()
	c.mu.Lock()
	c.err = errMsg
	c.mu.Unlock()
}

func (c *Collector) Complete(ts time.Time) {
	if ts.IsZero() {
		ts = time.Now()
	}

	c.mu.Lock()
	if ts.After(c.finished) {
		c.finished = ts
	}

	for kind, state := range c.active {
		phase := Phase{
			Kind:     kind,
			Start:    state.start,
			End:      ts,
			Duration: ts.Sub(state.start),
			Meta:     state.meta,
			Err:      "incomplete",
		}
		c.phases = append(c.phases, phase)
	}
	c.active = make(map[PhaseKind]*phaseState)
	c.completed = true
	c.mu.Unlock()
}

func (c *Collector) Timeline() *Timeline {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.phases) == 0 && len(c.active) == 0 && c.started.IsZero() {
		return nil
	}

	ph := make([]Phase, len(c.phases))
	copy(ph, c.phases)
	ph = normalizePhases(ph)
	start := c.started
	if start.IsZero() && len(ph) > 0 {
		start = ph[0].Start
	}

	finish := c.finished
	if finish.IsZero() && len(ph) > 0 {
		finish = ph[len(ph)-1].End
	}

	timeline := &Timeline{
		Started:   start,
		Completed: finish,
		Err:       c.err,
		Phases:    ph,
	}

	if !timeline.Started.IsZero() && !timeline.Completed.IsZero() &&
		!timeline.Completed.Before(timeline.Started) {
		timeline.Duration = timeline.Completed.Sub(timeline.Started)
	}
	return timeline
}

func (c *Collector) Merge(other *Collector) error {
	if other == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	other.mu.Lock()
	defer other.mu.Unlock()

	if len(other.active) > 0 {
		return errors.New("cannot merge collector with active phases")
	}

	c.phases = append(c.phases, other.phases...)
	if other.started.Before(c.started) || c.started.IsZero() {
		c.started = other.started
	}

	if other.finished.After(c.finished) {
		c.finished = other.finished
	}

	if c.err == "" {
		c.err = other.err
	}
	return nil
}

func (c *Collector) SortedPhases() []Phase {
	c.mu.Lock()
	defer c.mu.Unlock()

	ph := make([]Phase, len(c.phases))
	copy(ph, c.phases)
	sort.SliceStable(ph, func(i, j int) bool {
		if ph[i].Start.Equal(ph[j].Start) {
			return ph[i].End.Before(ph[j].End)
		}
		return ph[i].Start.Before(ph[j].Start)
	})
	return ph
}
