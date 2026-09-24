package headless

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// History is recorded even when the run stops on an error, so the partial
// result is not lost.
func (e *Engine) executeProfile(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
) (*engine.ProfileResult, error) {
	pl, err := core.PrepareProfile(doc, req, core.RunMeta{Env: env})
	if err != nil {
		return nil, err
	}
	acc := core.NewProfileAccumulator(pl)
	err = core.RunProfile(ctx, e.rq, acc, pl)
	snap := acc.Snapshot()
	secs := e.secretValues(doc, req, env)
	ent := snap.HistoryEntry(req, env, e.filePath(doc), func(text string, mask bool) string {
		return redactText(text, secs, mask)
	})
	if hs := e.history(); hs != nil {
		_ = hs.Append(ent)
	}
	if err != nil {
		return nil, err
	}
	return profileResult(pl, snap, ent.ProfileResults), nil
}

func profileResult(
	pl *core.ProfilePlan,
	snap core.ProfileSnapshot,
	res *history.ProfileResults,
) *engine.ProfileResult {
	st := snap.Progress.Status
	return &engine.ProfileResult{
		Environment: pl.Run.Env.Label(),
		Selection:   pl.Run.Env.Selection(),
		Summary:     snap.Summary(),
		Report:      profileReport(engine.ReqTitle(pl.Request), snap),
		StartedAt:   snap.Started,
		EndedAt:     snap.Ended,
		Duration:    snap.Ended.Sub(snap.Started),
		Count:       pl.Spec.Count,
		Warmup:      pl.Spec.Warmup,
		Delay:       pl.Spec.Delay,
		Success:     st == core.ProfilePass,
		Skipped:     st == core.ProfileSkipped,
		SkipReason:  snap.SkipReason,
		Canceled:    st == core.ProfileCanceled,
		Results:     res,
		Failures:    snap.Failures,
	}
}

func profileReport(title string, snap core.ProfileSnapshot) string {
	p := snap.Progress
	var b strings.Builder
	fmt.Fprintf(&b, "Profile: %s\n", title)
	fmt.Fprintf(&b, "Started: %s\n", snap.Started.Format(time.RFC3339))
	fmt.Fprintf(&b, "Ended: %s\n", snap.Ended.Format(time.RFC3339))
	fmt.Fprintf(&b, "Runs: %d total (%d warmup, %d measured)\n", p.Done, p.WarmupDone, p.Measured)
	fmt.Fprintf(&b, "Success: %d\n", p.Passed)
	fmt.Fprintf(&b, "Failures: %d\n", p.Failed)
	if st := snap.Stats; st.Count > 0 {
		fmt.Fprintf(
			&b,
			"Latency: min=%s p50=%s p95=%s max=%s\n",
			st.Min,
			st.Percentiles[50],
			st.Percentiles[95],
			st.Max,
		)
	}
	for _, f := range snap.Failures {
		label := "Failure"
		if f.Warmup {
			label = "Warmup failure"
		}
		fmt.Fprintf(&b, "%s %d: %s\n", label, f.Iteration, f.Reason)
	}
	return strings.TrimRight(b.String(), "\n")
}
