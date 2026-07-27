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

func (b *documentBuilder) handleScopedVariableDirective(
	name directive.Name,
	rest string,
	line int,
) bool {
	scopeToken := name.String()
	args := rest
	if name == directive.Var {
		scopeToken, args = directive.CutToken(rest)
		if scopeToken == "" {
			return false
		}
	}

	scope, secret, ok := directive.ParseSecretScope(scopeToken)
	if !ok {
		return false
	}
	varName, value := directive.ParseNameValue(args)
	b.addScopedVariable(varName, value, line, scope, secret)
	return true
}

func (b *documentBuilder) addConstant(name, value string, line int) {
	constant := restfile.Constant{
		Name:  name,
		Value: value,
		Line:  line,
	}
	b.file.consts = append(b.file.consts, constant)
}

func (b *documentBuilder) handleConstDirective(
	line int,
	name directive.Name,
	rest string,
) bool {
	if name != directive.Const {
		return false
	}
	if name, value := directive.ParseNameValue(rest); name != "" {
		b.addConstant(name, value, line)
	}
	return true
}
