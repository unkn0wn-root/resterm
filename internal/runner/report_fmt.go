package runner

import (
	"io"
	"sort"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/runx/report"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

func (r *Report) WriteJSON(w io.Writer) error {
	if w == nil {
		return ErrNilWriter
	}
	rep := ReportModel(r)
	return runfmt.WriteJSON(w, &rep)
}

func (r *Report) WriteJUnit(w io.Writer) error {
	if w == nil {
		return ErrNilWriter
	}
	rep := ReportModel(r)
	return runfmt.WriteJUnit(w, &rep)
}

func ExitCode(rep *Report, mode runfail.ExitMode) int {
	fmtRep := ReportModel(rep)
	return fmtRep.ExitCode(mode)
}

// ReportModel converts a runner report into the canonical runfmt model.
func ReportModel(rep *Report) runfmt.Report {
	if rep == nil {
		return runfmt.Report{}
	}
	out := runfmt.Report{
		SchemaVersion: str.FirstTrimmed(rep.SchemaVersion, runfmt.ReportSchemaVersion),
		Version:       str.Trim(rep.Version),
		FilePath:      rep.FilePath,
		EnvName:       str.Trim(rep.EnvName),
		StartedAt:     rep.StartedAt,
		EndedAt:       rep.EndedAt,
		Duration:      rep.Duration,
		Results:       make([]runfmt.Result, 0, len(rep.Results)),
		Total:         rep.Total,
		Passed:        rep.Passed,
		Failed:        rep.Failed,
		Skipped:       rep.Skipped,
		StopReason:    rep.StopReason,
	}
	for _, res := range rep.Results {
		out.Results = append(out.Results, toFormatResult(res))
	}
	return out
}

func toFormatResult(res Result) runfmt.Result {
	out := runfmt.Result{
		Kind:            string(res.Kind),
		Name:            str.Trim(res.Name),
		Method:          str.Trim(res.Method),
		Target:          str.Trim(res.Target),
		EffectiveTarget: str.Trim(res.EffectiveTarget),
		Environment:     str.Trim(res.Environment),
		Status:          resultStatusOf(res),
		Summary:         str.Trim(res.Summary),
		Duration:        resultDuration(res),
		Canceled:        res.Canceled,
		SkipReason:      str.Trim(res.SkipReason),
		Error:           errText(res.Err),
		ScriptError:     errText(res.ScriptErr),
		Failure:         formatResultFailure(res),
		HTTP:            formatHTTP(res.Response),
		GRPC:            formatGRPC(res.GRPC),
		Stream:          formatStream(res.Stream),
		Trace:           formatTrace(res.Trace),
		Tests:           formatTests(res.Tests),
		Compare:         formatCompare(res.Compare),
		Profile:         formatProfile(res.Profile),
		Steps:           formatSteps(res.Steps),
	}
	return out
}

func toFormatStep(step StepResult) runfmt.Step {
	out := runfmt.Step{
		Name:            str.Trim(step.Name),
		Method:          str.Trim(step.Method),
		Target:          str.Trim(step.Target),
		EffectiveTarget: str.Trim(step.EffectiveTarget),
		Environment:     str.Trim(step.Environment),
		Branch:          str.Trim(step.Branch),
		Iteration:       step.Iteration,
		Total:           step.Total,
		Status:          stepStatusOf(step),
		Summary:         str.Trim(step.Summary),
		Duration:        step.Duration,
		Canceled:        step.Canceled,
		SkipReason:      str.Trim(step.SkipReason),
		Error:           errText(step.Err),
		ScriptError:     errText(step.ScriptErr),
		Failure:         formatStepFailure(step),
		HTTP:            formatHTTP(step.Response),
		GRPC:            formatGRPC(step.GRPC),
		Stream:          formatStream(step.Stream),
		Trace:           formatTrace(step.Trace),
		Tests:           formatTests(step.Tests),
	}
	return out
}

func formatSteps(src []StepResult) []runfmt.Step {
	if len(src) == 0 {
		return nil
	}
	out := make([]runfmt.Step, 0, len(src))
	for _, step := range src {
		out = append(out, toFormatStep(step))
	}
	return out
}

func resultStatusOf(res Result) runfmt.Status {
	if res.Skipped {
		return runfmt.StatusSkip
	}
	if resultFailed(res) {
		return runfmt.StatusFail
	}
	return runfmt.StatusPass
}

func stepStatusOf(step StepResult) runfmt.Status {
	if step.Skipped {
		return runfmt.StatusSkip
	}
	if stepFailed(step) {
		return runfmt.StatusFail
	}
	return runfmt.StatusPass
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func formatHTTP(resp *httpclient.Response) *runfmt.HTTP {
	if resp == nil {
		return nil
	}
	return &runfmt.HTTP{
		Status:     str.Trim(resp.Status),
		StatusCode: resp.StatusCode,
		Protocol:   str.Trim(resp.Proto),
	}
}

func formatGRPC(resp *grpcclient.Response) *runfmt.GRPC {
	if resp == nil {
		return nil
	}
	return &runfmt.GRPC{
		Code:          resp.StatusCode.String(),
		StatusCode:    int(resp.StatusCode),
		StatusMessage: str.Trim(resp.StatusMessage),
	}
}

func formatTests(src []scripts.TestResult) []runfmt.Test {
	if len(src) == 0 {
		return nil
	}
	out := make([]runfmt.Test, 0, len(src))
	for _, test := range src {
		out = append(out, runfmt.Test{
			Name:    str.Trim(test.Name),
			Message: str.Trim(test.Message),
			Passed:  test.Passed,
			Elapsed: test.Elapsed,
		})
	}
	return out
}

func formatCompare(info *CompareInfo) *runfmt.Compare {
	if info == nil {
		return nil
	}
	return &runfmt.Compare{Baseline: str.Trim(info.Baseline)}
}

func formatProfile(prof *ProfileInfo) *runfmt.Profile {
	if prof == nil {
		return nil
	}
	out := &runfmt.Profile{
		Count:  prof.Count,
		Warmup: prof.Warmup,
		Delay:  prof.Delay,
	}
	if prof.Results != nil {
		out.TotalRuns = prof.Results.TotalRuns
		out.WarmupRuns = prof.Results.WarmupRuns
		out.SuccessfulRuns = prof.Results.SuccessfulRuns
		out.FailedRuns = prof.Results.FailedRuns
		out.Latency = formatLatency(prof.Results.Latency)
		out.Percentiles = formatPercentiles(prof.Results.Percentiles)
		out.Histogram = formatHistogram(prof.Results.Histogram)
	}
	if len(prof.Failures) > 0 {
		out.Failures = make([]runfmt.ProfileFailure, 0, len(prof.Failures))
		for _, fail := range prof.Failures {
			out.Failures = append(out.Failures, runfmt.ProfileFailure{
				Iteration:  fail.Iteration,
				Warmup:     fail.Warmup,
				Reason:     str.Trim(fail.Reason),
				Status:     str.Trim(fail.Status),
				StatusCode: fail.StatusCode,
				Duration:   fail.Duration,
				Failure:    formatProfileFailure(fail),
			})
		}
	}
	return out
}

func formatResultFailure(res Result) *runfmt.Failure {
	return formatRunFailure(resultFailure(res))
}

func formatStepFailure(step StepResult) *runfmt.Failure {
	return formatRunFailure(stepFailure(step))
}

func formatProfileFailure(fail ProfileFailure) *runfmt.Failure {
	return formatRunFailure(fail.Failure)
}

func formatRunFailure(failure runfail.Failure) *runfmt.Failure {
	if failure.Code == "" {
		return nil
	}
	return runfmt.FromFailure(runfail.New(
		failure.Code,
		str.Trim(failure.Message),
		str.Trim(failure.Source),
	))
}

func formatLatency(lat *history.ProfileLatency) *runfmt.Latency {
	if lat == nil {
		return nil
	}
	return &runfmt.Latency{
		Count:  lat.Count,
		Min:    lat.Min,
		Max:    lat.Max,
		Mean:   lat.Mean,
		Median: lat.Median,
		StdDev: lat.StdDev,
	}
}

func formatPercentiles(src []history.ProfilePercentile) []runfmt.Percentile {
	if len(src) == 0 {
		return nil
	}
	items := append([]history.ProfilePercentile(nil), src...)
	sort.Slice(items, func(i, j int) bool { return items[i].Percentile < items[j].Percentile })
	out := make([]runfmt.Percentile, 0, len(items))
	for _, item := range items {
		out = append(out, runfmt.Percentile{
			Percentile: item.Percentile,
			Value:      item.Value,
		})
	}
	return out
}

func formatHistogram(src []history.ProfileHistogramBin) []runfmt.HistBin {
	if len(src) == 0 {
		return nil
	}
	out := make([]runfmt.HistBin, 0, len(src))
	for _, item := range src {
		out = append(out, runfmt.HistBin{
			From:  item.From,
			To:    item.To,
			Count: item.Count,
		})
	}
	return out
}

func formatStream(info *StreamInfo) *runfmt.Stream {
	if info == nil {
		return nil
	}
	out := &runfmt.Stream{
		Kind:           str.Trim(info.Kind),
		EventCount:     info.EventCount,
		TranscriptPath: str.Trim(info.TranscriptPath),
	}
	if len(info.Summary) > 0 {
		out.Summary = runfmt.CloneAnyMap(info.Summary)
	}
	return out
}

func formatTrace(info *TraceInfo) *runfmt.Trace {
	if info == nil || info.Summary == nil {
		return nil
	}
	out := &runfmt.Trace{
		Duration:     info.Summary.Duration,
		Error:        str.Trim(info.Summary.Error),
		ArtifactPath: str.Trim(info.ArtifactPath),
	}
	if bud := info.Summary.Budgets; bud != nil {
		out.Budget = &runfmt.TraceBudget{
			Total:     bud.Total,
			Tolerance: bud.Tolerance,
			Phases:    runfmt.CloneDurationMap(bud.Phases),
		}
	}
	if len(info.Summary.Breaches) > 0 {
		out.Breaches = make([]runfmt.TraceBreach, 0, len(info.Summary.Breaches))
		for _, breach := range info.Summary.Breaches {
			out.Breaches = append(out.Breaches, runfmt.TraceBreach{
				Kind:   str.Trim(breach.Kind),
				Limit:  breach.Limit,
				Actual: breach.Actual,
				Over:   breach.Over,
			})
		}
	}
	return out
}
