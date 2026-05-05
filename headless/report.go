package headless

import (
	"encoding/json"
	"time"

	"github.com/unkn0wn-root/resterm/internal/runx/fail"
	"github.com/unkn0wn-root/resterm/internal/runx/report"
)

// Kind identifies the executed result type.
type Kind string

const (
	KindRequest  Kind = "request"
	KindWorkflow Kind = "workflow"
	KindForEach  Kind = "for-each"
	KindCompare  Kind = "compare"
	KindProfile  Kind = "profile"
)

// String implements fmt.Stringer.
func (k Kind) String() string {
	return string(k)
}

// IsValid reports whether k is a known result kind.
func (k Kind) IsValid() bool {
	switch k {
	case KindRequest, KindWorkflow, KindForEach, KindCompare, KindProfile:
		return true
	default:
		return false
	}
}

// Status reports whether a result passed, failed, or was skipped.
type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// String implements fmt.Stringer.
func (s Status) String() string {
	return string(s)
}

// IsValid reports whether s is a known result status.
func (s Status) IsValid() bool {
	switch s {
	case StatusPass, StatusFail, StatusSkip:
		return true
	default:
		return false
	}
}

// StopReason explains why a run stopped before all selected work completed.
// The zero value means the run completed normally.
type StopReason string

const (
	// StopReasonFailFast means Options.FailFast stopped execution after a failure.
	StopReasonFailFast StopReason = "fail_fast"
)

// Report contains the results of a headless run.
type Report struct {
	SchemaVersion string
	Version       string
	FilePath      string
	EnvName       string
	StartedAt     time.Time
	EndedAt       time.Time
	Duration      time.Duration
	Results       []Result
	Total         int
	Passed        int
	Failed        int
	Skipped       int
	StopReason    StopReason
}

// HasFailures reports whether the report contains any failed results.
func (r *Report) HasFailures() bool {
	if r == nil {
		return false
	}
	if r.Failed > 0 {
		return true
	}
	for _, res := range r.Results {
		if res.Failed() {
			return true
		}
	}
	return false
}

// MarshalJSON writes the canonical report JSON format.
func (r Report) MarshalJSON() ([]byte, error) {
	return json.Marshal(toFormatReport(&r))
}

// Result contains one executed request, workflow, compare run, or profile run.
type Result struct {
	Kind        Kind
	Name        string
	Method      string
	Target      string
	Environment string
	Status      Status
	Summary     string
	Duration    time.Duration
	Canceled    bool
	SkipReason  string
	Error       string
	ScriptError string
	Failure     *Failure
	HTTP        *HTTP
	GRPC        *GRPC
	Stream      *Stream
	Trace       *Trace
	Tests       []Test
	Compare     *Compare
	Profile     *Profile
	Steps       []Step
}

// MarshalJSON writes the canonical result JSON format.
func (r Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(toFormatResult(r))
}

// Failed reports whether the result represents a failure.
func (r Result) Failed() bool {
	return r.effectiveStatus() == StatusFail
}

func (r Result) effectiveStatus() Status {
	return status(r.Status, r.hasFailureEvidence())
}

func (r Result) hasFailureEvidence() bool {
	return r.Failure != nil ||
		hasFailure(r.Canceled, r.Error, r.ScriptError, r.Trace, r.Tests)
}

// Step contains one workflow or compare step result.
type Step struct {
	Name        string
	Method      string
	Target      string
	Environment string
	Branch      string
	Iteration   int
	Total       int
	Status      Status
	Summary     string
	Duration    time.Duration
	Canceled    bool
	SkipReason  string
	Error       string
	ScriptError string
	Failure     *Failure
	HTTP        *HTTP
	GRPC        *GRPC
	Stream      *Stream
	Trace       *Trace
	Tests       []Test
}

// MarshalJSON writes the canonical step JSON format.
func (s Step) MarshalJSON() ([]byte, error) {
	return json.Marshal(toFormatStep(s))
}

// Failed reports whether the step represents a failure.
func (s Step) Failed() bool {
	return s.effectiveStatus() == StatusFail
}

func (s Step) effectiveStatus() Status {
	return status(s.Status, s.hasFailureEvidence())
}

func (s Step) hasFailureEvidence() bool {
	return s.Failure != nil ||
		hasFailure(s.Canceled, s.Error, s.ScriptError, s.Trace, s.Tests)
}

// skip wins, otherwise any failure evidence makes the result fail.
func status(s Status, failed bool) Status {
	if s == StatusSkip {
		return StatusSkip
	}
	if s == StatusFail || failed {
		return StatusFail
	}
	return StatusPass
}

func hasFailure(
	canceled bool,
	errText string,
	scriptErrText string,
	trace *Trace,
	tests []Test,
) bool {
	return canceled || errText != "" || scriptErrText != "" ||
		traceFailed(trace) || anyTestFailed(tests)
}

func traceFailed(trace *Trace) bool {
	return trace != nil && len(trace.Breaches) > 0
}

func anyTestFailed(tests []Test) bool {
	for _, test := range tests {
		if !test.Passed {
			return true
		}
	}
	return false
}

// HTTP contains HTTP response summary fields.
type HTTP struct {
	Status     string `json:"status,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

// GRPC contains gRPC response summary fields.
type GRPC struct {
	Code          string `json:"code,omitempty"`
	StatusCode    int    `json:"statusCode,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
}

// Test contains one assertion result.
type Test struct {
	Name    string        `json:"name,omitempty"`
	Message string        `json:"message,omitempty"`
	Passed  bool          `json:"passed"`
	Elapsed time.Duration `json:"elapsed,omitempty"`
}

// Compare contains compare-run summary fields.
type Compare struct {
	Baseline string `json:"baseline,omitempty"`
}

// Profile contains profile-run summary fields.
type Profile struct {
	Count          int              `json:"count,omitempty"`
	Warmup         int              `json:"warmup,omitempty"`
	Delay          time.Duration    `json:"delay,omitempty"`
	TotalRuns      int              `json:"totalRuns,omitempty"`
	WarmupRuns     int              `json:"warmupRuns,omitempty"`
	SuccessfulRuns int              `json:"successfulRuns,omitempty"`
	FailedRuns     int              `json:"failedRuns,omitempty"`
	Latency        *Latency         `json:"latency,omitempty"`
	Percentiles    []Percentile     `json:"percentiles,omitempty"`
	Histogram      []HistBin        `json:"histogram,omitempty"`
	Failures       []ProfileFailure `json:"failures,omitempty"`
}

// ProfileFailure contains one failed profile iteration.
type ProfileFailure struct {
	Iteration  int           `json:"iteration,omitempty"`
	Warmup     bool          `json:"warmup,omitempty"`
	Reason     string        `json:"reason,omitempty"`
	Status     string        `json:"status,omitempty"`
	StatusCode int           `json:"statusCode,omitempty"`
	Duration   time.Duration `json:"duration,omitempty"`
	Failure    *Failure      `json:"failure,omitempty"`
}

// Latency contains aggregate profile latency statistics.
type Latency struct {
	Count  int           `json:"count,omitempty"`
	Min    time.Duration `json:"min,omitempty"`
	Max    time.Duration `json:"max,omitempty"`
	Mean   time.Duration `json:"mean,omitempty"`
	Median time.Duration `json:"median,omitempty"`
	StdDev time.Duration `json:"stdDev,omitempty"`
}

// Percentile contains one profile percentile.
type Percentile struct {
	Percentile int           `json:"percentile"`
	Value      time.Duration `json:"value,omitempty"`
}

// HistBin contains one profile histogram bin.
type HistBin struct {
	From  time.Duration `json:"from,omitempty"`
	To    time.Duration `json:"to,omitempty"`
	Count int           `json:"count,omitempty"`
}

// Stream contains streaming response metadata.
type Stream struct {
	Kind           string         `json:"kind,omitempty"`
	EventCount     int            `json:"eventCount,omitempty"`
	Summary        map[string]any `json:"summary,omitempty"`
	TranscriptPath string         `json:"transcriptPath,omitempty"`
}

// Trace contains trace summary metadata.
type Trace struct {
	Duration     time.Duration `json:"duration,omitempty"`
	Error        string        `json:"error,omitempty"`
	Budget       *TraceBudget  `json:"budget,omitempty"`
	Breaches     []TraceBreach `json:"breaches,omitempty"`
	ArtifactPath string        `json:"artifactPath,omitempty"`
}

// TraceBudget contains trace budget limits.
type TraceBudget struct {
	Total     time.Duration            `json:"total,omitempty"`
	Tolerance time.Duration            `json:"tolerance,omitempty"`
	Phases    map[string]time.Duration `json:"phases,omitempty"`
}

// TraceBreach contains one trace budget breach.
type TraceBreach struct {
	Kind   string        `json:"kind,omitempty"`
	Limit  time.Duration `json:"limit,omitempty"`
	Actual time.Duration `json:"actual,omitempty"`
	Over   time.Duration `json:"over,omitempty"`
}

func toFormatReport(rep *Report) runfmt.Report {
	if rep == nil {
		return runfmt.Report{}
	}
	out := runfmt.Report{
		SchemaVersion: rep.SchemaVersion,
		Version:       rep.Version,
		FilePath:      rep.FilePath,
		EnvName:       rep.EnvName,
		StartedAt:     rep.StartedAt,
		EndedAt:       rep.EndedAt,
		Duration:      rep.Duration,
		Results:       make([]runfmt.Result, 0, len(rep.Results)),
		Total:         rep.Total,
		Passed:        rep.Passed,
		Failed:        rep.Failed,
		Skipped:       rep.Skipped,
		StopReason:    string(rep.StopReason),
	}
	for _, res := range rep.Results {
		out.Results = append(out.Results, toFormatResult(res))
	}
	return out
}

func toFormatResult(res Result) runfmt.Result {
	out := runfmt.Result{
		Kind:        string(res.Kind),
		Name:        res.Name,
		Method:      res.Method,
		Target:      res.Target,
		Environment: res.Environment,
		Status:      runfmt.Status(res.effectiveStatus()),
		Summary:     res.Summary,
		Duration:    res.Duration,
		Canceled:    res.Canceled,
		SkipReason:  res.SkipReason,
		Error:       res.Error,
		ErrorDetail: runfmt.ErrorDetailFromError(
			errorsFromText(res.Error),
		),
		ScriptError: res.ScriptError,
		ScriptErrorDetail: runfmt.ErrorDetailFromError(
			errorsFromText(res.ScriptError),
		),
		Failure: toFormatResultFailure(res),
		HTTP:    toFormatHTTP(res.HTTP),
		GRPC:    toFormatGRPC(res.GRPC),
		Stream:  toFormatStream(res.Stream),
		Trace:   toFormatTrace(res.Trace),
		Compare: toFormatCompare(res.Compare),
		Profile: toFormatProfile(res.Profile),
	}
	if len(res.Tests) > 0 {
		out.Tests = make([]runfmt.Test, 0, len(res.Tests))
		for _, test := range res.Tests {
			out.Tests = append(out.Tests, runfmt.Test{
				Name:    test.Name,
				Message: test.Message,
				Passed:  test.Passed,
				Elapsed: test.Elapsed,
			})
		}
	}
	if len(res.Steps) > 0 {
		out.Steps = make([]runfmt.Step, 0, len(res.Steps))
		for _, step := range res.Steps {
			out.Steps = append(out.Steps, toFormatStep(step))
		}
	}
	return out
}

func toFormatStep(step Step) runfmt.Step {
	out := runfmt.Step{
		Name:        step.Name,
		Method:      step.Method,
		Target:      step.Target,
		Environment: step.Environment,
		Branch:      step.Branch,
		Iteration:   step.Iteration,
		Total:       step.Total,
		Status:      runfmt.Status(step.effectiveStatus()),
		Summary:     step.Summary,
		Duration:    step.Duration,
		Canceled:    step.Canceled,
		SkipReason:  step.SkipReason,
		Error:       step.Error,
		ErrorDetail: runfmt.ErrorDetailFromError(
			errorsFromText(step.Error),
		),
		ScriptError: step.ScriptError,
		ScriptErrorDetail: runfmt.ErrorDetailFromError(
			errorsFromText(step.ScriptError),
		),
		Failure: toFormatStepFailure(step),
		HTTP:    toFormatHTTP(step.HTTP),
		GRPC:    toFormatGRPC(step.GRPC),
		Stream:  toFormatStream(step.Stream),
		Trace:   toFormatTrace(step.Trace),
	}
	if len(step.Tests) > 0 {
		out.Tests = make([]runfmt.Test, 0, len(step.Tests))
		for _, test := range step.Tests {
			out.Tests = append(out.Tests, runfmt.Test{
				Name:    test.Name,
				Message: test.Message,
				Passed:  test.Passed,
				Elapsed: test.Elapsed,
			})
		}
	}
	return out
}

func toFormatHTTP(http *HTTP) *runfmt.HTTP {
	if http == nil {
		return nil
	}
	return &runfmt.HTTP{
		Status:     http.Status,
		StatusCode: http.StatusCode,
		Protocol:   http.Protocol,
	}
}

func toFormatGRPC(grpc *GRPC) *runfmt.GRPC {
	if grpc == nil {
		return nil
	}
	return &runfmt.GRPC{
		Code:          grpc.Code,
		StatusCode:    grpc.StatusCode,
		StatusMessage: grpc.StatusMessage,
	}
}

func toFormatCompare(cmp *Compare) *runfmt.Compare {
	if cmp == nil {
		return nil
	}
	return &runfmt.Compare{Baseline: cmp.Baseline}
}

func toFormatProfile(prof *Profile) *runfmt.Profile {
	if prof == nil {
		return nil
	}
	out := &runfmt.Profile{
		Count:          prof.Count,
		Warmup:         prof.Warmup,
		Delay:          prof.Delay,
		TotalRuns:      prof.TotalRuns,
		WarmupRuns:     prof.WarmupRuns,
		SuccessfulRuns: prof.SuccessfulRuns,
		FailedRuns:     prof.FailedRuns,
		Latency:        toFormatLatency(prof.Latency),
	}
	if len(prof.Percentiles) > 0 {
		out.Percentiles = make([]runfmt.Percentile, 0, len(prof.Percentiles))
		for _, pct := range prof.Percentiles {
			out.Percentiles = append(out.Percentiles, runfmt.Percentile{
				Percentile: pct.Percentile,
				Value:      pct.Value,
			})
		}
	}
	if len(prof.Histogram) > 0 {
		out.Histogram = make([]runfmt.HistBin, 0, len(prof.Histogram))
		for _, bin := range prof.Histogram {
			out.Histogram = append(out.Histogram, runfmt.HistBin{
				From:  bin.From,
				To:    bin.To,
				Count: bin.Count,
			})
		}
	}
	if len(prof.Failures) > 0 {
		out.Failures = make([]runfmt.ProfileFailure, 0, len(prof.Failures))
		for _, fail := range prof.Failures {
			out.Failures = append(out.Failures, runfmt.ProfileFailure{
				Iteration:  fail.Iteration,
				Warmup:     fail.Warmup,
				Reason:     fail.Reason,
				Status:     fail.Status,
				StatusCode: fail.StatusCode,
				Duration:   fail.Duration,
				Failure:    toFormatFailure(fail.Failure),
			})
		}
	}
	return out
}

func toFormatFailure(f *Failure) *runfmt.Failure {
	if f == nil {
		return nil
	}
	out := runfmt.FromFailure(runfail.New(runfail.Code(f.Code), f.Message, f.Source))
	if out == nil {
		return nil
	}
	out.Chain = runfmt.CloneFailureChain(f.Chain)
	out.Frames = runfmt.CloneFailureFrames(f.Frames)
	return out
}

func toFormatResultFailure(res Result) *runfmt.Failure {
	if res.Failure != nil {
		return toFormatFailure(res.Failure)
	}
	if res.effectiveStatus() != StatusFail {
		return nil
	}
	switch {
	case res.Canceled:
		return runfmt.FromFailure(runfail.Canceled("canceled", "canceled"))
	case res.Error != "":
		return runfmt.FromFailure(runfail.FromErrorSource(errorsFromText(res.Error), "error"))
	case res.ScriptError != "":
		return runfmt.FromFailure(runfail.Script(res.ScriptError, "scriptError"))
	case anyTestFailed(res.Tests):
		return runfmt.FromFailure(runfail.Assertion(publicTestFailureMessage(res.Tests), "tests"))
	case traceFailed(res.Trace):
		return runfmt.FromFailure(runfail.TraceBudget(publicTraceFailureMessage(res.Trace)))
	default:
		return runfmt.FromFailure(runfail.Assertion(res.Summary, "status"))
	}
}

func toFormatStepFailure(step Step) *runfmt.Failure {
	if step.Failure != nil {
		return toFormatFailure(step.Failure)
	}
	if step.effectiveStatus() != StatusFail {
		return nil
	}
	switch {
	case step.Canceled:
		return runfmt.FromFailure(runfail.Canceled("canceled", "canceled"))
	case step.Error != "":
		return runfmt.FromFailure(runfail.FromErrorSource(errorsFromText(step.Error), "error"))
	case step.ScriptError != "":
		return runfmt.FromFailure(runfail.Script(step.ScriptError, "scriptError"))
	case anyTestFailed(step.Tests):
		return runfmt.FromFailure(runfail.Assertion(publicTestFailureMessage(step.Tests), "tests"))
	case traceFailed(step.Trace):
		return runfmt.FromFailure(runfail.TraceBudget(publicTraceFailureMessage(step.Trace)))
	default:
		return runfmt.FromFailure(runfail.Assertion(step.Summary, "status"))
	}
}

type textError string

func (e textError) Error() string { return string(e) }

func errorsFromText(s string) error {
	if s == "" {
		return nil
	}
	return textError(s)
}

func publicTestFailureMessage(tests []Test) string {
	return runfail.FirstTestFailureMessage(tests, func(test Test) runfail.TestFailureFields {
		return runfail.TestFailureFields{
			Name:    test.Name,
			Message: test.Message,
			Passed:  test.Passed,
		}
	})
}

func publicTraceFailureMessage(trace *Trace) string {
	if trace == nil {
		return "trace budget breached"
	}
	return runfail.FirstTraceBudgetBreachMessage(
		trace.Breaches,
		func(breach TraceBreach) runfail.TraceBudgetBreachFields {
			return runfail.TraceBudgetBreachFields{
				Kind:   breach.Kind,
				Limit:  breach.Limit,
				Actual: breach.Actual,
				Over:   breach.Over,
			}
		},
	)
}

func toFormatLatency(lat *Latency) *runfmt.Latency {
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

func toFormatStream(stream *Stream) *runfmt.Stream {
	if stream == nil {
		return nil
	}
	out := &runfmt.Stream{
		Kind:           stream.Kind,
		EventCount:     stream.EventCount,
		TranscriptPath: stream.TranscriptPath,
	}
	if len(stream.Summary) > 0 {
		out.Summary = runfmt.CloneAnyMap(stream.Summary)
	}
	return out
}

func toFormatTrace(trace *Trace) *runfmt.Trace {
	if trace == nil {
		return nil
	}
	out := &runfmt.Trace{
		Duration:     trace.Duration,
		Error:        trace.Error,
		ArtifactPath: trace.ArtifactPath,
	}
	if bud := trace.Budget; bud != nil {
		out.Budget = &runfmt.TraceBudget{
			Total:     bud.Total,
			Tolerance: bud.Tolerance,
			Phases:    runfmt.CloneDurationMap(bud.Phases),
		}
	}
	if len(trace.Breaches) > 0 {
		out.Breaches = make([]runfmt.TraceBreach, 0, len(trace.Breaches))
		for _, breach := range trace.Breaches {
			out.Breaches = append(out.Breaches, runfmt.TraceBreach{
				Kind:   breach.Kind,
				Limit:  breach.Limit,
				Actual: breach.Actual,
				Over:   breach.Over,
			})
		}
	}
	return out
}
