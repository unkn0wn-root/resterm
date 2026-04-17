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
	"sort"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/headless"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
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
	Kind                      ResultKind
	Name                      string
	Method                    string
	Target                    string
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
	requestText string
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

	sel := newSelectSpec(opts.Select)
	target, err := selectTarget(doc, sel)
	if err != nil {
		return nil, err
	}

	if target.workflow != nil {
		if opts.Profile || len(opts.CompareTargets) > 0 {
			return nil, usageError("--workflow cannot be combined with --compare or --profile")
		}
		rep.Results = make([]Result, 0, 1)
		out, err := exec.ExecuteWorkflowContext(ctx, doc, target.workflow, opts.EnvName)
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

	rep.Results = make([]Result, 0, len(target.requests))
	for _, req := range target.requests {
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

type selectSpec struct {
	request  string
	workflow string
	tag      string
	all      bool
	line     int
}

type selectedTarget struct {
	requests []*restfile.Request
	workflow *restfile.Workflow
}

func newSelectSpec(sel Select) selectSpec {
	return selectSpec{
		request:  strings.TrimSpace(sel.Request),
		workflow: strings.TrimSpace(sel.Workflow),
		tag:      strings.TrimSpace(sel.Tag),
		all:      sel.All,
		line:     sel.Line,
	}
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

func selectTarget(doc *restfile.Document, sel selectSpec) (selectedTarget, error) {
	if sel.line > 0 {
		return selectByLine(doc, sel)
	}
	if sel.workflow != "" {
		if sel.all || sel.request != "" || sel.tag != "" {
			return selectedTarget{}, usageError("--workflow cannot be combined with --request, --tag, or --all")
		}
		wf, err := selectWorkflow(doc, sel.workflow)
		if err != nil {
			return selectedTarget{}, err
		}
		return selectedTarget{workflow: wf}, nil
	}
	reqs, err := selectRequests(doc, sel)
	if err != nil {
		return selectedTarget{}, err
	}
	return selectedTarget{requests: reqs}, nil
}

func selectRequests(doc *restfile.Document, sel selectSpec) ([]*restfile.Request, error) {
	if doc == nil || len(doc.Requests) == 0 {
		return nil, usageError("no requests found")
	}

	if sel.all && sel.line > 0 {
		return nil, usageError("--all cannot be combined with --line")
	}
	if sel.all && (sel.request != "" || sel.tag != "") {
		return nil, usageError("--all cannot be combined with --request or --tag")
	}
	if sel.request != "" && sel.line > 0 {
		return nil, usageError("--request cannot be combined with --line")
	}
	if sel.tag != "" && sel.line > 0 {
		return nil, usageError("--tag cannot be combined with --line")
	}
	if sel.request != "" && sel.tag != "" {
		return nil, usageError("--request cannot be combined with --tag")
	}

	if sel.all {
		return append([]*restfile.Request(nil), doc.Requests...), nil
	}

	if sel.request != "" {
		return selectByRequestName(doc.Requests, sel.request)
	}

	if sel.tag != "" {
		return selectByTag(doc.Requests, sel.tag)
	}

	if len(doc.Requests) == 1 {
		return []*restfile.Request{doc.Requests[0]}, nil
	}
	return nil, usageError("multiple requests found; use --request, --tag, --line, or --all")
}

func selectByLine(doc *restfile.Document, sel selectSpec) (selectedTarget, error) {
	if sel.workflow != "" || sel.request != "" || sel.tag != "" || sel.all {
		return selectedTarget{}, usageError("--line cannot be combined with --workflow, --request, --tag, or --all")
	}
	if sel.line <= 0 {
		return selectedTarget{}, usageError("--line must be greater than zero")
	}

	reqs := selectRequestsByLine(doc, sel.line)
	wfs := selectWorkflowsByLine(doc, sel.line)
	switch total := len(reqs) + len(wfs); total {
	case 0:
		return selectedTarget{}, usageError("line %d did not match any request or workflow", sel.line)
	case 1:
		if len(wfs) == 1 {
			return selectedTarget{workflow: wfs[0]}, nil
		}
		return selectedTarget{requests: reqs}, nil
	default:
		return selectedTarget{}, usageError("line %d matched %d entries", sel.line, total)
	}
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

func selectRequestsByLine(doc *restfile.Document, line int) []*restfile.Request {
	if doc == nil || line <= 0 {
		return nil
	}
	out := make([]*restfile.Request, 0, 1)
	for _, req := range doc.Requests {
		if req == nil || !lineInRange(line, req.LineRange) {
			continue
		}
		out = append(out, req)
	}
	return out
}

func selectWorkflowsByLine(doc *restfile.Document, line int) []*restfile.Workflow {
	if doc == nil || line <= 0 {
		return nil
	}
	out := make([]*restfile.Workflow, 0, 1)
	for i := range doc.Workflows {
		wf := &doc.Workflows[i]
		if !lineInRange(line, wf.LineRange) {
			continue
		}
		out = append(out, wf)
	}
	return out
}

func lineInRange(line int, rg restfile.LineRange) bool {
	if line <= 0 || rg.Start <= 0 {
		return false
	}
	end := rg.End
	if end < rg.Start {
		end = rg.Start
	}
	return line >= rg.Start && line <= end
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
	name := strings.TrimSpace(req.Metadata.Name)
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
		if method := strings.TrimSpace(req.GRPC.FullMethod); method != "" {
			return method
		}
		if target := strings.TrimSpace(req.GRPC.Target); target != "" {
			return target
		}
	}
	return strings.TrimSpace(req.URL)
}

func requestTarget(
	req *restfile.Request,
	resp *httpclient.Response,
) string {
	if resp != nil {
		if target := strings.TrimSpace(resp.EffectiveURL); target != "" {
			return target
		}
	}
	return requestSourceTarget(req)
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
		Target:      requestTarget(runReq, res.Response),
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
		requestText: strings.TrimSpace(res.RequestText),
		transcript:  bytes.Clone(res.Transcript),
	}
	if res.Explain != nil {
		item.SetUnresolvedTemplateVars(explainMissingTemplateVars(res.Explain))
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
		Target:      requestSourceTarget(req),
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
		Target:      requestTarget(req, row.Response),
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
		requestText: "",
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
		Target:      requestSourceTarget(req),
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
	target := strings.TrimSpace(step.Target)
	if step.Response != nil {
		if effective := strings.TrimSpace(step.Response.EffectiveURL); effective != "" {
			target = effective
		}
	}
	return StepResult{
		Name:        strings.TrimSpace(step.Name),
		Method:      strings.TrimSpace(step.Method),
		Target:      target,
		Branch:      strings.TrimSpace(step.Branch),
		Iteration:   step.Iteration,
		Total:       step.Total,
		Summary:     strings.TrimSpace(step.Summary),
		Duration:    step.Duration,
		Response:    step.Response,
		GRPC:        step.GRPC,
		Err:         step.Err,
		Tests:       cloneTests(step.Tests),
		ScriptErr:   step.ScriptErr,
		Passed:      step.Success,
		Skipped:     step.Skipped,
		Canceled:    step.Canceled,
		Stream:      streamResult(step.Stream),
		Trace:       traceResult(step.Response),
		requestText: "",
		transcript:  bytes.Clone(step.Transcript),
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
		name := strings.TrimSpace(item.Name)
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
