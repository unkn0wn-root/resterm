package parser

import (
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/parser/directive/lex"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	dvalue "github.com/unkn0wn-root/resterm/internal/parser/directive/value"
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

	key, rest := lex.SplitDirective(directive)
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
	if b.handleAuthDirective(line, key, rest) {
		return
	}
	if b.handleSSHDirective(line, key, rest) {
		return
	}
	if b.handleK8sDirective(line, key, rest) {
		return
	}
	if b.handlePatchDirective(line, key, rest) {
		return
	}
	if b.handleFileSettingsDirective(key, rest) {
		return
	}

	startedRequest := !b.inRequest
	b.ensureRequest(line)
	if b.handleRequestBuilderDirective(key, rest) {
		return
	}
	if b.handleRequestMetadataDirective(line, key, rest) {
		return
	}
	if startedRequest {
		// Unknown directive outside requests should be ignored, not create a
		// synthetic request that captures subsequent shorthand vars.
		b.inRequest = false
		b.request = nil
	}
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
	if name, value := options.ParseNameValue(rest); name != "" {
		b.addConstant(name, value, line)
	}
	return true
}

func (b *documentBuilder) handleAuthDirective(line int, key, rest string) bool {
	if key != "auth" {
		return false
	}

	dir, ok, err := parseAuthDirective(rest)
	if !ok {
		return true
	}
	if err != nil {
		b.addError(line, err.Error())
		return true
	}

	switch dir.Scope {
	case restfile.AuthScopeFile, restfile.AuthScopeGlobal:
		if b.inRequest {
			b.addError(
				line,
				"@auth "+restfile.AuthScopeLabel(
					dir.Scope,
				)+" scope must be declared outside a request",
			)
			return true
		}
		if dir.Disable || dir.Spec == nil {
			return true
		}
		spec := restfile.CloneAuthSpecValue(*dir.Spec)
		spec.SourcePath = b.doc.Path
		b.authDefs = append(b.authDefs, restfile.AuthProfile{
			Scope:      dir.Scope,
			Name:       dir.Name,
			Spec:       spec,
			Line:       line,
			SourcePath: b.doc.Path,
		})
	case restfile.AuthScopeRequest:
		b.ensureRequest(line)
		if dir.Disable {
			b.request.metadata.Auth = nil
			b.request.metadata.AuthDisabled = true
			return true
		}
		if dir.Spec != nil {
			spec := restfile.CloneAuthSpec(dir.Spec)
			spec.SourcePath = b.doc.Path
			b.request.metadata.Auth = spec
			b.request.metadata.AuthDisabled = false
		}
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

func (b *documentBuilder) handleK8sDirective(line int, key, rest string) bool {
	if key != "k8s" {
		return false
	}
	b.handleK8s(line, rest)
	return true
}

func (b *documentBuilder) handlePatchDirective(line int, key, rest string) bool {
	if key != "patch" {
		return false
	}
	if b.inRequest {
		b.addError(line, "@patch must be declared outside a request")
		return true
	}
	spec, err := parsePatchSpec(rest, line)
	if err != nil {
		b.addError(line, err.Error())
		return true
	}
	spec.SourcePath = b.doc.Path
	b.patchDefs = append(b.patchDefs, spec)
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

func (b *documentBuilder) handleRequestMetadataDirective(line int, key, rest string) bool {
	switch key {
	case "name":
		if rest != "" {
			value := lex.TrimQuotes(strings.TrimSpace(rest))
			b.request.metadata.Name = value
		}
		return true
	case "description", "desc":
		if b.request.metadata.Description != "" {
			b.request.metadata.Description += "\n"
		}
		b.request.metadata.Description += rest
		return true
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
			if !slices.ContainsFunc(b.request.metadata.Tags, func(item string) bool {
				return strings.EqualFold(item, tag)
			}) {
				b.request.metadata.Tags = append(b.request.metadata.Tags, tag)
			}
		}
		return true
	case "no-log", "nolog":
		b.request.metadata.NoLog = true
		return true
	case "log-sensitive-headers", "log-secret-headers":
		if rest == "" {
			b.request.metadata.AllowSensitiveHeaders = true
			return true
		}
		if value, ok := dvalue.ParseBool(rest); ok {
			b.request.metadata.AllowSensitiveHeaders = value
		}
		return true
	case "settings":
		b.request.settings = applySettingsTokens(b.request.settings, rest)
		return true
	case "setting":
		key, value := lex.SplitDirective(rest)
		if key != "" {
			if b.request.settings == nil {
				b.request.settings = make(map[string]string)
			}
			b.request.settings[key] = value
		}
		return true
	case "timeout":
		if b.request.settings == nil {
			b.request.settings = make(map[string]string)
		}
		b.request.settings["timeout"] = rest
		return true
	case "var":
		name, value := options.ParseNameValue(rest)
		if name == "" {
			return true
		}
		variable := restfile.Variable{
			Name:   name,
			Value:  value,
			Line:   line,
			Scope:  restfile.ScopeRequest,
			Secret: false,
		}
		b.request.variables = append(b.request.variables, variable)
		return true
	case "script":
		if rest != "" {
			b.setScript(rest, "")
		}
		return true
	case "rts":
		b.setScript(rest, "rts")
		return true
	case "apply":
		spec, err := parseApplySpec(rest, line)
		if err != nil {
			b.addError(line, err.Error())
			return true
		}
		b.request.metadata.Applies = append(b.request.metadata.Applies, spec)
		return true
	case "capture":
		if capture, ok := b.parseCaptureDirective(rest, line); ok {
			b.request.metadata.Captures = append(b.request.metadata.Captures, capture)
		}
		return true
	case "assert":
		if spec, ok := b.parseAssertDirective(rest, line); ok {
			b.request.metadata.Asserts = append(b.request.metadata.Asserts, spec)
		} else {
			b.addError(line, "@assert expression missing")
		}
		return true
	case "when", "skip-if":
		negate := key == "skip-if"
		spec, err := parseConditionSpec(rest, line, negate)
		if err != nil {
			b.addError(line, err.Error())
			return true
		}
		if b.request.metadata.When != nil {
			b.addError(line, "@when directive already defined for this request")
			return true
		}
		b.request.metadata.When = spec
		return true
	case "for-each":
		spec, err := parseForEachSpec(rest, line)
		if err != nil {
			b.addError(line, err.Error())
			return true
		}
		if b.request.metadata.ForEach != nil {
			b.addError(line, "@for-each directive already defined for this request")
			return true
		}
		b.request.metadata.ForEach = spec
		return true
	case "profile":
		if spec := parseProfileSpec(rest); spec != nil {
			b.request.metadata.Profile = spec
		}
		return true
	case "trace":
		if spec := parseTraceSpec(rest); spec != nil {
			b.request.metadata.Trace = spec
		}
		return true
	case "compare":
		if b.request.metadata.Compare != nil {
			b.addError(line, "@compare directive already defined for this request")
			return true
		}
		spec, err := parseCompareDirective(rest)
		if err != nil {
			b.addError(line, err.Error())
			return true
		}
		b.request.metadata.Compare = spec
		return true
	}
	return false
}

func (b *documentBuilder) setScript(rest, lang string) {
	k, l := parseScriptSpec(rest)
	if lang != "" {
		l = normScriptLang(lang)
	}
	b.request.currentScriptKind = k
	b.request.currentScriptLang = l
}

func applySettingsTokens(dst map[string]string, raw string) map[string]string {
	opts := options.Parse(raw)
	if len(opts) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]string, len(opts))
	}
	for k, v := range opts {
		if k == "" {
			continue
		}
		dst[k] = v
	}
	return dst
}

func parseScriptSpec(rest string) (string, string) {
	fields := lex.TokenizeFields(rest)
	kind := ""
	lang := ""
	for _, field := range fields {
		if strings.Contains(field, "=") {
			continue
		}
		if kind == "" {
			kind = field
			continue
		}
		if lang == "" {
			if v, ok := scriptLangToken(field); ok {
				lang = v
			}
		}
	}
	params := options.ParseFields(fields)
	if v := params["lang"]; v != "" {
		lang = v
	}
	if v := params["language"]; v != "" && lang == "" {
		lang = v
	}
	return normScriptKind(kind), normScriptLang(lang)
}

func scriptLangToken(tok string) (string, bool) {
	out := strings.ToLower(strings.TrimSpace(tok))
	switch out {
	case "js", "javascript":
		return "js", true
	case "rts", "restermlang":
		return "rts", true
	default:
		return "", false
	}
}
