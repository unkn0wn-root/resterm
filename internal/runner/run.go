package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/headless"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runfmt"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type Select struct {
	Request  string
	Workflow string
	Tag      string
	All      bool
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

type UsageError struct {
	err error
}

// ErrNilWriter reports an attempt to write a report to a nil io.Writer.
var ErrNilWriter = errors.New("runner: nil writer")

func (e UsageError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e UsageError) Unwrap() error {
	return e.err
}

func IsUsageError(err error) bool {
	var target UsageError
	return errors.As(err, &target)
}

type Report struct {
	Version   string
	FilePath  string
	EnvName   string
	StartedAt time.Time
	EndedAt   time.Time
	Duration  time.Duration
	Results   []Result
	Total     int
	Passed    int
	Failed    int
	Skipped   int
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
	Kind        ResultKind
	Name        string
	Method      string
	Target      string
	Environment string
	Summary     string
	Duration    time.Duration
	Passed      bool
	Canceled    bool
	Response    *httpclient.Response
	GRPC        *grpcclient.Response
	Err         error
	Tests       []scripts.TestResult
	ScriptErr   error
	Skipped     bool
	SkipReason  string
	Stream      *StreamInfo
	Trace       *TraceInfo
	Compare     *CompareInfo
	Profile     *ProfileInfo
	Steps       []StepResult
	transcript  []byte
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
	Name        string
	Method      string
	Target      string
	Environment string
	Branch      string
	Iteration   int
	Total       int
	Summary     string
	Duration    time.Duration
	Response    *httpclient.Response
	GRPC        *grpcclient.Response
	Err         error
	Tests       []scripts.TestResult
	ScriptErr   error
	Passed      bool
	Skipped     bool
	SkipReason  string
	Canceled    bool
	Stream      *StreamInfo
	Trace       *TraceInfo
	transcript  []byte
}

func Run(opts Options) (*Report, error) {
	return RunContext(context.Background(), opts)
}

func RunContext(ctx context.Context, opts Options) (*Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	path := strings.TrimSpace(opts.FilePath)
	if path == "" {
		return nil, usageError("--file is required")
	}
	start := time.Now()

	data := bytes.Clone(opts.FileContent)
	if data == nil {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		data = raw
	}

	doc := parser.Parse(path, data)
	if len(doc.Errors) > 0 {
		err := doc.Errors[0]
		return nil, fmt.Errorf("parse error at line %d: %s", err.Line, err.Message)
	}

	work := opts.WorkspaceRoot
	if work == "" {
		work = filepath.Dir(path)
	}
	artifactDir, err := absCleanPath(opts.ArtifactDir)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact dir: %w", err)
	}
	if opts.Profile && len(opts.CompareTargets) > 0 {
		return nil, usageError("--profile cannot be combined with --compare")
	}
	paths, err := resolveStatePaths(opts)
	if err != nil {
		return nil, fmt.Errorf("resolve runner state: %w", err)
	}
	hist := openHistoryStore(paths, opts)

	exec := headless.New(engine.Config{
		FilePath:        path,
		Client:          opts.Client,
		EnvironmentSet:  opts.EnvSet,
		EnvironmentName: opts.EnvName,
		EnvironmentFile: opts.EnvironmentFile,
		CompareTargets:  append([]string(nil), opts.CompareTargets...),
		CompareBase:     strings.TrimSpace(opts.CompareBase),
		HTTPOptions:     opts.HTTPOptions,
		GRPCOptions:     opts.GRPCOptions,
		WorkspaceRoot:   work,
		Recursive:       opts.Recursive,
		History:         hist,
	})
	defer func() { _ = exec.Close() }()
	if err := loadRunnerState(exec, paths, opts); err != nil {
		return nil, fmt.Errorf("load runner state: %w", err)
	}

	rep := &Report{
		Version:   strings.TrimSpace(opts.Version),
		FilePath:  path,
		EnvName:   strings.TrimSpace(opts.EnvName),
		StartedAt: start,
	}

	if name := strings.TrimSpace(opts.Select.Workflow); name != "" {
		if opts.Select.All || strings.TrimSpace(opts.Select.Request) != "" ||
			strings.TrimSpace(opts.Select.Tag) != "" {
			return nil, usageError("--workflow cannot be combined with --request, --tag, or --all")
		}
		if opts.Profile || len(opts.CompareTargets) > 0 {
			return nil, usageError("--workflow cannot be combined with --compare or --profile")
		}
		wf, err := selectWorkflow(doc, name)
		if err != nil {
			return nil, err
		}
		rep.Results = make([]Result, 0, 1)
		out, err := exec.ExecuteWorkflowContext(ctx, doc, wf, opts.EnvName)
		if err != nil {
			return nil, err
		}
		rep.add(workflowRunResult(*out, opts.EnvName))
		rep.EndedAt = time.Now()
		rep.Duration = rep.EndedAt.Sub(rep.StartedAt)
		if err := saveRunnerState(exec, paths, opts); err != nil {
			return nil, fmt.Errorf("save runner state: %w", err)
		}
		if err := rep.writeArtifacts(artifactDir); err != nil {
			return nil, err
		}
		return rep, nil
	}

	reqs, err := selectRequests(doc, opts.Select)
	if err != nil {
		return nil, err
	}
	rep.Results = make([]Result, 0, len(reqs))
	for _, req := range reqs {
		runReq := cloneReq(req)
		if opts.Profile && runReq.Metadata.Profile == nil {
			runReq.Metadata.Profile = &restfile.ProfileSpec{}
		}
		res, err := exec.ExecuteRequestContext(ctx, doc, runReq, opts.EnvName)
		if err != nil {
			return nil, err
		}
		if res.Workflow != nil {
			rep.add(workflowRunResult(*res.Workflow, opts.EnvName))
			continue
		}
		if res.Compare != nil {
			rep.add(compareRunResult(runReq, *res.Compare, opts.EnvName))
			continue
		}
		if res.Profile != nil {
			rep.add(profileRunResult(runReq, *res.Profile, opts.EnvName))
			continue
		}
		rep.add(requestRunResult(runReq, res, opts.EnvName))
	}
	rep.EndedAt = time.Now()
	rep.Duration = rep.EndedAt.Sub(rep.StartedAt)
	if err := saveRunnerState(exec, paths, opts); err != nil {
		return nil, fmt.Errorf("save runner state: %w", err)
	}
	if err := rep.writeArtifacts(artifactDir); err != nil {
		return nil, err
	}
	return rep, nil
}

func usageError(format string, args ...any) error {
	return UsageError{err: fmt.Errorf(format, args...)}
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

func selectRequests(doc *restfile.Document, sel Select) ([]*restfile.Request, error) {
	if doc == nil || len(doc.Requests) == 0 {
		return nil, usageError("no requests found")
	}

	if sel.All && (strings.TrimSpace(sel.Request) != "" || strings.TrimSpace(sel.Tag) != "") {
		return nil, usageError("--all cannot be combined with --request or --tag")
	}
	if strings.TrimSpace(sel.Request) != "" && strings.TrimSpace(sel.Tag) != "" {
		return nil, usageError("--request cannot be combined with --tag")
	}

	if sel.All {
		return append([]*restfile.Request(nil), doc.Requests...), nil
	}

	if name := strings.TrimSpace(sel.Request); name != "" {
		return selectByRequestName(doc.Requests, name)
	}

	if tag := strings.TrimSpace(sel.Tag); tag != "" {
		return selectByTag(doc.Requests, tag)
	}

	if len(doc.Requests) == 1 {
		return []*restfile.Request{doc.Requests[0]}, nil
	}
	return nil, usageError("multiple requests found; use --request, --tag, or --all")
}

func selectWorkflow(doc *restfile.Document, name string) (*restfile.Workflow, error) {
	if doc == nil || len(doc.Workflows) == 0 {
		return nil, usageError("no workflows found")
	}
	var out []*restfile.Workflow
	for i := range doc.Workflows {
		wf := &doc.Workflows[i]
		if strings.EqualFold(strings.TrimSpace(wf.Name), name) {
			out = append(out, wf)
		}
	}
	switch len(out) {
	case 0:
		return nil, usageError("workflow %q not found", name)
	case 1:
		return out[0], nil
	default:
		return nil, usageError("workflow %q matched %d entries", name, len(out))
	}
}

func selectByRequestName(reqs []*restfile.Request, name string) ([]*restfile.Request, error) {
	var out []*restfile.Request
	for _, req := range reqs {
		if strings.EqualFold(strings.TrimSpace(req.Metadata.Name), name) {
			out = append(out, req)
		}
	}
	switch len(out) {
	case 0:
		return nil, usageError("request %q not found", name)
	case 1:
		return out, nil
	default:
		return nil, usageError("request %q matched %d entries", name, len(out))
	}
}

func selectByTag(reqs []*restfile.Request, tag string) ([]*restfile.Request, error) {
	var out []*restfile.Request
	for _, req := range reqs {
		for _, item := range req.Metadata.Tags {
			if strings.EqualFold(strings.TrimSpace(item), tag) {
				out = append(out, req)
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, usageError("tag %q did not match any requests", tag)
	}
	return out, nil
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

func resultLabel(item Result) string {
	switch {
	case item.Skipped:
		return "SKIP"
	case resultFailed(item):
		return "FAIL"
	default:
		return "PASS"
	}
}

func resultLine(item Result) string {
	switch item.Kind {
	case ResultKindWorkflow, ResultKindForEach:
		return workflowLine(item)
	case ResultKindCompare:
		return compareLine(item)
	case ResultKindProfile:
		return profileLine(item)
	}
	base := fmt.Sprintf("%s %s", requestMethodValue(item.Method), resultName(item))
	return lineWithDetail(base, func() string {
		switch {
		case item.Skipped:
			return item.SkipReason
		case item.Err != nil:
			return item.Err.Error()
		case item.ScriptErr != nil:
			return item.ScriptErr.Error()
		}

		failed := failedTests(item.Tests)
		if len(failed) > 0 {
			return fmt.Sprintf("%d test(s) failed", len(failed))
		}
		if msg := traceFailureText(item.Trace); msg != "" {
			return msg
		}

		status := resultStatus(item)
		dur := resultDuration(item)
		switch {
		case status == "" && dur <= 0:
			return ""
		case dur <= 0:
			return status
		case status == "":
			return dur.String()
		default:
			return fmt.Sprintf("%s in %s", status, dur)
		}
	})
}

func workflowLine(item Result) string {
	base := fmt.Sprintf("%s %s", requestMethodValue(item.Method), resultName(item))
	return lineWithDetail(base, func() string {
		pass, fail, skip := stepCounts(item.Steps)
		detail := fmt.Sprintf("%d passed, %d failed, %d skipped", pass, fail, skip)
		if item.Canceled {
			detail += ", canceled"
		}
		if dur := resultDuration(item); dur > 0 {
			detail = fmt.Sprintf("%s in %s", detail, dur)
		}
		return detail
	})
}

func compareLine(item Result) string {
	base := fmt.Sprintf("%s %s", requestMethodValue(item.Method), resultName(item))
	return lineWithDetail(base, func() string {
		pass, fail, skip := stepCounts(item.Steps)
		detail := fmt.Sprintf("%d passed, %d failed, %d skipped", pass, fail, skip)
		if item.Compare != nil {
			if baseline := item.Compare.Baseline; baseline != "" {
				detail = fmt.Sprintf("baseline: %s, %s", baseline, detail)
			}
		}
		if item.Canceled {
			detail += ", canceled"
		}
		if dur := resultDuration(item); dur > 0 {
			detail = fmt.Sprintf("%s in %s", detail, dur)
		}
		return detail
	})
}

func profileLine(item Result) string {
	base := fmt.Sprintf("%s %s", requestMethodValue(item.Method), resultName(item))
	return lineWithDetail(base, func() string {
		prof := item.Profile
		if prof == nil || prof.Results == nil {
			return item.Summary
		}
		detail := fmt.Sprintf(
			"%d total, %d success, %d failure",
			prof.Results.TotalRuns,
			prof.Results.SuccessfulRuns,
			prof.Results.FailedRuns,
		)
		if prof.Results.WarmupRuns > 0 {
			detail = fmt.Sprintf("%s, %d warmup", detail, prof.Results.WarmupRuns)
		}
		if item.Canceled {
			detail += ", canceled"
		}
		if dur := resultDuration(item); dur > 0 {
			detail = fmt.Sprintf("%s in %s", detail, dur)
		}
		return detail
	})
}

func lineWithDetail(base string, detail func() string) string {
	if detail == nil {
		return base
	}
	text := detail()
	if text == "" {
		return base
	}
	return fmt.Sprintf("%s [%s]", base, text)
}

func failedTests(tests []scripts.TestResult) []scripts.TestResult {
	out := make([]scripts.TestResult, 0, len(tests))
	for _, test := range tests {
		if !test.Passed {
			out = append(out, test)
		}
	}
	return out
}

func resultStatus(item Result) string {
	switch {
	case item.Response != nil:
		return strings.TrimSpace(item.Response.Status)
	case item.GRPC != nil:
		status := item.GRPC.StatusCode.String()
		if msg := strings.TrimSpace(item.GRPC.StatusMessage); msg != "" &&
			!strings.EqualFold(msg, status) {
			status = fmt.Sprintf("%s (%s)", status, msg)
		}
		return status
	default:
		return ""
	}
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

func stepStatus(step StepResult) string {
	switch {
	case step.Response != nil:
		return strings.TrimSpace(step.Response.Status)
	case step.GRPC != nil:
		status := step.GRPC.StatusCode.String()
		if msg := strings.TrimSpace(step.GRPC.StatusMessage); msg != "" &&
			!strings.EqualFold(msg, status) {
			status = fmt.Sprintf("%s (%s)", status, msg)
		}
		return status
	default:
		return ""
	}
}

func requestName(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	name := strings.TrimSpace(req.Metadata.Name)
	if name != "" {
		return name
	}
	return requestTarget(req)
}

func requestTarget(req *restfile.Request) string {
	if req == nil {
		return ""
	}
	if req.GRPC != nil {
		if method := strings.TrimSpace(req.GRPC.FullMethod); method != "" {
			return method
		}
		if target := strings.TrimSpace(req.GRPC.Target); target != "" {
			return target
		}
	}
	return strings.TrimSpace(req.URL)
}

func requestMethod(req *restfile.Request) string {
	if req == nil {
		return "REQ"
	}
	if req.GRPC != nil {
		return "GRPC"
	}
	m := requestMethodValue(req.Method)
	if req.WebSocket != nil {
		return "WS"
	}
	if req.SSE != nil {
		return "SSE"
	}
	return m
}

func requestMethodValue(method string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "REQ"
	}
	return method
}

func resultName(item Result) string {
	name := item.Name
	if name != "" {
		return name
	}
	target := item.Target
	if target == "" {
		return "<unnamed>"
	}
	if len(target) > 80 {
		return target[:77] + "..."
	}
	return target
}

func reportTargetLabel(r *Report) string {
	if r == nil {
		return "request(s)"
	}
	for _, item := range r.Results {
		if item.Kind != ResultKindRequest {
			return "target(s)"
		}
	}
	return "request(s)"
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
	envName := strings.TrimSpace(res.Environment)
	if envName == "" {
		envName = strings.TrimSpace(fallbackEnv)
	}
	item := Result{
		Kind:        ResultKindRequest,
		Name:        requestName(runReq),
		Method:      requestMethod(runReq),
		Target:      requestTarget(runReq),
		Environment: envName,
		Response:    res.Response,
		GRPC:        res.GRPC,
		Err:         res.Err,
		Tests:       cloneTests(res.Tests),
		ScriptErr:   res.ScriptErr,
		Skipped:     res.Skipped,
		SkipReason:  strings.TrimSpace(res.SkipReason),
		Stream:      streamResult(res.Stream),
		Trace:       traceResult(res.Response),
		transcript:  bytes.Clone(res.Transcript),
	}
	item.Passed = !item.Skipped && !requestFailed(item)
	return item
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
	envName := strings.TrimSpace(res.Environment)
	if envName == "" {
		envName = strings.TrimSpace(fallbackEnv)
	}
	item := Result{
		Kind:        ResultKindCompare,
		Name:        requestName(req),
		Method:      "COMPARE",
		Target:      requestTarget(req),
		Environment: envName,
		Summary:     strings.TrimSpace(res.Summary),
		Duration:    compareDuration(res.Rows),
		Passed:      res.Success,
		Skipped:     res.Skipped,
		Canceled:    res.Canceled,
		Compare: &CompareInfo{
			Baseline: strings.TrimSpace(res.Baseline),
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
		Name:        strings.TrimSpace(row.Environment),
		Method:      requestMethod(req),
		Target:      requestTarget(req),
		Environment: strings.TrimSpace(row.Environment),
		Summary:     strings.TrimSpace(row.Summary),
		Duration:    row.Duration,
		Response:    row.Response,
		GRPC:        row.GRPC,
		Err:         row.Err,
		Tests:       cloneTests(row.Tests),
		ScriptErr:   row.ScriptErr,
		Passed:      row.Success,
		Skipped:     row.Skipped,
		SkipReason:  strings.TrimSpace(row.SkipReason),
		Canceled:    row.Canceled,
		Stream:      streamResult(row.Stream),
		Trace:       traceResult(row.Response),
		transcript:  bytes.Clone(row.Transcript),
	}
}

func profileRunResult(req *restfile.Request, res engine.ProfileResult, fallbackEnv string) Result {
	envName := strings.TrimSpace(res.Environment)
	if envName == "" {
		envName = strings.TrimSpace(fallbackEnv)
	}
	item := Result{
		Kind:        ResultKindProfile,
		Name:        requestName(req),
		Method:      "PROFILE",
		Target:      requestTarget(req),
		Environment: envName,
		Summary:     strings.TrimSpace(res.Summary),
		Duration:    res.Duration,
		Passed:      res.Success,
		Skipped:     res.Skipped,
		SkipReason:  strings.TrimSpace(res.SkipReason),
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
			Reason:     strings.TrimSpace(failure.Reason),
			Status:     strings.TrimSpace(failure.Status),
			StatusCode: failure.StatusCode,
			Duration:   failure.Duration,
		})
	}
	return out
}

func workflowRunResult(res engine.WorkflowResult, fallbackEnv string) Result {
	kind := ResultKindWorkflow
	if strings.EqualFold(strings.TrimSpace(res.Kind), string(ResultKindForEach)) {
		kind = ResultKindForEach
	}
	envName := strings.TrimSpace(res.Environment)
	if envName == "" {
		envName = strings.TrimSpace(fallbackEnv)
	}
	item := Result{
		Kind:        kind,
		Name:        strings.TrimSpace(res.Name),
		Method:      strings.ToUpper(strings.TrimSpace(res.Kind)),
		Environment: envName,
		Summary:     strings.TrimSpace(res.Summary),
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
	return StepResult{
		Name:       strings.TrimSpace(step.Name),
		Method:     strings.TrimSpace(step.Method),
		Target:     strings.TrimSpace(step.Target),
		Branch:     strings.TrimSpace(step.Branch),
		Iteration:  step.Iteration,
		Total:      step.Total,
		Summary:    strings.TrimSpace(step.Summary),
		Duration:   step.Duration,
		Response:   step.Response,
		GRPC:       step.GRPC,
		Err:        step.Err,
		Tests:      cloneTests(step.Tests),
		ScriptErr:  step.ScriptErr,
		Passed:     step.Success,
		Skipped:    step.Skipped,
		Canceled:   step.Canceled,
		Stream:     streamResult(step.Stream),
		Trace:      traceResult(step.Response),
		transcript: bytes.Clone(step.Transcript),
	}
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

func traceFailureText(info *TraceInfo) string {
	if !traceFailed(info) {
		return ""
	}
	breach := info.Summary.Breaches[0]
	label := strings.TrimSpace(breach.Kind)
	if label == "" {
		label = "trace"
	}
	if breach.Over > 0 {
		return fmt.Sprintf("trace budget breach %s (+%s)", label, breach.Over)
	}
	if breach.Limit > 0 && breach.Actual > 0 {
		return fmt.Sprintf("trace budget breach %s (%s > %s)", label, breach.Actual, breach.Limit)
	}
	return fmt.Sprintf("trace budget breach %s", label)
}

func streamResult(info *scripts.StreamInfo) *StreamInfo {
	if info == nil {
		return nil
	}
	out := &StreamInfo{
		Kind:       strings.TrimSpace(info.Kind),
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
	for key, value := range src {
		out[key] = value
	}
	return out
}

func cloneTests(src []scripts.TestResult) []scripts.TestResult {
	if len(src) == 0 {
		return nil
	}
	out := make([]scripts.TestResult, 0, len(src))
	for _, test := range src {
		out = append(out, scripts.TestResult{
			Name:    strings.TrimSpace(test.Name),
			Message: strings.TrimSpace(test.Message),
			Passed:  test.Passed,
			Elapsed: test.Elapsed,
		})
	}
	return out
}

func (r *Report) writeArtifacts(dir string) error {
	if r == nil {
		return nil
	}
	if dir == "" {
		return nil
	}
	streamsDir := filepath.Join(dir, "streams")
	tracesDir := filepath.Join(dir, "traces")
	for i := range r.Results {
		item := &r.Results[i]
		if path, err := writeStreamArtifact(
			streamsDir,
			i+1,
			0,
			resultName(*item),
			item.Stream,
			item.transcript,
		); err != nil {
			return err
		} else if item.Stream != nil {
			item.Stream.TranscriptPath = path
		}
		if path, err := writeTraceArtifact(
			tracesDir,
			i+1,
			0,
			resultName(*item),
			item.Trace,
		); err != nil {
			return err
		} else if item.Trace != nil {
			item.Trace.ArtifactPath = path
		}
		for j := range item.Steps {
			step := &item.Steps[j]
			if path, err := writeStreamArtifact(
				streamsDir,
				i+1,
				j+1,
				stepName(*step),
				step.Stream,
				step.transcript,
			); err != nil {
				return err
			} else if step.Stream != nil {
				step.Stream.TranscriptPath = path
			}
			if path, err := writeTraceArtifact(
				tracesDir,
				i+1,
				j+1,
				stepName(*step),
				step.Trace,
			); err != nil {
				return err
			} else if step.Trace != nil {
				step.Trace.ArtifactPath = path
			}
		}
	}
	return nil
}

func writeStreamArtifact(
	base string,
	resultIndex int,
	stepIndex int,
	name string,
	stream *StreamInfo,
	transcript []byte,
) (string, error) {
	if stream == nil || len(transcript) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	file := fmt.Sprintf("result-%03d", resultIndex)
	if stepIndex > 0 {
		file += fmt.Sprintf("-step-%03d", stepIndex)
	}
	if slug := streamArtifactSlug(name); slug != "" {
		file += "-" + slug
	}
	if kind := strings.ToLower(strings.TrimSpace(stream.Kind)); kind != "" {
		file += "-" + kind
	}
	path := filepath.Join(base, file+".json")
	if err := os.WriteFile(path, transcript, 0o644); err != nil {
		return "", fmt.Errorf("write stream artifact: %w", err)
	}
	return path, nil
}

func writeTraceArtifact(
	base string,
	resultIndex int,
	stepIndex int,
	name string,
	trace *TraceInfo,
) (string, error) {
	if trace == nil || trace.Summary == nil {
		return "", nil
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	file := fmt.Sprintf("result-%03d", resultIndex)
	if stepIndex > 0 {
		file += fmt.Sprintf("-step-%03d", stepIndex)
	}
	if slug := streamArtifactSlug(name); slug != "" {
		file += "-" + slug
	}
	path := filepath.Join(base, file+"-trace.json")
	data, err := json.MarshalIndent(trace.Summary, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal trace artifact: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write trace artifact: %w", err)
	}
	return path, nil
}

func streamArtifactSlug(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if lastDash || b.Len() == 0 {
			continue
		}
		b.WriteByte('-')
		lastDash = true
	}
	return strings.Trim(b.String(), "-")
}

func stepCounts(steps []StepResult) (pass, fail, skip int) {
	for _, step := range steps {
		switch {
		case step.Skipped:
			skip++
		case stepFailed(step):
			fail++
		default:
			pass++
		}
	}
	return pass, fail, skip
}

func stepLabel(step StepResult) string {
	switch {
	case step.Canceled:
		return "CANCELED"
	case step.Skipped:
		return "SKIP"
	case stepFailed(step):
		return "FAIL"
	default:
		return "PASS"
	}
}

func stepLine(step StepResult) string {
	base := stepName(step)
	return lineWithDetail(base, func() string {
		switch {
		case step.Canceled:
			return step.Summary
		case step.Skipped:
			if step.SkipReason != "" {
				return step.SkipReason
			}
			return step.Summary
		case step.Err != nil:
			return step.Err.Error()
		case step.ScriptErr != nil:
			return step.ScriptErr.Error()
		}

		failed := failedTests(step.Tests)
		if len(failed) > 0 {
			return fmt.Sprintf("%d test(s) failed", len(failed))
		}
		if msg := traceFailureText(step.Trace); msg != "" {
			return msg
		}
		if stepFailed(step) {
			return step.Summary
		}

		status := stepStatus(step)
		dur := step.Duration
		switch {
		case status == "" && dur <= 0:
			return ""
		case dur <= 0:
			return status
		case status == "":
			return dur.String()
		default:
			return fmt.Sprintf("%s in %s", status, dur)
		}
	})
}

func stepName(step StepResult) string {
	name := step.Name
	if name != "" {
		return name
	}
	if env := step.Environment; env != "" {
		return env
	}
	target := step.Target
	if target != "" {
		return target
	}
	return "<step>"
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
