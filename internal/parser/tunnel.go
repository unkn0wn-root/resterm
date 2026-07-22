package parser

import (
	"errors"

	k8sbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/k8s"
	sshbuilder "github.com/unkn0wn-root/resterm/internal/parser/builder/ssh"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (b *documentBuilder) handleSSH(line int, rest string) {
	res, err := sshbuilder.ParseDirective(rest)
	if err != nil {
		b.addError(line, err.Error())
		return
	}

	if res.Scope == restfile.SSHScopeRequest {
		b.ensureRequest(line)
		if b.request.k8s != nil {
			b.addError(line, "@ssh cannot be combined with @k8s on the same request")
			return
		}
		if b.request.ssh != nil {
			b.addError(line, "@ssh already defined for this request")
			return
		}
		if res.PersistIgnored {
			b.addWarning(line, "@ssh request scope ignores persist")
		}
		b.request.ssh = res.Spec
		return
	}

	if res.Scope == restfile.SSHScopeGlobal || res.Scope == restfile.SSHScopeFile {
		res.Profile.Scope = res.Scope
		b.file.ssh = append(b.file.ssh, res.Profile)
	}
}

func (b *documentBuilder) handleK8s(line int, rest string) {
	res, err := k8sbuilder.ParseDirective(rest)
	if err != nil {
		b.addError(line, err.Error())
		var dirErr *k8sbuilder.DirectiveError
		if errors.As(err, &dirErr) {
			b.addInvalidK8sProfile(line, dirErr.Profile, err.Error())
		}
		return
	}

	if res.Scope == restfile.K8sScopeRequest {
		b.ensureRequest(line)
		if b.request.ssh != nil {
			b.addError(line, "@k8s cannot be combined with @ssh on the same request")
			return
		}
		if b.request.k8s != nil {
			b.addError(line, "@k8s already defined for this request")
			return
		}
		if res.PersistIgnored {
			b.addWarning(line, "@k8s request scope ignores persist")
		}
		b.request.k8s = res.Spec
		return
	}

	if res.Scope == restfile.K8sScopeGlobal || res.Scope == restfile.K8sScopeFile {
		res.Profile.Scope = res.Scope
		res.Profile.Line = line
		b.file.k8s = append(b.file.k8s, res.Profile)
	}
}

func (b *documentBuilder) addInvalidK8sProfile(
	line int,
	prof restfile.K8sProfile,
	message string,
) {
	if prof.Scope != restfile.K8sScopeGlobal && prof.Scope != restfile.K8sScopeFile {
		return
	}
	prof.Line = line
	prof.Invalid = true
	prof.Error = message
	b.file.k8s = append(b.file.k8s, prof)
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
