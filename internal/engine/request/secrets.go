package request

import (
	"slices"

	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// SecretSources collects values that must be redacted from run output.
type SecretSources struct {
	Doc      *restfile.Document
	Req      *restfile.Request
	Env      vars.ResolvedEnv
	FilePath string
	Files    *rtrun.Files
	Globals  *rtrun.Globals
	Extra    []string
}

// Secrets returns values longest first so overlapping secrets are fully masked.
func (s SecretSources) Secrets() []string {
	var out vars.Secrets
	out.Add(s.Env.Secrets()...)
	if s.Req != nil {
		addSecretDecls(&out, s.Req.Variables)
	}
	if s.Doc != nil {
		addSecretDecls(&out, s.Doc.Variables)
		addSecretDecls(&out, s.Doc.Globals)
	}
	for _, v := range s.Files.Snapshot(s.Env.Scope(), s.FilePath) {
		if v.Secret {
			out.Add(v.Value)
		}
	}
	for _, v := range s.Globals.Snapshot(s.Env.Scope()) {
		if v.Secret {
			out.Add(v.Value)
		}
	}
	out.Add(s.Extra...)

	vals := out.Values()
	slices.SortStableFunc(vals, func(a, b string) int { return len(b) - len(a) })
	return vals
}

func addSecretDecls(out *vars.Secrets, xs []restfile.Variable) {
	for _, v := range xs {
		if v.Secret {
			out.Add(v.Value)
		}
	}
}
