package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

var workflowFailureAliases = map[string]restfile.WorkflowFailureMode{
	string(restfile.WorkflowOnFailureStop): restfile.WorkflowOnFailureStop,
	"fail":                                 restfile.WorkflowOnFailureStop,
	"abort":                                restfile.WorkflowOnFailureStop,
	string(restfile.WorkflowOnFailureContinue): restfile.WorkflowOnFailureContinue,
	"skip": restfile.WorkflowOnFailureContinue,
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
			Name:             str.Trim(name),
			Tags:             []string{},
			DefaultOnFailure: restfile.WorkflowOnFailureStop,
		},
	}
}

func (b *workflowBuilder) touch(line int) {
	if line > b.end {
		b.end = line
	}
}

// Keep recognized aliases out of the unconsumed options exposed to tooling.
func (b *workflowBuilder) applyOptions(opts directive.Options) error {
	if opts.Len() == 0 {
		return nil
	}
	if val, ok := opts.PopAny("on-failure", "onfailure"); ok {
		if mode, ok := parseWorkflowFailureMode(val); ok {
			b.wf.DefaultOnFailure = mode
		}
	}
	err := opts.Conflicts(directive.Workflow)
	if opts.Len() == 0 {
		return err
	}
	if b.wf.Options == nil {
		b.wf.Options = make(map[string]string, opts.Len())
	}
	opts.CopyTo(b.wf.Options)
	return err
}

func (b *workflowBuilder) handleDirective(
	call directive.Call,
	line int,
) (bool, error) {
	if err := b.flushOpen(call.Name, line); err != nil {
		return true, err
	}
	if handled, err := b.handleWorkflowMeta(call.Name, call.Args, line); handled {
		return true, err
	}
	if handled, err := b.handleWorkflowCondition(call, line); handled {
		return true, err
	}
	if handled, err := b.handleWorkflowSwitch(call.Name, call.Args, line); handled {
		return true, err
	}
	if handled, err := b.handleWorkflowIf(call.Name, call.Args, line); handled {
		return true, err
	}
	return false, nil
}

func (b *workflowBuilder) flushOpen(name directive.Name, line int) error {
	if b.sw != nil && name != directive.Case && name != directive.Default {
		return b.flushFlow(line)
	}
	if b.ifb != nil && name != directive.Elif && name != directive.Else {
		return b.flushFlow(line)
	}
	return nil
}

func (b *workflowBuilder) handleWorkflowMeta(
	name directive.Name,
	rest string,
	line int,
) (bool, error) {
	switch name {
	case directive.Description:
		if rest == "" {
			return true, nil
		}
		b.wf.Description = appendDesc(b.wf.Description, rest)
		b.touch(line)
		return true, nil
	case directive.Tag:
		tags := parseTagList(rest)
		if len(tags) == 0 {
			return true, nil
		}
		b.wf.Tags = appendTagsFold(b.wf.Tags, tags)
		b.touch(line)
		return true, nil
	default:
		return false, nil
	}
}

func (b *workflowBuilder) handleWorkflowCondition(
	call directive.Call,
	line int,
) (bool, error) {
	switch call.Name {
	case directive.When:
		if err := b.requireNoPending(); err != nil {
			return true, err
		}
		spec, err := parseConditionSpec(
			call.Args,
			line,
			call.Spelling == directive.SkipIf,
		)
		if err != nil {
			return true, err
		}
		if b.pendWhen != nil {
			return true, errors.New("@when directive already defined for next step")
		}
		b.pendWhen = spec
		b.touch(line)
		return true, nil
	case directive.ForEach:
		if err := b.requireNoPending(); err != nil {
			return true, err
		}
		spec, err := parseForEachSpec(call.Args, line)
		if err != nil {
			return true, err
		}
		if b.pendEach != nil {
			return true, errors.New("@for-each directive already defined for next step")
		}
		b.pendEach = spec
		b.touch(line)
		return true, nil
	default:
		return false, nil
	}
}

func (b *workflowBuilder) handleWorkflowSwitch(
	name directive.Name,
	rest string,
	line int,
) (bool, error) {
	switch name {
	case directive.Switch:
		if err := b.requireNoPending(); err != nil {
			return true, err
		}
		if err := b.flushFlow(line); err != nil {
			return true, err
		}
		expr := str.Trim(rest)
		if expr == "" {
			return true, errors.New("@switch expression missing")
		}
		b.sw = &workflowSwitchBuilder{expr: expr, line: line}
		b.touch(line)
		return true, nil
	case directive.Case:
		if b.sw == nil {
			return true, errors.New("@case without @switch")
		}
		if err := b.sw.addCase(rest, line); err != nil {
			return true, err
		}
		b.touch(line)
		return true, nil
	case directive.Default:
		if b.sw == nil {
			return true, errors.New("@default without @switch")
		}
		if err := b.sw.addDefault(rest, line); err != nil {
			return true, err
		}
		b.touch(line)
		return true, nil
	default:
		return false, nil
	}
}

func (b *workflowBuilder) handleWorkflowIf(
	name directive.Name,
	rest string,
	line int,
) (bool, error) {
	switch name {
	case directive.If:
		if err := b.requireNoPending(); err != nil {
			return true, err
		}
		if err := b.flushFlow(line); err != nil {
			return true, err
		}
		cond, run, fail, err := parseExprRun(directive.If, rest, "@if expression missing")
		if err != nil {
			return true, err
		}
		b.ifb = &workflowIfBuilder{
			then: restfile.WorkflowIfBranch{Cond: cond, Run: run, Fail: fail, Line: line},
			line: line,
		}
		b.touch(line)
		return true, nil
	case directive.Elif:
		if b.ifb == nil {
			return true, errors.New("@elif without @if")
		}
		cond, run, fail, err := parseExprRun(directive.Elif, rest, "@elif expression missing")
		if err != nil {
			return true, err
		}
		b.ifb.elifs = append(
			b.ifb.elifs,
			restfile.WorkflowIfBranch{Cond: cond, Run: run, Fail: fail, Line: line},
		)
		b.touch(line)
		return true, nil
	case directive.Else:
		if b.ifb == nil {
			return true, errors.New("@else without @if")
		}
		if b.ifb.els != nil {
			return true, errors.New("@else already defined")
		}
		opts, err := directive.ParseOptions(directive.Else, rest)
		if err != nil {
			return true, err
		}
		run, fail, err := parseWorkflowRunOptions(name, opts)
		if err != nil {
			return true, err
		}
		b.ifb.els = &restfile.WorkflowIfBranch{Run: run, Fail: fail, Line: line}
		b.touch(line)
		return true, nil
	default:
		return false, nil
	}
}

func (b *workflowBuilder) requireNoPending() error {
	if b.pendWhen != nil {
		return errors.New("@when must be followed by @step")
	}
	if b.pendEach != nil {
		return errors.New("@for-each must be followed by @step")
	}
	return nil
}

func (b *workflowBuilder) flushFlow(line int) error {
	if b.sw != nil {
		if len(b.sw.cases) == 0 && b.sw.def == nil {
			return errors.New("@switch requires at least one @case or @default")
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
	return nil
}

func (sw *workflowSwitchBuilder) addCase(rest string, line int) error {
	expr, run, fail, err := parseExprRun(directive.Case, rest, "@case expression missing")
	if err != nil {
		return err
	}
	sw.cases = append(
		sw.cases,
		restfile.WorkflowSwitchCase{Expr: expr, Run: run, Fail: fail, Line: line},
	)
	return nil
}

func (sw *workflowSwitchBuilder) addDefault(rest string, line int) error {
	if sw.def != nil {
		return errors.New("@default already defined")
	}
	opts, err := directive.ParseOptions(directive.Default, rest)
	if err != nil {
		return err
	}
	run, fail, err := parseWorkflowRunOptions(directive.Default, opts)
	if err != nil {
		return err
	}
	sw.def = &restfile.WorkflowSwitchCase{Run: run, Fail: fail, Line: line}
	return nil
}

func parseExprRun(name directive.Name, rest, miss string) (expr, run, fail string, err error) {
	expr, tail := cutBranch(rest)
	if expr == "" {
		return "", "", "", errors.New(miss)
	}
	opts, err := directive.ParseOptions(name, tail)
	if err != nil {
		return "", "", "", err
	}
	run, fail, err = parseWorkflowRunOptions(name, opts)
	if err != nil {
		return "", "", "", err
	}
	return expr, run, fail, nil
}

// cutBranch ignores options inside strings, comments, and nested groups.
// It slices the original text so quoted option values keep their spacing.
func cutBranch(rest string) (expr, opts string) {
	mask := rts.Mask(rest)
	for _, f := range directive.FieldSpans(mask) {
		if f.Eq >= 0 {
			return strings.TrimSpace(rest[:f.Start]), rest[f.Start:]
		}
	}
	return strings.TrimSpace(rest), ""
}

func parseWorkflowRunOptions(name directive.Name, opts directive.Options) (run, fail string, err error) {
	run, _ = opts.First("run", "using")
	fail = opts.Get("fail")
	if err := opts.Conflicts(name); err != nil {
		return "", "", err
	}
	if run == "" && fail == "" {
		return "", "", errors.New("missing a run= or fail= option")
	}
	if run != "" && fail != "" {
		return "", "", errors.New("cannot combine run and fail")
	}
	return run, fail, nil
}

func (b *workflowBuilder) addStep(line int, rest string) error {
	if err := b.flushFlow(line); err != nil {
		return err
	}
	name, opts, err := parseStepSpec(rest)
	if err != nil {
		return err
	}
	use, _ := opts.PopAny("using", "run")
	if use == "" {
		return errors.New("@step missing using request")
	}
	step := restfile.WorkflowStep{
		Kind:      restfile.WorkflowStepKindRequest,
		Name:      name,
		Using:     use,
		OnFailure: b.wf.DefaultOnFailure,
		Line:      line,
	}
	if val := opts.Pop("on-failure"); val != "" {
		if mode, ok := parseWorkflowFailureMode(val); ok {
			step.OnFailure = mode
		}
	}
	// A step with a bad expect option is still added so the workflow keeps
	// its shape. The error is reported next to it.
	expErr := errors.Join(opts.Conflicts(directive.Step), applyStepOpts(&step, opts))
	b.applyPending(&step)
	b.wf.Steps = append(b.wf.Steps, step)
	b.touch(line)
	return expErr
}

// The alias is the first word. Quoting it lets it hold spaces or an equals sign,
// while bare words after it stay options the way @step has always read them. The
// name= option is a fallback for a step that opens with an option.
func parseStepSpec(rest string) (string, directive.Options, error) {
	if str.Trim(rest) == "" {
		return "", directive.Options{}, errors.New("@step missing content")
	}
	name, tail := directive.CutName(rest)
	opts, err := directive.ParseOptions(directive.Step, tail)
	if err != nil {
		return "", directive.Options{}, err
	}
	if nm := opts.Pop("name"); name == "" {
		name = nm
	}
	return name, opts, nil
}

func applyStepOpts(step *restfile.WorkflowStep, opts directive.Options) error {
	if opts.Len() == 0 {
		return nil
	}
	var errs []string
	var left map[string]string
	for key, val := range opts.All() {
		switch {
		case strings.HasPrefix(key, "expect."):
			switch suf := strings.TrimPrefix(key, "expect."); suf {
			case "":
			case "status":
				if str.Trim(val) == "" {
					errs = append(errs, "expect.status requires a value")
					continue
				}
				step.Expect.Status = val
			case "statuscode":
				t := str.Trim(val)
				if t == "" {
					errs = append(errs, "expect.statuscode requires a value")
					continue
				}
				n, err := strconv.Atoi(t)
				if err != nil {
					errs = append(errs, fmt.Sprintf("expect.statuscode must be an integer, got %q", val))
					continue
				}
				step.Expect.StatusCode = &n
			default:
				if step.Expect.Extra == nil {
					step.Expect.Extra = make(map[string]string)
				}
				step.Expect.Extra[suf] = val
			}
		case strings.HasPrefix(key, "vars."):
			key = str.Trim(key)
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
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
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

func parseWorkflowFailureMode(value string) (restfile.WorkflowFailureMode, bool) {
	s := strings.TrimSpace(strings.ToLower(value))
	if s == "" {
		return "", false
	}
	mode, ok := workflowFailureAliases[s]
	return mode, ok
}

func parseTagList(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || r == ','
	})
	var tags []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			tags = append(tags, part)
		}
	}
	return tags
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
