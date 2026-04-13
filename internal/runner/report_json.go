package runner

import (
	"encoding/json"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
)

type jsonReport struct {
	Version    string       `json:"version,omitempty"`
	FilePath   string       `json:"filePath"`
	EnvName    string       `json:"envName,omitempty"`
	StartedAt  time.Time    `json:"startedAt"`
	EndedAt    time.Time    `json:"endedAt"`
	DurationMs int64        `json:"durationMs"`
	Summary    jsonSummary  `json:"summary"`
	Results    []jsonResult `json:"results"`
}

type jsonSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

type jsonResult struct {
	Kind        string       `json:"kind,omitempty"`
	Name        string       `json:"name,omitempty"`
	Method      string       `json:"method,omitempty"`
	Target      string       `json:"target,omitempty"`
	Environment string       `json:"environment,omitempty"`
	Status      string       `json:"status"`
	Summary     string       `json:"summary,omitempty"`
	Canceled    bool         `json:"canceled,omitempty"`
	SkipReason  string       `json:"skipReason,omitempty"`
	Error       string       `json:"error,omitempty"`
	ScriptError string       `json:"scriptError,omitempty"`
	DurationMs  int64        `json:"durationMs,omitempty"`
	HTTP        *jsonHTTP    `json:"http,omitempty"`
	GRPC        *jsonGRPC    `json:"grpc,omitempty"`
	Stream      *jsonStream  `json:"stream,omitempty"`
	Trace       *jsonTrace   `json:"trace,omitempty"`
	Tests       []jsonTest   `json:"tests,omitempty"`
	Compare     *jsonCompare `json:"compare,omitempty"`
	Profile     *jsonProfile `json:"profile,omitempty"`
	Steps       []jsonStep   `json:"steps,omitempty"`
}

type jsonHTTP struct {
	Status     string `json:"status,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

type jsonGRPC struct {
	Code          string `json:"code,omitempty"`
	StatusCode    int    `json:"statusCode,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
}

type jsonTest struct {
	Name      string `json:"name,omitempty"`
	Message   string `json:"message,omitempty"`
	Passed    bool   `json:"passed"`
	ElapsedMs int64  `json:"elapsedMs,omitempty"`
}

type jsonCompare struct {
	Baseline string `json:"baseline,omitempty"`
}

type jsonProfile struct {
	Count          int                  `json:"count,omitempty"`
	Warmup         int                  `json:"warmup,omitempty"`
	DelayMs        int64                `json:"delayMs,omitempty"`
	TotalRuns      int                  `json:"totalRuns,omitempty"`
	WarmupRuns     int                  `json:"warmupRuns,omitempty"`
	SuccessfulRuns int                  `json:"successfulRuns,omitempty"`
	FailedRuns     int                  `json:"failedRuns,omitempty"`
	Latency        *jsonProfileLatency  `json:"latency,omitempty"`
	Percentiles    []jsonPercentile     `json:"percentiles,omitempty"`
	Histogram      []jsonHistogramBin   `json:"histogram,omitempty"`
	Failures       []jsonProfileFailure `json:"failures,omitempty"`
}

type jsonProfileLatency struct {
	Count    int   `json:"count,omitempty"`
	MinMs    int64 `json:"minMs,omitempty"`
	MaxMs    int64 `json:"maxMs,omitempty"`
	MeanMs   int64 `json:"meanMs,omitempty"`
	MedianMs int64 `json:"medianMs,omitempty"`
	StdDevMs int64 `json:"stdDevMs,omitempty"`
}

type jsonPercentile struct {
	Percentile int   `json:"percentile"`
	ValueMs    int64 `json:"valueMs,omitempty"`
}

type jsonHistogramBin struct {
	FromMs int64 `json:"fromMs,omitempty"`
	ToMs   int64 `json:"toMs,omitempty"`
	Count  int   `json:"count,omitempty"`
}

type jsonProfileFailure struct {
	Iteration  int    `json:"iteration,omitempty"`
	Warmup     bool   `json:"warmup,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Status     string `json:"status,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
}

type jsonStream struct {
	Kind           string         `json:"kind,omitempty"`
	EventCount     int            `json:"eventCount,omitempty"`
	Summary        map[string]any `json:"summary,omitempty"`
	TranscriptPath string         `json:"transcriptPath,omitempty"`
}

type jsonTrace struct {
	DurationMs   int64             `json:"durationMs,omitempty"`
	Error        string            `json:"error,omitempty"`
	Budgets      *jsonTraceBudget  `json:"budgets,omitempty"`
	Breaches     []jsonTraceBreach `json:"breaches,omitempty"`
	ArtifactPath string            `json:"artifactPath,omitempty"`
}

type jsonTraceBudget struct {
	TotalMs     int64            `json:"totalMs,omitempty"`
	ToleranceMs int64            `json:"toleranceMs,omitempty"`
	Phases      map[string]int64 `json:"phases,omitempty"`
}

type jsonTraceBreach struct {
	Kind     string `json:"kind,omitempty"`
	LimitMs  int64  `json:"limitMs,omitempty"`
	ActualMs int64  `json:"actualMs,omitempty"`
	OverMs   int64  `json:"overMs,omitempty"`
}

type jsonStep struct {
	Name        string      `json:"name,omitempty"`
	Method      string      `json:"method,omitempty"`
	Target      string      `json:"target,omitempty"`
	Environment string      `json:"environment,omitempty"`
	Branch      string      `json:"branch,omitempty"`
	Iteration   int         `json:"iteration,omitempty"`
	Total       int         `json:"total,omitempty"`
	Status      string      `json:"status"`
	Summary     string      `json:"summary,omitempty"`
	Canceled    bool        `json:"canceled,omitempty"`
	SkipReason  string      `json:"skipReason,omitempty"`
	Error       string      `json:"error,omitempty"`
	ScriptError string      `json:"scriptError,omitempty"`
	DurationMs  int64       `json:"durationMs,omitempty"`
	HTTP        *jsonHTTP   `json:"http,omitempty"`
	GRPC        *jsonGRPC   `json:"grpc,omitempty"`
	Stream      *jsonStream `json:"stream,omitempty"`
	Trace       *jsonTrace  `json:"trace,omitempty"`
	Tests       []jsonTest  `json:"tests,omitempty"`
}

func (r *Report) WriteJSON(w io.Writer) error {
	if r == nil {
		return nil
	}
	if w == nil {
		w = io.Discard
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r.json())
}

func (r *Report) json() jsonReport {
	out := jsonReport{
		Version:    strings.TrimSpace(r.Version),
		FilePath:   r.FilePath,
		EnvName:    strings.TrimSpace(r.EnvName),
		StartedAt:  r.StartedAt,
		EndedAt:    r.EndedAt,
		DurationMs: durMS(r.Duration),
		Summary: jsonSummary{
			Total:   r.Total,
			Passed:  r.Passed,
			Failed:  r.Failed,
			Skipped: r.Skipped,
		},
		Results: make([]jsonResult, 0, len(r.Results)),
	}
	for _, item := range r.Results {
		out.Results = append(out.Results, item.json())
	}
	return out
}

func (item Result) json() jsonResult {
	out := jsonResult{
		Kind:        string(item.Kind),
		Name:        strings.TrimSpace(item.Name),
		Method:      requestMethodValue(item.Method),
		Target:      strings.TrimSpace(item.Target),
		Environment: strings.TrimSpace(item.Environment),
		Status:      jsonResultStatus(item),
		Summary:     strings.TrimSpace(item.Summary),
		Canceled:    item.Canceled,
		SkipReason:  strings.TrimSpace(item.SkipReason),
		DurationMs:  durMS(resultDuration(item)),
	}
	if item.Err != nil {
		out.Error = item.Err.Error()
	}
	if item.ScriptErr != nil {
		out.ScriptError = item.ScriptErr.Error()
	}
	if item.Response != nil {
		out.HTTP = &jsonHTTP{
			Status:     strings.TrimSpace(item.Response.Status),
			StatusCode: item.Response.StatusCode,
			Protocol:   strings.TrimSpace(item.Response.Proto),
		}
	}
	if item.GRPC != nil {
		out.GRPC = &jsonGRPC{
			Code:          item.GRPC.StatusCode.String(),
			StatusCode:    int(item.GRPC.StatusCode),
			StatusMessage: strings.TrimSpace(item.GRPC.StatusMessage),
		}
	}
	if item.Stream != nil {
		out.Stream = jsonStreamInfo(item.Stream)
	}
	if item.Trace != nil {
		out.Trace = jsonTraceInfo(item.Trace)
	}
	if len(item.Tests) > 0 {
		out.Tests = make([]jsonTest, 0, len(item.Tests))
		for _, test := range item.Tests {
			out.Tests = append(out.Tests, jsonTest{
				Name:      strings.TrimSpace(test.Name),
				Message:   strings.TrimSpace(test.Message),
				Passed:    test.Passed,
				ElapsedMs: durMS(test.Elapsed),
			})
		}
	}
	if item.Compare != nil {
		out.Compare = &jsonCompare{Baseline: strings.TrimSpace(item.Compare.Baseline)}
	}
	if item.Profile != nil {
		out.Profile = jsonProfileInfo(item.Profile)
	}
	if len(item.Steps) > 0 {
		out.Steps = make([]jsonStep, 0, len(item.Steps))
		for _, step := range item.Steps {
			out.Steps = append(out.Steps, step.json())
		}
	}
	return out
}

func (step StepResult) json() jsonStep {
	out := jsonStep{
		Name:        strings.TrimSpace(step.Name),
		Method:      requestMethodValue(step.Method),
		Target:      strings.TrimSpace(step.Target),
		Environment: strings.TrimSpace(step.Environment),
		Branch:      strings.TrimSpace(step.Branch),
		Iteration:   step.Iteration,
		Total:       step.Total,
		Status:      jsonStepStatus(step),
		Summary:     strings.TrimSpace(step.Summary),
		Canceled:    step.Canceled,
		SkipReason:  strings.TrimSpace(step.SkipReason),
		DurationMs:  durMS(step.Duration),
	}
	if step.Err != nil {
		out.Error = step.Err.Error()
	}
	if step.ScriptErr != nil {
		out.ScriptError = step.ScriptErr.Error()
	}
	if step.Response != nil {
		out.HTTP = &jsonHTTP{
			Status:     strings.TrimSpace(step.Response.Status),
			StatusCode: step.Response.StatusCode,
			Protocol:   strings.TrimSpace(step.Response.Proto),
		}
	}
	if step.GRPC != nil {
		out.GRPC = &jsonGRPC{
			Code:          step.GRPC.StatusCode.String(),
			StatusCode:    int(step.GRPC.StatusCode),
			StatusMessage: strings.TrimSpace(step.GRPC.StatusMessage),
		}
	}
	if step.Stream != nil {
		out.Stream = jsonStreamInfo(step.Stream)
	}
	if step.Trace != nil {
		out.Trace = jsonTraceInfo(step.Trace)
	}
	if len(step.Tests) > 0 {
		out.Tests = make([]jsonTest, 0, len(step.Tests))
		for _, test := range step.Tests {
			out.Tests = append(out.Tests, jsonTest{
				Name:      strings.TrimSpace(test.Name),
				Message:   strings.TrimSpace(test.Message),
				Passed:    test.Passed,
				ElapsedMs: durMS(test.Elapsed),
			})
		}
	}
	return out
}

func jsonResultStatus(item Result) string {
	if item.Skipped {
		return "skip"
	}
	if resultFailed(item) {
		return "fail"
	}
	return "pass"
}

func jsonStepStatus(step StepResult) string {
	if step.Skipped {
		return "skip"
	}
	if stepFailed(step) {
		return "fail"
	}
	return "pass"
}

func jsonProfileInfo(prof *ProfileInfo) *jsonProfile {
	if prof == nil {
		return nil
	}
	out := &jsonProfile{
		Count:   prof.Count,
		Warmup:  prof.Warmup,
		DelayMs: durMS(prof.Delay),
	}
	if prof.Results != nil {
		out.TotalRuns = prof.Results.TotalRuns
		out.WarmupRuns = prof.Results.WarmupRuns
		out.SuccessfulRuns = prof.Results.SuccessfulRuns
		out.FailedRuns = prof.Results.FailedRuns
		out.Latency = jsonProfileLatencyInfo(prof.Results.Latency)
		out.Percentiles = jsonPercentiles(prof.Results.Percentiles)
		out.Histogram = jsonHistogram(prof.Results.Histogram)
	}
	if len(prof.Failures) > 0 {
		out.Failures = make([]jsonProfileFailure, 0, len(prof.Failures))
		for _, failure := range prof.Failures {
			out.Failures = append(out.Failures, jsonProfileFailure{
				Iteration:  failure.Iteration,
				Warmup:     failure.Warmup,
				Reason:     strings.TrimSpace(failure.Reason),
				Status:     strings.TrimSpace(failure.Status),
				StatusCode: failure.StatusCode,
				DurationMs: durMS(failure.Duration),
			})
		}
	}
	return out
}

func jsonProfileLatencyInfo(lat *history.ProfileLatency) *jsonProfileLatency {
	if lat == nil {
		return nil
	}
	return &jsonProfileLatency{
		Count:    lat.Count,
		MinMs:    durMS(lat.Min),
		MaxMs:    durMS(lat.Max),
		MeanMs:   durMS(lat.Mean),
		MedianMs: durMS(lat.Median),
		StdDevMs: durMS(lat.StdDev),
	}
}

func jsonPercentiles(src []history.ProfilePercentile) []jsonPercentile {
	if len(src) == 0 {
		return nil
	}
	items := append([]history.ProfilePercentile(nil), src...)
	sort.Slice(items, func(i, j int) bool { return items[i].Percentile < items[j].Percentile })
	out := make([]jsonPercentile, 0, len(items))
	for _, item := range items {
		out = append(out, jsonPercentile{
			Percentile: item.Percentile,
			ValueMs:    durMS(item.Value),
		})
	}
	return out
}

func jsonHistogram(src []history.ProfileHistogramBin) []jsonHistogramBin {
	if len(src) == 0 {
		return nil
	}
	out := make([]jsonHistogramBin, 0, len(src))
	for _, item := range src {
		out = append(out, jsonHistogramBin{
			FromMs: durMS(item.From),
			ToMs:   durMS(item.To),
			Count:  item.Count,
		})
	}
	return out
}

func jsonStreamInfo(info *StreamInfo) *jsonStream {
	if info == nil {
		return nil
	}
	out := &jsonStream{
		Kind:           strings.TrimSpace(info.Kind),
		EventCount:     info.EventCount,
		TranscriptPath: strings.TrimSpace(info.TranscriptPath),
	}
	if len(info.Summary) > 0 {
		out.Summary = jsonAnyMap(info.Summary)
	}
	return out
}

func jsonTraceInfo(info *TraceInfo) *jsonTrace {
	if info == nil || info.Summary == nil {
		return nil
	}
	out := &jsonTrace{
		DurationMs:   durMS(info.Summary.Duration),
		Error:        strings.TrimSpace(info.Summary.Error),
		ArtifactPath: strings.TrimSpace(info.ArtifactPath),
	}
	if bud := info.Summary.Budgets; bud != nil {
		out.Budgets = &jsonTraceBudget{
			TotalMs:     durMS(bud.Total),
			ToleranceMs: durMS(bud.Tolerance),
		}
		if len(bud.Phases) > 0 {
			out.Budgets.Phases = make(map[string]int64, len(bud.Phases))
			for key, value := range bud.Phases {
				out.Budgets.Phases[key] = durMS(value)
			}
		}
	}
	if len(info.Summary.Breaches) > 0 {
		out.Breaches = make([]jsonTraceBreach, 0, len(info.Summary.Breaches))
		for _, breach := range info.Summary.Breaches {
			out.Breaches = append(out.Breaches, jsonTraceBreach{
				Kind:     strings.TrimSpace(breach.Kind),
				LimitMs:  durMS(breach.Limit),
				ActualMs: durMS(breach.Actual),
				OverMs:   durMS(breach.Over),
			})
		}
	}
	return out
}

func jsonAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for key, value := range src {
		out[key] = jsonAnyValue(value)
	}
	return out
}

func jsonAnyValue(v any) any {
	switch x := v.(type) {
	case time.Duration:
		return durMS(x)
	case map[string]any:
		return jsonAnyMap(x)
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, jsonAnyValue(item))
		}
		return out
	default:
		return x
	}
}

func durMS(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	return d.Milliseconds()
}
