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
	w := workflowWriter{b: &b, fail: wf.DefaultOnFailure}
	w.writeHead(directive.Workflow, name)
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
	b    *strings.Builder
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
	if step.When != nil {
		name := directive.When
		if step.When.Negate {
			name = directive.SkipIf
		}
		w.writeLine(name, step.When.Expression)
	}
	if step.ForEach != nil {
		w.writeLine(directive.ForEach, step.ForEach.Expr+" as "+step.ForEach.Var)
	}

	w.writeHead(directive.Step, directive.Quote(strings.TrimSpace(step.Name)))
	w.writeOption("using", step.Using)
	if step.OnFailure != w.fail {
		w.writeOption("on-failure", string(step.OnFailure))
	}
	if step.Expect.Status != "" {
		w.writeOption("expect.status", step.Expect.Status)
	}
	if step.Expect.StatusCode != nil {
		w.writeOption("expect.statuscode", strconv.Itoa(*step.Expect.StatusCode))
	}
	w.writeOptions("expect.", step.Expect.Extra)
	w.writeOptions("", step.Vars)
	w.writeOptions("", step.Options)
	w.b.WriteString("\n")
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
	w.writeLine(directive.Switch, flow.Expr)
	for _, br := range flow.Cases {
		w.writeBranch(directive.Case, br.Expr, br.Run, br.Fail)
	}
	if br := flow.Default; br != nil {
		w.writeBranch(directive.Default, br.Expr, br.Run, br.Fail)
	}
}

func (w workflowWriter) writeBranch(name directive.Name, arg, run, fail string) {
	w.writeHead(name, arg)
	switch {
	case run != "":
		w.writeOption("run", run)
	case fail != "":
		w.writeOption("fail", fail)
	}
	w.b.WriteString("\n")
}

func (w workflowWriter) writeLine(name directive.Name, arg string) {
	w.writeHead(name, arg)
	w.b.WriteString("\n")
}

func (w workflowWriter) writeHead(name directive.Name, arg string) {
	w.b.WriteString(name.Comment())
	if arg != "" {
		w.b.WriteString(" ")
		w.b.WriteString(arg)
	}
}

func (w workflowWriter) writeWorkflowOptions(wf restfile.Workflow) {
	if wf.DefaultOnFailure == restfile.WorkflowOnFailureContinue {
		w.writeOption("on-failure", string(restfile.WorkflowOnFailureContinue))
	}
	for _, key := range sortedKeys(wf.Options) {
		if strings.HasPrefix(key, "vars.") {
			w.writeOption(key, wf.Options[key])
		}
	}
}

func (w workflowWriter) writeOptions(prefix string, opts map[string]string) {
	for _, key := range sortedKeys(opts) {
		w.writeOption(prefix+key, opts[key])
	}
}

func (w workflowWriter) writeOption(key, value string) {
	w.b.WriteString(" ")
	w.b.WriteString(key)
	w.b.WriteString("=")
	w.b.WriteString(directive.Quote(value))
}
