package headless

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (e *Engine) executeWorkflow(
	ctx context.Context,
	doc *restfile.Document,
	wf *restfile.Workflow,
	env string,
) (*engine.WorkflowResult, error) {
	if wf == nil {
		return nil, fmt.Errorf("workflow is nil")
	}
	pl, err := core.PrepareWorkflow(doc, *wf, core.RunMeta{Env: e.env(env)})
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
	env string,
) (*engine.WorkflowResult, error) {
	pl, err := core.PrepareForEach(doc, req, core.RunMeta{Env: e.env(env)})
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
		env:  strings.TrimSpace(pl.Run.Env),
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
		name:   workflowStepLabel(step, meta.Branch, meta.Iter, meta.Total),
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
		out.method = requestMethod(out.execReq)
		out.target = requestTarget(out.execReq)
	case req != nil:
		out.method = requestMethod(req)
		out.target = requestTarget(req)
	}
	if out.skip {
		out.msg = strings.TrimSpace(res.SkipReason)
	}
	if out.err != nil {
		out.msg = strings.TrimSpace(out.err.Error())
		if errors.Is(out.err, context.Canceled) {
			out.cancel = true
		}
	}
	return out
}

func (e *Engine) buildWorkflowResult(st *wfState) *engine.WorkflowResult {
	out := &engine.WorkflowResult{
		Kind:        string(st.kind),
		Name:        strings.TrimSpace(st.wf.Name),
		Environment: st.env,
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
	if name := strings.TrimSpace(st.wf.Name); name != "" {
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
	name := strings.TrimSpace(st.wf.Name)
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
		Environment: st.env,
		RequestName: name,
		FilePath:    e.filePath(st.doc),
		Method:      restfile.HistoryMethodWorkflow,
		URL:         name,
		Status:      out.Summary,
		Duration:    out.Duration,
		BodySnippet: out.Report,
		RequestText: redactText(workflowDefinition(st), e.secretValues(st.doc, nil, st.env), true),
		Description: strings.TrimSpace(st.wf.Description),
		Tags:        normalizedTags(st.wf.Tags),
	}
	_ = hs.Append(ent)
}

func workflowDefinition(st *wfState) string {
	if st == nil {
		return ""
	}
	var b strings.Builder
	name := strings.TrimSpace(st.wf.Name)
	if name == "" {
		name = fmt.Sprintf("workflow-%d", st.start.Unix())
	}
	b.WriteString("# @workflow ")
	b.WriteString(name)
	if st.wf.DefaultOnFailure == restfile.WorkflowOnFailureContinue {
		b.WriteString(" on-failure=continue")
	}
	for k, v := range st.wf.Options {
		if strings.HasPrefix(k, "vars.") {
			fmt.Fprintf(&b, " %s=%s", k, v)
		}
	}
	b.WriteString("\n")
	for _, step := range st.wf.Steps {
		writeWorkflowStep(&b, st.wf.DefaultOnFailure, step)
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeWorkflowStep(
	b *strings.Builder,
	df restfile.WorkflowFailureMode,
	step restfile.WorkflowStep,
) {
	if b == nil {
		return
	}
	switch step.Kind {
	case restfile.WorkflowStepKindIf:
		writeIfStep(b, step.If)
	case restfile.WorkflowStepKindSwitch:
		writeSwitchStep(b, step.Switch)
	default:
		if step.When != nil {
			tag := "@when"
			if step.When.Negate {
				tag = "@skip-if"
			}
			fmt.Fprintf(b, "# %s %s\n", tag, step.When.Expression)
		}
		if step.ForEach != nil {
			fmt.Fprintf(b, "# @for-each %s as %s\n", step.ForEach.Expr, step.ForEach.Var)
		}
		b.WriteString("# @step ")
		if name := strings.TrimSpace(step.Name); name != "" {
			b.WriteString(name)
			b.WriteString(" ")
		}
		b.WriteString("using=")
		b.WriteString(step.Using)
		if step.OnFailure != df {
			b.WriteString(" on-failure=")
			b.WriteString(string(step.OnFailure))
		}
		for k, v := range step.Expect {
			fmt.Fprintf(b, " expect.%s=%s", k, v)
		}
		for k, v := range step.Vars {
			fmt.Fprintf(b, " vars.%s=%s", k, v)
		}
		for k, v := range step.Options {
			fmt.Fprintf(b, " %s=%s", k, v)
		}
		b.WriteString("\n")
	}
}

func writeIfStep(b *strings.Builder, blk *restfile.WorkflowIf) {
	if b == nil || blk == nil {
		return
	}
	fmt.Fprintf(b, "# @if %s%s\n", blk.Then.Cond, runFailSuffix(blk.Then.Run, blk.Then.Fail))
	for _, br := range blk.Elifs {
		fmt.Fprintf(b, "# @elif %s%s\n", br.Cond, runFailSuffix(br.Run, br.Fail))
	}
	if blk.Else != nil {
		fmt.Fprintf(b, "# @else%s\n", runFailSuffix(blk.Else.Run, blk.Else.Fail))
	}
}

func writeSwitchStep(b *strings.Builder, blk *restfile.WorkflowSwitch) {
	if b == nil || blk == nil {
		return
	}
	fmt.Fprintf(b, "# @switch %s\n", blk.Expr)
	for _, br := range blk.Cases {
		fmt.Fprintf(b, "# @case %s%s\n", br.Expr, runFailSuffix(br.Run, br.Fail))
	}
	if blk.Default != nil {
		fmt.Fprintf(b, "# @default%s\n", runFailSuffix(blk.Default.Run, blk.Default.Fail))
	}
}

func runFailSuffix(run, fail string) string {
	if run != "" {
		return " run=" + quoteOpt(run)
	}
	if fail != "" {
		return " fail=" + quoteOpt(fail)
	}
	return ""
}

func quoteOpt(v string) string {
	if v == "" {
		return v
	}
	if strings.ContainsAny(v, " \t\"") {
		return strconv.Quote(v)
	}
	return v
}

func stepOrDefault(step restfile.WorkflowStep) restfile.WorkflowStep {
	if step.Kind == "" {
		step.Kind = restfile.WorkflowStepKindRequest
	}
	return step
}
