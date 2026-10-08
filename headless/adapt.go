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
		EffectiveTarget:      m.EffectiveTarget,
		Environment:          m.Environment,
		EnvironmentSelection: m.EnvironmentSelection,
		Status:               Status(m.Status),
		Summary:              m.Summary,
		Duration:             m.Duration,
		Canceled:             m.Canceled,
		SkipReason:           m.SkipReason,
		Error:                m.Error,
		ErrorDetail:          rendered(m.ErrorDetail),
		ScriptError:          m.ScriptError,
		ScriptErrorDetail:    rendered(m.ScriptErrorDetail),
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
		EffectiveTarget:      m.EffectiveTarget,
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
		ErrorDetail:          rendered(m.ErrorDetail),
		ScriptError:          m.ScriptError,
		ScriptErrorDetail:    rendered(m.ScriptErrorDetail),
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
		EffectiveTarget:      r.EffectiveTarget,
		Environment:          r.Environment,
		EnvironmentSelection: r.EnvironmentSelection,
		Status:               runfmt.Status(o.effectiveStatus()),
		Summary:              r.Summary,
		Duration:             r.Duration,
		Canceled:             r.Canceled,
		SkipReason:           r.SkipReason,
		Error:                r.Error,
		ErrorDetail:          errDetail(r.Error, r.ErrorDetail),
		ScriptError:          r.ScriptError,
		ScriptErrorDetail:    errDetail(r.ScriptError, r.ScriptErrorDetail),
		Failure:              o.failureModel(),
		HTTP:                 (*runfmt.HTTP)(r.HTTP),
		GRPC:                 (*runfmt.GRPC)(r.GRPC),
		Stream:               r.Stream.model(),
		Trace:                r.Trace.model(),
		Tests:                convert(r.Tests, Test.model),
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
		EffectiveTarget:      s.EffectiveTarget,
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
		ErrorDetail:          errDetail(s.Error, s.ErrorDetail),
		ScriptError:          s.ScriptError,
		ScriptErrorDetail:    errDetail(s.ScriptError, s.ScriptErrorDetail),
		Failure:              o.failureModel(),
		HTTP:                 (*runfmt.HTTP)(s.HTTP),
		GRPC:                 (*runfmt.GRPC)(s.GRPC),
		Stream:               s.Stream.model(),
		Trace:                s.Trace.model(),
		Tests:                convert(s.Tests, Test.model),
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
		Latency:        p.Latency.model(),
		Percentiles:    convert(p.Percentiles, Percentile.model),
		Histogram:      convert(p.Histogram, HistBin.model),
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
		Budget:       t.Budget.model(),
		Breaches:     convert(t.Breaches, TraceBreach.model),
		ArtifactPath: t.ArtifactPath,
	}
}

func (t Test) model() runfmt.Test { return runfmt.Test(t) }

func (l *Latency) model() *runfmt.Latency { return (*runfmt.Latency)(l) }

func (p Percentile) model() runfmt.Percentile { return runfmt.Percentile(p) }

func (b HistBin) model() runfmt.HistBin { return runfmt.HistBin(b) }

func (s *Stream) model() *runfmt.Stream { return (*runfmt.Stream)(s) }

func (b *TraceBudget) model() *runfmt.TraceBudget { return (*runfmt.TraceBudget)(b) }

func (b TraceBreach) model() runfmt.TraceBreach { return runfmt.TraceBreach(b) }

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
	case streamFailed(o.stream):
		f = runfail.FromErrorSource(textError(o.stream.Error), "stream")
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
		if mf := measuredFailure(o.profile); mf != nil {
			return mf.model()
		}
		if st := failedStep(o.steps); st != nil {
			return st.outcome().failureModel()
		}
		f = runfail.Assertion(o.summary, "status")
	}
	return runfmt.FromFailure(f)
}

type textError string

func (e textError) Error() string { return string(e) }

// The writers read only Rendered, so the other detail fields can stay empty.
func errDetail(msg, text string) *runfmt.ErrorDetail {
	switch {
	case text != "":
		return &runfmt.ErrorDetail{Message: msg, Rendered: text}
	case msg != "":
		return runfmt.ErrorDetailFromError(textError(msg))
	default:
		return nil
	}
}

func rendered(d *runfmt.ErrorDetail) string {
	if d == nil {
		return ""
	}
	return d.Rendered
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
