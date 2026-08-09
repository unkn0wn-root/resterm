package parser

import (
	"errors"

	"github.com/unkn0wn-root/resterm/internal/directive"
	k8sbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/k8s"
	sshbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/ssh"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (b *documentBuilder) handleSSHDirective(d directiveLine) directiveOutcome {
	if d.Name != directive.SSH {
		return directiveIgnored
	}

	res, err := sshbuilder.ParseDirective(d.Args)
	b.report(d.no, err)
	if fatalErr(err) {
		return directiveRejected
	}

	switch res.Scope {
	case directive.ScopeRequest:
		b.ensureRequest(d.no)
		if b.request.k8s != nil {
			b.addError(d.no, "@ssh cannot be combined with @k8s on the same request")
			return directiveRejected
		}
		if b.request.ssh != nil {
			b.addError(d.no, "@ssh already defined for this request")
			return directiveRejected
		}
		if res.PersistIgnored {
			b.addWarning(d.no, "@ssh request scope ignores persist")
		}
		b.request.ssh = res.Spec
	case directive.ScopeGlobal, directive.ScopeFile:
		res.Profile.Scope = res.Scope
		b.file.ssh = append(b.file.ssh, res.Profile)
	}
	return directiveApplied
}

func (b *documentBuilder) handleK8sDirective(d directiveLine) directiveOutcome {
	if d.Name != directive.K8s {
		return directiveIgnored
	}

	res, err := k8sbuilder.ParseDirective(d.Args)
	b.report(d.no, err)
	if fatalErr(err) {
		var dirErr *k8sbuilder.DirectiveError
		if errors.As(err, &dirErr) {
			b.addInvalidK8sProfile(d.no, dirErr.Profile, err.Error())
		}
		return directiveRejected
	}

	switch res.Scope {
	case directive.ScopeRequest:
		b.ensureRequest(d.no)
		if b.request.ssh != nil {
			b.addError(d.no, "@k8s cannot be combined with @ssh on the same request")
			return directiveRejected
		}
		if b.request.k8s != nil {
			b.addError(d.no, "@k8s already defined for this request")
			return directiveRejected
		}
		if res.PersistIgnored {
			b.addWarning(d.no, "@k8s request scope ignores persist")
		}
		b.request.k8s = res.Spec
	case directive.ScopeGlobal, directive.ScopeFile:
		res.Profile.Scope = res.Scope
		res.Profile.Line = d.no
		b.file.k8s = append(b.file.k8s, res.Profile)
	}
	return directiveApplied
}

func (b *documentBuilder) addInvalidK8sProfile(
	line int,
	prof restfile.K8sProfile,
	message string,
) {
	if prof.Scope != directive.ScopeGlobal && prof.Scope != directive.ScopeFile {
		return
	}
	prof.Line = line
	prof.Invalid = true
	prof.Error = message
	b.file.k8s = append(b.file.k8s, prof)
}
