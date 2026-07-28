package headless

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/restwriter"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (e *Engine) executeWorkflow(
	ctx context.Context,
	doc *restfile.Document,
	wf *restfile.Workflow,
	env vars.Environment,
) (*engine.WorkflowResult, error) {
	if wf == nil {
		return nil, fmt.Errorf("workflow is nil")
	}
	pl, err := core.PrepareWorkflow(doc, *wf, core.RunMeta{Env: env})
	if err != nil {
		return nil, err
	}
	cl := newWfCollector(pl)
	if err := core.RunPlan(ctx, e.rq, cl, pl); err != nil {
		return nil, err
	}
	out := e.buildWorkflowResult(cl.st)
	e.recordWorkflow(cl.st, out)
	return out, nil
}

func (e *Engine) executeForEach(
	ctx context.Context,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
) (*engine.WorkflowResult, error) {
	pl, err := core.PrepareForEach(doc, req, core.RunMeta{Env: env})
	if err != nil {
		return nil, err
	}
	cl := newWfCollector(pl)
	if err := core.RunPlan(ctx, e.rq, cl, pl); err != nil {
		return nil, err
	}
	return e.buildWorkflowResult(cl.st), nil
}

type wfCollector struct {
	st *wfState
}

func newWfCollector(pl *core.WorkflowPlan) *wfCollector {
	if pl == nil {
		return &wfCollector{}
	}
	st := &wfState{
		doc:  pl.Doc,
		wf:   pl.Workflow,
		env:  pl.Run.Env,
		kind: wfKindForPlan(pl.Run.Mode),
		res:  make([]wfStepRes, 0, len(pl.Steps)),
	}
	if len(pl.Steps) > 0 {
		st.steps = make([]wfRuntime, 0, len(pl.Steps))
		for _, item := range pl.Steps {
			st.steps = append(st.steps, wfRuntime{
				step: item.Step,
				req:  item.Req,
			})
		}
	}
	return &wfCollector{st: st}
}

func wfKindForPlan(mode core.Mode) wfOrigin {
	if mode == core.ModeForEach {
		return wfKindForEach
	}
	return wfKindWorkflow
}

func (c *wfCollector) OnEvt(_ context.Context, e core.Evt) error {
	if c == nil || c.st == nil || e == nil {
		return nil
	}
	switch v := e.(type) {
	case core.RunStart:
		c.st.start = v.Meta.At
	case core.RunDone:
		c.st.end = v.Meta.At
		c.st.canceled = v.Canceled
	case core.WfStepDone:
		c.st.res = append(c.st.res, c.stepRes(v))
	}
	return nil
}

func (c *wfCollector) stepRes(ev core.WfStepDone) wfStepRes {
	step, req := c.lookup(ev.Step.Index)
	if stepResultUsesExec(ev.Result) {
		return makeStepRes(step, req, ev.Result, ev.Step.Branch, ev.Step.Iter, ev.Step.Total)
	}
	return manualStepRes(step, req, ev.Step, ev.Result)
}

func (c *wfCollector) lookup(i int) (restfile.WorkflowStep, *restfile.Request) {
	if c == nil || c.st == nil || i < 0 || i >= len(c.st.steps) {
		return restfile.WorkflowStep{}, nil
	}
	return c.st.steps[i].step, c.st.steps[i].req
}

func stepResultUsesExec(res engine.RequestResult) bool {
	return res.Executed != nil ||
		res.Response != nil ||
		res.GRPC != nil ||
		res.Stream != nil ||
		len(res.Transcript) > 0 ||
		len(res.Tests) > 0 ||
		res.ScriptErr != nil ||
		strings.TrimSpace(res.RequestText) != ""
}

func manualStepRes(
	step restfile.WorkflowStep,
	req *restfile.Request,
	meta core.StepMeta,
	res engine.RequestResult,
) wfStepRes {
	out := wfStepRes{
		step:   stepOrDefault(step),
		name:   core.StepLabel(step, meta.Branch, meta.Iter, meta.Total),
		branch: meta.Branch,
		iter:   meta.Iter,
		total:  meta.Total,
		err:    res.Err,
		skip:   res.Skipped,
	}
	if res.Executed != nil {
		out.execReq = request.CloneRequest(res.Executed)
	} else {
		out.execReq = request.CloneRequest(req)
	}
	out.reqText = strings.TrimSpace(res.RequestText)
	if out.reqText == "" && out.execReq != nil {
		out.reqText = request.RenderRequestText(out.execReq)
	}
	switch {
	case out.execReq != nil:
		out.method = engine.ReqMethod(out.execReq)
		out.target = engine.ReqTarget(out.execReq)
	case req != nil:
		out.method = engine.ReqMethod(req)
		out.target = engine.ReqTarget(req)
	}
	if out.skip {
		out.msg = strings.TrimSpace(res.SkipReason)
	}
	if out.err != nil {
		out.msg = out.err.Error()
		if errors.Is(out.err, context.Canceled) {
			out.cancel = true
		}
	}
	return out
}

func (e *Engine) buildWorkflowResult(st *wfState) *engine.WorkflowResult {
	out := &engine.WorkflowResult{
		Kind:        string(st.kind),
		Name:        st.wf.Name,
		Environment: st.env.Label(),
		Selection:   st.env.Selection(),
		Summary:     workflowSummary(st),
		Report:      workflowReport(st),
		StartedAt:   st.start,
		EndedAt:     st.end,
		Duration:    st.end.Sub(st.start),
		Steps:       make([]engine.WorkflowStep, 0, len(st.res)),
	}
	out.Canceled = st.canceled
	allSkip := len(st.res) > 0
	fail := false
	for _, item := range st.res {
		step := toWorkflowStep(item)
		step.Selection = out.Selection
		out.Steps = append(out.Steps, step)
		if step.Canceled {
			out.Canceled = true
		}
		if !step.Skipped {
			allSkip = false
		}
		if step.Canceled || (!step.Skipped && !step.Success) {
			fail = true
		}
	}
	out.Skipped = allSkip
	out.Success = !out.Canceled && !out.Skipped && !fail
	return out
}

func workflowSummary(st *wfState) string {
	if st == nil {
		return "Workflow complete"
	}
	title := "Workflow"
	if st.kind == wfKindForEach {
		title = "For-each"
	}
	if name := st.wf.Name; name != "" {
		title += " " + name
	}
	if st.canceled {
		done := len(st.res)
		total := len(st.steps)
		step := done
		if done < total {
			step = done + 1
		}
		if step <= 0 {
			step = 1
		}
		if total == 0 {
			total = step
		}
		if step > total {
			step = total
		}
		return fmt.Sprintf("%s canceled at step %d/%d", title, step, total)
	}
	ok := 0
	skip := 0
	fail := 0
	for _, res := range st.res {
		switch {
		case res.skip:
			skip++
		case res.ok:
			ok++
		default:
			fail++
		}
	}
	if fail == 0 {
		if skip > 0 {
			return fmt.Sprintf("%s completed: %d passed, %d skipped", title, ok, skip)
		}
		return fmt.Sprintf("%s completed: %d/%d steps passed", title, ok, len(st.res))
	}
	last := st.res[len(st.res)-1]
	reason := strings.TrimSpace(last.msg)
	if reason == "" {
		reason = "step failed"
	}
	return fmt.Sprintf("%s failed at step %s: %s", title, last.name, reason)
}

func workflowReport(st *wfState) string {
	if st == nil {
		return ""
	}
	var b strings.Builder
	label := "Workflow"
	if st.kind == wfKindForEach {
		label = "For-each"
	}
	name := st.wf.Name
	if name == "" {
		name = label
	}
	fmt.Fprintf(&b, "%s: %s\n", label, name)
	fmt.Fprintf(&b, "Started: %s\n", st.start.Format(time.RFC3339))
	if !st.end.IsZero() {
		fmt.Fprintf(&b, "Ended: %s\n", st.end.Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "Steps: %d\n\n", len(st.steps))
	for i, res := range st.res {
		b.WriteString(workflowLine(i, res))
		b.WriteString("\n")
		if msg := strings.TrimSpace(res.msg); msg != "" {
			fmt.Fprintf(&b, "    %s\n", msg)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (e *Engine) recordWorkflow(st *wfState, out *engine.WorkflowResult) {
	hs := e.history()
	if hs == nil || st == nil || st.kind == wfKindForEach || out == nil {
		return
	}
	name := history.NormalizeWorkflowName(st.wf.Name)
	if name == "" {
		name = "Workflow"
	}
	now := time.Now()
	ent := history.Entry{
		ID:          fmt.Sprintf("%d", now.UnixNano()),
		ExecutedAt:  now,
		Environment: st.env.Label(),
		EnvironmentSelection: history.EnvironmentSelection(
			st.env.Selection().Groups(),
		),
		RequestName: name,
		FilePath:    e.filePath(st.doc),
		Method:      restfile.HistoryMethodWorkflow,
		URL:         name,
		Status:      out.Summary,
		Duration:    out.Duration,
		BodySnippet: out.Report,
		RequestText: redactText(workflowDefinition(st), e.secretValues(st.doc, nil, st.env), true),
		Description: strings.TrimSpace(st.wf.Description),
		Tags:        engine.Tags(st.wf.Tags),
	}
	_ = hs.Append(ent)
}

func workflowDefinition(st *wfState) string {
	if st == nil {
		return ""
	}
	fallback := fmt.Sprintf("workflow-%d", st.start.Unix())
	return restwriter.RenderWorkflow(st.wf, fallback)
}

func stepOrDefault(step restfile.WorkflowStep) restfile.WorkflowStep {
	if step.Kind == "" {
		step.Kind = restfile.WorkflowStepKindRequest
	}
	return step
}
