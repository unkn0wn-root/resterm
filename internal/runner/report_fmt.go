package runner

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"io"
	"sort"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/runx/report"
	"github.com/unkn0wn-root/resterm/internal/scriptapi"
	"github.com/unkn0wn-root/resterm/internal/str"
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
		SchemaVersion:        str.FirstTrimmed(rep.SchemaVersion, runfmt.ReportSchemaVersion),
		Version:              strings.TrimSpace(rep.Version),
		FilePath:             rep.FilePath,
		EnvName:              strings.TrimSpace(rep.EnvName),
		EnvironmentSelection: rep.EnvironmentSelection,
		StartedAt:            rep.StartedAt,
		EndedAt:              rep.EndedAt,
		Duration:             rep.Duration,
		Results:              make([]runfmt.Result, 0, len(rep.Results)),
		Total:                rep.Total,
		Passed:               rep.Passed,
		Failed:               rep.Failed,
		Skipped:              rep.Skipped,
		StopReason:           rep.StopReason,
		Warnings:             rep.WarningTexts(),
	}
	for _, res := range rep.Results {
		out.Results = append(out.Results, toFormatResult(res, rep.secrets))
	}
	return out
}

// WarningTexts adds one warning per short secret, with secret values masked.
func (r *Report) WarningTexts() []string {
	out := maskEach(r.secrets, r.Warnings)
	for _, name := range r.shortSecrets {
		out = append(out, fmt.Sprintf(
			"secret %q is shorter than %d characters, so reports may also mask the same word in unrelated text",
			r.secrets.Text(name),
			shortSecret,
		))
	}
	return out
}

func toFormatResult(res Result, sec redact.Set) runfmt.Result {
	err, scriptErr := diag.Redact(res.Err, sec.Text), diag.Redact(res.ScriptErr, sec.Text)
	out := runfmt.Result{
		Kind:                 string(res.Kind),
		Name:                 sec.Text(strings.TrimSpace(res.Name)),
		Method:               sec.Text(strings.TrimSpace(res.Method)),
		Target:               sec.Text(strings.TrimSpace(res.Target)),
		EffectiveTarget:      sec.Text(strings.TrimSpace(res.EffectiveTarget)),
		Environment:          strings.TrimSpace(res.Environment),
		EnvironmentSelection: res.EnvironmentSelection,
		Status:               resultStatusOf(res),
		Summary:              sec.Text(strings.TrimSpace(res.Summary)),
		Duration:             resultDuration(res),
		Canceled:             res.Canceled,
		SkipReason:           sec.Text(strings.TrimSpace(res.SkipReason)),
		Warnings:             maskEach(sec, res.Warnings),
		Error:                errText(err),
		ErrorDetail:          runfmt.ErrorDetailFromError(err),
		ScriptError:          errText(scriptErr),
		ScriptErrorDetail:    runfmt.ErrorDetailFromError(scriptErr),
		HTTP:                 formatHTTP(res.Response, sec),
		GRPC:                 formatGRPC(res.GRPC, sec),
		Stream:               formatStream(res.Stream, sec),
		Response:             res.report,
		Trace:                formatTrace(res.Trace, sec),
		Tests:                formatTests(res.Tests, sec),
		Compare:              formatCompare(res.Compare),
		Profile:              formatProfile(res.Profile, sec),
		Steps:                formatSteps(res.Steps, sec),
	}
	// Step and profile failures arrive masked.
	switch f, from := resultFailure(res); from {
	case runfail.OriginProfile:
		i := slices.IndexFunc(out.Profile.Failures, func(f runfmt.ProfileFailure) bool { return !f.Warmup })
		out.Failure = out.Profile.Failures[i].Failure
	case runfail.OriginStep:
		i := slices.IndexFunc(out.Steps, func(s runfmt.Step) bool { return s.Failure != nil })
		out.Failure = out.Steps[i].Failure
	default:
		out.Failure = formatRunFailure(f, cmp.Or(out.ErrorDetail, out.ScriptErrorDetail), sec)
	}
	return out
}

func toFormatStep(step StepResult, sec redact.Set) runfmt.Step {
	err, scriptErr := diag.Redact(step.Err, sec.Text), diag.Redact(step.ScriptErr, sec.Text)
	out := runfmt.Step{
		Name:                 sec.Text(strings.TrimSpace(step.Name)),
		Method:               sec.Text(strings.TrimSpace(step.Method)),
		Target:               sec.Text(strings.TrimSpace(step.Target)),
		EffectiveTarget:      sec.Text(strings.TrimSpace(step.EffectiveTarget)),
		Environment:          strings.TrimSpace(step.Environment),
		EnvironmentSelection: step.EnvironmentSelection,
		Branch:               sec.Text(strings.TrimSpace(step.Branch)),
		Iteration:            step.Iteration,
		Total:                step.Total,
		Status:               stepStatusOf(step),
		Summary:              sec.Text(strings.TrimSpace(step.Summary)),
		Duration:             step.Duration,
		Canceled:             step.Canceled,
		SkipReason:           sec.Text(strings.TrimSpace(step.SkipReason)),
		Error:                errText(err),
		ErrorDetail:          runfmt.ErrorDetailFromError(err),
		ScriptError:          errText(scriptErr),
		ScriptErrorDetail:    runfmt.ErrorDetailFromError(scriptErr),
		HTTP:                 formatHTTP(step.Response, sec),
		GRPC:                 formatGRPC(step.GRPC, sec),
		Stream:               formatStream(step.Stream, sec),
		Response:             step.report,
		Trace:                formatTrace(step.Trace, sec),
		Tests:                formatTests(step.Tests, sec),
	}
	out.Failure = formatRunFailure(stepFailure(step), cmp.Or(out.ErrorDetail, out.ScriptErrorDetail), sec)
	return out
}

func formatSteps(src []StepResult, sec redact.Set) []runfmt.Step {
	if len(src) == 0 {
		return nil
	}
	out := make([]runfmt.Step, 0, len(src))
	for _, step := range src {
		out = append(out, toFormatStep(step, sec))
	}
	return out
}

func maskEach(sec redact.Set, src []string) []string {
	out := slices.Clone(src)
	for i := range out {
		out[i] = sec.Text(out[i])
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

func formatHTTP(resp *httpx.Response, sec redact.Set) *runfmt.HTTP {
	if resp == nil {
		return nil
	}
	return &runfmt.HTTP{
		Status:     sec.Text(strings.TrimSpace(resp.Status)),
		StatusCode: resp.StatusCode,
		Protocol:   strings.TrimSpace(resp.Proto),
	}
}

func formatGRPC(resp *grpcx.Response, sec redact.Set) *runfmt.GRPC {
	if resp == nil {
		return nil
	}
	out := &runfmt.GRPC{
		Code:          resp.StatusCode.String(),
		StatusCode:    int(resp.StatusCode),
		StatusMessage: sec.Text(strings.TrimSpace(resp.StatusMessage)),
		StatusDetails: slices.Clone(resp.StatusDetails),
	}
	for i, d := range out.StatusDetails {
		out.StatusDetails[i] = string(sec.JSON([]byte(d)))
	}
	return out
}

func formatTests(src []scriptapi.TestResult, sec redact.Set) []runfmt.Test {
	if len(src) == 0 {
		return nil
	}
	out := make([]runfmt.Test, 0, len(src))
	for _, test := range src {
		out = append(out, runfmt.Test{
			Name:    sec.Text(strings.TrimSpace(test.Name)),
			Message: sec.Text(strings.TrimSpace(test.Message)),
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
	return &runfmt.Compare{
		Baseline: strings.TrimSpace(info.Baseline),
		Group:    strings.TrimSpace(info.Group),
	}
}

func formatProfile(prof *ProfileInfo, sec redact.Set) *runfmt.Profile {
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
				Reason:     sec.Text(strings.TrimSpace(fail.Reason)),
				Status:     sec.Text(strings.TrimSpace(fail.Status)),
				StatusCode: fail.StatusCode,
				Duration:   fail.Duration,
				Failure: formatRunFailure(
					fail.Failure,
					runfmt.ErrorDetailFromError(diag.Redact(fail.Err, sec.Text)),
					sec,
				),
			})
		}
	}
	return out
}

func formatRunFailure(failure runfail.Failure, detail *runfmt.ErrorDetail, sec redact.Set) *runfmt.Failure {
	if failure.Code == "" {
		return nil
	}
	return runfmt.AttachErrorDetail(runfmt.FromFailure(runfail.New(
		failure.Code,
		sec.Text(strings.TrimSpace(failure.Message)),
		sec.Text(strings.TrimSpace(failure.Source)),
	)), detail)
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

func formatStream(info *StreamInfo, sec redact.Set) *runfmt.Stream {
	if info == nil {
		return nil
	}
	out := &runfmt.Stream{
		Kind:           strings.TrimSpace(info.Kind),
		EventCount:     info.EventCount,
		TranscriptPath: strings.TrimSpace(info.TranscriptPath),
	}
	if info.Err != nil {
		out.Error = sec.Text(info.Err.Error())
	}
	if len(info.Summary) > 0 {
		out.Summary = runfmt.CloneAnyMap(info.Summary)
		for k, v := range out.Summary {
			if s, ok := v.(string); ok {
				out.Summary[k] = sec.Text(s)
			}
		}
	}
	return out
}

func formatTrace(info *TraceInfo, sec redact.Set) *runfmt.Trace {
	if info == nil || info.Summary == nil {
		return nil
	}
	out := &runfmt.Trace{
		Duration:     info.Summary.Duration,
		Error:        sec.Text(strings.TrimSpace(info.Summary.Error)),
		ArtifactPath: strings.TrimSpace(info.ArtifactPath),
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
				Kind:   strings.TrimSpace(breach.Kind),
				Limit:  breach.Limit,
				Actual: breach.Actual,
				Over:   breach.Over,
			})
		}
	}
	return out
}
