package headless

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (e *Engine) executeProfile(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env string,
) (*engine.ProfileResult, error) {
	pl, err := core.PrepareProfile(doc, req, core.RunMeta{Env: e.env(env)})
	if err != nil {
		return nil, err
	}
	cl := newProCollector(pl)
	if err := core.RunProfile(ctx, e.rq, cl, pl); err != nil {
		return nil, err
	}
	out := buildProfileResult(cl.st)
	e.recordProfile(doc, cl.st, out, cl.last)
	return out, nil
}

type proCollector struct {
	st   *profileState
	last engine.RequestResult
}

func newProCollector(pl *core.ProfilePlan) *proCollector {
	if pl == nil {
		return &proCollector{}
	}
	return &proCollector{
		st: &profileState{
			req:   request.CloneRequest(pl.Request),
			env:   strings.TrimSpace(pl.Run.Env),
			spec:  pl.Spec,
			total: pl.Total,
			ok:    make([]time.Duration, 0, pl.Spec.Count),
			fail:  make([]engine.ProfileFailure, 0, pl.Spec.Count/2+1),
		},
	}
}

func (c *proCollector) OnEvt(_ context.Context, e core.Evt) error {
	if c == nil || c.st == nil || e == nil {
		return nil
	}
	switch v := e.(type) {
	case core.RunStart:
		c.st.start = v.Meta.At
	case core.ProIterDone:
		c.last = v.Result
		c.applyIter(v)
	case core.RunDone:
		c.st.end = v.Meta.At
		if v.Canceled {
			c.st.cancel = true
			if strings.TrimSpace(c.st.cancelMsg) == "" {
				c.st.cancelMsg = "Profiling canceled"
			}
		}
	}
	return nil
}

func (c *proCollector) applyIter(ev core.ProIterDone) {
	st := c.st
	st.idx++
	if ev.Result.Err != nil && errors.Is(ev.Result.Err, context.Canceled) {
		st.cancel = true
		st.cancelMsg = "Profiling canceled"
		return
	}
	if ev.Iter.RunIndex > 0 && st.mStart.IsZero() {
		st.mStart = ev.Meta.At
	}
	if ev.Result.Skipped {
		st.skip = true
		st.skipMsg = strings.TrimSpace(ev.Result.SkipReason)
		return
	}
	dur := time.Duration(0)
	if ev.Result.Response != nil {
		dur = ev.Result.Response.Duration
	}
	ok, outcome := profileOutcome(ev.Result)
	if !ev.Iter.Warmup {
		st.mEnd = ev.Meta.At
	}
	if ok {
		if !ev.Iter.Warmup {
			st.ok = append(st.ok, dur)
		}
		return
	}
	fail := engine.ProfileFailure{
		Iteration: ev.Iter.Index + 1,
		Warmup:    ev.Iter.Warmup,
		Reason:    outcome.reason,
		Duration:  dur,
		Failure:   outcome.failure,
	}
	if ev.Result.Response != nil {
		fail.Status = ev.Result.Response.Status
		fail.StatusCode = ev.Result.Response.StatusCode
	}
	st.fail = append(st.fail, fail)
}

func buildProfileResult(st *profileState) *engine.ProfileResult {
	stats := analysis.LatencyStats{}
	if st != nil && len(st.ok) > 0 {
		stats = analysis.ComputeLatencyStats(st.ok, analysis.DefaultProfilePercentiles(), 10)
	}
	out := &engine.ProfileResult{}
	if st == nil {
		return out
	}
	out.Environment = st.env
	out.Summary = profileSummary(st)
	out.Report = profileReport(st, stats)
	out.StartedAt = st.start
	out.EndedAt = st.end
	out.Duration = st.end.Sub(st.start)
	out.Count = st.spec.Count
	out.Warmup = st.spec.Warmup
	out.Delay = st.spec.Delay
	out.Skipped = st.skip
	out.SkipReason = st.skipMsg
	out.Canceled = st.cancel
	out.Results = buildProfileResults(st, stats)
	out.Failures = append([]engine.ProfileFailure(nil), st.fail...)
	out.Success = !out.Canceled && !out.Skipped && len(st.fail) == 0 && len(st.ok) == st.spec.Count
	return out
}

func profileSummary(st *profileState) string {
	if st == nil {
		return "Profiling complete"
	}
	if st.skip {
		if st.skipMsg == "" {
			return "Profiling skipped: condition evaluated to false"
		}
		return "Profiling skipped: " + st.skipMsg
	}
	if st.cancel {
		return fmt.Sprintf(
			"Profiling canceled after %d/%d runs (%d/%d measured)",
			st.idx,
			st.total,
			len(st.ok)+len(st.fail),
			st.spec.Count,
		)
	}
	return fmt.Sprintf(
		"Profiling complete: %d/%d success (%d failure, %d warmup)",
		len(st.ok),
		st.spec.Count,
		len(st.fail),
		min(st.idx, st.spec.Warmup),
	)
}

func profileReport(st *profileState, stats analysis.LatencyStats) string {
	if st == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Profile: %s\n", engine.ReqTitle(st.req))
	fmt.Fprintf(&b, "Started: %s\n", st.start.Format(time.RFC3339))
	fmt.Fprintf(&b, "Ended: %s\n", st.end.Format(time.RFC3339))
	fmt.Fprintf(
		&b,
		"Runs: %d total (%d warmup, %d measured)\n",
		st.idx,
		st.spec.Warmup,
		len(st.ok)+len(st.fail),
	)
	fmt.Fprintf(&b, "Success: %d\n", len(st.ok))
	fmt.Fprintf(&b, "Failures: %d\n", len(st.fail))
	if stats.Count > 0 {
		fmt.Fprintf(&b, "Latency: min=%s p50=%s p95=%s max=%s\n",
			stats.Min,
			stats.Percentiles[50],
			stats.Percentiles[95],
			stats.Max,
		)
	}
	for _, fail := range st.fail {
		fmt.Fprintf(&b, "Failure %d: %s\n", fail.Iteration, fail.Reason)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (e *Engine) recordProfile(
	doc *restfile.Document,
	st *profileState,
	out *engine.ProfileResult,
	last engine.RequestResult,
) {
	hs := e.history()
	if hs == nil || st == nil || st.req == nil || out == nil {
		return
	}
	now := time.Now()
	status := "profile completed"
	code := 0
	switch {
	case out.Skipped:
		status = "SKIPPED"
		if out.SkipReason != "" {
			status += ": " + out.SkipReason
		}
	case out.Canceled:
		status = st.cancelMsg
		if status == "" {
			status = "Profiling canceled"
		}
	case last.Response != nil:
		status = last.Response.Status
		code = last.Response.StatusCode
	case last.Err != nil:
		status = last.Err.Error()
	case len(st.fail) > 0:
		status = st.fail[len(st.fail)-1].Reason
		code = st.fail[len(st.fail)-1].StatusCode
	}
	ent := history.Entry{
		ID:          fmt.Sprintf("%d", now.UnixNano()),
		ExecutedAt:  now,
		Environment: st.env,
		RequestName: engine.ReqID(st.req),
		FilePath:    e.filePath(doc),
		Method:      st.req.Method,
		URL:         st.req.URL,
		Status:      status,
		StatusCode:  code,
		Duration:    out.Duration,
		BodySnippet: "<profile run – see profileResults>",
		RequestText: redactText(
			request.RenderRequestText(st.req),
			e.secretValues(doc, st.req, st.env),
			!st.req.Metadata.AllowSensitiveHeaders,
		),
		Description:    strings.TrimSpace(st.req.Metadata.Description),
		Tags:           engine.Tags(st.req.Metadata.Tags),
		ProfileResults: cloneProfileResults(out.Results),
	}
	_ = hs.Append(ent)
}

func cloneProfileResults(src *history.ProfileResults) *history.ProfileResults {
	if src == nil {
		return nil
	}
	out := *src
	if src.Latency != nil {
		lat := *src.Latency
		out.Latency = &lat
	}
	if len(src.Percentiles) > 0 {
		out.Percentiles = append([]history.ProfilePercentile(nil), src.Percentiles...)
	}
	if len(src.Histogram) > 0 {
		out.Histogram = append([]history.ProfileHistogramBin(nil), src.Histogram...)
	}
	return &out
}
