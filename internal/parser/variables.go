package parser

import (
	"regexp"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

var variableLineRe = regexp.MustCompile(
	`^@(?:(global(?:-secret)?|file(?:-secret)?|request(?:-secret)?)\s+)?([A-Za-z0-9_.-]+)(?:\s*(?::|=)\s*(.+?)|\s+(\S.*))$`,
)

func (b *documentBuilder) handleVariableLine(ln line) bool {
	matches := variableLineRe.FindStringSubmatch(ln.text)
	if matches == nil {
		return false
	}
	name := matches[2]
	valueCandidate := matches[3]
	if valueCandidate == "" {
		valueCandidate = matches[4]
	}
	value := strings.TrimSpace(valueCandidate)

	// A shorthand variable without a scope belongs to whichever block it sits in.
	scope, secret, ok := directive.ParseSecretScope(matches[1])
	if !ok {
		scope = directive.ScopeRequest
		if !b.inRequest {
			scope = directive.ScopeFile
		}
	}
	b.addScopedVariable(name, value, ln.no, scope, secret)
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) addScopedVariable(
	name, value string,
	line int,
	scope directive.Scope,
	secret bool,
) {
	if name == "" {
		return
	}
	variable := restfile.Variable{
		Name:   name,
		Value:  value,
		Line:   line,
		Scope:  scope,
		Secret: secret,
	}
	switch scope {
	case directive.ScopeGlobal:
		b.file.globals = append(b.file.globals, variable)
	case directive.ScopeFile:
		b.file.vars = append(b.file.vars, variable)
	case directive.ScopeRequest:
		b.ensureRequest(line)
		b.request.variables = append(b.request.variables, variable)
	}
}

func (b *documentBuilder) handleScopedVariableDirective(d directiveLine) directiveOutcome {
	token, args := d.Name.String(), d.Args
	if d.Name == directive.Var {
		token, args = directive.CutToken(d.Args)
		if token == "" {
			return directiveIgnored
		}
	}

	scope, secret, ok := directive.ParseSecretScope(token)
	if !ok {
		return directiveIgnored
	}
	name, value := directive.ParseNameValue(args)
	b.addScopedVariable(name, value, d.no, scope, secret)
	return directiveApplied
}

func (b *documentBuilder) addConstant(name, value string, line int) {
	constant := restfile.Constant{
		Name:  name,
		Value: value,
		Line:  line,
	}
	b.file.consts = append(b.file.consts, constant)
}

func (b *documentBuilder) handleConstDirective(d directiveLine) directiveOutcome {
	if d.Name != directive.Const {
		return directiveIgnored
	}
	if name, value := directive.ParseNameValue(d.Args); name != "" {
		b.addConstant(name, value, d.no)
	}
	return directiveApplied
}
