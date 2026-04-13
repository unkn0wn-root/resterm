package headless

import (
	"maps"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func reportFromRunner(rep *runner.Report) *Report {
	if rep == nil {
		return nil
	}
	out := &Report{
		Version:   strings.TrimSpace(rep.Version),
		FilePath:  rep.FilePath,
		EnvName:   strings.TrimSpace(rep.EnvName),
		StartedAt: rep.StartedAt,
		EndedAt:   rep.EndedAt,
		Duration:  rep.Duration,
		Results:   make([]Result, 0, len(rep.Results)),
		Total:     rep.Total,
		Passed:    rep.Passed,
		Failed:    rep.Failed,
		Skipped:   rep.Skipped,
	}
	for _, item := range rep.Results {
		out.Results = append(out.Results, resultFromRunner(item))
	}
	return out
}

func resultFromRunner(item runner.Result) Result {
	out := Result{
		Kind:        Kind(item.Kind),
		Name:        strings.TrimSpace(item.Name),
		Method:      strings.TrimSpace(item.Method),
		Target:      strings.TrimSpace(item.Target),
		Environment: strings.TrimSpace(item.Environment),
		Summary:     strings.TrimSpace(item.Summary),
		Duration:    resultDur(item),
		Canceled:    item.Canceled,
		SkipReason:  strings.TrimSpace(item.SkipReason),
		Error:       errText(item.Err),
		ScriptError: errText(item.ScriptErr),
		HTTP:        httpFromRunner(item.Response),
		GRPC:        grpcFromRunner(item.GRPC),
		Stream:      streamFromRunner(item.Stream),
		Trace:       traceFromRunner(item.Trace),
		Tests:       testsFromRunner(item.Tests),
		Compare:     compareFromRunner(item.Compare),
		Profile:     profileFromRunner(item.Profile),
		Steps:       stepsFromRunner(item.Steps),
	}
	out.Status = resultStatusOf(item)
	return out
}

func stepFromRunner(step runner.StepResult) Step {
	out := Step{
		Name:        strings.TrimSpace(step.Name),
		Method:      strings.TrimSpace(step.Method),
		Target:      strings.TrimSpace(step.Target),
		Environment: strings.TrimSpace(step.Environment),
		Branch:      strings.TrimSpace(step.Branch),
		Iteration:   step.Iteration,
		Total:       step.Total,
		Summary:     strings.TrimSpace(step.Summary),
		Duration:    step.Duration,
		Canceled:    step.Canceled,
		SkipReason:  strings.TrimSpace(step.SkipReason),
		Error:       errText(step.Err),
		ScriptError: errText(step.ScriptErr),
		HTTP:        httpFromRunner(step.Response),
		GRPC:        grpcFromRunner(step.GRPC),
		Stream:      streamFromRunner(step.Stream),
		Trace:       traceFromRunner(step.Trace),
		Tests:       testsFromRunner(step.Tests),
	}
	out.Status = stepStatusOf(step)
	return out
}

func stepsFromRunner(src []runner.StepResult) []Step {
	if len(src) == 0 {
		return nil
	}
	out := make([]Step, 0, len(src))
	for _, step := range src {
		out = append(out, stepFromRunner(step))
	}
	return out
}

func resultStatusOf(item runner.Result) Status {
	if item.Skipped {
		return StatusSkip
	}
	if item.Canceled || item.Err != nil || item.ScriptErr != nil || runnerTraceFailed(item.Trace) {
		return StatusFail
	}
	for _, test := range item.Tests {
		if !test.Passed {
			return StatusFail
		}
	}
	if item.Passed {
		return StatusPass
	}
	return StatusFail
}

func stepStatusOf(step runner.StepResult) Status {
	if step.Skipped {
		return StatusSkip
	}
	if step.Canceled || step.Err != nil || step.ScriptErr != nil || runnerTraceFailed(step.Trace) {
		return StatusFail
	}
	for _, test := range step.Tests {
		if !test.Passed {
			return StatusFail
		}
	}
	if step.Passed {
		return StatusPass
	}
	return StatusFail
}

func runnerTraceFailed(info *runner.TraceInfo) bool {
	return info != nil && info.Summary != nil && len(info.Summary.Breaches) > 0
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func resultDur(item runner.Result) time.Duration {
	if item.Duration > 0 {
		return item.Duration
	}
	switch {
	case item.Response != nil:
		return item.Response.Duration
	case item.GRPC != nil:
		return item.GRPC.Duration
	default:
		return 0
	}
}

func httpFromRunner(resp *httpclient.Response) *HTTP {
	if resp == nil {
		return nil
	}
	return &HTTP{
		Status:     strings.TrimSpace(resp.Status),
		StatusCode: resp.StatusCode,
		Protocol:   strings.TrimSpace(resp.Proto),
	}
}

func grpcFromRunner(resp *grpcclient.Response) *GRPC {
	if resp == nil {
		return nil
	}
	return &GRPC{
		Code:          resp.StatusCode.String(),
		StatusCode:    int(resp.StatusCode),
		StatusMessage: strings.TrimSpace(resp.StatusMessage),
	}
}

func testsFromRunner(src []scripts.TestResult) []Test {
	if len(src) == 0 {
		return nil
	}
	out := make([]Test, 0, len(src))
	for _, test := range src {
		out = append(out, Test{
			Name:    strings.TrimSpace(test.Name),
			Message: strings.TrimSpace(test.Message),
			Passed:  test.Passed,
			Elapsed: test.Elapsed,
		})
	}
	return out
}

func compareFromRunner(info *runner.CompareInfo) *Compare {
	if info == nil {
		return nil
	}
	return &Compare{Baseline: strings.TrimSpace(info.Baseline)}
}

func profileFromRunner(prof *runner.ProfileInfo) *Profile {
	if prof == nil {
		return nil
	}
	out := &Profile{
		Count:  prof.Count,
		Warmup: prof.Warmup,
		Delay:  prof.Delay,
	}
	if prof.Results != nil {
		out.TotalRuns = prof.Results.TotalRuns
		out.WarmupRuns = prof.Results.WarmupRuns
		out.SuccessfulRuns = prof.Results.SuccessfulRuns
		out.FailedRuns = prof.Results.FailedRuns
		out.Latency = latencyFromHistory(prof.Results.Latency)
		out.Percentiles = percentilesFromHistory(prof.Results.Percentiles)
		out.Histogram = histFromHistory(prof.Results.Histogram)
	}
	if len(prof.Failures) > 0 {
		out.Failures = make([]ProfileFail, 0, len(prof.Failures))
		for _, failure := range prof.Failures {
			out.Failures = append(out.Failures, ProfileFail{
				Iteration:  failure.Iteration,
				Warmup:     failure.Warmup,
				Reason:     strings.TrimSpace(failure.Reason),
				Status:     strings.TrimSpace(failure.Status),
				StatusCode: failure.StatusCode,
				Duration:   failure.Duration,
			})
		}
	}
	return out
}

func latencyFromHistory(lat *history.ProfileLatency) *Latency {
	if lat == nil {
		return nil
	}
	return &Latency{
		Count:  lat.Count,
		Min:    lat.Min,
		Max:    lat.Max,
		Mean:   lat.Mean,
		Median: lat.Median,
		StdDev: lat.StdDev,
	}
}

func percentilesFromHistory(src []history.ProfilePercentile) []Percentile {
	if len(src) == 0 {
		return nil
	}
	out := make([]Percentile, 0, len(src))
	for _, item := range src {
		out = append(out, Percentile{
			Percentile: item.Percentile,
			Value:      item.Value,
		})
	}
	return out
}

func histFromHistory(src []history.ProfileHistogramBin) []HistBin {
	if len(src) == 0 {
		return nil
	}
	out := make([]HistBin, 0, len(src))
	for _, item := range src {
		out = append(out, HistBin{
			From:  item.From,
			To:    item.To,
			Count: item.Count,
		})
	}
	return out
}

func streamFromRunner(info *runner.StreamInfo) *Stream {
	if info == nil {
		return nil
	}
	out := &Stream{
		Kind:           strings.TrimSpace(info.Kind),
		EventCount:     info.EventCount,
		TranscriptPath: strings.TrimSpace(info.TranscriptPath),
	}
	if len(info.Summary) > 0 {
		out.Summary = cloneAnyMap(info.Summary)
	}
	return out
}

func traceFromRunner(info *runner.TraceInfo) *Trace {
	if info == nil || info.Summary == nil {
		return nil
	}
	out := &Trace{
		Duration:     info.Summary.Duration,
		Error:        strings.TrimSpace(info.Summary.Error),
		ArtifactPath: strings.TrimSpace(info.ArtifactPath),
	}
	if bud := info.Summary.Budgets; bud != nil {
		out.Budget = &TraceBudget{
			Total:     bud.Total,
			Tolerance: bud.Tolerance,
			Phases:    cloneDurMap(bud.Phases),
		}
	}
	if len(info.Summary.Breaches) > 0 {
		out.Breaches = make([]TraceBreach, 0, len(info.Summary.Breaches))
		for _, breach := range info.Summary.Breaches {
			out.Breaches = append(out.Breaches, TraceBreach{
				Kind:   strings.TrimSpace(breach.Kind),
				Limit:  breach.Limit,
				Actual: breach.Actual,
				Over:   breach.Over,
			})
		}
	}
	return out
}

func cloneDurMap(src map[string]time.Duration) map[string]time.Duration {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]time.Duration, len(src))
	maps.Copy(out, src)
	return out
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return cloneAnyMap(x)
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, cloneAny(item))
		}
		return out
	default:
		return x
	}
}
