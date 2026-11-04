package request

import (
	"slices"
	"unicode/utf8"

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

// ShortSecrets returns names, never values, sorted.
func ShortSecrets(doc *restfile.Document, env vars.Environment, n int) []string {
	refs := ResolveEnvironment(env, doc, nil).Refs()
	var out []string
	check := func(name, text string, authored, secret bool) {
		if !secret && !isEnvRef(authored, text) {
			return
		}
		if v := declaredValue(refs, authored, text).Text; v != "" && utf8.RuneCountInString(v) < n {
			out = append(out, name)
		}
	}
	for name, text := range env.Values() {
		check(name, text, true, false)
	}
	for _, c := range doc.Constants {
		check(c.Name, c.Value, c.Authored, false)
	}
	lists := [][]restfile.Variable{doc.Variables, doc.Globals}
	for _, req := range doc.Requests {
		lists = append(lists, req.Variables)
	}
	for _, vs := range lists {
		for _, v := range vs {
			check(v.Name, v.Value, v.Authored, v.Secret)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func addSecretDecls(out *vars.Secrets, xs []restfile.Variable) {
	for _, v := range xs {
		if v.Secret {
			out.Add(v.Value)
		}
	}
}
