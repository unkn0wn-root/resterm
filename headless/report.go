package headless

import (
	"encoding/json"
	"time"
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
	// StopReasonCanceled means the run context was canceled.
	StopReasonCanceled StopReason = "canceled"
)

// Report contains the results of a headless run.
type Report struct {
	SchemaVersion        string
	Version              string
	FilePath             string
	EnvName              string
	EnvironmentSelection EnvironmentSelection
	StartedAt            time.Time
	EndedAt              time.Time
	Duration             time.Duration
	Results              []Result
	Total                int
	Passed               int
	Failed               int
	Skipped              int
	StopReason           StopReason
	// Warnings holds parse warnings with their source location. They never
	// affect HasFailures or the exit code.
	Warnings []string
}

// HasFailures reports whether the report failed, which is when ExitCode is not 0.
func (r *Report) HasFailures() bool {
	return r.ExitCode(ExitCodeSummary) != ExitPass
}

// MarshalJSON writes the canonical report JSON format.
func (r Report) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.model())
}

// Result contains one executed request, workflow, compare run, or profile run.
type Result struct {
	Kind                 Kind
	Name                 string
	Method               string
	Target               string
	EffectiveTarget      string
	Environment          string
	EnvironmentSelection EnvironmentSelection
	Status               Status
	Summary              string
	Duration             time.Duration
	Canceled             bool
	SkipReason           string
	Error                string
	ErrorDetail          string
	ScriptError          string
	ScriptErrorDetail    string
	Failure              *Failure
	HTTP                 *HTTP
	GRPC                 *GRPC
	Stream               *Stream
	Trace                *Trace
	Tests                []Test
	Compare              *Compare
	Profile              *Profile
	Steps                []Step
}

// MarshalJSON writes the canonical result JSON format.
func (r Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.model())
}

// Failed reports whether the result failed. A failed step, profile run or stream counts too.
func (r Result) Failed() bool {
	return r.outcome().effectiveStatus() == StatusFail
}

func (r Result) outcome() outcome {
	return outcome{
		status:    r.Status,
		summary:   r.Summary,
		canceled:  r.Canceled,
		err:       r.Error,
		scriptErr: r.ScriptError,
		failure:   r.Failure,
		stream:    r.Stream,
		trace:     r.Trace,
		tests:     r.Tests,
		profile:   r.Profile,
		steps:     r.Steps,
	}
}

// Step contains one workflow or compare step result.
type Step struct {
	Name                 string
	Method               string
	Target               string
	EffectiveTarget      string
	Environment          string
	EnvironmentSelection EnvironmentSelection
	Branch               string
	Iteration            int
	Total                int
	Status               Status
	Summary              string
	Duration             time.Duration
	Canceled             bool
	SkipReason           string
	Error                string
	ErrorDetail          string
	ScriptError          string
	ScriptErrorDetail    string
	Failure              *Failure
	HTTP                 *HTTP
	GRPC                 *GRPC
	Stream               *Stream
	Trace                *Trace
	Tests                []Test
}

// MarshalJSON writes the canonical step JSON format.
func (s Step) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.model())
}

// Failed reports whether the step represents a failure.
func (s Step) Failed() bool {
	return s.outcome().effectiveStatus() == StatusFail
}

func (s Step) outcome() outcome {
	return outcome{
		status:    s.Status,
		summary:   s.Summary,
		canceled:  s.Canceled,
		err:       s.Error,
		scriptErr: s.ScriptError,
		failure:   s.Failure,
		stream:    s.Stream,
		trace:     s.Trace,
		tests:     s.Tests,
	}
}

// outcome applies the runner's failure rules from internal/runner/failure.go.
type outcome struct {
	status    Status
	summary   string
	canceled  bool
	err       string
	scriptErr string
	failure   *Failure
	stream    *Stream
	trace     *Trace
	tests     []Test
	profile   *Profile
	steps     []Step
}

// skip wins, otherwise any failure evidence makes the result fail.
func (o outcome) effectiveStatus() Status {
	if o.status == StatusSkip {
		return StatusSkip
	}
	failed := o.failure != nil || o.canceled || o.err != "" || o.scriptErr != "" ||
		streamFailed(o.stream) || anyTestFailed(o.tests) || traceFailed(o.trace) ||
		measuredFailure(o.profile) != nil || failedStep(o.steps) != nil
	if o.status == StatusFail || failed {
		return StatusFail
	}
	return StatusPass
}

func streamFailed(stream *Stream) bool {
	return stream != nil && stream.Error != ""
}

func measuredFailure(prof *Profile) *Failure {
	if prof == nil {
		return nil
	}
	for _, f := range prof.Failures {
		if !f.Warmup && f.Failure != nil {
			return f.Failure
		}
	}
	return nil
}

func failedStep(steps []Step) *Step {
	for i := range steps {
		if steps[i].Failed() {
			return &steps[i]
		}
	}
	return nil
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
	Code          string   `json:"code,omitempty"`
	StatusCode    int      `json:"statusCode,omitempty"`
	StatusMessage string   `json:"statusMessage,omitempty"`
	StatusDetails []string `json:"statusDetails,omitempty"`
}

// Test contains one assertion result.
type Test struct {
	Name    string
	Message string
	Passed  bool
	Elapsed time.Duration
}

// MarshalJSON writes the test the way it appears in the report JSON.
func (t Test) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.model())
}

// Compare contains compare-run summary fields.
type Compare struct {
	Baseline string `json:"baseline,omitempty"`
	Group    string `json:"group,omitempty"`
}

// Profile contains profile-run summary fields.
type Profile struct {
	Count          int
	Warmup         int
	Delay          time.Duration
	TotalRuns      int
	WarmupRuns     int
	SuccessfulRuns int
	FailedRuns     int
	Latency        *Latency
	Percentiles    []Percentile
	Histogram      []HistBin
	Failures       []ProfileFailure
}

// MarshalJSON writes the profile the way it appears in the report JSON.
func (p Profile) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.model())
}

// ProfileFailure contains one failed profile iteration.
type ProfileFailure struct {
	Iteration  int
	Warmup     bool
	Reason     string
	Status     string
	StatusCode int
	Duration   time.Duration
	Failure    *Failure
}

// MarshalJSON writes the profile failure the way it appears in the report JSON.
func (p ProfileFailure) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.model())
}

// Latency contains aggregate profile latency statistics.
type Latency struct {
	Count  int
	Min    time.Duration
	Max    time.Duration
	Mean   time.Duration
	Median time.Duration
	StdDev time.Duration
}

// MarshalJSON writes the latency the way it appears in the report JSON.
func (l Latency) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.model())
}

// Percentile contains one profile percentile.
type Percentile struct {
	Percentile int
	Value      time.Duration
}

// MarshalJSON writes the percentile the way it appears in the report JSON.
func (p Percentile) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.model())
}

// HistBin contains one profile histogram bin.
type HistBin struct {
	From  time.Duration
	To    time.Duration
	Count int
}

// MarshalJSON writes the bin the way it appears in the report JSON.
func (h HistBin) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.model())
}

// Stream contains streaming response metadata.
type Stream struct {
	Kind           string
	EventCount     int
	Summary        map[string]any
	TranscriptPath string
	Error          string
}

// MarshalJSON writes the stream the way it appears in the report JSON.
func (s Stream) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.model())
}

// Trace contains trace summary metadata.
type Trace struct {
	Duration     time.Duration
	Error        string
	Budget       *TraceBudget
	Breaches     []TraceBreach
	ArtifactPath string
}

// MarshalJSON writes the trace the way it appears in the report JSON.
func (t Trace) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.model())
}

// TraceBudget contains trace budget limits.
type TraceBudget struct {
	Total     time.Duration
	Tolerance time.Duration
	Phases    map[string]time.Duration
}

// MarshalJSON writes the budget the way it appears in the report JSON.
func (t TraceBudget) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.model())
}

// TraceBreach contains one trace budget breach.
type TraceBreach struct {
	Kind   string
	Limit  time.Duration
	Actual time.Duration
	Over   time.Duration
}

// MarshalJSON writes the breach the way it appears in the report JSON.
func (t TraceBreach) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.model())
}
