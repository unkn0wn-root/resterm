package runfmt

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/unkn0wn-root/resterm/internal/runx/fail"
)

func (rep *Report) UnmarshalJSON(b []byte) error { return decode(b, rep, jsonReport.model) }

func (res *Result) UnmarshalJSON(b []byte) error { return decode(b, res, jsonResult.model) }

func (step *Step) UnmarshalJSON(b []byte) error { return decode(b, step, jsonStep.model) }

func (failure *Failure) UnmarshalJSON(b []byte) error { return decode(b, failure, jsonFailure.model) }

func (frame *FailureFrame) UnmarshalJSON(b []byte) error {
	return decode(b, frame, jsonFailureFrame.model)
}

func (test *Test) UnmarshalJSON(b []byte) error { return decode(b, test, jsonTest.model) }

func (prof *Profile) UnmarshalJSON(b []byte) error { return decode(b, prof, jsonProfile.model) }

func (lat *Latency) UnmarshalJSON(b []byte) error { return decode(b, lat, jsonLatency.model) }

func (pct *Percentile) UnmarshalJSON(b []byte) error { return decode(b, pct, jsonPercentile.model) }

func (bin *HistBin) UnmarshalJSON(b []byte) error { return decode(b, bin, jsonHistBin.model) }

func (fail *ProfileFailure) UnmarshalJSON(b []byte) error {
	return decode(b, fail, jsonProfileFailure.model)
}

func (stream *Stream) UnmarshalJSON(b []byte) error { return decode(b, stream, jsonStream.model) }

func (trace *Trace) UnmarshalJSON(b []byte) error { return decode(b, trace, jsonTrace.model) }

func (bud *TraceBudget) UnmarshalJSON(b []byte) error { return decode(b, bud, jsonTraceBudget.model) }

func (breach *TraceBreach) UnmarshalJSON(b []byte) error {
	return decode(b, breach, jsonTraceBreach.model)
}

func (j jsonReport) model() Report {
	return Report{
		SchemaVersion:        j.SchemaVersion,
		Version:              j.Version,
		FilePath:             j.FilePath,
		EnvName:              j.EnvName,
		EnvironmentSelection: j.EnvironmentSelection,
		StartedAt:            j.StartedAt,
		EndedAt:              j.EndedAt,
		Duration:             msDur(j.DurationMs),
		Results:              convert(j.Results, jsonResult.model),
		Total:                j.Summary.Total,
		Passed:               j.Summary.Passed,
		Failed:               j.Summary.Failed,
		Skipped:              j.Summary.Skipped,
		StopReason:           j.Summary.StopReason,
		Warnings:             j.Warnings,
	}
}

func (j jsonResult) model() Result {
	return Result{
		Kind:                 j.Kind,
		Name:                 j.Name,
		Method:               j.Method,
		Target:               j.Target,
		EffectiveTarget:      j.EffectiveTarget,
		Environment:          j.Environment,
		EnvironmentSelection: j.EnvironmentSelection,
		Status:               Status(j.Status),
		Summary:              j.Summary,
		Duration:             msDur(j.DurationMs),
		Canceled:             j.Canceled,
		SkipReason:           j.SkipReason,
		Warnings:             j.Warnings,
		Error:                j.Error,
		ScriptError:          j.ScriptError,
		Failure:              modelOf(j.Failure, jsonFailure.model),
		HTTP:                 modelOf(j.HTTP, jsonHTTP.model),
		GRPC:                 modelOf(j.GRPC, jsonGRPC.model),
		Stream:               modelOf(j.Stream, jsonStream.model),
		Trace:                modelOf(j.Trace, jsonTrace.model),
		Tests:                convert(j.Tests, jsonTest.model),
		Compare:              modelOf(j.Compare, jsonCompare.model),
		Profile:              modelOf(j.Profile, jsonProfile.model),
		Steps:                convert(j.Steps, jsonStep.model),
	}
}

func (j jsonStep) model() Step {
	return Step{
		Name:                 j.Name,
		Method:               j.Method,
		Target:               j.Target,
		EffectiveTarget:      j.EffectiveTarget,
		Environment:          j.Environment,
		EnvironmentSelection: j.EnvironmentSelection,
		Branch:               j.Branch,
		Iteration:            j.Iteration,
		Total:                j.Total,
		Status:               Status(j.Status),
		Summary:              j.Summary,
		Duration:             msDur(j.DurationMs),
		Canceled:             j.Canceled,
		SkipReason:           j.SkipReason,
		Error:                j.Error,
		ScriptError:          j.ScriptError,
		Failure:              modelOf(j.Failure, jsonFailure.model),
		HTTP:                 modelOf(j.HTTP, jsonHTTP.model),
		GRPC:                 modelOf(j.GRPC, jsonGRPC.model),
		Stream:               modelOf(j.Stream, jsonStream.model),
		Trace:                modelOf(j.Trace, jsonTrace.model),
		Tests:                convert(j.Tests, jsonTest.model),
	}
}

func (j jsonHTTP) model() HTTP { return HTTP(j) }

func (j jsonGRPC) model() GRPC { return GRPC(j) }

func (j jsonCompare) model() Compare { return Compare(j) }

func (j jsonFailure) model() Failure {
	return Failure{
		Code:     runfail.Code(j.Code),
		Category: runfail.Category(j.Category),
		ExitCode: j.ExitCode,
		Message:  j.Message,
		Source:   j.Source,
		Chain:    convert(j.Chain, jsonFailureChain.model),
		Frames:   convert(j.Frames, jsonFailureFrame.model),
	}
}

func (j jsonFailureChain) model() FailureChain {
	return FailureChain{
		Code:      j.Code,
		Component: j.Component,
		Kind:      j.Kind,
		Message:   j.Message,
		Children:  convert(j.Children, jsonFailureChain.model),
	}
}

func (j jsonFailureFrame) model() FailureFrame {
	out := FailureFrame{Name: j.Name}
	if j.Pos != nil {
		out.Pos = FailurePos(*j.Pos)
	}
	return out
}

func (j jsonTest) model() Test {
	return Test{Name: j.Name, Message: j.Message, Passed: j.Passed, Elapsed: msDur(j.ElapsedMs)}
}

func (j jsonProfile) model() Profile {
	return Profile{
		Count:          j.Count,
		Warmup:         j.Warmup,
		Delay:          msDur(j.DelayMs),
		TotalRuns:      j.TotalRuns,
		WarmupRuns:     j.WarmupRuns,
		SuccessfulRuns: j.SuccessfulRuns,
		FailedRuns:     j.FailedRuns,
		Latency:        modelOf(j.Latency, jsonLatency.model),
		Percentiles:    convert(j.Percentiles, jsonPercentile.model),
		Histogram:      convert(j.Histogram, jsonHistBin.model),
		Failures:       convert(j.Failures, jsonProfileFailure.model),
	}
}

func (j jsonLatency) model() Latency {
	return Latency{
		Count:  j.Count,
		Min:    msDur(j.MinMs),
		Max:    msDur(j.MaxMs),
		Mean:   msDur(j.MeanMs),
		Median: msDur(j.MedianMs),
		StdDev: msDur(j.StdDevMs),
	}
}

func (j jsonPercentile) model() Percentile {
	return Percentile{Percentile: j.Percentile, Value: msDur(j.ValueMs)}
}

func (j jsonHistBin) model() HistBin {
	return HistBin{From: msDur(j.FromMs), To: msDur(j.ToMs), Count: j.Count}
}

func (j jsonProfileFailure) model() ProfileFailure {
	return ProfileFailure{
		Iteration:  j.Iteration,
		Warmup:     j.Warmup,
		Reason:     j.Reason,
		Status:     j.Status,
		StatusCode: j.StatusCode,
		Duration:   msDur(j.DurationMs),
		Failure:    modelOf(j.Failure, jsonFailure.model),
	}
}

func (j jsonStream) model() Stream {
	return Stream{
		Kind:           j.Kind,
		EventCount:     j.EventCount,
		Summary:        j.Summary,
		TranscriptPath: j.TranscriptPath,
		Error:          j.Error,
	}
}

func (j jsonTrace) model() Trace {
	return Trace{
		Duration:     msDur(j.DurationMs),
		Error:        j.Error,
		Budget:       modelOf(j.Budgets, jsonTraceBudget.model),
		Breaches:     convert(j.Breaches, jsonTraceBreach.model),
		ArtifactPath: j.ArtifactPath,
	}
}

func (j jsonTraceBudget) model() TraceBudget {
	out := TraceBudget{Total: msDur(j.TotalMs), Tolerance: msDur(j.ToleranceMs)}
	if len(j.Phases) > 0 {
		out.Phases = make(map[string]time.Duration, len(j.Phases))
		for key, val := range j.Phases {
			out.Phases[key] = msDur(val)
		}
	}
	return out
}

func (j jsonTraceBreach) model() TraceBreach {
	return TraceBreach{
		Kind:   j.Kind,
		Limit:  msDur(j.LimitMs),
		Actual: msDur(j.ActualMs),
		Over:   msDur(j.OverMs),
	}
}

// A JSON null leaves dst unchanged, like encoding/json does for plain values.
func decode[J, M any](b []byte, dst *M, model func(J) M) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return nil
	}
	var j J
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*dst = model(j)
	return nil
}

func modelOf[J, M any](j *J, model func(J) M) *M {
	if j == nil {
		return nil
	}
	m := model(*j)
	return &m
}

func convert[J, M any](src []J, model func(J) M) []M {
	if len(src) == 0 {
		return nil
	}
	out := make([]M, len(src))
	for i, v := range src {
		out[i] = model(v)
	}
	return out
}

func msDur(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}
