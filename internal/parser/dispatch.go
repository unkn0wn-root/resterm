package parser

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type directiveOutcome uint8

const (
	directiveIgnored directiveOutcome = iota
	directiveApplied
	directiveRejected
)

// lines covers the directive and its continuation lines. Positions use lines.Start.
type parsedDirective struct {
	directive.Call
	lines  restfile.LineRange
	argCol int
}

// Only the opening line carries a column. An expression starting on a
// continuation line would count the newline as another character on the line
// above, so it gets none.
func (d parsedDirective) exprCol(expr string) int {
	if d.argCol <= 0 || expr == "" {
		return 0
	}
	off := strings.LastIndex(d.Args, expr)
	if off < 0 || strings.Contains(d.Args[:off], "\n") {
		return 0
	}
	return d.argCol + off
}

func (d parsedDirective) setExprCol(col *int, expr string) {
	if c := d.exprCol(expr); c > 0 {
		*col = c
	}
}

// Directives are offered to handlers before their required values are checked.
// This lets inactive features decline their directives without producing errors;
// the router reports anything left unclaimed as a warning.
// The request handler stays last because checking it may open a request for a
// directive that belongs to another context.
var directiveHandlers = []func(*documentBuilder, parsedDirective) directiveOutcome{
	(*documentBuilder).handleMockDirective,
	(*documentBuilder).handleWorkflowStart,
	(*documentBuilder).handleUseDirective,
	(*documentBuilder).handleWorkflowDirective,
	(*documentBuilder).handleScopedVariableDirective,
	(*documentBuilder).handleConstDirective,
	(*documentBuilder).handleAuthDirective,
	(*documentBuilder).handleSSHDirective,
	(*documentBuilder).handleK8sDirective,
	(*documentBuilder).handlePatchDirective,
	(*documentBuilder).handleFileSettingsDirective,
	(*documentBuilder).handleRequestDirective,
}

func (b *documentBuilder) routeDirective(d parsedDirective) directiveOutcome {
	// Handlers may transition between workflow, file, and request contexts.
	// An unknown name has no possible owner, so keep it away from that
	// stateful routing and leave the current context untouched.
	if !d.Name.Known() {
		if b.workflow != nil {
			return b.reject(
				d,
				d.Spelling.Tag()+" is not a known Resterm directive in a workflow",
			)
		}
		b.addWarning(d.lines.Start, ignoredDirectiveWarning(d.Call))
		return directiveIgnored
	}
	out := b.claimDirective(d)
	if out == directiveIgnored {
		b.addWarning(d.lines.Start, ignoredDirectiveWarning(d.Call))
		return out
	}
	if out == directiveApplied && d.Name.ValueRequired() && !directive.HasValue(d.Args) {
		b.addError(d.lines.Start, d.Spelling.Tag()+" value missing")
		return directiveRejected
	}
	return out
}

func ignoredDirectiveWarning(call directive.Call) string {
	tag := call.Spelling.Tag()
	if call.Name.Known() {
		return tag + " is not valid in the current context and was ignored"
	}
	return tag + " is not a known Resterm directive and was ignored"
}

func (b *documentBuilder) claimDirective(d parsedDirective) directiveOutcome {
	for _, handle := range directiveHandlers {
		if out := handle(b, d); out != directiveIgnored {
			return out
		}
	}
	return directiveIgnored
}

// Errors point at the line the directive opened on, not the one that closed it.
func (b *documentBuilder) reject(d parsedDirective, message string) directiveOutcome {
	b.addError(d.lines.Start, message)
	return directiveRejected
}

func (b *documentBuilder) redeclared(d parsedDirective) bool {
	if !b.inRequest || !b.request.declared[d.Name] {
		return false
	}
	b.addError(d.lines.Start, d.Name.Tag()+" directive already defined for this request")
	return true
}

func (b *documentBuilder) markDeclared(d parsedDirective) {
	if b.inRequest && d.Name.DeclaredOnce() {
		b.request.declared[d.Name] = true
	}
}

// Request directives may open a request before its method line. Probe them on a
// detached builder first so an ignored directive cannot flush an active
// workflow. Claimed directives adopt that request after the workflow is closed.
func (b *documentBuilder) handleRequestDirective(d parsedDirective) directiveOutcome {
	if b.inRequest {
		return b.applyRequestDirective(d)
	}

	probe := &documentBuilder{doc: &restfile.Document{Path: b.doc.Path}}
	probe.ensureRequest(d.lines.Start)
	out := probe.applyRequestDirective(d)
	if out == directiveIgnored {
		return out
	}

	b.flushWorkflow(d.lines.Start - 1)
	b.inRequest = true
	b.request = probe.request
	b.doc.Errors = append(b.doc.Errors, probe.doc.Errors...)
	b.doc.Warnings = append(b.doc.Warnings, probe.doc.Warnings...)
	return out
}

func (b *documentBuilder) applyRequestDirective(d parsedDirective) directiveOutcome {
	if handled, err := b.request.protoDirective(d.Name, d.Args); handled {
		b.report(d.lines.Start, err)
		if fatalErr(err) {
			return directiveRejected
		}
		return directiveApplied
	}
	if d.Name == directive.Body && b.request.handleBodyDirective(d.Args) {
		return directiveApplied
	}
	if out := b.handleRequestMetadataDirective(d); out != directiveIgnored {
		return out
	}
	return directiveIgnored
}

func (b *documentBuilder) handleWorkflowStart(d parsedDirective) directiveOutcome {
	switch d.Name {
	case directive.Workflow:
		if err := b.startWorkflow(d.lines.Start, d.Args); err != nil {
			return b.reject(d, err.Error())
		}
		return directiveApplied
	case directive.Step:
		if b.workflow == nil {
			return directiveIgnored
		}
		if err := b.workflow.addStep(d.lines.Start, d.Args); err != nil {
			return b.reject(d, err.Error())
		}
		return directiveApplied
	default:
		return directiveIgnored
	}
}

func (b *documentBuilder) handleWorkflowDirective(d parsedDirective) directiveOutcome {
	if b.workflow == nil || b.inRequest {
		return directiveIgnored
	}
	handled, err := b.workflow.handleDirective(d.Call, d.lines.Start)
	if !handled {
		return directiveIgnored
	}
	// The workflow owns the directive even when it is rejected, so its range covers every line.
	b.workflow.touch(d.lines.End)
	if err != nil {
		return b.reject(d, err.Error())
	}
	return directiveApplied
}

func (b *documentBuilder) handleUseDirective(d parsedDirective) directiveOutcome {
	if d.Name != directive.Use {
		return directiveIgnored
	}
	spec, err := parseUseSpec(d.Args, d.lines.Start)
	if err != nil {
		return b.reject(d, err.Error())
	}
	if b.inRequest && b.request != nil {
		b.request.metadata.Uses = append(b.request.metadata.Uses, spec)
	} else {
		b.file.uses = append(b.file.uses, spec)
	}
	return directiveApplied
}

func (b *documentBuilder) handleAuthDirective(d parsedDirective) directiveOutcome {
	if d.Name != directive.Auth {
		return directiveIgnored
	}

	dir, err := parseAuthDirective(d.Args)
	if err != nil {
		return b.reject(d, err.Error())
	}

	switch dir.Scope {
	case directive.ScopeFile, directive.ScopeGlobal:
		if b.inRequest {
			return b.reject(d, "@auth "+dir.Scope.String()+" scope must be declared outside a request")
		}
		if dir.Disable || dir.Spec == nil {
			return directiveApplied
		}
		spec := *dir.Spec.Clone()
		spec.SourcePath = b.doc.Path
		spec.Line = d.lines.Start
		b.file.auth = append(b.file.auth, restfile.AuthProfile{
			Scope:      dir.Scope,
			Name:       dir.Name,
			Spec:       spec,
			Line:       d.lines.Start,
			SourcePath: b.doc.Path,
		})
	case directive.ScopeRequest:
		b.ensureRequest(d.lines.Start)
		if dir.Disable {
			b.request.metadata.Auth = nil
			b.request.metadata.AuthDisabled = true
			return directiveApplied
		}
		if dir.Spec != nil {
			spec := dir.Spec.Clone()
			spec.SourcePath = b.doc.Path
			spec.Line = d.lines.Start
			b.request.metadata.Auth = spec
			b.request.metadata.AuthDisabled = false
		}
	}

	return directiveApplied
}

func (b *documentBuilder) handlePatchDirective(d parsedDirective) directiveOutcome {
	if d.Name != directive.Patch {
		return directiveIgnored
	}
	if b.inRequest {
		return b.reject(d, "@patch must be declared outside a request")
	}
	spec, err := parsePatchSpec(d.Args, d.lines.Start)
	if err != nil {
		return b.reject(d, err.Error())
	}
	d.setExprCol(&spec.Col, spec.Expression)
	spec.SourcePath = b.doc.Path
	b.file.patches = append(b.file.patches, spec)
	return directiveApplied
}

func (b *documentBuilder) handleFileSettingsDirective(d parsedDirective) directiveOutcome {
	if b.inRequest {
		return directiveIgnored
	}
	var (
		settings map[string]string
		err      error
	)
	switch d.Name {
	case directive.Setting:
		settings, err = putSetting(b.file.settings, d.Args)
	case directive.Settings:
		settings, err = applySettingsTokens(b.file.settings, d.Args, directive.Settings)
	default:
		return directiveIgnored
	}
	b.file.settings = settings
	b.report(d.lines.Start, err)
	return directiveApplied
}
