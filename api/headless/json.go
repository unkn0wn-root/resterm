package headless

import (
	"encoding/json"
	"io"
	"sort"
	"strings"
	"time"
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

// WriteJSON writes r as indented JSON.
// If r is nil, WriteJSON is a no-op. If w is nil, WriteJSON returns ErrNilWriter.
func (r *Report) WriteJSON(w io.Writer) error {
	if r == nil {
		return nil
	}
	if w == nil {
		return ErrNilWriter
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r.json())
}

func (r *Report) json() jsonReport {
	out := jsonReport{
		Version:    r.Version,
		FilePath:   r.FilePath,
		EnvName:    r.EnvName,
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
		Name:        item.Name,
		Method:      requestMethodValue(item.Method),
		Target:      item.Target,
		Environment: item.Environment,
		Status:      strings.ToLower(resultLabel(item)),
		Summary:     item.Summary,
		Canceled:    item.Canceled,
		SkipReason:  item.SkipReason,
		Error:       item.Error,
		ScriptError: item.ScriptError,
		DurationMs:  durMS(item.Duration),
		HTTP:        item.HTTP.json(),
		GRPC:        item.GRPC.json(),
		Stream:      item.Stream.json(),
		Trace:       item.Trace.json(),
		Compare:     item.Compare.json(),
		Profile:     item.Profile.json(),
	}
	if len(item.Tests) > 0 {
		out.Tests = make([]jsonTest, 0, len(item.Tests))
		for _, test := range item.Tests {
			out.Tests = append(out.Tests, jsonTest{
				Name:      test.Name,
				Message:   test.Message,
				Passed:    test.Passed,
				ElapsedMs: durMS(test.Elapsed),
			})
		}
	}
	if len(item.Steps) > 0 {
		out.Steps = make([]jsonStep, 0, len(item.Steps))
		for _, step := range item.Steps {
			out.Steps = append(out.Steps, step.json())
		}
	}
	return out
}

func (step Step) json() jsonStep {
	out := jsonStep{
		Name:        step.Name,
		Method:      requestMethodValue(step.Method),
		Target:      step.Target,
		Environment: step.Environment,
		Branch:      step.Branch,
		Iteration:   step.Iteration,
		Total:       step.Total,
		Status:      strings.ToLower(stepLabel(step)),
		Summary:     step.Summary,
		Canceled:    step.Canceled,
		SkipReason:  step.SkipReason,
		Error:       step.Error,
		ScriptError: step.ScriptError,
		DurationMs:  durMS(step.Duration),
		HTTP:        step.HTTP.json(),
		GRPC:        step.GRPC.json(),
		Stream:      step.Stream.json(),
		Trace:       step.Trace.json(),
	}
	if len(step.Tests) > 0 {
		out.Tests = make([]jsonTest, 0, len(step.Tests))
		for _, test := range step.Tests {
			out.Tests = append(out.Tests, jsonTest{
				Name:      test.Name,
				Message:   test.Message,
				Passed:    test.Passed,
				ElapsedMs: durMS(test.Elapsed),
			})
		}
	}
	return out
}

func (h *HTTP) json() *jsonHTTP {
	if h == nil {
		return nil
	}
	return &jsonHTTP{
		Status:     h.Status,
		StatusCode: h.StatusCode,
		Protocol:   h.Protocol,
	}
}

func (g *GRPC) json() *jsonGRPC {
	if g == nil {
		return nil
	}
	return &jsonGRPC{
		Code:          g.Code,
		StatusCode:    g.StatusCode,
		StatusMessage: g.StatusMessage,
	}
}

func (c *Compare) json() *jsonCompare {
	if c == nil {
		return nil
	}
	return &jsonCompare{Baseline: c.Baseline}
}

func (p *Profile) json() *jsonProfile {
	if p == nil {
		return nil
	}
	out := &jsonProfile{
		Count:          p.Count,
		Warmup:         p.Warmup,
		DelayMs:        durMS(p.Delay),
		TotalRuns:      p.TotalRuns,
		WarmupRuns:     p.WarmupRuns,
		SuccessfulRuns: p.SuccessfulRuns,
		FailedRuns:     p.FailedRuns,
		Latency:        p.Latency.json(),
		Histogram:      p.histogramJSON(),
	}
	if len(p.Percentiles) > 0 {
		items := append([]Percentile(nil), p.Percentiles...)
		sort.Slice(items, func(i, j int) bool { return items[i].Percentile < items[j].Percentile })
		out.Percentiles = make([]jsonPercentile, 0, len(items))
		for _, item := range items {
			out.Percentiles = append(out.Percentiles, jsonPercentile{
				Percentile: item.Percentile,
				ValueMs:    durMS(item.Value),
			})
		}
	}
	if len(p.Failures) > 0 {
		out.Failures = make([]jsonProfileFailure, 0, len(p.Failures))
		for _, failure := range p.Failures {
			out.Failures = append(out.Failures, jsonProfileFailure{
				Iteration:  failure.Iteration,
				Warmup:     failure.Warmup,
				Reason:     failure.Reason,
				Status:     failure.Status,
				StatusCode: failure.StatusCode,
				DurationMs: durMS(failure.Duration),
			})
		}
	}
	return out
}

func (l *Latency) json() *jsonLatency {
	if l == nil {
		return nil
	}
	return &jsonLatency{
		Count:    l.Count,
		MinMs:    durMS(l.Min),
		MaxMs:    durMS(l.Max),
		MeanMs:   durMS(l.Mean),
		MedianMs: durMS(l.Median),
		StdDevMs: durMS(l.StdDev),
	}
}

func (p *Profile) histogramJSON() []jsonHistBin {
	if p == nil || len(p.Histogram) == 0 {
		return nil
	}
	out := make([]jsonHistBin, 0, len(p.Histogram))
	for _, item := range p.Histogram {
		out = append(out, jsonHistBin{
			FromMs: durMS(item.From),
			ToMs:   durMS(item.To),
			Count:  item.Count,
		})
	}
	return out
}

func (s *Stream) json() *jsonStream {
	if s == nil {
		return nil
	}
	out := &jsonStream{
		Kind:           s.Kind,
		EventCount:     s.EventCount,
		TranscriptPath: s.TranscriptPath,
	}
	if len(s.Summary) > 0 {
		out.Summary = jsonAnyMap(s.Summary)
	}
	return out
}

func (t *Trace) json() *jsonTrace {
	if t == nil {
		return nil
	}
	out := &jsonTrace{
		DurationMs:   durMS(t.Duration),
		Error:        t.Error,
		ArtifactPath: t.ArtifactPath,
	}
	if t.Budget != nil {
		out.Budgets = &jsonTraceBudget{
			TotalMs:     durMS(t.Budget.Total),
			ToleranceMs: durMS(t.Budget.Tolerance),
		}
		if len(t.Budget.Phases) > 0 {
			out.Budgets.Phases = make(map[string]int64, len(t.Budget.Phases))
			for key, value := range t.Budget.Phases {
				out.Budgets.Phases[key] = durMS(value)
			}
		}
	}
	if len(t.Breaches) > 0 {
		out.Breaches = make([]jsonTraceBreach, 0, len(t.Breaches))
		for _, breach := range t.Breaches {
			out.Breaches = append(out.Breaches, jsonTraceBreach{
				Kind:     breach.Kind,
				LimitMs:  durMS(breach.Limit),
				ActualMs: durMS(breach.Actual),
				OverMs:   durMS(breach.Over),
			})
		}
	}
	return out
}
