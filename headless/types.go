package headless

import (
	"errors"
	"time"
)

type EnvSet map[string]map[string]string

type Opt struct {
	Version        string   `json:"version,omitempty"`
	FilePath       string   `json:"filePath,omitempty"`
	FileContent    []byte   `json:"-"`
	Workspace      string   `json:"workspace,omitempty"`
	Recursive      bool     `json:"recursive,omitempty"`
	ArtifactDir    string   `json:"artifactDir,omitempty"`
	StateDir       string   `json:"stateDir,omitempty"`
	PersistGlobals bool     `json:"persistGlobals,omitempty"`
	PersistAuth    bool     `json:"persistAuth,omitempty"`
	History        bool     `json:"history,omitempty"`
	Envs           EnvSet   `json:"envs,omitempty"`
	EnvName        string   `json:"envName,omitempty"`
	EnvFile        string   `json:"envFile,omitempty"`
	CompareTargets []string `json:"compareTargets,omitempty"`
	CompareBase    string   `json:"compareBase,omitempty"`
	Profile        bool     `json:"profile,omitempty"`
	HTTP           HTTPOpt  `json:"http,omitempty"`
	GRPC           GRPCOpt  `json:"grpc,omitempty"`
	Select         Select   `json:"select,omitempty"`
}

type Select struct {
	Request  string `json:"request,omitempty"`
	Workflow string `json:"workflow,omitempty"`
	Tag      string `json:"tag,omitempty"`
	All      bool   `json:"all,omitempty"`
}

type HTTPOpt struct {
	Timeout  time.Duration `json:"timeout,omitempty"`
	Follow   *bool         `json:"follow,omitempty"`
	Insecure bool          `json:"insecure,omitempty"`
	Proxy    string        `json:"proxy,omitempty"`
}

type GRPCOpt struct {
	Plaintext *bool `json:"plaintext,omitempty"`
}

type Kind string

const (
	KindRequest  Kind = "request"
	KindWorkflow Kind = "workflow"
	KindForEach  Kind = "for-each"
	KindCompare  Kind = "compare"
	KindProfile  Kind = "profile"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPass, StatusFail, StatusSkip:
		return true
	default:
		return false
	}
}

// Failed reports whether the result represents a failure.
func (r Result) Failed() bool {
	if r.Status == StatusSkip {
		return false
	}
	return r.Status == StatusFail || r.Canceled || r.Error != "" || r.ScriptError != "" ||
		traceFailed(r.Trace) || anyTestFailed(r.Tests)
}

// Failed reports whether the step represents a failure.
func (s Step) Failed() bool {
	if s.Status == StatusSkip {
		return false
	}
	return s.Status == StatusFail || s.Canceled || s.Error != "" || s.ScriptError != "" ||
		traceFailed(s.Trace) || anyTestFailed(s.Tests)
}

type Report struct {
	Version   string        `json:"version,omitempty"`
	FilePath  string        `json:"filePath"`
	EnvName   string        `json:"envName,omitempty"`
	StartedAt time.Time     `json:"startedAt"`
	EndedAt   time.Time     `json:"endedAt"`
	Duration  time.Duration `json:"duration,omitempty"`
	Results   []Result      `json:"results,omitempty"`
	Total     int           `json:"total"`
	Passed    int           `json:"passed"`
	Failed    int           `json:"failed"`
	Skipped   int           `json:"skipped"`
}

// HasFailures reports whether the report contains any failed results.
func (r *Report) HasFailures() bool {
	if r == nil {
		return false
	}
	if r.Failed > 0 {
		return true
	}
	for _, item := range r.Results {
		if item.Failed() {
			return true
		}
	}
	return false
}

type Result struct {
	Kind        Kind          `json:"kind,omitempty"`
	Name        string        `json:"name,omitempty"`
	Method      string        `json:"method,omitempty"`
	Target      string        `json:"target,omitempty"`
	Environment string        `json:"environment,omitempty"`
	Status      Status        `json:"status"`
	Summary     string        `json:"summary,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	Canceled    bool          `json:"canceled,omitempty"`
	SkipReason  string        `json:"skipReason,omitempty"`
	Error       string        `json:"error,omitempty"`
	ScriptError string        `json:"scriptError,omitempty"`
	HTTP        *HTTP         `json:"http,omitempty"`
	GRPC        *GRPC         `json:"grpc,omitempty"`
	Stream      *Stream       `json:"stream,omitempty"`
	Trace       *Trace        `json:"trace,omitempty"`
	Tests       []Test        `json:"tests,omitempty"`
	Compare     *Compare      `json:"compare,omitempty"`
	Profile     *Profile      `json:"profile,omitempty"`
	Steps       []Step        `json:"steps,omitempty"`
}

type Step struct {
	Name        string        `json:"name,omitempty"`
	Method      string        `json:"method,omitempty"`
	Target      string        `json:"target,omitempty"`
	Environment string        `json:"environment,omitempty"`
	Branch      string        `json:"branch,omitempty"`
	Iteration   int           `json:"iteration,omitempty"`
	Total       int           `json:"total,omitempty"`
	Status      Status        `json:"status"`
	Summary     string        `json:"summary,omitempty"`
	Duration    time.Duration `json:"duration,omitempty"`
	Canceled    bool          `json:"canceled,omitempty"`
	SkipReason  string        `json:"skipReason,omitempty"`
	Error       string        `json:"error,omitempty"`
	ScriptError string        `json:"scriptError,omitempty"`
	HTTP        *HTTP         `json:"http,omitempty"`
	GRPC        *GRPC         `json:"grpc,omitempty"`
	Stream      *Stream       `json:"stream,omitempty"`
	Trace       *Trace        `json:"trace,omitempty"`
	Tests       []Test        `json:"tests,omitempty"`
}

type HTTP struct {
	Status     string `json:"status,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

type GRPC struct {
	Code          string `json:"code,omitempty"`
	StatusCode    int    `json:"statusCode,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
}

type Test struct {
	Name    string        `json:"name,omitempty"`
	Message string        `json:"message,omitempty"`
	Passed  bool          `json:"passed"`
	Elapsed time.Duration `json:"elapsed,omitempty"`
}

type Compare struct {
	Baseline string `json:"baseline,omitempty"`
}

type Profile struct {
	Count          int           `json:"count,omitempty"`
	Warmup         int           `json:"warmup,omitempty"`
	Delay          time.Duration `json:"delay,omitempty"`
	TotalRuns      int           `json:"totalRuns,omitempty"`
	WarmupRuns     int           `json:"warmupRuns,omitempty"`
	SuccessfulRuns int           `json:"successfulRuns,omitempty"`
	FailedRuns     int           `json:"failedRuns,omitempty"`
	Latency        *Latency      `json:"latency,omitempty"`
	Percentiles    []Percentile  `json:"percentiles,omitempty"`
	Histogram      []HistBin     `json:"histogram,omitempty"`
	Failures       []ProfileFail `json:"failures,omitempty"`
}

type ProfileFail struct {
	Iteration  int           `json:"iteration,omitempty"`
	Warmup     bool          `json:"warmup,omitempty"`
	Reason     string        `json:"reason,omitempty"`
	Status     string        `json:"status,omitempty"`
	StatusCode int           `json:"statusCode,omitempty"`
	Duration   time.Duration `json:"duration,omitempty"`
}

type Latency struct {
	Count  int           `json:"count,omitempty"`
	Min    time.Duration `json:"min,omitempty"`
	Max    time.Duration `json:"max,omitempty"`
	Mean   time.Duration `json:"mean,omitempty"`
	Median time.Duration `json:"median,omitempty"`
	StdDev time.Duration `json:"stdDev,omitempty"`
}

type Percentile struct {
	Percentile int           `json:"percentile"`
	Value      time.Duration `json:"value,omitempty"`
}

type HistBin struct {
	From  time.Duration `json:"from,omitempty"`
	To    time.Duration `json:"to,omitempty"`
	Count int           `json:"count,omitempty"`
}

type Stream struct {
	Kind           string         `json:"kind,omitempty"`
	EventCount     int            `json:"eventCount,omitempty"`
	Summary        map[string]any `json:"summary,omitempty"`
	TranscriptPath string         `json:"transcriptPath,omitempty"`
}

type Trace struct {
	Duration     time.Duration `json:"duration,omitempty"`
	Error        string        `json:"error,omitempty"`
	Budget       *TraceBudget  `json:"budget,omitempty"`
	Breaches     []TraceBreach `json:"breaches,omitempty"`
	ArtifactPath string        `json:"artifactPath,omitempty"`
}

type TraceBudget struct {
	Total     time.Duration            `json:"total,omitempty"`
	Tolerance time.Duration            `json:"tolerance,omitempty"`
	Phases    map[string]time.Duration `json:"phases,omitempty"`
}

type TraceBreach struct {
	Kind   string        `json:"kind,omitempty"`
	Limit  time.Duration `json:"limit,omitempty"`
	Actual time.Duration `json:"actual,omitempty"`
	Over   time.Duration `json:"over,omitempty"`
}

func traceFailed(info *Trace) bool {
	return info != nil && len(info.Breaches) > 0
}

func anyTestFailed(tests []Test) bool {
	for _, test := range tests {
		if !test.Passed {
			return true
		}
	}
	return false
}

// ErrUsage reports invalid input or options passed to the headless API.
type ErrUsage struct {
	err error
}

var ErrNilWriter = errors.New("headless: nil writer")

func (e ErrUsage) Error() string {
	if e.err == nil {
		return "usage error"
	}
	return e.err.Error()
}

func (e ErrUsage) Unwrap() error {
	return e.err
}

func IsUsageError(err error) bool {
	var target ErrUsage
	return errors.As(err, &target)
}
