package ui

import (
	"sort"

	"github.com/unkn0wn-root/resterm/internal/intellisense"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) refreshCompletionScope() {
	scope := buildCompletionScope(m.doc, m.cfg.EnvironmentSet, m.cfg.EnvironmentName)
	m.editor.SetCompletionScope(scope)
}

func buildCompletionScope(
	doc *restfile.Document,
	set vars.EnvironmentSet,
	env string,
) intellisense.Scope {
	var scope intellisense.Scope
	seen := make(map[string]struct{})
	add := func(name, origin string, secret bool) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		scope.Variables = append(scope.Variables, intellisense.VarRef{
			Name:   name,
			Origin: origin,
			Secret: secret,
		})
	}

	if doc != nil {
		for _, v := range doc.Globals {
			add(v.Name, "global", v.Secret)
		}
		for _, v := range doc.Variables {
			add(v.Name, "file", v.Secret)
		}
		for _, req := range doc.Requests {
			for _, v := range req.Variables {
				add(v.Name, "request", v.Secret)
			}
		}
		for _, c := range doc.Constants {
			add(c.Name, "const", false)
		}
		scope.Profiles = intellisense.ProfileSet{
			Patch: profileNames(doc.Patches, func(p restfile.PatchProfile) string { return p.Name }),
			SSH:   profileNames(doc.SSH, func(p restfile.SSHProfile) string { return p.Name }),
			K8s:   profileNames(doc.K8s, func(p restfile.K8sProfile) string { return p.Name }),
		}
	}

	// Environment keys fill in only where a declared variable did not.
	for _, key := range sortedKeys(vars.EnvValues(set, env)) {
		add(key, "env", false)
	}
	scope.Environments = environmentNames(set)
	return scope
}

func environmentNames(set vars.EnvironmentSet) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		if name == vars.SharedEnvKey {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func profileNames[T any](profiles []T, name func(T) string) []string {
	out := make([]string, len(profiles))
	for i, p := range profiles {
		out[i] = name(p)
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
