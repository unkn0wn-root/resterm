package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type expectValidator func(raw string) error

const (
	wfKeyDesc    = "description"
	wfKeyDescAlt = "desc"
	wfKeyTag     = "tag"
	wfKeyTags    = "tags"
	wfKeyWhen    = "when"
	wfKeySkipIf  = "skip-if"
	wfKeyForEach = "for-each"
	wfKeySwitch  = "switch"
	wfKeyCase    = "case"
	wfKeyDefault = "default"
	wfKeyIf      = "if"
	wfKeyElif    = "elif"
	wfKeyElse    = "else"
	wfOptOnFail  = "on-failure"
	wfOptOnFail2 = "onfailure"
	wfOptRun     = "run"
	wfOptUsing   = "using"
	wfOptFail    = "fail"
	wfOptName    = "name"
	wfPreExpect  = "expect."
	wfPreVars    = "vars."
)

var expectValidators = map[string]expectValidator{
	"status": func(raw string) error {
		if trim(raw) == "" {
			return fmt.Errorf("expect.status requires a value")
		}
		return nil
	},
	"statuscode": func(raw string) error {
		t := trim(raw)
		if t == "" {
			return fmt.Errorf("expect.statuscode requires a value")
		}
		if _, err := strconv.Atoi(t); err != nil {
			return fmt.Errorf("expect.statuscode must be an integer, got %q", raw)
		}
		return nil
	},
}

type workflowBuilder struct {
	start    int
	end      int
	wf       restfile.Workflow
	pendWhen *restfile.ConditionSpec
	pendEach *restfile.ForEachSpec
	sw       *workflowSwitchBuilder
	ifb      *workflowIfBuilder
}

func newWorkflowBuilder(line int, name string) *workflowBuilder {
	return &workflowBuilder{
		start: line,
		end:   line,
		wf: restfile.Workflow{
			Name:             trim(name),
			Tags:             []string{},
			DefaultOnFailure: restfile.WorkflowOnFailureStop,
		},
	}
}

type workflowSwitchBuilder struct {
	expr  string
	cases []restfile.WorkflowSwitchCase
	def   *restfile.WorkflowSwitchCase
	line  int
}

type workflowIfBuilder struct {
	then  restfile.WorkflowIfBranch
	elifs []restfile.WorkflowIfBranch
	els   *restfile.WorkflowIfBranch
	line  int
}

func (b *workflowBuilder) touch(line int) {
	if line > b.end {
		b.end = line
	}
}

func (b *workflowBuilder) applyOptions(opts map[string]string) {
	if len(opts) == 0 {
		return
	}
	if mode, ok := popFailMode(opts, wfOptOnFail, wfOptOnFail2); ok {
		b.wf.DefaultOnFailure = mode
	}
	if len(opts) == 0 {
		return
	}
	if b.wf.Options == nil {
		b.wf.Options = make(map[string]string, len(opts))
	}
	for key, value := range opts {
		b.wf.Options[key] = value
	}
}

func (b *workflowBuilder) handleDirective(key, rest string, line int) (bool, string) {
	key = normKey(key)
	if err := b.flushOpen(key, line); err != "" {
		return true, err
	}
	if handled, err := b.handleWorkflowMeta(key, rest, line); handled {
		return true, err
	}
	if handled, err := b.handleWorkflowCondition(key, rest, line); handled {
		return true, err
	}
	if handled, err := b.handleWorkflowSwitch(key, rest, line); handled {
		return true, err
	}
	if handled, err := b.handleWorkflowIf(key, rest, line); handled {
		return true, err
	}
	return false, ""
}

func (b *workflowBuilder) flushOpen(key string, line int) string {
	if b.sw != nil && key != wfKeyCase && key != wfKeyDefault {
		return b.flushFlow(line)
	}
	if b.ifb != nil && key != wfKeyElif && key != wfKeyElse {
		return b.flushFlow(line)
	}
	return ""
}

func (b *workflowBuilder) handleWorkflowMeta(key, rest string, line int) (bool, string) {
	switch key {
	case wfKeyDesc, wfKeyDescAlt:
		if rest == "" {
			return true, ""
		}
		if b.wf.Description != "" {
			b.wf.Description += "\n"
		}
		b.wf.Description += rest
		b.touch(line)
		return true, ""
	case wfKeyTag, wfKeyTags:
		tags := parseTagList(rest)
		if len(tags) == 0 {
			return true, ""
		}
		for _, tag := range tags {
			if !contains(b.wf.Tags, tag) {
				b.wf.Tags = append(b.wf.Tags, tag)
			}
		}
		b.touch(line)
		return true, ""
	default:
		return false, ""
	}
}

func (b *workflowBuilder) handleWorkflowCondition(key, rest string, line int) (bool, string) {
	switch key {
	case wfKeyWhen, wfKeySkipIf:
		if err := b.requireNoPending(); err != "" {
			return true, err
		}
		spec, err := parseConditionSpec(rest, line, key == wfKeySkipIf)
		if err != nil {
			return true, err.Error()
		}
		if b.pendWhen != nil {
			return true, "@when directive already defined for next step"
		}
		b.pendWhen = spec
		b.touch(line)
		return true, ""
	case wfKeyForEach:
		if err := b.requireNoPending(); err != "" {
			return true, err
		}
		spec, err := parseForEachSpec(rest, line)
		if err != nil {
			return true, err.Error()
		}
		if b.pendEach != nil {
			return true, "@for-each directive already defined for next step"
		}
		b.pendEach = spec
		b.touch(line)
		return true, ""
	default:
		return false, ""
	}
}

func (b *workflowBuilder) handleWorkflowSwitch(key, rest string, line int) (bool, string) {
	switch key {
	case wfKeySwitch:
		if err := b.requireNoPending(); err != "" {
			return true, err
		}
		if err := b.flushFlow(line); err != "" {
			return true, err
		}
		expr := trim(rest)
		if expr == "" {
			return true, "@switch expression missing"
		}
		b.sw = &workflowSwitchBuilder{expr: expr, line: line}
		b.touch(line)
		return true, ""
	case wfKeyCase:
		if b.sw == nil {
			return true, "@case without @switch"
		}
		if err := b.sw.addCase(rest, line); err != "" {
			return true, err
		}
		b.touch(line)
		return true, ""
	case wfKeyDefault:
		if b.sw == nil {
			return true, "@default without @switch"
		}
		if err := b.sw.addDefault(rest, line); err != "" {
			return true, err
		}
		b.touch(line)
		return true, ""
	default:
		return false, ""
	}
}

func (b *workflowBuilder) handleWorkflowIf(key, rest string, line int) (bool, string) {
	switch key {
	case wfKeyIf:
		if err := b.requireNoPending(); err != "" {
			return true, err
		}
		if err := b.flushFlow(line); err != "" {
			return true, err
		}
		cond, run, fail, err := parseExprRun(rest, "@if expression missing")
		if err != "" {
			return true, err
		}
		b.ifb = &workflowIfBuilder{
			then: restfile.WorkflowIfBranch{Cond: cond, Run: run, Fail: fail, Line: line},
			line: line,
		}
		b.touch(line)
		return true, ""
	case wfKeyElif:
		if b.ifb == nil {
			return true, "@elif without @if"
		}
		cond, run, fail, err := parseExprRun(rest, "@elif expression missing")
		if err != "" {
			return true, err
		}
		b.ifb.elifs = append(
			b.ifb.elifs,
			restfile.WorkflowIfBranch{Cond: cond, Run: run, Fail: fail, Line: line},
		)
		b.touch(line)
		return true, ""
	case wfKeyElse:
		if b.ifb == nil {
			return true, "@else without @if"
		}
		if b.ifb.els != nil {
			return true, "@else already defined"
		}
		opts := parseOptionTokens(rest)
		run, fail, err := parseWorkflowRunOptions(opts)
		if err != "" {
			return true, err
		}
		b.ifb.els = &restfile.WorkflowIfBranch{Run: run, Fail: fail, Line: line}
		b.touch(line)
		return true, ""
	default:
		return false, ""
	}
}

func (b *workflowBuilder) requireNoPending() string {
	if b.pendWhen != nil {
		return "@when must be followed by @step"
	}
	if b.pendEach != nil {
		return "@for-each must be followed by @step"
	}
	return ""
}

func (b *workflowBuilder) flushFlow(line int) string {
	if b.sw != nil {
		if len(b.sw.cases) == 0 && b.sw.def == nil {
			return "@switch requires at least one @case or @default"
		}
		step := restfile.WorkflowStep{
			Kind: restfile.WorkflowStepKindSwitch,
			Switch: &restfile.WorkflowSwitch{
				Expr:    b.sw.expr,
				Cases:   b.sw.cases,
				Default: b.sw.def,
				Line:    b.sw.line,
			},
			Line:      b.sw.line,
			OnFailure: b.wf.DefaultOnFailure,
		}
		b.wf.Steps = append(b.wf.Steps, step)
		b.sw = nil
		b.touch(line)
	}
	if b.ifb != nil {
		step := restfile.WorkflowStep{
			Kind: restfile.WorkflowStepKindIf,
			If: &restfile.WorkflowIf{
				Cond:  b.ifb.then.Cond,
				Then:  b.ifb.then,
				Elifs: b.ifb.elifs,
				Else:  b.ifb.els,
				Line:  b.ifb.line,
			},
			Line:      b.ifb.line,
			OnFailure: b.wf.DefaultOnFailure,
		}
		b.wf.Steps = append(b.wf.Steps, step)
		b.ifb = nil
		b.touch(line)
	}
	return ""
}

func (sw *workflowSwitchBuilder) addCase(rest string, line int) string {
	expr, run, fail, err := parseExprRun(rest, "@case expression missing")
	if err != "" {
		return err
	}
	sw.cases = append(
		sw.cases,
		restfile.WorkflowSwitchCase{Expr: expr, Run: run, Fail: fail, Line: line},
	)
	return ""
}

func (sw *workflowSwitchBuilder) addDefault(rest string, line int) string {
	if sw.def != nil {
		return "@default already defined"
	}
	opts := parseOptionTokens(rest)
	run, fail, err := parseWorkflowRunOptions(opts)
	if err != "" {
		return err
	}
	sw.def = &restfile.WorkflowSwitchCase{Run: run, Fail: fail, Line: line}
	return ""
}

func parseExprRun(rest, miss string) (string, string, string, string) {
	expr, opts := splitExprOptions(rest)
	expr = trim(expr)
	if expr == "" {
		return "", "", "", miss
	}
	run, fail, err := parseWorkflowRunOptions(opts)
	if err != "" {
		return "", "", "", err
	}
	return expr, run, fail, ""
}

func parseWorkflowRunOptions(opts map[string]string) (string, string, string) {
	run := trim(opts[wfOptRun])
	if run == "" {
		run = trim(opts[wfOptUsing])
	}
	fail := trim(opts[wfOptFail])
	if run == "" && fail == "" {
		return "", "", "expected run=... or fail=..."
	}
	if run != "" && fail != "" {
		return "", "", "cannot combine run and fail"
	}
	return run, fail, ""
}

func (b *workflowBuilder) addStep(line int, rest string) string {
	if err := b.flushFlow(line); err != "" {
		return err
	}
	name, opts, err := parseStepSpec(rest)
	if err != "" {
		return err
	}
	use := popOptAny(opts, wfOptUsing, wfOptRun)
	if use == "" {
		return "@step missing using request"
	}
	step := restfile.WorkflowStep{
		Kind:      restfile.WorkflowStepKindRequest,
		Name:      name,
		Using:     use,
		OnFailure: b.wf.DefaultOnFailure,
		Line:      line,
	}
	if val := popOpt(opts, wfOptOnFail); val != "" {
		if mode, ok := parseWorkflowFailureMode(val); ok {
			step.OnFailure = mode
		}
	}
	expErr := applyStepOpts(&step, opts)
	b.applyPending(&step)
	b.wf.Steps = append(b.wf.Steps, step)
	b.touch(line)
	return expErr
}

func parseStepSpec(rest string) (string, map[string]string, string) {
	rem := trim(rest)
	if rem == "" {
		return "", nil, "@step missing content"
	}
	name := ""
	tok, tail := splitFirst(rem)
	if tok != "" && !strings.Contains(tok, "=") {
		name = tok
		rem = tail
	}
	opts := parseOptionTokens(rem)
	if nm, ok := opts[wfOptName]; ok {
		if name == "" {
			name = nm
		}
		delete(opts, wfOptName)
	}
	return name, opts, ""
}

func applyStepOpts(step *restfile.WorkflowStep, opts map[string]string) string {
	if len(opts) == 0 {
		return ""
	}
	var errs []string
	var left map[string]string
	for key, val := range opts {
		switch {
		case strings.HasPrefix(key, wfPreExpect):
			suf := strings.TrimPrefix(key, wfPreExpect)
			if suf == "" {
				continue
			}
			if validate, ok := expectValidators[suf]; ok {
				if err := validate(val); err != nil {
					errs = append(errs, err.Error())
					continue
				}
			}
			if step.Expect == nil {
				step.Expect = make(map[string]string)
			}
			step.Expect[suf] = val
		case strings.HasPrefix(key, wfPreVars):
			key = trim(key)
			if key == "" {
				continue
			}
			if step.Vars == nil {
				step.Vars = make(map[string]string)
			}
			step.Vars[key] = val
		default:
			if left == nil {
				left = make(map[string]string)
			}
			left[key] = val
		}
	}
	if len(left) > 0 {
		step.Options = left
	}
	if len(errs) > 0 {
		return strings.Join(errs, "; ")
	}
	return ""
}

func (b *workflowBuilder) applyPending(step *restfile.WorkflowStep) {
	if b.pendWhen != nil {
		step.When = b.pendWhen
		b.pendWhen = nil
	}
	if b.pendEach != nil {
		step.Kind = restfile.WorkflowStepKindForEach
		step.ForEach = &restfile.WorkflowForEach{
			Expr: b.pendEach.Expression,
			Var:  b.pendEach.Var,
			Line: b.pendEach.Line,
		}
		b.pendEach = nil
	}
}

func popFailMode(opts map[string]string, keys ...string) (restfile.WorkflowFailureMode, bool) {
	for _, key := range keys {
		val, ok := opts[key]
		if !ok {
			continue
		}
		delete(opts, key)
		if mode, ok := parseWorkflowFailureMode(val); ok {
			return mode, true
		}
	}
	return "", false
}

func (b *workflowBuilder) build(line int) restfile.Workflow {
	if line > 0 {
		b.touch(line)
	}
	b.wf.LineRange = restfile.LineRange{Start: b.start, End: b.end}
	if b.wf.LineRange.End < b.wf.LineRange.Start {
		b.wf.LineRange.End = b.wf.LineRange.Start
	}
	return b.wf
}
