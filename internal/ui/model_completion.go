package ui

import (
	"sort"

	"github.com/unkn0wn-root/resterm/internal/intellisense"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func (m *Model) refreshCompletionScope() {
	scope := buildCompletionScope(m.doc, m.cfg.Catalog, m.cfg.Selection)
	m.editor.SetCompletionScope(scope)
}

func buildCompletionScope(
	doc *restfile.Document,
	cat vars.Catalog,
	sel vars.Selection,
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
	if env, err := cat.Resolve(sel); err == nil {
		for _, key := range sortedKeys(env.Values()) {
			add(key, "env", false)
		}
	}
	scope.Environments, scope.EnvironmentGroups = completionEnvironments(cat)
	return scope
}

func completionEnvironments(cat vars.Catalog) ([]string, map[string][]string) {
	if !cat.Grouped() {
		return cat.Names(), nil
	}
	groups := make(map[string][]string, cat.Len())
	for _, group := range cat.Groups() {
		groups[group.Name] = group.ProfileNames()
	}
	return nil, groups
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
