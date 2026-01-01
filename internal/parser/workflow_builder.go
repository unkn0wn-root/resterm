package parser

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type workflowBuilder struct {
	startLine      int
	endLine        int
	workflow       restfile.Workflow
	pendingWhen    *restfile.ConditionSpec
	pendingForEach *restfile.ForEachSpec
	openSwitch     *workflowSwitchBuilder
	openIf         *workflowIfBuilder
}

func newWorkflowBuilder(line int, name string) *workflowBuilder {
	return &workflowBuilder{
		startLine: line,
		endLine:   line,
		workflow: restfile.Workflow{
			Name:             strings.TrimSpace(name),
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

func (s *workflowBuilder) touch(line int) {
	if line > s.endLine {
		s.endLine = line
	}
}

func (s *workflowBuilder) applyOptions(opts map[string]string) {
	if len(opts) == 0 {
		return
	}
	leftovers := make(map[string]string)
	for key, value := range opts {
		switch key {
		case "on-failure", "onfailure":
			if mode, ok := parseWorkflowFailureMode(value); ok {
				s.workflow.DefaultOnFailure = mode
			}
		default:
			leftovers[key] = value
		}
	}
	if len(leftovers) > 0 {
		if s.workflow.Options == nil {
			s.workflow.Options = make(map[string]string, len(leftovers))
		}
		for key, value := range leftovers {
			s.workflow.Options[key] = value
		}
	}
}

func (s *workflowBuilder) handleDirective(key, rest string, line int) (bool, string) {
	if s.openSwitch != nil && key != "case" && key != "default" {
		if err := s.flushFlow(line); err != "" {
			return true, err
		}
	}
	if s.openIf != nil && key != "elif" && key != "else" {
		if err := s.flushFlow(line); err != "" {
			return true, err
		}
	}
	if handled, err := s.handleWorkflowMeta(key, rest, line); handled {
		return true, err
	}
	if handled, err := s.handleWorkflowCondition(key, rest, line); handled {
		return true, err
	}
	if handled, err := s.handleWorkflowSwitch(key, rest, line); handled {
		return true, err
	}
	if handled, err := s.handleWorkflowIf(key, rest, line); handled {
		return true, err
	}
	return false, ""
}

func (s *workflowBuilder) handleWorkflowMeta(key, rest string, line int) (bool, string) {
	switch key {
	case "description", "desc":
		if rest == "" {
			return true, ""
		}
		if s.workflow.Description != "" {
			s.workflow.Description += "\n"
		}
		s.workflow.Description += rest
		s.touch(line)
		return true, ""
	case "tag", "tags":
		tags := parseTagList(rest)
		if len(tags) == 0 {
			return true, ""
		}
		for _, tag := range tags {
			if !contains(s.workflow.Tags, tag) {
				s.workflow.Tags = append(s.workflow.Tags, tag)
			}
		}
		s.touch(line)
		return true, ""
	default:
		return false, ""
	}
}

func (s *workflowBuilder) handleWorkflowCondition(key, rest string, line int) (bool, string) {
	switch key {
	case "when", "skip-if":
		if err := s.requireNoPending(); err != "" {
			return true, err
		}
		spec, err := parseConditionSpec(rest, line, key == "skip-if")
		if err != nil {
			return true, err.Error()
		}
		if s.pendingWhen != nil {
			return true, "@when directive already defined for next step"
		}
		s.pendingWhen = spec
		s.touch(line)
		return true, ""
	case "for-each":
		if err := s.requireNoPending(); err != "" {
			return true, err
		}
		spec, err := parseForEachSpec(rest, line)
		if err != nil {
			return true, err.Error()
		}
		if s.pendingForEach != nil {
			return true, "@for-each directive already defined for next step"
		}
		s.pendingForEach = spec
		s.touch(line)
		return true, ""
	default:
		return false, ""
	}
}

func (s *workflowBuilder) handleWorkflowSwitch(key, rest string, line int) (bool, string) {
	switch key {
	case "switch":
		if err := s.requireNoPending(); err != "" {
			return true, err
		}
		if err := s.flushFlow(line); err != "" {
			return true, err
		}
		expr := strings.TrimSpace(rest)
		if expr == "" {
			return true, "@switch expression missing"
		}
		s.openSwitch = &workflowSwitchBuilder{expr: expr, line: line}
		s.touch(line)
		return true, ""
	case "case":
		if s.openSwitch == nil {
			return true, "@case without @switch"
		}
		if err := s.openSwitch.addCase(rest, line); err != "" {
			return true, err
		}
		s.touch(line)
		return true, ""
	case "default":
		if s.openSwitch == nil {
			return true, "@default without @switch"
		}
		if err := s.openSwitch.addDefault(rest, line); err != "" {
			return true, err
		}
		s.touch(line)
		return true, ""
	default:
		return false, ""
	}
}

func (s *workflowBuilder) handleWorkflowIf(key, rest string, line int) (bool, string) {
	switch key {
	case "if":
		if err := s.requireNoPending(); err != "" {
			return true, err
		}
		if err := s.flushFlow(line); err != "" {
			return true, err
		}
		cond, opts := splitExprOptions(rest)
		cond = strings.TrimSpace(cond)
		if cond == "" {
			return true, "@if expression missing"
		}
		run, fail, err := parseWorkflowRunOptions(opts)
		if err != "" {
			return true, err
		}
		s.openIf = &workflowIfBuilder{
			then: restfile.WorkflowIfBranch{Cond: cond, Run: run, Fail: fail, Line: line},
			line: line,
		}
		s.touch(line)
		return true, ""
	case "elif":
		if s.openIf == nil {
			return true, "@elif without @if"
		}
		cond, opts := splitExprOptions(rest)
		cond = strings.TrimSpace(cond)
		if cond == "" {
			return true, "@elif expression missing"
		}
		run, fail, err := parseWorkflowRunOptions(opts)
		if err != "" {
			return true, err
		}
		s.openIf.elifs = append(
			s.openIf.elifs,
			restfile.WorkflowIfBranch{Cond: cond, Run: run, Fail: fail, Line: line},
		)
		s.touch(line)
		return true, ""
	case "else":
		if s.openIf == nil {
			return true, "@else without @if"
		}
		if s.openIf.els != nil {
			return true, "@else already defined"
		}
		opts := parseOptionTokens(rest)
		run, fail, err := parseWorkflowRunOptions(opts)
		if err != "" {
			return true, err
		}
		s.openIf.els = &restfile.WorkflowIfBranch{Run: run, Fail: fail, Line: line}
		s.touch(line)
		return true, ""
	default:
		return false, ""
	}
}

func (s *workflowBuilder) requireNoPending() string {
	if s.pendingWhen != nil {
		return "@when must be followed by @step"
	}
	if s.pendingForEach != nil {
		return "@for-each must be followed by @step"
	}
	return ""
}

func (s *workflowBuilder) flushFlow(line int) string {
	if s.openSwitch != nil {
		if len(s.openSwitch.cases) == 0 && s.openSwitch.def == nil {
			return "@switch requires at least one @case or @default"
		}
		step := restfile.WorkflowStep{
			Kind: restfile.WorkflowStepKindSwitch,
			Switch: &restfile.WorkflowSwitch{
				Expr:    s.openSwitch.expr,
				Cases:   s.openSwitch.cases,
				Default: s.openSwitch.def,
				Line:    s.openSwitch.line,
			},
			Line:      s.openSwitch.line,
			OnFailure: s.workflow.DefaultOnFailure,
		}
		s.workflow.Steps = append(s.workflow.Steps, step)
		s.openSwitch = nil
		s.touch(line)
	}
	if s.openIf != nil {
		step := restfile.WorkflowStep{
			Kind: restfile.WorkflowStepKindIf,
			If: &restfile.WorkflowIf{
				Cond:  s.openIf.then.Cond,
				Then:  s.openIf.then,
				Elifs: s.openIf.elifs,
				Else:  s.openIf.els,
				Line:  s.openIf.line,
			},
			Line:      s.openIf.line,
			OnFailure: s.workflow.DefaultOnFailure,
		}
		s.workflow.Steps = append(s.workflow.Steps, step)
		s.openIf = nil
		s.touch(line)
	}
	return ""
}

func (b *workflowSwitchBuilder) addCase(rest string, line int) string {
	expr, opts := splitExprOptions(rest)
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "@case expression missing"
	}
	run, fail, err := parseWorkflowRunOptions(opts)
	if err != "" {
		return err
	}
	b.cases = append(
		b.cases,
		restfile.WorkflowSwitchCase{Expr: expr, Run: run, Fail: fail, Line: line},
	)
	return ""
}

func (b *workflowSwitchBuilder) addDefault(rest string, line int) string {
	if b.def != nil {
		return "@default already defined"
	}
	opts := parseOptionTokens(rest)
	run, fail, err := parseWorkflowRunOptions(opts)
	if err != "" {
		return err
	}
	b.def = &restfile.WorkflowSwitchCase{Run: run, Fail: fail, Line: line}
	return ""
}

func parseWorkflowRunOptions(opts map[string]string) (string, string, string) {
	run := strings.TrimSpace(opts["run"])
	if run == "" {
		run = strings.TrimSpace(opts["using"])
	}
	fail := strings.TrimSpace(opts["fail"])
	if run == "" && fail == "" {
		return "", "", "expected run=... or fail=..."
	}
	if run != "" && fail != "" {
		return "", "", "cannot combine run and fail"
	}
	return run, fail, ""
}

func (s *workflowBuilder) addStep(line int, rest string) string {
	if err := s.flushFlow(line); err != "" {
		return err
	}
	remainder := strings.TrimSpace(rest)
	if remainder == "" {
		return "@step missing content"
	}
	name := ""
	firstToken, remainderAfterFirst := splitFirst(remainder)
	if firstToken != "" && !strings.Contains(firstToken, "=") {
		name = firstToken
		remainder = remainderAfterFirst
	}
	options := parseOptionTokens(remainder)
	if explicitName, ok := options["name"]; ok {
		if name == "" {
			name = explicitName
		}
		delete(options, "name")
	}
	using := options["using"]
	if using == "" {
		using = options["run"]
	}
	if using == "" {
		return "@step missing using request"
	}
	delete(options, "using")
	delete(options, "run")
	step := restfile.WorkflowStep{
		Kind:      restfile.WorkflowStepKindRequest,
		Name:      name,
		Using:     strings.TrimSpace(using),
		OnFailure: s.workflow.DefaultOnFailure,
		Line:      line,
	}
	if mode, ok := options["on-failure"]; ok {
		if parsed, ok := parseWorkflowFailureMode(mode); ok {
			step.OnFailure = parsed
		}
		delete(options, "on-failure")
	}
	if len(options) > 0 {
		leftover := make(map[string]string)
		for key, value := range options {
			switch {
			case strings.HasPrefix(key, "expect."):
				suffix := strings.TrimPrefix(key, "expect.")
				if suffix == "" {
					continue
				}
				if step.Expect == nil {
					step.Expect = make(map[string]string)
				}
				step.Expect[suffix] = value
			case strings.HasPrefix(key, "vars."):
				sanitized := strings.TrimSpace(key)
				if sanitized == "" {
					continue
				}
				if step.Vars == nil {
					step.Vars = make(map[string]string)
				}
				step.Vars[sanitized] = value
			default:
				leftover[key] = value
			}
		}
		if len(leftover) > 0 {
			step.Options = leftover
		}
	}
	if s.pendingWhen != nil {
		step.When = s.pendingWhen
		s.pendingWhen = nil
	}
	if s.pendingForEach != nil {
		step.Kind = restfile.WorkflowStepKindForEach
		step.ForEach = &restfile.WorkflowForEach{
			Expr: s.pendingForEach.Expression,
			Var:  s.pendingForEach.Var,
			Line: s.pendingForEach.Line,
		}
		s.pendingForEach = nil
	}
	s.workflow.Steps = append(s.workflow.Steps, step)
	s.touch(line)
	return ""
}

func (s *workflowBuilder) build(line int) restfile.Workflow {
	if line > 0 {
		s.touch(line)
	}
	s.workflow.LineRange = restfile.LineRange{Start: s.startLine, End: s.endLine}
	if s.workflow.LineRange.End < s.workflow.LineRange.Start {
		s.workflow.LineRange.End = s.workflow.LineRange.Start
	}
	return s.workflow
}
