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

type directiveLine struct {
	directive.Call
	no     int
	argCol int
}

func (d directiveLine) exprCol(expr string) int {
	if d.argCol <= 0 || expr == "" {
		return 0
	}
	off := strings.LastIndex(d.Args, expr)
	if off < 0 {
		return 0
	}
	return d.argCol + off
}

// Directives are offered to handlers before their required values are checked.
// This lets inactive features ignore their directives without producing errors.
// The request handler stays last because checking it may open a request for a
// directive that belongs to another context.
var directiveHandlers = []func(*documentBuilder, directiveLine) directiveOutcome{
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

func (b *documentBuilder) routeDirective(d directiveLine) directiveOutcome {
	out := b.claimDirective(d)
	if out == directiveApplied && d.Name.ValueRequired() && !directive.HasValue(d.Args) {
		b.addError(d.no, d.Spelling.Tag()+" value missing")
		return directiveRejected
	}
	return out
}

func (b *documentBuilder) claimDirective(d directiveLine) directiveOutcome {
	for _, handle := range directiveHandlers {
		if out := handle(b, d); out != directiveIgnored {
			return out
		}
	}
	return directiveIgnored
}

func (b *documentBuilder) redeclared(d directiveLine) bool {
	if !b.inRequest || !b.request.declared[d.Name] {
		return false
	}
	b.addError(d.no, d.Name.Tag()+" directive already defined for this request")
	return true
}

func (b *documentBuilder) markDeclared(d directiveLine) {
	if b.inRequest && d.Name.DeclaredOnce() {
		b.request.declared[d.Name] = true
	}
}

// Request directives are allowed to open a request while they are being checked.
// If no request handler accepts the directive, restore the previous state so a
// following shorthand variable remains file scoped.
func (b *documentBuilder) handleRequestDirective(d directiveLine) directiveOutcome {
	opened := !b.inRequest
	b.ensureRequest(d.no)
	if handled, err := b.request.protoDirective(d.Name, d.Args); handled {
		b.report(d.no, err)
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
	if opened {
		b.inRequest = false
		b.request = nil
	}
	return directiveIgnored
}

func (b *documentBuilder) handleWorkflowStart(d directiveLine) directiveOutcome {
	switch d.Name {
	case directive.Workflow:
		b.startWorkflow(d.no, d.Args)
		return directiveApplied
	case directive.Step:
		if b.workflow == nil {
			return directiveApplied
		}
		if err := b.workflow.addStep(d.no, d.Args); err != nil {
			b.addError(d.no, err.Error())
			return directiveRejected
		}
		return directiveApplied
	default:
		return directiveIgnored
	}
}

func (b *documentBuilder) handleWorkflowDirective(d directiveLine) directiveOutcome {
	if b.workflow == nil || b.inRequest {
		return directiveIgnored
	}
	handled, err := b.workflow.handleDirective(d.Call, d.no)
	switch {
	case !handled:
		return directiveIgnored
	case err != nil:
		b.addError(d.no, err.Error())
		return directiveRejected
	default:
		return directiveApplied
	}
}

func (b *documentBuilder) handleUseDirective(d directiveLine) directiveOutcome {
	if d.Name != directive.Use {
		return directiveIgnored
	}
	spec, err := parseUseSpec(d.Args, d.no)
	if err != nil {
		b.addError(d.no, err.Error())
		return directiveRejected
	}
	if b.inRequest && b.request != nil {
		b.request.metadata.Uses = append(b.request.metadata.Uses, spec)
	} else {
		b.file.uses = append(b.file.uses, spec)
	}
	return directiveApplied
}

func (b *documentBuilder) handleAuthDirective(d directiveLine) directiveOutcome {
	if d.Name != directive.Auth {
		return directiveIgnored
	}

	dir, err := parseAuthDirective(d.Args)
	if err != nil {
		b.addError(d.no, err.Error())
		return directiveRejected
	}

	switch dir.Scope {
	case directive.ScopeFile, directive.ScopeGlobal:
		if b.inRequest {
			b.addError(
				d.no,
				"@auth "+dir.Scope.String()+" scope must be declared outside a request",
			)
			return directiveRejected
		}
		if dir.Disable || dir.Spec == nil {
			return directiveApplied
		}
		spec := *dir.Spec.Clone()
		spec.SourcePath = b.doc.Path
		spec.Line = d.no
		b.file.auth = append(b.file.auth, restfile.AuthProfile{
			Scope:      dir.Scope,
			Name:       dir.Name,
			Spec:       spec,
			Line:       d.no,
			SourcePath: b.doc.Path,
		})
	case directive.ScopeRequest:
		b.ensureRequest(d.no)
		if dir.Disable {
			b.request.metadata.Auth = nil
			b.request.metadata.AuthDisabled = true
			return directiveApplied
		}
		if dir.Spec != nil {
			spec := dir.Spec.Clone()
			spec.SourcePath = b.doc.Path
			spec.Line = d.no
			b.request.metadata.Auth = spec
			b.request.metadata.AuthDisabled = false
		}
	}

	return directiveApplied
}

func (b *documentBuilder) handlePatchDirective(d directiveLine) directiveOutcome {
	if d.Name != directive.Patch {
		return directiveIgnored
	}
	if b.inRequest {
		b.addError(d.no, "@patch must be declared outside a request")
		return directiveRejected
	}
	spec, err := parsePatchSpec(d.Args, d.no)
	if err != nil {
		b.addError(d.no, err.Error())
		return directiveRejected
	}
	if c := d.exprCol(spec.Expression); c > 0 {
		spec.Col = c
	}
	spec.SourcePath = b.doc.Path
	b.file.patches = append(b.file.patches, spec)
	return directiveApplied
}

func (b *documentBuilder) handleFileSettingsDirective(d directiveLine) directiveOutcome {
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
	b.report(d.no, err)
	return directiveApplied
}
