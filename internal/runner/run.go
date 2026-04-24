package runner

import (
	"bytes"
	"context"
	"io"
	"maps"
	"sort"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runfmt"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	str "github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Select struct {
	Request  string
	Workflow string
	Tag      string
	All      bool
	Line     int
}

type Options struct {
	Version         string
	FilePath        string
	FileContent     []byte
	WorkspaceRoot   string
	Recursive       bool
	ArtifactDir     string
	StateDir        string
	PersistGlobals  bool
	PersistAuth     bool
	History         bool
	FailFast        bool
	EnvSet          vars.EnvironmentSet
	EnvName         string
	EnvironmentFile string
	CompareTargets  []string
	CompareBase     string
	Profile         bool
	HTTPOptions     httpclient.Options
	GRPCOptions     grpcclient.Options
	Client          *httpclient.Client
	Select          Select
}

const stopReasonFailFast = "fail_fast"

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

type ResultKind string

const (
	ResultKindRequest  ResultKind = "request"
	ResultKindWorkflow ResultKind = "workflow"
	ResultKindForEach  ResultKind = "for-each"
	ResultKindCompare  ResultKind = "compare"
	ResultKindProfile  ResultKind = "profile"
)

type Result struct {
	Kind                      ResultKind
	Name                      string
	Method                    string
	Target                    string
	EffectiveTarget           string
	Environment               string
	Summary                   string
	Duration                  time.Duration
	Passed                    bool
	Canceled                  bool
	Response                  *httpclient.Response
	GRPC                      *grpcclient.Response
	Err                       error
	Tests                     []scripts.TestResult
	ScriptErr                 error
	Skipped                   bool
	SkipReason                string
	Stream                    *StreamInfo
	Trace                     *TraceInfo
	Compare                   *CompareInfo
	Profile                   *ProfileInfo
	Steps                     []StepResult
	requestText               string
	transcript                []byte
	unresolvedTemplateVars    []string
	unresolvedTemplateVarsSet bool
}

type CompareInfo struct {
	Baseline string
}

type ProfileInfo struct {
	Count    int
	Warmup   int
	Delay    time.Duration
	Results  *history.ProfileResults
	Failures []ProfileFailure
}

type ProfileFailure struct {
	Iteration  int
	Warmup     bool
	Reason     string
	Status     string
	StatusCode int
	Duration   time.Duration
}

type StreamInfo struct {
	Kind           string
	EventCount     int
	Summary        map[string]any
	TranscriptPath string
}

type TraceInfo struct {
	Summary      *history.TraceSummary
	ArtifactPath string
}

type StepResult struct {
	Name            string
	Method          string
	Target          string
	EffectiveTarget string
	Environment     string
	Branch          string
	Iteration       int
	Total           int
	Summary         string
	Duration        time.Duration
	Response        *httpclient.Response
	GRPC            *grpcclient.Response
	Err             error
	Tests           []scripts.TestResult
	ScriptErr       error
	Passed          bool
	Skipped         bool
	SkipReason      string
	Canceled        bool
	Stream          *StreamInfo
	Trace           *TraceInfo
	requestText     string
	transcript      []byte
}

func Run(opts Options) (*Report, error) {
	return RunContext(context.Background(), opts)
}

func RunContext(ctx context.Context, opts Options) (*Report, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	pl, err := Build(opts)
	if err != nil {
		return nil, err
	}
	return RunPlan(ctx, pl)
}

func (r *Report) add(item Result) {
	r.Results = append(r.Results, item)
	r.Total++
	switch {
	case item.Skipped:
		r.Skipped++
	case resultFailed(item):
		r.Failed++
	default:
		r.Passed++
	}
}

func (r *Report) Success() bool {
	return r.Failed == 0
}

func (r *Report) WriteText(w io.Writer) error {
	if w == nil {
		return ErrNilWriter
	}
	rep := NormalizeReport(r)
	return runfmt.WriteText(w, &rep)
}

func resultFailed(item Result) bool {
	if item.Canceled || item.Err != nil || item.ScriptErr != nil || traceFailed(item.Trace) {
		return true
	}
	for _, test := range item.Tests {
		if !test.Passed {
			return true
		}
	}
	return !item.Passed
}

func resultDuration(item Result) time.Duration {
	if item.Duration > 0 {
		return item.Duration
	}
	switch {
	case item.Response != nil:
		return item.Response.Duration
	case item.GRPC != nil:
		return item.GRPC.Duration
	default:
		return 0
	}
}

func requestName(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	name := str.Trim(req.Metadata.Name)
	if name != "" {
		return name
	}
	return requestSourceTarget(req)
}

func requestSourceTarget(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if req.GRPC != nil {
		if target := str.FirstTrimmed(req.GRPC.FullMethod, req.GRPC.Target); target != "" {
			return target
		}
	}
	return str.Trim(req.URL)
}

func requestTarget(req *restfile.Request, resp *httpclient.Response) string {
	return effectiveURL(resp, requestSourceTarget(req))
}

func effectiveURL(resp *httpclient.Response, fallback string) string {
	if resp != nil {
		if target := str.Trim(resp.EffectiveURL); target != "" {
			return target
		}
	}
	return fallback
}

func requestMethod(req *restfile.Request) string {
	if req == nil {
		return "REQ"
	}
	switch {
	case req.GRPC != nil:
		return "GRPC"
	case req.WebSocket != nil:
		return "WS"
	case req.SSE != nil:
		return "SSE"
	default:
		return requestMethodValue(req.Method)
	}
}

func requestMethodValue(method string) string {
	method = str.UpperTrim(method)
	if method == "" {
		return "REQ"
	}
	return method
}

func cloneReq(req *restfile.Request) *restfile.Request {
	if req == nil {
		return nil
	}
	clone := *req
	if req.Metadata.Profile != nil {
		spec := *req.Metadata.Profile
		clone.Metadata.Profile = &spec
	}
	if req.Metadata.Compare != nil {
		spec := *req.Metadata.Compare
		if len(req.Metadata.Compare.Environments) > 0 {
			spec.Environments = append([]string(nil), req.Metadata.Compare.Environments...)
		}
		clone.Metadata.Compare = &spec
	}
	return &clone
}

func requestRunResult(req *restfile.Request, res engine.RequestResult, fallbackEnv string) Result {
	runReq := req
	if res.Executed != nil {
		runReq = res.Executed
	}
	envName := str.FirstTrimmed(res.Environment, fallbackEnv)
	item := Result{
		Kind:            ResultKindRequest,
		Name:            requestName(runReq),
		Method:          requestMethod(runReq),
		Target:          requestSourceTarget(runReq),
		EffectiveTarget: requestTarget(runReq, res.Response),
		Environment:     envName,
		Response:        res.Response,
		GRPC:            res.GRPC,
		Err:             res.Err,
		Tests:           cloneTests(res.Tests),
		ScriptErr:       res.ScriptErr,
		Skipped:         res.Skipped,
		SkipReason:      str.Trim(res.SkipReason),
		Stream:          streamResult(res.Stream),
		Trace:           traceResult(res.Response),
		requestText:     str.Trim(res.RequestText),
		transcript:      bytes.Clone(res.Transcript),
	}
	if res.Explain != nil {
		item.SetUnresolvedTemplateVars(explainMissingTemplateVars(res.Explain))
	}
	item.Passed = !item.Skipped && !requestFailed(item)
	return item
}

func skippedRequestResult(req *restfile.Request, fallbackEnv, reason string) Result {
	runReq := cloneReq(req)
	return Result{
		Kind:        ResultKindRequest,
		Name:        requestName(runReq),
		Method:      requestMethod(runReq),
		Target:      requestSourceTarget(runReq),
		Environment: str.Trim(fallbackEnv),
		Skipped:     true,
		SkipReason:  str.Trim(reason),
		Passed:      false,
	}
}

func requestFailed(item Result) bool {
	if item.Err != nil || item.ScriptErr != nil {
		return true
	}
	for _, test := range item.Tests {
		if !test.Passed {
			return true
		}
	}
	return traceFailed(item.Trace)
}

func compareRunResult(req *restfile.Request, res engine.CompareResult, fallbackEnv string) Result {
	envName := str.FirstTrimmed(res.Environment, fallbackEnv)
	item := Result{
		Kind:        ResultKindCompare,
		Name:        requestName(req),
		Method:      "COMPARE",
		Target:      requestSourceTarget(req),
		Environment: envName,
		Summary:     str.Trim(res.Summary),
		Duration:    compareDuration(res.Rows),
		Passed:      res.Success,
		Skipped:     res.Skipped,
		Canceled:    res.Canceled,
		Compare: &CompareInfo{
			Baseline: str.Trim(res.Baseline),
		},
		Steps: make([]StepResult, 0, len(res.Rows)),
	}
	for _, row := range res.Rows {
		item.Steps = append(item.Steps, compareStepResult(req, row))
	}
	item.Passed = !item.Skipped && !item.Canceled && stepsPassed(item.Steps)
	return item
}

func compareDuration(rows []engine.CompareRow) time.Duration {
	var total time.Duration
	for _, row := range rows {
		total += row.Duration
	}
	return total
}

func compareStepResult(req *restfile.Request, row engine.CompareRow) StepResult {
	return StepResult{
		Name:            str.Trim(row.Environment),
		Method:          requestMethod(req),
		Target:          requestSourceTarget(req),
		EffectiveTarget: requestTarget(req, row.Response),
		Environment:     str.Trim(row.Environment),
		Summary:         str.Trim(row.Summary),
		Duration:        row.Duration,
		Response:        row.Response,
		GRPC:            row.GRPC,
		Err:             row.Err,
		Tests:           cloneTests(row.Tests),
		ScriptErr:       row.ScriptErr,
		Passed:          row.Success,
		Skipped:         row.Skipped,
		SkipReason:      str.Trim(row.SkipReason),
		Canceled:        row.Canceled,
		Stream:          streamResult(row.Stream),
		Trace:           traceResult(row.Response),
		requestText:     "",
		transcript:      bytes.Clone(row.Transcript),
	}
}

func profileRunResult(req *restfile.Request, res engine.ProfileResult, fallbackEnv string) Result {
	envName := str.FirstTrimmed(res.Environment, fallbackEnv)
	item := Result{
		Kind:        ResultKindProfile,
		Name:        requestName(req),
		Method:      "PROFILE",
		Target:      requestSourceTarget(req),
		Environment: envName,
		Summary:     str.Trim(res.Summary),
		Duration:    res.Duration,
		Passed:      res.Success,
		Skipped:     res.Skipped,
		SkipReason:  str.Trim(res.SkipReason),
		Canceled:    res.Canceled,
		Profile: &ProfileInfo{
			Count:    res.Count,
			Warmup:   res.Warmup,
			Delay:    res.Delay,
			Results:  cloneProfileResults(res.Results),
			Failures: profileFailures(res.Failures),
		},
	}
	return item
}

func cloneProfileResults(results *history.ProfileResults) *history.ProfileResults {
	if results == nil {
		return nil
	}
	out := *results
	if results.Latency != nil {
		lat := *results.Latency
		out.Latency = &lat
	}
	if len(results.Percentiles) > 0 {
		out.Percentiles = append([]history.ProfilePercentile(nil), results.Percentiles...)
	}
	if len(results.Histogram) > 0 {
		out.Histogram = append([]history.ProfileHistogramBin(nil), results.Histogram...)
	}
	return &out
}

func profileFailures(src []engine.ProfileFailure) []ProfileFailure {
	if len(src) == 0 {
		return nil
	}
	out := make([]ProfileFailure, 0, len(src))
	for _, failure := range src {
		out = append(out, ProfileFailure{
			Iteration:  failure.Iteration,
			Warmup:     failure.Warmup,
			Reason:     str.Trim(failure.Reason),
			Status:     str.Trim(failure.Status),
			StatusCode: failure.StatusCode,
			Duration:   failure.Duration,
		})
	}
	return out
}

func workflowRunResult(res engine.WorkflowResult, fallbackEnv string) Result {
	kind := ResultKindWorkflow
	if strings.EqualFold(str.Trim(string(res.Kind)), string(ResultKindForEach)) {
		kind = ResultKindForEach
	}
	envName := str.FirstTrimmed(res.Environment, fallbackEnv)
	item := Result{
		Kind:        kind,
		Name:        str.Trim(res.Name),
		Method:      str.UpperTrim(string(res.Kind)),
		Environment: envName,
		Summary:     str.Trim(res.Summary),
		Duration:    res.Duration,
		Passed:      res.Success,
		Skipped:     res.Skipped,
		Canceled:    res.Canceled,
		Steps:       make([]StepResult, 0, len(res.Steps)),
	}
	if item.Method == "" {
		item.Method = "WORKFLOW"
	}
	for _, step := range res.Steps {
		item.Steps = append(item.Steps, workflowStepResult(step))
	}
	item.Passed = !item.Skipped && !item.Canceled && stepsPassed(item.Steps)
	return item
}

func workflowStepResult(step engine.WorkflowStep) StepResult {
	target := str.Trim(step.Target)
	return StepResult{
		Name:            str.Trim(step.Name),
		Method:          str.Trim(step.Method),
		Target:          target,
		EffectiveTarget: effectiveURL(step.Response, target),
		Branch:          str.Trim(step.Branch),
		Iteration:       step.Iteration,
		Total:           step.Total,
		Summary:         str.Trim(step.Summary),
		Duration:        step.Duration,
		Response:        step.Response,
		GRPC:            step.GRPC,
		Err:             step.Err,
		Tests:           cloneTests(step.Tests),
		ScriptErr:       step.ScriptErr,
		Passed:          step.Success,
		Skipped:         step.Skipped,
		Canceled:        step.Canceled,
		Stream:          streamResult(step.Stream),
		Trace:           traceResult(step.Response),
		requestText:     "",
		transcript:      bytes.Clone(step.Transcript),
	}
}

func explainMissingTemplateVars(rep *xplain.Report) []string {
	if rep == nil || len(rep.Vars) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(rep.Vars))
	out := make([]string, 0, len(rep.Vars))
	for _, item := range rep.Vars {
		if !item.Missing {
			continue
		}
		name := str.Trim(item.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func traceResult(resp *httpclient.Response) *TraceInfo {
	if resp == nil {
		return nil
	}
	sum := history.NewTraceSummary(resp.Timeline, resp.TraceReport)
	if sum == nil {
		return nil
	}
	return &TraceInfo{Summary: sum}
}

func traceFailed(info *TraceInfo) bool {
	return info != nil && info.Summary != nil && len(info.Summary.Breaches) > 0
}

func streamResult(info *scripts.StreamInfo) *StreamInfo {
	if info == nil {
		return nil
	}
	out := &StreamInfo{
		Kind:       str.Trim(info.Kind),
		EventCount: streamEventCount(info),
	}
	if len(info.Summary) > 0 {
		out.Summary = cloneStreamSummary(info.Summary)
	}
	return out
}

func streamEventCount(info *scripts.StreamInfo) int {
	if info == nil {
		return 0
	}
	if n := len(info.Events); n > 0 {
		return n
	}
	if info.Summary == nil {
		return 0
	}
	switch v := info.Summary["eventCount"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func cloneStreamSummary(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	maps.Copy(out, src)
	return out
}

func cloneTests(src []scripts.TestResult) []scripts.TestResult {
	if len(src) == 0 {
		return nil
	}
	out := make([]scripts.TestResult, 0, len(src))
	for _, test := range src {
		out = append(out, scripts.TestResult{
			Name:    str.Trim(test.Name),
			Message: str.Trim(test.Message),
			Passed:  test.Passed,
			Elapsed: test.Elapsed,
		})
	}
	return out
}

func stepFailed(step StepResult) bool {
	if step.Canceled || step.Err != nil || step.ScriptErr != nil || traceFailed(step.Trace) {
		return true
	}
	for _, test := range step.Tests {
		if !test.Passed {
			return true
		}
	}
	return !step.Passed
}

func stepsPassed(steps []StepResult) bool {
	if len(steps) == 0 {
		return false
	}
	for _, step := range steps {
		if step.Skipped {
			continue
		}
		if stepFailed(step) {
			return false
		}
	}
	return true
}
