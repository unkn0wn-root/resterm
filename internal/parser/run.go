package parser

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (b *documentBuilder) addRun(d parsedDirective) directiveOutcome {
	if err := declareRun(&b.request.runVars, d); err != nil {
		return b.rejectError(d, err)
	}
	return directiveApplied
}

func (b *workflowBuilder) addRun(d parsedDirective) error {
	if err := declareRun(&b.wf.RunVars, d); err != nil {
		return err
	}
	b.touch(d.lines.Start)
	return nil
}

func declareRun(dst *[]restfile.RunVar, d parsedDirective) error {
	sub, rest := directive.CutToken(d.Args)
	switch strings.ToLower(sub) {
	case directive.RunVarWord:
		v, err := parseRunVar(d, rest)
		if err != nil {
			return err
		}
		*dst, err = declareRunVar(*dst, v)
		return err
	case "":
		return fmt.Errorf("%s requires '%s <name> = <value>'", directive.Run.Tag(), directive.RunVarWord)
	default:
		return fmt.Errorf("%s subcommand %q is unknown, use %s", directive.Run.Tag(), sub, directive.RunVarWord)
	}
}

func parseRunVar(d parsedDirective, rest string) (restfile.RunVar, error) {
	if rest == "" {
		return restfile.RunVar{}, fmt.Errorf("%s name missing", directive.RunVarTag)
	}
	name, value := directive.ParseNameValue(rest)
	if name == "" {
		tok, _ := directive.CutToken(rest)
		return restfile.RunVar{}, fmt.Errorf("%s name %q is invalid", directive.RunVarTag, tok)
	}
	// Run values are public, and env: would be left as plain text.
	if _, ok := vars.EnvRefKey(value); ok {
		return restfile.RunVar{}, fmt.Errorf(
			"%s cannot read env: references, use @file or @request",
			directive.RunVarTag,
		)
	}
	v := restfile.RunVar{Name: name, Value: value, Line: d.lines.Start}
	d.setExprCol(&v.Col, value)
	return v, nil
}

func declareRunVar(vs []restfile.RunVar, v restfile.RunVar) ([]restfile.RunVar, error) {
	for _, o := range vs {
		if strings.EqualFold(o.Name, v.Name) {
			return vs, fmt.Errorf("%s %q is already declared", directive.RunVarTag, v.Name)
		}
	}
	return append(vs, v), nil
}
