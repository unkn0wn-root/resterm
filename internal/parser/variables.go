package parser

import (
	"regexp"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	dscope "github.com/unkn0wn-root/resterm/internal/parser/directive/scope"
	"github.com/unkn0wn-root/resterm/internal/parser/lexer"
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
	scopeToken, secret := dscope.ParseToken(matches[1])
	name := matches[2]
	valueCandidate := matches[3]
	if valueCandidate == "" {
		valueCandidate = matches[4]
	}
	value := strings.TrimSpace(valueCandidate)
	switch scopeToken {
	case "global":
		b.addScopedVariable(name, value, ln.no, restfile.ScopeGlobal, secret)
	case "request":
		b.addScopedVariable(name, value, ln.no, restfile.ScopeRequest, secret)
	case "file":
		b.addScopedVariable(name, value, ln.no, restfile.ScopeFile, secret)
	default:
		scope := restfile.ScopeRequest
		if !b.inRequest {
			scope = restfile.ScopeFile
		}
		b.addScopedVariable(name, value, ln.no, scope, secret)
	}
	b.appendLine(ln.raw)
	return true
}

func (b *documentBuilder) addScopedVariable(
	name, value string,
	line int,
	scope restfile.VariableScope,
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
	case restfile.ScopeGlobal:
		b.file.globals = append(b.file.globals, variable)
	case restfile.ScopeFile:
		b.file.vars = append(b.file.vars, variable)
	case restfile.ScopeRequest:
		b.ensureRequest(line)
		b.request.variables = append(b.request.variables, variable)
	}
}

func (b *documentBuilder) handleScopedVariableDirective(key, rest string, line int) bool {
	scopeToken := key
	args := rest
	if key == "var" {
		scopeToken, args = lexer.SplitFirst(rest)
		if scopeToken == "" {
			return false
		}
	}

	scopeStr, secret := dscope.ParseToken(scopeToken)
	name, value := options.ParseNameValue(args)

	switch scopeStr {
	case "global":
		b.addScopedVariable(name, value, line, restfile.ScopeGlobal, secret)
	case "file":
		b.addScopedVariable(name, value, line, restfile.ScopeFile, secret)
	case "request":
		b.addScopedVariable(name, value, line, restfile.ScopeRequest, secret)
	default:
		return false
	}
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

func (b *documentBuilder) handleConstDirective(line int, key, rest string) bool {
	if key != "const" {
		return false
	}
	if name, value := options.ParseNameValue(rest); name != "" {
		b.addConstant(name, value, line)
	}
	return true
}
