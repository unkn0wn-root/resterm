package parser

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/parser/lexer"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (b *documentBuilder) handleComment(line, baseCol int, text string) {
	if !strings.HasPrefix(text, "@") {
		return
	}

	directive := strings.TrimSpace(text[1:])
	if directive == "" {
		return
	}

	key, rest := lexer.SplitDirective(directive)
	if key == "" {
		return
	}
	argCol := directiveArgCol(text, baseCol)

	if b.handleMockDirective(line, key, rest) {
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
	if b.handlePatchDirective(line, argCol, key, rest) {
		return
	}
	if b.handleFileSettingsDirective(key, rest) {
		return
	}

	b.handleRequestDirective(line, argCol, key, rest)
}

// handleRequestDirective routes request scoped directives and creates the
// request on demand. When the directive turns out to be unknown, that fresh
// request is rolled back again. Leaving it open would swallow shorthand
// variables that belong to the file rather than to any request.
func (b *documentBuilder) handleRequestDirective(no, argCol int, key, rest string) bool {
	startedRequest := !b.inRequest
	b.ensureRequest(no)
	switch {
	case b.request.protoDirective(key, rest):
		return true
	case key == "body" && b.request.handleBodyDirective(rest):
		return true
	case b.handleRequestMetadataDirective(no, argCol, key, rest):
		return true
	}
	if startedRequest {
		b.inRequest = false
		b.request = nil
	}
	return false
}

// directiveArgCol returns the 1-based source column of a directive's argument
// (the text after the "@<key>" token), given baseCol, the column of text[0] (the
// '@'). The key token is measured from text rather than the parsed key
// so trailing markers (e.g. the colon in "@assert:") are accounted for.
func directiveArgCol(text string, baseCol int) int {
	if baseCol <= 0 {
		return 0
	}
	body := text[1:] // after '@'
	lead := len(body) - len(strings.TrimLeft(body, " \t"))
	tok := body[lead:]
	if i := strings.IndexAny(tok, " \t"); i >= 0 {
		tok = tok[:i]
	}
	afterTok := body[lead+len(tok):]
	gap := len(afterTok) - len(strings.TrimLeft(afterTok, " \t"))
	return baseCol + 1 + lead + len(tok) + gap
}

// exprCol returns the 1-based source column of expr within rest, the directive
// argument that begins at argCol. The expression is the trailing component of
// rest for these directives so LastIndex locates it. 0 for unknown.
func exprCol(rest, expr string, argCol int) int {
	if argCol <= 0 || expr == "" {
		return 0
	}
	off := strings.LastIndex(rest, expr)
	if off < 0 {
		return 0
	}
	return argCol + off
}

func (b *documentBuilder) handleWorkflowStart(line int, key, rest string) bool {
	switch key {
	case "workflow":
		b.startWorkflow(line, rest)
		return true
	case "step":
		if b.workflow != nil {
			if err := b.workflow.addStep(line, rest); err != nil {
				b.addError(line, err.Error())
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
		b.file.uses = append(b.file.uses, spec)
	}
	return true
}

func (b *documentBuilder) handleWorkflowDirective(line int, key, rest string) bool {
	if b.workflow == nil || b.inRequest {
		return false
	}
	if handled, err := b.workflow.handleDirective(key, rest, line); handled {
		if err != nil {
			b.addError(line, err.Error())
		}
		return true
	}
	return false
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
		b.file.auth = append(b.file.auth, restfile.AuthProfile{
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

func (b *documentBuilder) handlePatchDirective(line, argCol int, key, rest string) bool {
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
	if c := exprCol(rest, spec.Expression, argCol); c > 0 {
		spec.Col = c
	}
	spec.SourcePath = b.doc.Path
	b.file.patches = append(b.file.patches, spec)
	return true
}

func (b *documentBuilder) handleFileSettingsDirective(key, rest string) bool {
	if b.inRequest {
		return false
	}
	switch key {
	case "setting":
		b.file.settings = putSetting(b.file.settings, rest)
		return true
	case "settings":
		b.file.settings = applySettingsTokens(b.file.settings, rest)
		return true
	default:
		return false
	}
}

func (b *documentBuilder) handleBlockComment(ln line) bool {
	if b.inBlock {
		content, closed := parseBlockCommentLine(ln.text)
		if content != "" {
			b.handleComment(ln.no, 0, content)
		}
		b.appendLine(ln.raw)
		if closed {
			b.inBlock = false
		}
		return true
	}

	if ln.isBlockCommentStart() {
		content, closed := cutBlockCommentStart(ln.text)
		if content != "" {
			b.handleComment(ln.no, 0, content)
		}
		b.appendLine(ln.raw)
		if !closed {
			b.inBlock = true
		}
		return true
	}
	return false
}

func (b *documentBuilder) handleCommentLine(ln line) bool {
	if commentText, col, ok := stripComment(ln.text); ok {
		// col counts from the start of the trimmed text. Adding the offset of
		// the trimmed text inside the raw line turns it into a source column.
		base := strings.Index(ln.raw, ln.text) + col
		b.handleComment(ln.no, base, commentText)
		b.appendLine(ln.raw)
		return true
	}
	return false
}
