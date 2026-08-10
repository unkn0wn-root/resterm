package restwriter

import (
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// fallback names an otherwise unnamed workflow. The final newline is left off
// because callers embed the rendered metadata in larger documents and reports.
func RenderWorkflow(wf restfile.Workflow, fallback string) string {
	name := strings.TrimSpace(wf.Name)
	if name == "" {
		name = strings.TrimSpace(fallback)
	}
	if name == "" {
		name = "workflow"
	}

	var b strings.Builder
	w := workflowWriter{directiveWriter: directiveWriter{b: &b}, fail: wf.DefaultOnFailure}
	w.head(directive.Workflow, name)
	w.writeWorkflowOptions(wf)
	b.WriteString("\n")
	renderDescription(&b, wf.Description)
	renderTags(&b, wf.Tags)

	for _, step := range wf.Steps {
		w.writeStep(step)
	}
	return strings.TrimRight(b.String(), "\n")
}

type workflowWriter struct {
	directiveWriter
	fail restfile.WorkflowFailureMode
}

func (w workflowWriter) writeStep(step restfile.WorkflowStep) {
	switch step.Kind {
	case restfile.WorkflowStepKindIf:
		w.writeIf(step.If)
	case restfile.WorkflowStepKindSwitch:
		w.writeSwitch(step.Switch)
	default:
		w.writeRequest(step)
	}
}

func (w workflowWriter) writeRequest(step restfile.WorkflowStep) {
	writeOne(w.directiveWriter, step.When, conditionArg)
	writeOne(w.directiveWriter, step.ForEach, stepForEachArg)

	w.head(directive.Step, directive.Quote(strings.TrimSpace(step.Name)))
	w.option("using", step.Using)
	if step.OnFailure != w.fail {
		w.option("on-failure", string(step.OnFailure))
	}
	if step.Expect.Status != "" {
		w.option("expect.status", step.Expect.Status)
	}
	if step.Expect.StatusCode != nil {
		w.option("expect.statuscode", strconv.Itoa(*step.Expect.StatusCode))
	}
	w.writeOptions("expect.", step.Expect.Extra)
	w.writeOptions("", step.Vars)
	w.writeOptions("", step.Options)
	w.end()
}

func (w workflowWriter) writeIf(flow *restfile.WorkflowIf) {
	if flow == nil {
		return
	}
	w.writeBranch(directive.If, flow.Then.Cond, flow.Then.Run, flow.Then.Fail)
	for _, br := range flow.Elifs {
		w.writeBranch(directive.Elif, br.Cond, br.Run, br.Fail)
	}
	if br := flow.Else; br != nil {
		w.writeBranch(directive.Else, br.Cond, br.Run, br.Fail)
	}
}

func (w workflowWriter) writeSwitch(flow *restfile.WorkflowSwitch) {
	if flow == nil {
		return
	}
	w.line(directive.Switch, flow.Expr)
	for _, br := range flow.Cases {
		w.writeBranch(directive.Case, br.Expr, br.Run, br.Fail)
	}
	if br := flow.Default; br != nil {
		w.writeBranch(directive.Default, br.Expr, br.Run, br.Fail)
	}
}

func (w workflowWriter) writeBranch(name directive.Name, arg, run, fail string) {
	w.head(name, arg)
	switch {
	case run != "":
		w.option("run", run)
	case fail != "":
		w.option("fail", fail)
	}
	w.end()
}

func (w workflowWriter) writeWorkflowOptions(wf restfile.Workflow) {
	if wf.DefaultOnFailure == restfile.WorkflowOnFailureContinue {
		w.option("on-failure", string(restfile.WorkflowOnFailureContinue))
	}
	for _, key := range sortedKeys(wf.Options) {
		if strings.HasPrefix(key, "vars.") {
			w.option(key, wf.Options[key])
		}
	}
}

func (w workflowWriter) writeOptions(prefix string, opts map[string]string) {
	for _, key := range sortedKeys(opts) {
		w.option(prefix+key, opts[key])
	}
}
