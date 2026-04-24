package runfmt

import (
	"time"

	"github.com/unkn0wn-root/resterm/internal/runfail"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

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
	StopReason    string
}

type Result struct {
	Kind            string
	Name            string
	Method          string
	Target          string
	EffectiveTarget string
	Environment     string
	Status          Status
	Summary         string
	Duration        time.Duration
	Canceled        bool
	SkipReason      string
	Error           string
	ScriptError     string
	Failure         *Failure
	HTTP            *HTTP
	GRPC            *GRPC
	Stream          *Stream
	Trace           *Trace
	Tests           []Test
	Compare         *Compare
	Profile         *Profile
	Steps           []Step
}

type Step struct {
	Name            string
	Method          string
	Target          string
	EffectiveTarget string
	Environment     string
	Branch          string
	Iteration       int
	Total           int
	Status          Status
	Summary         string
	Duration        time.Duration
	Canceled        bool
	SkipReason      string
	Error           string
	ScriptError     string
	Failure         *Failure
	HTTP            *HTTP
	GRPC            *GRPC
	Stream          *Stream
	Trace           *Trace
	Tests           []Test
}

type HTTP struct {
	Status     string
	StatusCode int
	Protocol   string
}

type GRPC struct {
	Code          string
	StatusCode    int
	StatusMessage string
}

type Test struct {
	Name    string
	Message string
	Passed  bool
	Elapsed time.Duration
}

type Compare struct {
	Baseline string
}

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

type ProfileFailure struct {
	Iteration  int
	Warmup     bool
	Reason     string
	Status     string
	StatusCode int
	Duration   time.Duration
	Failure    *Failure
}

type Failure struct {
	Code     runfail.Code     `json:"code,omitempty"`
	Category runfail.Category `json:"category,omitempty"`
	ExitCode int              `json:"exitCode,omitempty"`
	Message  string           `json:"message,omitempty"`
	Source   string           `json:"source,omitempty"`
}

type Latency struct {
	Count  int
	Min    time.Duration
	Max    time.Duration
	Mean   time.Duration
	Median time.Duration
	StdDev time.Duration
}

type Percentile struct {
	Percentile int
	Value      time.Duration
}

type HistBin struct {
	From  time.Duration
	To    time.Duration
	Count int
}

type Stream struct {
	Kind           string
	EventCount     int
	Summary        map[string]any
	TranscriptPath string
}

type Trace struct {
	Duration     time.Duration
	Error        string
	Budget       *TraceBudget
	Breaches     []TraceBreach
	ArtifactPath string
}

type TraceBudget struct {
	Total     time.Duration
	Tolerance time.Duration
	Phases    map[string]time.Duration
}

type TraceBreach struct {
	Kind   string
	Limit  time.Duration
	Actual time.Duration
	Over   time.Duration
}

func traceFailed(info *Trace) bool {
	return info != nil && len(info.Breaches) > 0
}
