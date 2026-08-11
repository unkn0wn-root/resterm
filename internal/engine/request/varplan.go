package request

import (
	"maps"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type variableSource uint8

const (
	sourceUnknown variableSource = iota
	sourceConst
	sourceScript
	sourceWorkflow
	sourceRequest
	sourceRuntimeGlobal
	sourceDocumentGlobal
	sourceRuntimeFile
	sourceFile
	sourceEnvironment
)

type sourceTraits struct {
	// label identifies the source in traces and qualified {{label.name}} lookups.
	label string
	// template enables nested expansion for authored values; runtime values stay literal.
	template bool
	// hidden excludes values from script-facing vars; constants are not overridable.
	hidden bool
}

var sourceTable = [...]sourceTraits{
	sourceUnknown:        {},
	sourceConst:          {label: "const", template: true, hidden: true},
	sourceScript:         {label: "script"},
	sourceWorkflow:       {label: "workflow"},
	sourceRequest:        {label: "request", template: true},
	sourceRuntimeGlobal:  {label: "global"},
	sourceDocumentGlobal: {label: "document-global", template: true},
	sourceRuntimeFile:    {label: "file"},
	sourceFile:           {label: "file", template: true},
	sourceEnvironment:    {label: "environment"},
}

func (s variableSource) traits() sourceTraits { return sourceTable[s] }

type variableEntry struct {
	name  string
	value string
}

type variableLayer struct {
	source variableSource
	// vals holds one value per name, keyed by the form the winning
	// declaration used.
	vals map[string]string
}

// variablePlan is the shared precedence order for template and script lookups.
// Layers run from highest to lowest precedence. RTS locals, @use aliases, and
// host bindings use separate lexical scoping and are not included.
type variablePlan struct {
	layers []variableLayer
}

type runVars struct {
	scripts map[string]string
	// overlay contains workflow step and @for-each bindings.
	overlay map[string]string
}

type varSources struct {
	doc     *restfile.Document
	req     *restfile.Request
	env     vars.Environment
	globals map[string]vars.GlobalMutation
	sec     secrecy
	run     runVars
}

// buildVariablePlan defines the precedence shared by template and script lookups.
func (e *Engine) buildVariablePlan(src varSources) variablePlan {
	plan := variablePlan{layers: make([]variableLayer, 0, len(sourceTable))}
	plan.add(sourceConst, constEntries(src.doc))
	plan.add(sourceScript, sortedEntries(src.run.scripts))
	plan.add(sourceWorkflow, sortedEntries(src.run.overlay))
	plan.add(sourceRequest, requestEntries(src.req, src.sec))
	plan.add(sourceRuntimeGlobal, globalEntries(src.globals))
	plan.add(sourceDocumentGlobal, docGlobalEntries(src.doc, src.sec))
	plan.add(sourceRuntimeFile, e.runtimeFileEntries(src.doc, src.env, src.sec))
	plan.add(sourceFile, docVarEntries(src.doc, src.sec))
	plan.add(sourceEnvironment, sortedEntries(src.env.Values()))
	return plan
}

// add appends the layer source contributes, or nothing when the source
// declared no variables. Every layer is built here, so one holding two forms
// of a name or a value map the source never filled cannot reach a plan.
func (p *variablePlan) add(source variableSource, entries []variableEntry) {
	vals := collapseEntries(entries)
	if len(vals) == 0 {
		return
	}
	p.layers = append(p.layers, variableLayer{source: source, vals: vals})
}

// providers exposes each layer to template resolution. Authored values use
// template providers so nested placeholders still expand.
func (p variablePlan) providers() []vars.Provider {
	out := make([]vars.Provider, 0, len(p.layers)+1)
	for _, l := range p.layers {
		t := l.source.traits()
		if t.template {
			out = append(out, vars.NewTemplateProvider(t.label, l.vals))
			continue
		}
		out = append(out, vars.NewMapProvider(t.label, l.vals))
	}
	// Process environment values are not enumerable, so they remain the final
	// fallback outside the plan.
	return append(out, vars.EnvProvider{})
}

// values flattens the plan for RTS and JavaScript while preserving provider precedence.
func (p variablePlan) values() map[string]string {
	out := make(map[string]string)
	claimed := make(map[string]struct{})
	for _, l := range p.layers {
		if l.source.traits().hidden {
			continue
		}
		for name, value := range l.vals {
			key := vars.NameKey(name)
			if _, taken := claimed[key]; taken {
				continue
			}
			claimed[key] = struct{}{}
			out[name] = value
		}
	}
	return out
}

// collapseEntries reduces a layer's declarations to a single value per name,
// keyed by the form the winning declaration used. Entries are walked in order
// and a later one replaces an earlier form of the same name, which is the rule
// environment merging and @capture upserts already follow.
func collapseEntries(entries []variableEntry) map[string]string {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		vars.Upsert(out, e.name, e.value)
	}
	return out
}

func constEntries(doc *restfile.Document) []variableEntry {
	if doc == nil {
		return nil
	}
	out := make([]variableEntry, 0, len(doc.Constants))
	for _, c := range doc.Constants {
		out = append(out, variableEntry{name: c.Name, value: c.Value})
	}
	return out
}

func requestEntries(req *restfile.Request, sec secrecy) []variableEntry {
	if req == nil {
		return nil
	}
	return declaredEntries(req.Variables, sec)
}

func docGlobalEntries(doc *restfile.Document, sec secrecy) []variableEntry {
	if doc == nil {
		return nil
	}
	return declaredEntries(doc.Globals, sec)
}

func docVarEntries(doc *restfile.Document, sec secrecy) []variableEntry {
	if doc == nil {
		return nil
	}
	return declaredEntries(doc.Variables, sec)
}

func declaredEntries(xs []restfile.Variable, sec secrecy) []variableEntry {
	out := make([]variableEntry, 0, len(xs))
	for _, v := range xs {
		if sec == omitSecrets && v.Secret {
			continue
		}
		out = append(out, variableEntry{name: v.Name, value: v.Value})
	}
	return out
}

func globalEntries(globs map[string]vars.GlobalMutation) []variableEntry {
	out := make([]variableEntry, 0, len(globs))
	for _, key := range slices.Sorted(maps.Keys(globs)) {
		g := globs[key]
		name := strings.TrimSpace(g.Name)
		if name == "" {
			name = strings.TrimSpace(key)
		}
		if name == "" || g.Delete {
			continue
		}
		out = append(out, variableEntry{name: name, value: g.Value})
	}
	return out
}

func (e *Engine) runtimeFileEntries(
	doc *restfile.Document,
	env vars.Environment,
	sec secrecy,
) []variableEntry {
	fs := e.rt.Files()
	if fs == nil {
		return nil
	}
	snap := fs.Snapshot(env.Scope(), e.filePath(doc))
	out := make([]variableEntry, 0, len(snap))
	for _, key := range slices.Sorted(maps.Keys(snap)) {
		v := snap[key]
		if sec == omitSecrets && v.Secret {
			continue
		}
		name := strings.TrimSpace(v.Name)
		if name == "" {
			name = key
		}
		out = append(out, variableEntry{name: name, value: v.Value})
	}
	return out
}

// sortedEntries orders a map source by name so one set of inputs always builds
// the same plan. A map has no declaration order, so the layers that come from
// one hold a single form per name already: writers go through vars.Upsert and
// the environment, global, and file stores key by name.
func sortedEntries(src map[string]string) []variableEntry {
	out := make([]variableEntry, 0, len(src))
	for _, name := range slices.Sorted(maps.Keys(src)) {
		out = append(out, variableEntry{name: name, value: src[name]})
	}
	return out
}
