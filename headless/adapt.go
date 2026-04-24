package headless

import (
	"github.com/unkn0wn-root/resterm/internal/runfmt"
	"github.com/unkn0wn-root/resterm/internal/runner"
)

func reportFromRunner(rep *runner.Report) *Report {
	if rep == nil {
		return nil
	}
	return reportFromFmt(runner.NormalizeReport(rep))
}

func reportFromFmt(rep runfmt.Report) *Report {
	out := &Report{
		SchemaVersion: rep.SchemaVersion,
		Version:       rep.Version,
		FilePath:      rep.FilePath,
		EnvName:       rep.EnvName,
		StartedAt:     rep.StartedAt,
		EndedAt:       rep.EndedAt,
		Duration:      rep.Duration,
		Results:       make([]Result, 0, len(rep.Results)),
		Total:         rep.Total,
		Passed:        rep.Passed,
		Failed:        rep.Failed,
		Skipped:       rep.Skipped,
		StopReason:    StopReason(rep.StopReason),
	}
	for _, res := range rep.Results {
		out.Results = append(out.Results, resultFromFmt(res))
	}
	return out
}

func resultFromFmt(res runfmt.Result) Result {
	out := Result{
		Kind:        Kind(res.Kind),
		Name:        res.Name,
		Method:      res.Method,
		Target:      res.Target,
		Environment: res.Environment,
		Status:      Status(res.Status),
		Summary:     res.Summary,
		Duration:    res.Duration,
		Canceled:    res.Canceled,
		SkipReason:  res.SkipReason,
		Error:       res.Error,
		ScriptError: res.ScriptError,
		Failure:     failureFromFmt(res.Failure),
		HTTP:        httpFromFmt(res.HTTP),
		GRPC:        grpcFromFmt(res.GRPC),
		Stream:      streamFromFmt(res.Stream),
		Trace:       traceFromFmt(res.Trace),
		Tests:       testsFromFmt(res.Tests),
		Compare:     compareFromFmt(res.Compare),
		Profile:     profileFromFmt(res.Profile),
		Steps:       stepsFromFmt(res.Steps),
	}
	return out
}

func stepFromFmt(step runfmt.Step) Step {
	out := Step{
		Name:        step.Name,
		Method:      step.Method,
		Target:      step.Target,
		Environment: step.Environment,
		Branch:      step.Branch,
		Iteration:   step.Iteration,
		Total:       step.Total,
		Status:      Status(step.Status),
		Summary:     step.Summary,
		Duration:    step.Duration,
		Canceled:    step.Canceled,
		SkipReason:  step.SkipReason,
		Error:       step.Error,
		ScriptError: step.ScriptError,
		Failure:     failureFromFmt(step.Failure),
		HTTP:        httpFromFmt(step.HTTP),
		GRPC:        grpcFromFmt(step.GRPC),
		Stream:      streamFromFmt(step.Stream),
		Trace:       traceFromFmt(step.Trace),
		Tests:       testsFromFmt(step.Tests),
	}
	return out
}

func stepsFromFmt(src []runfmt.Step) []Step {
	if len(src) == 0 {
		return nil
	}
	out := make([]Step, 0, len(src))
	for _, step := range src {
		out = append(out, stepFromFmt(step))
	}
	return out
}

func httpFromFmt(http *runfmt.HTTP) *HTTP {
	if http == nil {
		return nil
	}
	return &HTTP{
		Status:     http.Status,
		StatusCode: http.StatusCode,
		Protocol:   http.Protocol,
	}
}

func grpcFromFmt(grpc *runfmt.GRPC) *GRPC {
	if grpc == nil {
		return nil
	}
	return &GRPC{
		Code:          grpc.Code,
		StatusCode:    grpc.StatusCode,
		StatusMessage: grpc.StatusMessage,
	}
}

func testsFromFmt(src []runfmt.Test) []Test {
	if len(src) == 0 {
		return nil
	}
	out := make([]Test, 0, len(src))
	for _, test := range src {
		out = append(out, Test{
			Name:    test.Name,
			Message: test.Message,
			Passed:  test.Passed,
			Elapsed: test.Elapsed,
		})
	}
	return out
}

func compareFromFmt(cmp *runfmt.Compare) *Compare {
	if cmp == nil {
		return nil
	}
	return &Compare{Baseline: cmp.Baseline}
}

func profileFromFmt(prof *runfmt.Profile) *Profile {
	if prof == nil {
		return nil
	}
	out := &Profile{
		Count:          prof.Count,
		Warmup:         prof.Warmup,
		Delay:          prof.Delay,
		TotalRuns:      prof.TotalRuns,
		WarmupRuns:     prof.WarmupRuns,
		SuccessfulRuns: prof.SuccessfulRuns,
		FailedRuns:     prof.FailedRuns,
		Latency:        latencyFromFmt(prof.Latency),
	}
	if len(prof.Percentiles) > 0 {
		out.Percentiles = make([]Percentile, 0, len(prof.Percentiles))
		for _, pct := range prof.Percentiles {
			out.Percentiles = append(out.Percentiles, Percentile{
				Percentile: pct.Percentile,
				Value:      pct.Value,
			})
		}
	}
	if len(prof.Histogram) > 0 {
		out.Histogram = make([]HistBin, 0, len(prof.Histogram))
		for _, bin := range prof.Histogram {
			out.Histogram = append(out.Histogram, HistBin{
				From:  bin.From,
				To:    bin.To,
				Count: bin.Count,
			})
		}
	}
	if len(prof.Failures) > 0 {
		out.Failures = make([]ProfileFailure, 0, len(prof.Failures))
		for _, failure := range prof.Failures {
			out.Failures = append(out.Failures, ProfileFailure{
				Iteration:  failure.Iteration,
				Warmup:     failure.Warmup,
				Reason:     failure.Reason,
				Status:     failure.Status,
				StatusCode: failure.StatusCode,
				Duration:   failure.Duration,
				Failure:    failureFromFmt(failure.Failure),
			})
		}
	}
	return out
}

func failureFromFmt(f *runfmt.Failure) *Failure {
	if f == nil {
		return nil
	}
	return &Failure{
		Code:     FailureCode(f.Code),
		Category: FailureCategory(f.Category),
		ExitCode: f.ExitCode,
		Message:  f.Message,
		Source:   f.Source,
	}
}

func latencyFromFmt(lat *runfmt.Latency) *Latency {
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

func streamFromFmt(stream *runfmt.Stream) *Stream {
	if stream == nil {
		return nil
	}
	out := &Stream{
		Kind:           stream.Kind,
		EventCount:     stream.EventCount,
		TranscriptPath: stream.TranscriptPath,
		// NormalizeReport already deep-clones stream summary maps.
		Summary: stream.Summary,
	}
	return out
}

func traceFromFmt(trace *runfmt.Trace) *Trace {
	if trace == nil {
		return nil
	}
	out := &Trace{
		Duration:     trace.Duration,
		Error:        trace.Error,
		ArtifactPath: trace.ArtifactPath,
	}
	if bud := trace.Budget; bud != nil {
		out.Budget = &TraceBudget{
			Total:     bud.Total,
			Tolerance: bud.Tolerance,
			Phases:    bud.Phases,
		}
	}
	if len(trace.Breaches) > 0 {
		out.Breaches = make([]TraceBreach, 0, len(trace.Breaches))
		for _, breach := range trace.Breaches {
			out.Breaches = append(out.Breaches, TraceBreach{
				Kind:   breach.Kind,
				Limit:  breach.Limit,
				Actual: breach.Actual,
				Over:   breach.Over,
			})
		}
	}
	return out
}
