package headless

import (
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/runx/report"
)

func reportFromRunner(src *runner.Report) *Report {
	m := runner.ReportModel(src)
	out := &Report{
		SchemaVersion:        m.SchemaVersion,
		Version:              m.Version,
		FilePath:             m.FilePath,
		EnvName:              m.EnvName,
		EnvironmentSelection: m.EnvironmentSelection,
		StartedAt:            m.StartedAt,
		EndedAt:              m.EndedAt,
		Duration:             m.Duration,
		Results:              make([]Result, 0, len(m.Results)),
		Total:                m.Total,
		Passed:               m.Passed,
		Failed:               m.Failed,
		Skipped:              m.Skipped,
		StopReason:           StopReason(m.StopReason),
		Warnings:             m.Warnings,
	}
	for _, res := range m.Results {
		out.Results = append(out.Results, newResult(res))
	}
	return out
}

func newResult(m runfmt.Result) Result {
	return Result{
		Kind:                 Kind(m.Kind),
		Name:                 m.Name,
		Method:               m.Method,
		Target:               m.Target,
		Environment:          m.Environment,
		EnvironmentSelection: m.EnvironmentSelection,
		Status:               Status(m.Status),
		Summary:              m.Summary,
		Duration:             m.Duration,
		Canceled:             m.Canceled,
		SkipReason:           m.SkipReason,
		Error:                m.Error,
		ScriptError:          m.ScriptError,
		Failure:              newFailure(m.Failure),
		HTTP:                 (*HTTP)(m.HTTP),
		GRPC:                 (*GRPC)(m.GRPC),
		Stream:               (*Stream)(m.Stream),
		Trace:                newTrace(m.Trace),
		Tests:                convert(m.Tests, func(t runfmt.Test) Test { return Test(t) }),
		Compare:              (*Compare)(m.Compare),
		Profile:              newProfile(m.Profile),
		Steps:                convert(m.Steps, newStep),
	}
}

func newStep(m runfmt.Step) Step {
	return Step{
		Name:                 m.Name,
		Method:               m.Method,
		Target:               m.Target,
		Environment:          m.Environment,
		EnvironmentSelection: m.EnvironmentSelection,
		Branch:               m.Branch,
		Iteration:            m.Iteration,
		Total:                m.Total,
		Status:               Status(m.Status),
		Summary:              m.Summary,
		Duration:             m.Duration,
		Canceled:             m.Canceled,
		SkipReason:           m.SkipReason,
		Error:                m.Error,
		ScriptError:          m.ScriptError,
		Failure:              newFailure(m.Failure),
		HTTP:                 (*HTTP)(m.HTTP),
		GRPC:                 (*GRPC)(m.GRPC),
		Stream:               (*Stream)(m.Stream),
		Trace:                newTrace(m.Trace),
		Tests:                convert(m.Tests, func(t runfmt.Test) Test { return Test(t) }),
	}
}

func newProfile(m *runfmt.Profile) *Profile {
	if m == nil {
		return nil
	}
	return &Profile{
		Count:          m.Count,
		Warmup:         m.Warmup,
		Delay:          m.Delay,
		TotalRuns:      m.TotalRuns,
		WarmupRuns:     m.WarmupRuns,
		SuccessfulRuns: m.SuccessfulRuns,
		FailedRuns:     m.FailedRuns,
		Latency:        (*Latency)(m.Latency),
		Percentiles:    convert(m.Percentiles, func(p runfmt.Percentile) Percentile { return Percentile(p) }),
		Histogram:      convert(m.Histogram, func(b runfmt.HistBin) HistBin { return HistBin(b) }),
		Failures:       convert(m.Failures, newProfileFailure),
	}
}

func newProfileFailure(m runfmt.ProfileFailure) ProfileFailure {
	return ProfileFailure{
		Iteration:  m.Iteration,
		Warmup:     m.Warmup,
		Reason:     m.Reason,
		Status:     m.Status,
		StatusCode: m.StatusCode,
		Duration:   m.Duration,
		Failure:    newFailure(m.Failure),
	}
}

func newFailure(m *runfmt.Failure) *Failure {
	if m == nil {
		return nil
	}
	return &Failure{
		Code:     FailureCode(m.Code),
		Category: FailureCategory(m.Category),
		ExitCode: m.ExitCode,
		Message:  m.Message,
		Source:   m.Source,
		Chain:    m.Chain,
		Frames:   m.Frames,
	}
}

func newTrace(m *runfmt.Trace) *Trace {
	if m == nil {
		return nil
	}
	return &Trace{
		Duration:     m.Duration,
		Error:        m.Error,
		Budget:       (*TraceBudget)(m.Budget),
		Breaches:     convert(m.Breaches, func(b runfmt.TraceBreach) TraceBreach { return TraceBreach(b) }),
		ArtifactPath: m.ArtifactPath,
	}
}

// The writers only read the model, so it shares the report's maps and slices.
func (r *Report) model() runfmt.Report {
	out := runfmt.Report{
		SchemaVersion:        r.SchemaVersion,
		Version:              r.Version,
		FilePath:             r.FilePath,
		EnvName:              r.EnvName,
		EnvironmentSelection: r.EnvironmentSelection,
		StartedAt:            r.StartedAt,
		EndedAt:              r.EndedAt,
		Duration:             r.Duration,
		Results:              make([]runfmt.Result, 0, len(r.Results)),
		Total:                r.Total,
		Passed:               r.Passed,
		Failed:               r.Failed,
		Skipped:              r.Skipped,
		StopReason:           string(r.StopReason),
		Warnings:             r.Warnings,
	}
	for _, res := range r.Results {
		out.Results = append(out.Results, res.model())
	}
	return out
}

func (r Result) model() runfmt.Result {
	o := r.outcome()
	return runfmt.Result{
		Kind:                 string(r.Kind),
		Name:                 r.Name,
		Method:               r.Method,
		Target:               r.Target,
		Environment:          r.Environment,
		EnvironmentSelection: r.EnvironmentSelection,
		Status:               runfmt.Status(o.effectiveStatus()),
		Summary:              r.Summary,
		Duration:             r.Duration,
		Canceled:             r.Canceled,
		SkipReason:           r.SkipReason,
		Error:                r.Error,
		ErrorDetail:          errDetail(r.Error),
		ScriptError:          r.ScriptError,
		ScriptErrorDetail:    errDetail(r.ScriptError),
		Failure:              o.failureModel(),
		HTTP:                 (*runfmt.HTTP)(r.HTTP),
		GRPC:                 (*runfmt.GRPC)(r.GRPC),
		Stream:               (*runfmt.Stream)(r.Stream),
		Trace:                r.Trace.model(),
		Tests:                convert(r.Tests, func(t Test) runfmt.Test { return runfmt.Test(t) }),
		Compare:              (*runfmt.Compare)(r.Compare),
		Profile:              r.Profile.model(),
		Steps:                convert(r.Steps, Step.model),
	}
}

func (s Step) model() runfmt.Step {
	o := s.outcome()
	return runfmt.Step{
		Name:                 s.Name,
		Method:               s.Method,
		Target:               s.Target,
		Environment:          s.Environment,
		EnvironmentSelection: s.EnvironmentSelection,
		Branch:               s.Branch,
		Iteration:            s.Iteration,
		Total:                s.Total,
		Status:               runfmt.Status(o.effectiveStatus()),
		Summary:              s.Summary,
		Duration:             s.Duration,
		Canceled:             s.Canceled,
		SkipReason:           s.SkipReason,
		Error:                s.Error,
		ErrorDetail:          errDetail(s.Error),
		ScriptError:          s.ScriptError,
		ScriptErrorDetail:    errDetail(s.ScriptError),
		Failure:              o.failureModel(),
		HTTP:                 (*runfmt.HTTP)(s.HTTP),
		GRPC:                 (*runfmt.GRPC)(s.GRPC),
		Stream:               (*runfmt.Stream)(s.Stream),
		Trace:                s.Trace.model(),
		Tests:                convert(s.Tests, func(t Test) runfmt.Test { return runfmt.Test(t) }),
	}
}

func (p *Profile) model() *runfmt.Profile {
	if p == nil {
		return nil
	}
	return &runfmt.Profile{
		Count:          p.Count,
		Warmup:         p.Warmup,
		Delay:          p.Delay,
		TotalRuns:      p.TotalRuns,
		WarmupRuns:     p.WarmupRuns,
		SuccessfulRuns: p.SuccessfulRuns,
		FailedRuns:     p.FailedRuns,
		Latency:        (*runfmt.Latency)(p.Latency),
		Percentiles:    convert(p.Percentiles, func(v Percentile) runfmt.Percentile { return runfmt.Percentile(v) }),
		Histogram:      convert(p.Histogram, func(b HistBin) runfmt.HistBin { return runfmt.HistBin(b) }),
		Failures:       convert(p.Failures, ProfileFailure.model),
	}
}

func (f ProfileFailure) model() runfmt.ProfileFailure {
	return runfmt.ProfileFailure{
		Iteration:  f.Iteration,
		Warmup:     f.Warmup,
		Reason:     f.Reason,
		Status:     f.Status,
		StatusCode: f.StatusCode,
		Duration:   f.Duration,
		Failure:    f.Failure.model(),
	}
}

// Category and exit code are derived from Code, not copied from f.
func (f *Failure) model() *runfmt.Failure {
	if f == nil {
		return nil
	}
	out := runfmt.FromFailure(runfail.New(runfail.Code(f.Code), f.Message, f.Source))
	out.Chain = f.Chain
	out.Frames = f.Frames
	return out
}

func (t *Trace) model() *runfmt.Trace {
	if t == nil {
		return nil
	}
	return &runfmt.Trace{
		Duration:     t.Duration,
		Error:        t.Error,
		Budget:       (*runfmt.TraceBudget)(t.Budget),
		Breaches:     convert(t.Breaches, func(b TraceBreach) runfmt.TraceBreach { return runfmt.TraceBreach(b) }),
		ArtifactPath: t.ArtifactPath,
	}
}

func (o outcome) failureModel() *runfmt.Failure {
	if o.failure != nil {
		return o.failure.model()
	}
	if o.effectiveStatus() != StatusFail {
		return nil
	}
	var f runfail.Failure
	switch {
	case o.canceled:
		f = runfail.Canceled("canceled", "canceled")
	case o.err != "":
		f = runfail.FromErrorSource(textError(o.err), "error")
	case o.scriptErr != "":
		f = runfail.Script(o.scriptErr, "scriptError")
	case anyTestFailed(o.tests):
		msg := runfail.FirstTestFailureMessage(o.tests, func(t Test) runfail.TestFailureFields {
			return runfail.TestFailureFields{Name: t.Name, Message: t.Message, Passed: t.Passed}
		})
		f = runfail.Assertion(msg, "tests")
	case traceFailed(o.trace):
		msg := runfail.FirstTraceBudgetBreachMessage(
			o.trace.Breaches,
			func(b TraceBreach) runfail.TraceBudgetBreachFields { return runfail.TraceBudgetBreachFields(b) },
		)
		f = runfail.TraceBudget(msg)
	default:
		f = runfail.Assertion(o.summary, "status")
	}
	return runfmt.FromFailure(f)
}

type textError string

func (e textError) Error() string { return string(e) }

func errDetail(s string) *runfmt.ErrorDetail {
	if s == "" {
		return nil
	}
	return runfmt.ErrorDetailFromError(textError(s))
}

func convert[S, D any](src []S, f func(S) D) []D {
	if len(src) == 0 {
		return nil
	}
	out := make([]D, len(src))
	for i, v := range src {
		out[i] = f(v)
	}
	return out
}
