package parser

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (b *documentBuilder) handleComment(line int, text string) {
	if !strings.HasPrefix(text, "@") {
		return
	}

	directive := strings.TrimSpace(text[1:])
	if directive == "" {
		return
	}

	key, rest := splitDirective(directive)
	if key == "" {
		return
	}

	if b.handleWorkflowStart(line, key, rest) {
		return
	}
	if b.handleUseDirective(line, key, rest) {
		return
	}
	if b.handleWorkflowDirective(line, key, rest) {
		return
	}
	if b.handleScopedVariableDirective(key, rest, line) {
		return
	}
	if b.handleConstDirective(line, key, rest) {
		return
	}
	if b.handleSSHDirective(line, key, rest) {
		return
	}
	if b.handleFileSettingsDirective(key, rest) {
		return
	}

	if !b.ensureRequest(line) {
		return
	}
	if b.handleRequestBuilderDirective(key, rest) {
		return
	}
	b.handleRequestMetadataDirective(line, key, rest)
}

func (b *documentBuilder) handleWorkflowStart(line int, key, rest string) bool {
	switch key {
	case "workflow":
		b.startWorkflow(line, rest)
		return true
	case "step":
		if b.workflow != nil {
			if err := b.workflow.addStep(line, rest); err != "" {
				b.addError(line, err)
			}
		}
		return true
	default:
		return false
	}
}

func (b *documentBuilder) handleUseDirective(line int, key, rest string) bool {
	if key != "use" {
		return false
	}
	spec, err := parseUseSpec(rest, line)
	if err != nil {
		b.addError(line, err.Error())
		return true
	}
	if b.inRequest && b.request != nil {
		b.request.metadata.Uses = append(b.request.metadata.Uses, spec)
	} else {
		b.fileUses = append(b.fileUses, spec)
	}
	return true
}

func (b *documentBuilder) handleWorkflowDirective(line int, key, rest string) bool {
	if b.workflow == nil || b.inRequest {
		return false
	}
	if handled, errMsg := b.workflow.handleDirective(key, rest, line); handled {
		if errMsg != "" {
			b.addError(line, errMsg)
		}
		return true
	}
	return false
}

func (b *documentBuilder) handleConstDirective(line int, key, rest string) bool {
	if key != "const" {
		return false
	}
	if name, value := parseNameValue(rest); name != "" {
		b.addConstant(name, value, line)
	}
	return true
}

func (b *documentBuilder) handleSSHDirective(line int, key, rest string) bool {
	if key != "ssh" {
		return false
	}
	b.handleSSH(line, rest)
	return true
}

func (b *documentBuilder) handleFileSettingsDirective(key, rest string) bool {
	if b.inRequest {
		return false
	}
	switch key {
	case "setting":
		b.handleFileSetting(rest)
		return true
	case "settings":
		b.fileSettings = applySettingsTokens(b.fileSettings, rest)
		return true
	default:
		return false
	}
}

func (b *documentBuilder) handleRequestBuilderDirective(key, rest string) bool {
	if b.request.grpc.HandleDirective(key, rest) {
		return true
	}
	if b.request.websocket.HandleDirective(key, rest) {
		return true
	}
	if b.request.sse.HandleDirective(key, rest) {
		return true
	}
	if b.request.graphql.HandleDirective(key, rest) {
		return true
	}
	if key == "body" {
		return b.request.handleBodyDirective(rest)
	}
	return false
}

func (b *documentBuilder) handleRequestMetadataDirective(line int, key, rest string) {
	switch key {
	case "name":
		if rest != "" {
			value := trimQuotes(strings.TrimSpace(rest))
			b.request.metadata.Name = value
		}
	case "description", "desc":
		if b.request.metadata.Description != "" {
			b.request.metadata.Description += "\n"
		}
		b.request.metadata.Description += rest
	case "tag", "tags":
		tags := strings.Fields(rest)
		if len(tags) == 0 {
			tags = strings.Split(rest, ",")
		}
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if !contains(b.request.metadata.Tags, tag) {
				b.request.metadata.Tags = append(b.request.metadata.Tags, tag)
			}
		}
	case "no-log", "nolog":
		b.request.metadata.NoLog = true
	case "log-sensitive-headers", "log-secret-headers":
		if rest == "" {
			b.request.metadata.AllowSensitiveHeaders = true
			return
		}
		if value, ok := parseBool(rest); ok {
			b.request.metadata.AllowSensitiveHeaders = value
		}
	case "auth":
		spec := parseAuthSpec(rest)
		if spec != nil {
			b.request.metadata.Auth = spec
		}
	case "settings":
		if b.inRequest {
			b.request.settings = applySettingsTokens(b.request.settings, rest)
		} else {
			b.fileSettings = applySettingsTokens(b.fileSettings, rest)
		}
	case "setting":
		key, value := splitDirective(rest)
		if key != "" {
			if b.inRequest {
				if b.request.settings == nil {
					b.request.settings = make(map[string]string)
				}
				b.request.settings[key] = value
			} else {
				if b.fileSettings == nil {
					b.fileSettings = make(map[string]string)
				}
				b.fileSettings[key] = value
			}
		}
	case "timeout":
		if b.request.settings == nil {
			b.request.settings = make(map[string]string)
		}
		b.request.settings["timeout"] = rest
	case "var":
		name, value := parseNameValue(rest)
		if name == "" {
			return
		}
		variable := restfile.Variable{
			Name:   name,
			Value:  value,
			Line:   line,
			Scope:  restfile.ScopeRequest,
			Secret: false,
		}
		b.request.variables = append(b.request.variables, variable)
	case "script":
		if rest != "" {
			kind, lang := parseScriptSpec(rest)
			b.request.currentScriptKind = kind
			b.request.currentScriptLang = lang
		}
	case "apply":
		if spec, ok := parseApplySpec(rest, line); ok {
			b.request.metadata.Applies = append(b.request.metadata.Applies, spec)
		} else {
			b.addError(line, "@apply expression missing")
		}
	case "capture":
		if capture, ok := b.parseCaptureDirective(rest, line); ok {
			b.request.metadata.Captures = append(b.request.metadata.Captures, capture)
		}
	case "assert":
		if spec, ok := b.parseAssertDirective(rest, line); ok {
			b.request.metadata.Asserts = append(b.request.metadata.Asserts, spec)
		} else {
			b.addError(line, "@assert expression missing")
		}
	case "when", "skip-if":
		negate := key == "skip-if"
		spec, err := parseConditionSpec(rest, line, negate)
		if err != nil {
			b.addError(line, err.Error())
			return
		}
		if b.request.metadata.When != nil {
			b.addError(line, "@when directive already defined for this request")
			return
		}
		b.request.metadata.When = spec
	case "for-each":
		spec, err := parseForEachSpec(rest, line)
		if err != nil {
			b.addError(line, err.Error())
			return
		}
		if b.request.metadata.ForEach != nil {
			b.addError(line, "@for-each directive already defined for this request")
			return
		}
		b.request.metadata.ForEach = spec
	case "profile":
		if spec := parseProfileSpec(rest); spec != nil {
			b.request.metadata.Profile = spec
		}
	case "trace":
		if spec := parseTraceSpec(rest); spec != nil {
			b.request.metadata.Trace = spec
		}
	case "compare":
		if !b.ensureRequest(line) {
			return
		}
		if b.request.metadata.Compare != nil {
			b.addError(line, "@compare directive already defined for this request")
			return
		}
		spec, err := parseCompareDirective(rest)
		if err != nil {
			b.addError(line, err.Error())
			return
		}
		b.request.metadata.Compare = spec
	}
}
