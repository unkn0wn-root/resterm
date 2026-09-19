package core

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine"
	runfail "github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

const (
	profileSource = "profile"
	profileBins   = 10
)

type ProfileStatus uint8

const (
	ProfileRunning ProfileStatus = iota
	ProfilePass
	ProfileFail
	ProfileCanceled
	ProfileSkipped
	ProfileError
)

var profileStatusNames = [...]string{"running", "pass", "fail", "canceled", "skipped", "error"}

func (s ProfileStatus) String() string {
	if int(s) < len(profileStatusNames) {
		return profileStatusNames[s]
	}
	return "unknown"
}

func ParseProfileStatus(name string) (ProfileStatus, bool) {
	i := slices.Index(profileStatusNames[:], name)
	return ProfileStatus(max(i, 0)), i >= 0
}

type ProfileProgress struct {
	Status       ProfileStatus
	Total        int
	Done         int
	Warmup       int
	WarmupDone   int
	WarmupFailed int
	Count        int
	Measured     int
	Passed       int
	Failed       int
	Elapsed      time.Duration
}

type ProfileSnapshot struct {
	Progress ProfileProgress
	Delay    time.Duration
	Started  time.Time
	Ended    time.Time
	// Window is the time from the start of the first measured request to the end of the last.
	Window time.Duration
	// Active is the total time spent inside measured requests.
	Active time.Duration
	// Stats includes successful measured requests and is set when the run ends.
	Stats      analysis.LatencyStats
	Failures   []engine.ProfileFailure
	SkipReason string
	Err        error
	// Legacy identifies history records saved before failure details were kept.
	Legacy bool
}

func (s ProfileSnapshot) Summary() string {
	p := s.Progress
	switch p.Status {
	case ProfileRunning:
		return fmt.Sprintf("Profiling %d/%d runs", p.Done, p.Total)
	case ProfileSkipped:
		return "Profiling skipped: " + cmp.Or(s.SkipReason, "condition evaluated to false")
	case ProfileCanceled:
		return fmt.Sprintf(
			"Profiling canceled after %d/%d runs (%d/%d measured)",
			p.Done, p.Total, p.Measured, p.Count,
		)
	case ProfileError:
		return fmt.Sprintf("Profiling stopped after %d/%d runs: %v", p.Done, p.Total, s.Err)
	default:
		return fmt.Sprintf(
			"Profiling complete: %d/%d success (%d failure, %d warmup)",
			p.Passed, p.Count, p.Failed, p.WarmupDone,
		)
	}
}

func (s ProfileSnapshot) WallRate() float64 {
	return perSecond(s.Progress.Measured, s.Window)
}

func (s ProfileSnapshot) ActiveRate() float64 {
	return perSecond(s.Progress.Measured, s.Active)
}

func perSecond(n int, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(n) / d.Seconds()
}

// ProfileAccumulator builds snapshots from profile events and their timestamps.
type ProfileAccumulator struct {
	prog     ProfileProgress
	delay    time.Duration
	started  time.Time
	ended    time.Time
	from     time.Time
	to       time.Time
	active   time.Duration
	ok       []time.Duration
	fails    []engine.ProfileFailure
	canceled bool
	skipped  bool
	skipMsg  string
	err      error
	stats    analysis.LatencyStats
}

func NewProfileAccumulator(pl *ProfilePlan) *ProfileAccumulator {
	return &ProfileAccumulator{
		prog: ProfileProgress{
			Total:  pl.Total,
			Warmup: pl.Spec.Warmup,
			Count:  pl.Spec.Count,
		},
		delay: pl.Spec.Delay,
		ok:    make([]time.Duration, 0, pl.Spec.Count),
	}
}

func (a *ProfileAccumulator) OnEvt(_ context.Context, e Evt) error {
	a.Apply(e)
	return nil
}

func (a *ProfileAccumulator) Apply(e Evt) {
	at := MetaOf(e).At
	switch v := e.(type) {
	case RunStart:
		a.started = at
	case ProIterStart:
		if !v.Iter.Warmup && a.from.IsZero() {
			a.from = at
		}
	case ProIterDone:
		a.addIter(v)
	case RunDone:
		a.finish(v)
	}
	if !a.started.IsZero() {
		a.prog.Elapsed = at.Sub(a.started)
	}
}

func (a *ProfileAccumulator) Progress() ProfileProgress {
	return a.prog
}

func (a *ProfileAccumulator) Snapshot() ProfileSnapshot {
	s := ProfileSnapshot{
		Progress:   a.prog,
		Delay:      a.delay,
		Started:    a.started,
		Ended:      a.ended,
		Active:     a.active,
		Stats:      a.stats,
		Failures:   slices.Clone(a.fails),
		SkipReason: a.skipMsg,
		Err:        a.err,
	}
	s.Stats.Percentiles = maps.Clone(a.stats.Percentiles)
	s.Stats.Histogram = slices.Clone(a.stats.Histogram)
	if !a.from.IsZero() && !a.to.IsZero() {
		s.Window = a.to.Sub(a.from)
	}
	return s
}

// A canceled or skipped iteration never finished, so it is not counted.
func (a *ProfileAccumulator) addIter(e ProIterDone) {
	res := e.Result
	switch {
	case errors.Is(res.Err, context.Canceled):
		a.canceled = true
		return
	case res.Skipped:
		a.skipped = true
		a.skipMsg = strings.TrimSpace(res.SkipReason)
		return
	}

	a.prog.Done++
	f, failed := iterFailure(e.Iter, res)
	if failed {
		a.fails = append(a.fails, f)
	}
	if e.Iter.Warmup {
		a.prog.WarmupDone++
		if failed {
			a.prog.WarmupFailed++
		}
		return
	}

	dur := time.Duration(0)
	if res.Response != nil {
		dur = res.Response.Duration
	}
	a.prog.Measured++
	a.to = e.Meta.At
	a.active += dur
	if failed {
		a.prog.Failed++
		return
	}
	a.prog.Passed++
	a.ok = append(a.ok, dur)
}

// RunDone is the only event that records cancellation between iterations.
func (a *ProfileAccumulator) finish(e RunDone) {
	a.ended = e.Meta.At
	a.err = e.Err
	a.canceled = a.canceled || e.Canceled
	a.prog.Status = a.status()
	a.stats = analysis.ComputeLatencyStats(a.ok, analysis.DefaultProfilePercentiles(), profileBins)
}

func (a *ProfileAccumulator) status() ProfileStatus {
	switch {
	case a.err != nil:
		return ProfileError
	case a.canceled:
		return ProfileCanceled
	case a.skipped:
		return ProfileSkipped
	case a.prog.Failed > 0 || a.prog.Passed < a.prog.Count:
		return ProfileFail
	default:
		return ProfilePass
	}
}

func iterFailure(it IterMeta, res engine.RequestResult) (engine.ProfileFailure, bool) {
	f := engine.ProfileFailure{Iteration: it.Index + 1, Warmup: it.Warmup}
	if r := res.Response; r != nil {
		f.Status, f.StatusCode, f.Duration = r.Status, r.StatusCode, r.Duration
	}
	test := slices.IndexFunc(res.Tests, func(t scripts.TestResult) bool { return !t.Passed })
	switch {
	case res.Err != nil:
		f.Failure, f.Err = runfail.FromErrorSource(res.Err, profileSource), res.Err
	case f.StatusCode >= 400:
		f.Failure = runfail.Assertion(httpReason(f.Status, f.StatusCode), profileSource)
	case res.ScriptErr != nil:
		f.Failure, f.Err = runfail.Script(res.ScriptErr.Error(), profileSource), res.ScriptErr
	case test >= 0:
		t := res.Tests[test]
		f.Failure = runfail.Assertion(testReason(t.Name, t.Message), profileSource)
	case res.Response == nil:
		f.Failure = runfail.New(runfail.CodeProtocol, "no response", profileSource)
	default:
		return engine.ProfileFailure{}, false
	}
	f.Reason = f.Failure.Message
	return f, true
}

func httpReason(status string, code int) string {
	switch {
	case status != "":
		return "HTTP " + status
	case code != 0:
		return fmt.Sprintf("HTTP %d", code)
	default:
		return "HTTP request failed"
	}
}

func testReason(name, msg string) string {
	switch {
	case name != "" && msg != "":
		return fmt.Sprintf("Test failed: %s - %s", name, msg)
	case msg != "":
		return "Test failed: " + msg
	case name != "":
		return "Test failed: " + name
	default:
		return "Test failed"
	}
}
