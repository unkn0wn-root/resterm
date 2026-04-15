package headless

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/analysis"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"google.golang.org/grpc/codes"
)

type wfOrigin string

const (
	wfKindWorkflow wfOrigin = "workflow"
	wfKindForEach  wfOrigin = "for-each"
)

const (
	wfStatusPass     = "[PASS]"
	wfStatusFail     = "[FAIL]"
	wfStatusCanceled = "[CANCELED]"
	wfStatusSkipped  = "[SKIPPED]"
)

type wfState struct {
	doc      *restfile.Document
	wf       restfile.Workflow
	steps    []wfRuntime
	env      string
	kind     wfOrigin
	res      []wfStepRes
	start    time.Time
	end      time.Time
	canceled bool
}

type wfRuntime struct {
	step restfile.WorkflowStep
	req  *restfile.Request
}

type wfStepRes struct {
	step    restfile.WorkflowStep
	name    string
	method  string
	target  string
	branch  string
	iter    int
	total   int
	status  string
	msg     string
	ok      bool
	skip    bool
	cancel  bool
	dur     time.Duration
	http    *httpclient.Response
	grpc    *grpcclient.Response
	stream  *scripts.StreamInfo
	raw     []byte
	tests   []scripts.TestResult
	sErr    error
	err     error
	execReq *restfile.Request
	reqText string
}

type profileState struct {
	req       *restfile.Request
	env       string
	spec      restfile.ProfileSpec
	total     int
	idx       int
	start     time.Time
	end       time.Time
	mStart    time.Time
	mEnd      time.Time
	ok        []time.Duration
	fail      []engine.ProfileFailure
	skip      bool
	skipMsg   string
	cancel    bool
	cancelMsg string
}

func cloneHTTP(resp *httpclient.Response) *httpclient.Response {
	if resp == nil {
		return nil
	}
	out := *resp
	out.Headers = cloneHeader(resp.Headers)
	out.RequestHeaders = cloneHeader(resp.RequestHeaders)
	out.Body = copyBytes(resp.Body)
	if resp.Request != nil {
		out.Request = request.CloneRequest(resp.Request)
	}
	return &out
}

func cloneGRPC(resp *grpcclient.Response) *grpcclient.Response {
	if resp == nil {
		return nil
	}
	out := *resp
	out.Headers = cloneMap(resp.Headers)
	out.Trailers = cloneMap(resp.Trailers)
	out.Body = copyBytes(resp.Body)
	out.Wire = copyBytes(resp.Wire)
	return &out
}

func cloneMap(src map[string][]string) map[string][]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string][]string, len(src))
	for k, v := range src {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func cloneHeader(src http.Header) http.Header {
	if len(src) == 0 {
		return nil
	}
	out := make(http.Header, len(src))
	for k, v := range src {
		out[k] = append([]string(nil), v...)
	}
	return out
}

func cloneStream(info *scripts.StreamInfo) *scripts.StreamInfo {
	if info == nil {
		return nil
	}
	out := *info
	if len(info.Summary) > 0 {
		out.Summary = make(map[string]any, len(info.Summary))
		for k, v := range info.Summary {
			out.Summary[k] = v
		}
	}
	if len(info.Events) > 0 {
		out.Events = make([]map[string]any, len(info.Events))
		for i, ev := range info.Events {
			if ev == nil {
				continue
			}
			cp := make(map[string]any, len(ev))
			for k, v := range ev {
				cp[k] = v
			}
			out.Events[i] = cp
		}
	}
	return &out
}

func copyBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	return append([]byte(nil), src...)
}

func workflowLabel(step restfile.WorkflowStep) string {
	if step.Name != "" {
		return step.Name
	}
	switch step.Kind {
	case restfile.WorkflowStepKindIf:
		return "@if"
	case restfile.WorkflowStepKindSwitch:
		return "@switch"
	case restfile.WorkflowStepKindForEach:
		if step.Using != "" {
			return step.Using
		}
		return "@for-each"
	default:
		return step.Using
	}
}

func workflowStepLabel(step restfile.WorkflowStep, branch string, iter, total int) string {
	lbl := workflowLabel(step)
	if lbl == "" {
		lbl = "step"
	}
	if branch != "" {
		lbl = fmt.Sprintf("%s -> %s", lbl, branch)
	}
	if iter > 0 && total > 0 {
		lbl = fmt.Sprintf("%s (%d/%d)", lbl, iter, total)
	}
	return lbl
}

func workflowStatus(res wfStepRes) string {
	switch {
	case res.cancel:
		return wfStatusCanceled
	case res.skip:
		return wfStatusSkipped
	case res.ok:
		return wfStatusPass
	default:
		return wfStatusFail
	}
}

func workflowLine(i int, res wfStepRes) string {
	line := fmt.Sprintf("%d. %s %s", i+1, res.name, workflowStatus(res))
	if res.status != "" {
		line += fmt.Sprintf(" (%s)", res.status)
	}
	if res.dur > 0 {
		line += fmt.Sprintf(" [%s]", res.dur.Truncate(time.Millisecond))
	}
	return line
}

func makeStepRes(
	step restfile.WorkflowStep,
	req *restfile.Request,
	out engine.RequestResult,
	branch string,
	iter int,
	total int,
) wfStepRes {
	res := wfStepRes{
		step:    step,
		name:    workflowStepLabel(step, branch, iter, total),
		branch:  branch,
		iter:    iter,
		total:   total,
		http:    cloneHTTP(out.Response),
		grpc:    cloneGRPC(out.GRPC),
		stream:  cloneStream(out.Stream),
		raw:     copyBytes(out.Transcript),
		tests:   append([]scripts.TestResult(nil), out.Tests...),
		sErr:    out.ScriptErr,
		err:     out.Err,
		execReq: request.CloneRequest(out.Executed),
		reqText: out.RequestText,
		skip:    out.Skipped,
	}
	if res.execReq == nil {
		res.execReq = request.CloneRequest(req)
	}
	if res.reqText == "" && res.execReq != nil {
		res.reqText = request.RenderRequestText(res.execReq)
	}
	switch {
	case res.execReq != nil:
		res.method = engine.ReqMethod(res.execReq)
		res.target = engine.ReqTarget(res.execReq)
	case req != nil:
		res.method = engine.ReqMethod(req)
		res.target = engine.ReqTarget(req)
	}
	return evaluateStep(res)
}

func evaluateStep(res wfStepRes) wfStepRes {
	if res.skip {
		res.msg = strings.TrimSpace(res.msg)
		return res
	}
	ok := true
	msg := strings.TrimSpace(res.msg)
	switch {
	case res.err != nil:
		ok = false
		res.status = strings.TrimSpace(errdef.Message(res.err))
		msg = res.status
		if errors.Is(res.err, context.Canceled) {
			res.cancel = true
		}
	case res.http != nil:
		res.status = res.http.Status
		if res.http.Duration > 0 {
			res.dur = res.http.Duration
		}
		if res.http.StatusCode >= 400 && !hasStatusExp(res.step.Expect) {
			ok = false
			msg = fmt.Sprintf("unexpected status code %d", res.http.StatusCode)
		}
	case res.grpc != nil:
		res.status = res.grpc.StatusCode.String()
		if res.grpc.Duration > 0 {
			res.dur = res.grpc.Duration
		}
	default:
		ok = false
		msg = "request failed"
	}
	if ok && res.sErr != nil {
		ok = false
		msg = res.sErr.Error()
	}
	if ok {
		for _, t := range res.tests {
			if t.Passed {
				continue
			}
			ok = false
			if strings.TrimSpace(t.Message) != "" {
				msg = t.Message
			} else {
				msg = fmt.Sprintf("test failed: %s", t.Name)
			}
			break
		}
	}
	if ok {
		if exp, okExp := res.step.Expect["status"]; okExp {
			want := strings.TrimSpace(exp)
			if want == "" || !strings.EqualFold(want, strings.TrimSpace(res.status)) {
				ok = false
				msg = fmt.Sprintf("expected status %s", want)
			}
		}
		if exp, okExp := res.step.Expect["statuscode"]; okExp {
			want, err := strconv.Atoi(strings.TrimSpace(exp))
			if err != nil {
				ok = false
				msg = fmt.Sprintf("invalid expected status code %q", exp)
			} else {
				got := 0
				switch {
				case res.http != nil:
					got = res.http.StatusCode
				case res.grpc != nil:
					got = int(res.grpc.StatusCode)
				}
				if got != want {
					ok = false
					msg = fmt.Sprintf("expected status code %d", want)
				}
			}
		}
	}
	res.ok = ok
	res.msg = strings.TrimSpace(msg)
	return res
}

func hasStatusExp(exp map[string]string) bool {
	if len(exp) == 0 {
		return false
	}
	_, ok := exp["status"]
	if ok {
		return true
	}
	_, ok = exp["statuscode"]
	return ok
}

func toWorkflowStep(res wfStepRes) engine.WorkflowStep {
	sum := strings.TrimSpace(res.msg)
	if sum == "" {
		sum = strings.TrimSpace(res.status)
	}
	return engine.WorkflowStep{
		Name:       res.name,
		Method:     res.method,
		Target:     res.target,
		Branch:     res.branch,
		Iteration:  res.iter,
		Total:      res.total,
		Summary:    sum,
		Response:   cloneHTTP(res.http),
		GRPC:       cloneGRPC(res.grpc),
		Stream:     cloneStream(res.stream),
		Transcript: copyBytes(res.raw),
		Err:        res.err,
		Tests:      append([]scripts.TestResult(nil), res.tests...),
		ScriptErr:  res.sErr,
		Skipped:    res.skip,
		Canceled:   res.cancel,
		Success:    res.ok,
		Duration:   res.dur,
	}
}

func compareSuccess(row engine.CompareRow) bool {
	if row.Canceled || row.Skipped || row.Err != nil || row.ScriptErr != nil {
		return false
	}
	for _, t := range row.Tests {
		if !t.Passed {
			return false
		}
	}
	switch {
	case row.Response != nil:
		return row.Response.StatusCode < 400
	case row.GRPC != nil:
		return row.GRPC.StatusCode == codes.OK
	default:
		return false
	}
}

func compareStatus(row engine.CompareRow) (string, string) {
	switch {
	case row.Canceled:
		return "canceled", "-"
	case row.Skipped:
		return "skipped", "-"
	case row.Err != nil:
		return "error", ""
	case row.Response != nil:
		return row.Response.Status, fmt.Sprintf("%d", row.Response.StatusCode)
	case row.GRPC != nil:
		return row.GRPC.StatusCode.String(), fmt.Sprintf("%d", row.GRPC.StatusCode)
	default:
		return "pending", "-"
	}
}

func compareSummary(base, row engine.CompareRow) string {
	if row.Canceled {
		return "canceled"
	}
	if row.Skipped {
		if reason := strings.TrimSpace(row.SkipReason); reason != "" {
			return "skipped: " + reason
		}
		return "skipped"
	}
	if row.Err != nil {
		return "error: " + errdef.Message(row.Err)
	}
	if row.ScriptErr != nil {
		return "tests error: " + row.ScriptErr.Error()
	}
	n := 0
	for _, t := range row.Tests {
		if !t.Passed {
			n++
		}
	}
	if n > 0 {
		return fmt.Sprintf("%d test(s) failed", n)
	}
	if strings.EqualFold(base.Environment, row.Environment) {
		return "baseline"
	}
	switch {
	case row.Response != nil && base.Response != nil:
		return summarizeHTTP(base.Response, row.Response)
	case row.GRPC != nil && base.GRPC != nil:
		return summarizeGRPC(base.GRPC, row.GRPC)
	default:
		return "unavailable"
	}
}

func summarizeHTTP(base, row *httpclient.Response) string {
	if base == nil || row == nil {
		return "unavailable"
	}
	var diff []string
	if row.StatusCode != base.StatusCode {
		diff = append(diff, "status")
	}
	if !headersEqual(row.Headers, base.Headers) {
		diff = append(diff, "headers")
	}
	if !equalBytes(row.Body, base.Body) {
		diff = append(diff, "body")
	}
	if len(diff) == 0 {
		return "match"
	}
	return strings.Join(diff, ", ") + " differ"
}

func summarizeGRPC(base, row *grpcclient.Response) string {
	if base == nil || row == nil {
		return "unavailable"
	}
	var diff []string
	if row.StatusCode != base.StatusCode {
		diff = append(diff, "status")
	}
	if strings.TrimSpace(row.StatusMessage) != strings.TrimSpace(base.StatusMessage) {
		diff = append(diff, "message")
	}
	if strings.TrimSpace(row.Message) != strings.TrimSpace(base.Message) {
		diff = append(diff, "body")
	}
	if len(diff) == 0 {
		return "match"
	}
	return strings.Join(diff, ", ") + " differ"
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func headersEqual(a, b http.Header) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		w, ok := b[k]
		if !ok || len(v) != len(w) {
			return false
		}
		x := append([]string(nil), v...)
		y := append([]string(nil), w...)
		sort.Strings(x)
		sort.Strings(y)
		for i := range x {
			if x[i] != y[i] {
				return false
			}
		}
	}
	return true
}

func profileOutcome(out engine.RequestResult) (bool, string) {
	if out.Skipped {
		reason := strings.TrimSpace(out.SkipReason)
		if reason == "" {
			reason = "request skipped"
		}
		return false, reason
	}
	if out.Err != nil {
		return false, errdef.Message(out.Err)
	}
	if out.Response != nil && out.Response.StatusCode >= 400 {
		return false, fmt.Sprintf("HTTP %s", out.Response.Status)
	}
	if out.ScriptErr != nil {
		return false, out.ScriptErr.Error()
	}
	for _, t := range out.Tests {
		if t.Passed {
			continue
		}
		if strings.TrimSpace(t.Message) != "" {
			return false, fmt.Sprintf("Test failed: %s – %s", t.Name, t.Message)
		}
		return false, fmt.Sprintf("Test failed: %s", t.Name)
	}
	if out.Response == nil {
		return false, "no response"
	}
	return true, ""
}

func buildProfileResults(st *profileState, stats analysis.LatencyStats) *history.ProfileResults {
	if st == nil {
		return nil
	}
	out := &history.ProfileResults{
		TotalRuns:      st.idx,
		WarmupRuns:     min(st.idx, st.spec.Warmup),
		SuccessfulRuns: len(st.ok),
		FailedRuns:     len(st.fail),
	}
	if stats.Count == 0 {
		return out
	}
	out.Latency = &history.ProfileLatency{
		Count:  stats.Count,
		Min:    stats.Min,
		Max:    stats.Max,
		Mean:   stats.Mean,
		Median: stats.Median,
		StdDev: stats.StdDev,
	}
	if len(stats.Percentiles) > 0 {
		ps := make([]history.ProfilePercentile, 0, len(stats.Percentiles))
		for p, v := range stats.Percentiles {
			ps = append(ps, history.ProfilePercentile{Percentile: p, Value: v})
		}
		sort.Slice(ps, func(i, j int) bool { return ps[i].Percentile < ps[j].Percentile })
		out.Percentiles = ps
	}
	if len(stats.Histogram) > 0 {
		out.Histogram = make([]history.ProfileHistogramBin, len(stats.Histogram))
		for i, b := range stats.Histogram {
			out.Histogram[i] = history.ProfileHistogramBin{From: b.From, To: b.To, Count: b.Count}
		}
	}
	return out
}
