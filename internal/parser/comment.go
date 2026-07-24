package parser

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (b *documentBuilder) handleComment(line, baseCol int, text string) {
	call, ok := directive.Parse(text)
	if !ok {
		return
	}
	argCol := 0
	if baseCol > 0 {
		argCol = baseCol + call.ArgOffset
	}

	if b.handleMockDirective(line, call.Name, call.Args) {
		return
	}

	if b.handleWorkflowStart(line, call.Name, call.Args) {
		return
	}
	if b.handleUseDirective(line, call.Name, call.Args) {
		return
	}
	if b.handleWorkflowDirective(line, call) {
		return
	}
	if b.handleScopedVariableDirective(call.Name, call.Args, line) {
		return
	}
	if b.handleConstDirective(line, call.Name, call.Args) {
		return
	}
	if b.handleAuthDirective(line, call.Name, call.Args) {
		return
	}
	if b.handleSSHDirective(line, call.Name, call.Args) {
		return
	}
	if b.handleK8sDirective(line, call.Name, call.Args) {
		return
	}
	if b.handlePatchDirective(line, argCol, call.Name, call.Args) {
		return
	}
	if b.handleFileSettingsDirective(call.Name, call.Args) {
		return
	}

	b.handleRequestDirective(line, argCol, call)
}

// handleRequestDirective routes request scoped directives and creates the
// request on demand. When the directive turns out to be unknown, that fresh
// request is rolled back again. Leaving it open would swallow shorthand
// variables that belong to the file rather than to any request.
func (b *documentBuilder) handleRequestDirective(
	no, argCol int,
	call directive.Call,
) bool {
	startedRequest := !b.inRequest
	b.ensureRequest(no)
	if handled, err := b.request.protoDirective(call.Name, call.Args); handled {
		b.addErrors(no, err)
		return true
	}
	switch {
	case call.Name == directive.Body && b.request.handleBodyDirective(call.Args):
		return true
	case b.handleRequestMetadataDirective(no, argCol, call):
		return true
	}
	if startedRequest {
		b.inRequest = false
		b.request = nil
	}
	return false
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

func (b *documentBuilder) handleWorkflowStart(
	line int,
	name directive.Name,
	args string,
) bool {
	switch name {
	case directive.Workflow:
		b.startWorkflow(line, args)
		return true
	case directive.Step:
		if b.workflow != nil {
			if err := b.workflow.addStep(line, args); err != nil {
				b.addError(line, err.Error())
			}
		}
		return true
	default:
		return false
	}
}

func (b *documentBuilder) handleUseDirective(
	line int,
	name directive.Name,
	args string,
) bool {
	if name != directive.Use {
		return false
	}
	spec, err := parseUseSpec(args, line)
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

func (b *documentBuilder) handleWorkflowDirective(line int, call directive.Call) bool {
	if b.workflow == nil || b.inRequest {
		return false
	}
	if handled, err := b.workflow.handleDirective(call, line); handled {
		if err != nil {
			b.addError(line, err.Error())
		}
		return true
	}
	return false
}

func (b *documentBuilder) handleAuthDirective(
	line int,
	name directive.Name,
	args string,
) bool {
	if name != directive.Auth {
		return false
	}

	dir, ok, err := parseAuthDirective(args)
	if !ok {
		return true
	}
	if err != nil {
		b.addError(line, err.Error())
		return true
	}

	switch dir.Scope {
	case directive.ScopeFile, directive.ScopeGlobal:
		if b.inRequest {
			b.addError(
				line,
				"@auth "+dir.Scope.String()+" scope must be declared outside a request",
			)
			return true
		}
		if dir.Disable || dir.Spec == nil {
			return true
		}
		spec := *dir.Spec.Clone()
		spec.SourcePath = b.doc.Path
		b.file.auth = append(b.file.auth, restfile.AuthProfile{
			Scope:      dir.Scope,
			Name:       dir.Name,
			Spec:       spec,
			Line:       line,
			SourcePath: b.doc.Path,
		})
	case directive.ScopeRequest:
		b.ensureRequest(line)
		if dir.Disable {
			b.request.metadata.Auth = nil
			b.request.metadata.AuthDisabled = true
			return true
		}
		if dir.Spec != nil {
			spec := dir.Spec.Clone()
			spec.SourcePath = b.doc.Path
			b.request.metadata.Auth = spec
			b.request.metadata.AuthDisabled = false
		}
	}

	return true
}

func (b *documentBuilder) handlePatchDirective(
	line, argCol int,
	name directive.Name,
	args string,
) bool {
	if name != directive.Patch {
		return false
	}
	if b.inRequest {
		b.addError(line, "@patch must be declared outside a request")
		return true
	}
	spec, err := parsePatchSpec(args, line)
	if err != nil {
		b.addError(line, err.Error())
		return true
	}
	if c := exprCol(args, spec.Expression, argCol); c > 0 {
		spec.Col = c
	}
	spec.SourcePath = b.doc.Path
	b.file.patches = append(b.file.patches, spec)
	return true
}

func (b *documentBuilder) handleFileSettingsDirective(
	name directive.Name,
	args string,
) bool {
	if b.inRequest {
		return false
	}
	switch name {
	case directive.Setting:
		b.file.settings = putSetting(b.file.settings, args)
		return true
	case directive.Settings:
		b.file.settings = applySettingsTokens(b.file.settings, args)
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
