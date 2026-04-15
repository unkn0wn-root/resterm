package headless

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
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

const (
	workflowStepDefaultLabel = "step"
	workflowExpectStatus     = "status"
	workflowExpectStatusCode = "statuscode"
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
	label := strings.TrimSpace(workflowLabel(step))
	if label == "" {
		label = workflowStepDefaultLabel
	}
	if branch != "" {
		label = fmt.Sprintf("%s -> %s", label, branch)
	}
	if iter > 0 && total > 0 {
		label = fmt.Sprintf("%s (%d/%d)", label, iter, total)
	}
	return label
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
		tests:   slices.Clone(out.Tests),
		sErr:    out.ScriptErr,
		err:     out.Err,
		execReq: request.CloneRequest(out.Executed),
		reqText: out.RequestText,
		msg:     strings.TrimSpace(out.SkipReason),
		skip:    out.Skipped,
	}
	if res.execReq == nil {
		res.execReq = request.CloneRequest(req)
	}
	if res.reqText == "" && res.execReq != nil {
		res.reqText = request.RenderRequestText(res.execReq)
	}
	assignWorkflowStepIdentity(&res, req)
	return evaluateStep(res)
}

func assignWorkflowStepIdentity(res *wfStepRes, fallbackReq *restfile.Request) {
	if res == nil {
		return
	}

	switch {
	case res.execReq != nil:
		res.method = engine.ReqMethod(res.execReq)
		res.target = engine.ReqTarget(res.execReq)
	case fallbackReq != nil:
		res.method = engine.ReqMethod(fallbackReq)
		res.target = engine.ReqTarget(fallbackReq)
	}
}

func evaluateStep(res wfStepRes) wfStepRes {
	if res.skip {
		res.msg = strings.TrimSpace(res.msg)
		return res
	}

	ok, msg := workflowTransportOutcome(&res)
	if ok && res.sErr != nil {
		ok = false
		msg = res.sErr.Error()
	}
	if ok {
		if failMsg, failed := firstFailedWorkflowTest(res.tests); failed {
			ok = false
			msg = failMsg
		}
	}
	if ok {
		if expectMsg, failed := workflowExpectationFailure(res); failed {
			ok = false
			msg = expectMsg
		}
	}

	res.ok = ok
	res.msg = strings.TrimSpace(msg)
	return res
}

func workflowTransportOutcome(res *wfStepRes) (bool, string) {
	msg := strings.TrimSpace(res.msg)

	switch {
	case res.err != nil:
		res.status = strings.TrimSpace(errdef.Message(res.err))
		if res.status == "" {
			res.status = strings.TrimSpace(res.err.Error())
		}
		msg = res.status
		if errors.Is(res.err, context.Canceled) {
			res.cancel = true
		}
		return false, msg
	case res.http != nil:
		res.status = res.http.Status
		if res.http.Duration > 0 {
			res.dur = res.http.Duration
		}
		if res.http.StatusCode >= 400 && !hasStatusExpectation(res.step.Expect) {
			return false, fmt.Sprintf("unexpected status code %d", res.http.StatusCode)
		}
		return true, msg
	case res.grpc != nil:
		res.status = res.grpc.StatusCode.String()
		if res.grpc.Duration > 0 {
			res.dur = res.grpc.Duration
		}
		return true, msg
	default:
		return false, "request failed"
	}
}

func firstFailedWorkflowTest(tests []scripts.TestResult) (string, bool) {
	for _, test := range tests {
		if test.Passed {
			continue
		}
		if strings.TrimSpace(test.Message) != "" {
			return test.Message, true
		}
		return fmt.Sprintf("test failed: %s", test.Name), true
	}
	return "", false
}

func workflowExpectationFailure(res wfStepRes) (string, bool) {
	if exp, ok := res.step.Expect[workflowExpectStatus]; ok {
		want := strings.TrimSpace(exp)
		if want == "" || !strings.EqualFold(want, strings.TrimSpace(res.status)) {
			return fmt.Sprintf("expected status %s", want), true
		}
	}

	if exp, ok := res.step.Expect[workflowExpectStatusCode]; ok {
		want, err := strconv.Atoi(strings.TrimSpace(exp))
		if err != nil {
			return fmt.Sprintf("invalid expected status code %q", exp), true
		}
		got, ok := workflowStatusCode(res)
		if !ok || got != want {
			return fmt.Sprintf("expected status code %d", want), true
		}
	}

	return "", false
}

func workflowStatusCode(res wfStepRes) (int, bool) {
	switch {
	case res.http != nil:
		return res.http.StatusCode, true
	case res.grpc != nil:
		return int(res.grpc.StatusCode), true
	default:
		return 0, false
	}
}

func hasStatusExpectation(exp map[string]string) bool {
	if len(exp) == 0 {
		return false
	}
	_, hasStatus := exp[workflowExpectStatus]
	_, hasStatusCode := exp[workflowExpectStatusCode]
	return hasStatus || hasStatusCode
}

func toWorkflowStep(res wfStepRes) engine.WorkflowStep {
	summary := strings.TrimSpace(res.msg)
	if summary == "" {
		summary = strings.TrimSpace(res.status)
	}
	return engine.WorkflowStep{
		Name:       res.name,
		Method:     res.method,
		Target:     res.target,
		Branch:     res.branch,
		Iteration:  res.iter,
		Total:      res.total,
		Summary:    summary,
		Response:   cloneHTTP(res.http),
		GRPC:       cloneGRPC(res.grpc),
		Stream:     cloneStream(res.stream),
		Transcript: copyBytes(res.raw),
		Err:        res.err,
		Tests:      slices.Clone(res.tests),
		ScriptErr:  res.sErr,
		Skipped:    res.skip,
		Canceled:   res.cancel,
		Success:    res.ok,
		Duration:   res.dur,
	}
}
