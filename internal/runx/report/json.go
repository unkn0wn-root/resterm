package runfmt

import (
	"encoding/json"
	"io"
	"sort"
	"time"
)

type jsonReport struct {
	SchemaVersion string       `json:"schemaVersion"`
	Version       string       `json:"version,omitempty"`
	FilePath      string       `json:"filePath"`
	EnvName       string       `json:"envName,omitempty"`
	StartedAt     time.Time    `json:"startedAt"`
	EndedAt       time.Time    `json:"endedAt"`
	DurationMs    int64        `json:"durationMs"`
	Summary       jsonSummary  `json:"summary"`
	Results       []jsonResult `json:"results"`
}

type jsonSummary struct {
	Total        int      `json:"total"`
	Passed       int      `json:"passed"`
	Failed       int      `json:"failed"`
	Skipped      int      `json:"skipped"`
	StopReason   string   `json:"stopReason,omitempty"`
	ExitCode     int      `json:"exitCode"`
	FailureCodes []string `json:"failureCodes,omitempty"`
}

type jsonResult struct {
	Kind            string       `json:"kind,omitempty"`
	Name            string       `json:"name,omitempty"`
	Method          string       `json:"method,omitempty"`
	Target          string       `json:"target,omitempty"`
	EffectiveTarget string       `json:"effectiveTarget,omitempty"`
	Environment     string       `json:"environment,omitempty"`
	Status          string       `json:"status"`
	Summary         string       `json:"summary,omitempty"`
	Canceled        bool         `json:"canceled,omitempty"`
	SkipReason      string       `json:"skipReason,omitempty"`
	Error           string       `json:"error,omitempty"`
	ScriptError     string       `json:"scriptError,omitempty"`
	Failure         *jsonFailure `json:"failure,omitempty"`
	DurationMs      int64        `json:"durationMs,omitempty"`
	HTTP            *jsonHTTP    `json:"http,omitempty"`
	GRPC            *jsonGRPC    `json:"grpc,omitempty"`
	Stream          *jsonStream  `json:"stream,omitempty"`
	Trace           *jsonTrace   `json:"trace,omitempty"`
	Tests           []jsonTest   `json:"tests,omitempty"`
	Compare         *jsonCompare `json:"compare,omitempty"`
	Profile         *jsonProfile `json:"profile,omitempty"`
	Steps           []jsonStep   `json:"steps,omitempty"`
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

type jsonFailure struct {
	Code     string             `json:"code,omitempty"`
	Category string             `json:"category,omitempty"`
	ExitCode int                `json:"exitCode,omitempty"`
	Message  string             `json:"message,omitempty"`
	Source   string             `json:"source,omitempty"`
	Chain    []jsonFailureChain `json:"chain,omitempty"`
	Frames   []jsonFailureFrame `json:"frames,omitempty"`
}

type jsonFailureChain struct {
	Code      string             `json:"code,omitempty"`
	Component string             `json:"component,omitempty"`
	Kind      string             `json:"kind,omitempty"`
	Message   string             `json:"message,omitempty"`
	Children  []jsonFailureChain `json:"children,omitempty"`
}

type jsonFailureFrame struct {
	Name string          `json:"name,omitempty"`
	Pos  *jsonFailurePos `json:"pos,omitempty"`
}

type jsonFailurePos struct {
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
	Col  int    `json:"col,omitempty"`
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
	Latency        *jsonLatency         `json:"latency,omitempty"`
	Percentiles    []jsonPercentile     `json:"percentiles,omitempty"`
	Histogram      []jsonHistBin        `json:"histogram,omitempty"`
	Failures       []jsonProfileFailure `json:"failures,omitempty"`
}

type jsonLatency struct {
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

type jsonHistBin struct {
	FromMs int64 `json:"fromMs,omitempty"`
	ToMs   int64 `json:"toMs,omitempty"`
	Count  int   `json:"count,omitempty"`
}

type jsonProfileFailure struct {
	Iteration  int          `json:"iteration,omitempty"`
	Warmup     bool         `json:"warmup,omitempty"`
	Reason     string       `json:"reason,omitempty"`
	Status     string       `json:"status,omitempty"`
	StatusCode int          `json:"statusCode,omitempty"`
	DurationMs int64        `json:"durationMs,omitempty"`
	Failure    *jsonFailure `json:"failure,omitempty"`
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
	Name            string       `json:"name,omitempty"`
	Method          string       `json:"method,omitempty"`
	Target          string       `json:"target,omitempty"`
	EffectiveTarget string       `json:"effectiveTarget,omitempty"`
	Environment     string       `json:"environment,omitempty"`
	Branch          string       `json:"branch,omitempty"`
	Iteration       int          `json:"iteration,omitempty"`
	Total           int          `json:"total,omitempty"`
	Status          string       `json:"status"`
	Summary         string       `json:"summary,omitempty"`
	Canceled        bool         `json:"canceled,omitempty"`
	SkipReason      string       `json:"skipReason,omitempty"`
	Error           string       `json:"error,omitempty"`
	ScriptError     string       `json:"scriptError,omitempty"`
	Failure         *jsonFailure `json:"failure,omitempty"`
	DurationMs      int64        `json:"durationMs,omitempty"`
	HTTP            *jsonHTTP    `json:"http,omitempty"`
	GRPC            *jsonGRPC    `json:"grpc,omitempty"`
	Stream          *jsonStream  `json:"stream,omitempty"`
	Trace           *jsonTrace   `json:"trace,omitempty"`
	Tests           []jsonTest   `json:"tests,omitempty"`
}

func WriteJSON(w io.Writer, rep *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep.json())
}

func (rep Report) MarshalJSON() ([]byte, error) {
	return json.Marshal(rep.json())
}

func (rep Report) json() jsonReport {
	out := jsonReport{
		SchemaVersion: schemaVersion(rep.SchemaVersion),
		Version:       rep.Version,
		FilePath:      rep.FilePath,
		EnvName:       rep.EnvName,
		StartedAt:     rep.StartedAt,
		EndedAt:       rep.EndedAt,
		DurationMs:    durMS(rep.Duration),
		Summary: jsonSummary{
			Total:        rep.Total,
			Passed:       rep.Passed,
			Failed:       rep.Failed,
			Skipped:      rep.Skipped,
			StopReason:   rep.StopReason,
			ExitCode:     rep.ExitCode(""),
			FailureCodes: rep.FailureCodes(),
		},
		Results: make([]jsonResult, 0, len(rep.Results)),
	}
	for _, res := range rep.Results {
		out.Results = append(out.Results, res.json())
	}
	return out
}

func (res Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(res.json())
}

func (res Result) json() jsonResult {
	out := jsonResult{
		Kind:            res.Kind,
		Name:            res.Name,
		Method:          requestMethodValue(res.Method),
		Target:          res.Target,
		EffectiveTarget: effectiveTargetValue(res.Target, res.EffectiveTarget),
		Environment:     res.Environment,
		Status:          jsonStatus(res.Status),
		Summary:         res.Summary,
		Canceled:        res.Canceled,
		SkipReason:      res.SkipReason,
		Error:           res.Error,
		ScriptError:     res.ScriptError,
		Failure:         res.Failure.json(),
		DurationMs:      durMS(res.Duration),
		HTTP:            res.HTTP.json(),
		GRPC:            res.GRPC.json(),
		Stream:          res.Stream.json(),
		Trace:           res.Trace.json(),
		Compare:         res.Compare.json(),
		Profile:         res.Profile.json(),
	}
	if len(res.Tests) > 0 {
		out.Tests = make([]jsonTest, 0, len(res.Tests))
		for _, test := range res.Tests {
			out.Tests = append(out.Tests, test.json())
		}
	}
	if len(res.Steps) > 0 {
		out.Steps = make([]jsonStep, 0, len(res.Steps))
		for _, step := range res.Steps {
			out.Steps = append(out.Steps, step.json())
		}
	}
	return out
}

func (step Step) MarshalJSON() ([]byte, error) {
	return json.Marshal(step.json())
}

func (step Step) json() jsonStep {
	out := jsonStep{
		Name:            step.Name,
		Method:          requestMethodValue(step.Method),
		Target:          step.Target,
		EffectiveTarget: effectiveTargetValue(step.Target, step.EffectiveTarget),
		Environment:     step.Environment,
		Branch:          step.Branch,
		Iteration:       step.Iteration,
		Total:           step.Total,
		Status:          jsonStatus(step.Status),
		Summary:         step.Summary,
		Canceled:        step.Canceled,
		SkipReason:      step.SkipReason,
		Error:           step.Error,
		ScriptError:     step.ScriptError,
		Failure:         step.Failure.json(),
		DurationMs:      durMS(step.Duration),
		HTTP:            step.HTTP.json(),
		GRPC:            step.GRPC.json(),
		Stream:          step.Stream.json(),
		Trace:           step.Trace.json(),
	}
	if len(step.Tests) > 0 {
		out.Tests = make([]jsonTest, 0, len(step.Tests))
		for _, test := range step.Tests {
			out.Tests = append(out.Tests, test.json())
		}
	}
	return out
}

func jsonStatus(status Status) string {
	switch status {
	case StatusSkip:
		return "skip"
	case StatusFail:
		return "fail"
	default:
		return "pass"
	}
}

func (http *HTTP) json() *jsonHTTP {
	if http == nil {
		return nil
	}
	return &jsonHTTP{
		Status:     http.Status,
		StatusCode: http.StatusCode,
		Protocol:   http.Protocol,
	}
}

func (grpc *GRPC) json() *jsonGRPC {
	if grpc == nil {
		return nil
	}
	return &jsonGRPC{
		Code:          grpc.Code,
		StatusCode:    grpc.StatusCode,
		StatusMessage: grpc.StatusMessage,
	}
}

func (failure *Failure) json() *jsonFailure {
	if failure == nil {
		return nil
	}
	return &jsonFailure{
		Code:     string(failure.Code),
		Category: string(failure.Category),
		ExitCode: failure.ExitCode,
		Message:  failure.Message,
		Source:   failure.Source,
		Chain:    failureChainJSON(failure.Chain),
		Frames:   failureFramesJSON(failure.Frames),
	}
}

func failureChainJSON(src []FailureChain) []jsonFailureChain {
	if len(src) == 0 {
		return nil
	}
	chain := make([]jsonFailureChain, len(src))
	for i, entry := range src {
		chain[i] = jsonFailureChain{
			Code:      entry.Code,
			Component: entry.Component,
			Kind:      entry.Kind,
			Message:   entry.Message,
			Children:  failureChainJSON(entry.Children),
		}
	}
	return chain
}

func failureFramesJSON(src []FailureFrame) []jsonFailureFrame {
	if len(src) == 0 {
		return nil
	}
	frames := make([]jsonFailureFrame, len(src))
	for i, frame := range src {
		frames[i] = jsonFailureFrame{
			Name: frame.Name,
			Pos:  failurePosJSON(frame.Pos),
		}
	}
	return frames
}

func failurePosJSON(pos FailurePos) *jsonFailurePos {
	if pos == (FailurePos{}) {
		return nil
	}
	return &jsonFailurePos{Path: pos.Path, Line: pos.Line, Col: pos.Col}
}

func (test Test) json() jsonTest {
	return jsonTest{
		Name:      test.Name,
		Message:   test.Message,
		Passed:    test.Passed,
		ElapsedMs: durMS(test.Elapsed),
	}
}

func (cmp *Compare) json() *jsonCompare {
	if cmp == nil {
		return nil
	}
	return &jsonCompare{Baseline: cmp.Baseline}
}

func (prof *Profile) json() *jsonProfile {
	if prof == nil {
		return nil
	}
	out := &jsonProfile{
		Count:          prof.Count,
		Warmup:         prof.Warmup,
		DelayMs:        durMS(prof.Delay),
		TotalRuns:      prof.TotalRuns,
		WarmupRuns:     prof.WarmupRuns,
		SuccessfulRuns: prof.SuccessfulRuns,
		FailedRuns:     prof.FailedRuns,
		Latency:        prof.Latency.json(),
	}
	if len(prof.Percentiles) > 0 {
		items := append([]Percentile(nil), prof.Percentiles...)
		sort.Slice(items, func(i, j int) bool { return items[i].Percentile < items[j].Percentile })
		out.Percentiles = make([]jsonPercentile, 0, len(items))
		for _, item := range items {
			out.Percentiles = append(out.Percentiles, item.json())
		}
	}
	if len(prof.Histogram) > 0 {
		out.Histogram = make([]jsonHistBin, 0, len(prof.Histogram))
		for _, item := range prof.Histogram {
			out.Histogram = append(out.Histogram, item.json())
		}
	}
	if len(prof.Failures) > 0 {
		out.Failures = make([]jsonProfileFailure, 0, len(prof.Failures))
		for _, item := range prof.Failures {
			out.Failures = append(out.Failures, item.json())
		}
	}
	return out
}

func (lat *Latency) json() *jsonLatency {
	if lat == nil {
		return nil
	}
	return &jsonLatency{
		Count:    lat.Count,
		MinMs:    durMS(lat.Min),
		MaxMs:    durMS(lat.Max),
		MeanMs:   durMS(lat.Mean),
		MedianMs: durMS(lat.Median),
		StdDevMs: durMS(lat.StdDev),
	}
}

func (pct Percentile) json() jsonPercentile {
	return jsonPercentile{
		Percentile: pct.Percentile,
		ValueMs:    durMS(pct.Value),
	}
}

func (bin HistBin) json() jsonHistBin {
	return jsonHistBin{
		FromMs: durMS(bin.From),
		ToMs:   durMS(bin.To),
		Count:  bin.Count,
	}
}

func (fail ProfileFailure) json() jsonProfileFailure {
	return jsonProfileFailure{
		Iteration:  fail.Iteration,
		Warmup:     fail.Warmup,
		Reason:     fail.Reason,
		Status:     fail.Status,
		StatusCode: fail.StatusCode,
		DurationMs: durMS(fail.Duration),
		Failure:    fail.Failure.json(),
	}
}

func (stream *Stream) json() *jsonStream {
	if stream == nil {
		return nil
	}
	out := &jsonStream{
		Kind:           stream.Kind,
		EventCount:     stream.EventCount,
		TranscriptPath: stream.TranscriptPath,
	}
	if len(stream.Summary) > 0 {
		out.Summary = jsonAnyMap(stream.Summary)
	}
	return out
}

func (trace *Trace) json() *jsonTrace {
	if trace == nil {
		return nil
	}
	out := &jsonTrace{
		DurationMs:   durMS(trace.Duration),
		Error:        trace.Error,
		ArtifactPath: trace.ArtifactPath,
	}
	if bud := trace.Budget; bud != nil {
		out.Budgets = &jsonTraceBudget{
			TotalMs:     durMS(bud.Total),
			ToleranceMs: durMS(bud.Tolerance),
		}
		if len(bud.Phases) > 0 {
			out.Budgets.Phases = make(map[string]int64, len(bud.Phases))
			for key, val := range bud.Phases {
				out.Budgets.Phases[key] = durMS(val)
			}
		}
	}
	if len(trace.Breaches) > 0 {
		out.Breaches = make([]jsonTraceBreach, 0, len(trace.Breaches))
		for _, breach := range trace.Breaches {
			out.Breaches = append(out.Breaches, breach.json())
		}
	}
	return out
}

func (breach TraceBreach) json() jsonTraceBreach {
	return jsonTraceBreach{
		Kind:     breach.Kind,
		LimitMs:  durMS(breach.Limit),
		ActualMs: durMS(breach.Actual),
		OverMs:   durMS(breach.Over),
	}
}
