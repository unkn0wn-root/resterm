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
	return json.Marshal(r.model())
}

// Result contains one executed request, workflow, compare run, or profile run.
type Result struct {
	Kind                 Kind
	Name                 string
	Method               string
	Target               string
	Environment          string
	EnvironmentSelection EnvironmentSelection
	Status               Status
	Summary              string
	Duration             time.Duration
	Canceled             bool
	SkipReason           string
	Error                string
	ScriptError          string
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

// Failed reports whether the result represents a failure.
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
		trace:     r.Trace,
		tests:     r.Tests,
	}
}

// Step contains one workflow or compare step result.
type Step struct {
	Name                 string
	Method               string
	Target               string
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
	ScriptError          string
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
		trace:     s.Trace,
		tests:     s.Tests,
	}
}

type outcome struct {
	status    Status
	summary   string
	canceled  bool
	err       string
	scriptErr string
	failure   *Failure
	trace     *Trace
	tests     []Test
}

// skip wins, otherwise any failure evidence makes the result fail.
func (o outcome) effectiveStatus() Status {
	if o.status == StatusSkip {
		return StatusSkip
	}
	failed := o.failure != nil || o.canceled || o.err != "" || o.scriptErr != "" ||
		traceFailed(o.trace) || anyTestFailed(o.tests)
	if o.status == StatusFail || failed {
		return StatusFail
	}
	return StatusPass
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
	Name    string        `json:"name,omitempty"`
	Message string        `json:"message,omitempty"`
	Passed  bool          `json:"passed"`
	Elapsed time.Duration `json:"elapsed,omitempty"`
}

// Compare contains compare-run summary fields.
type Compare struct {
	Baseline string `json:"baseline,omitempty"`
	Group    string `json:"group,omitempty"`
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
	Error          string         `json:"error,omitempty"`
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
