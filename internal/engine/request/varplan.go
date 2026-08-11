package request

import (
	"maps"
	"slices"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type variableSource uint8

const (
	sourceUnknown variableSource = iota
	sourceConst
	sourceScript
	sourceWorkflow
	sourceRequestCapture
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
	sourceRequestCapture: {label: "request"},
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
	vals   vars.NameMap[string]
}

// variablePlan is the shared precedence order for template and script lookups.
// Layers run from highest to lowest precedence. RTS locals, @use aliases, and
// host bindings use separate lexical scoping and are not included.
type variablePlan struct {
	layers []variableLayer
}

type runVars struct {
	scripts vars.NameMap[string]
	// overlay contains workflow step and @for-each bindings.
	overlay vars.NameMap[string]
}

type varSources struct {
	doc     *restfile.Document
	req     *restfile.Request
	env     vars.Environment
	globals vars.Globals
	sec     secrecy
	run     runVars
}

// Variable plans keep template and script lookups at the same precedence. Each
// plan includes every source layer because resolver memoization uses provider
// positions. Authored values may expand nested templates, while runtime values
// such as captures remain literal.
func (e *Engine) buildVariablePlan(src varSources) variablePlan {
	plan := variablePlan{layers: make([]variableLayer, 0, len(sourceTable))}
	plan.add(sourceConst, constEntries(src.doc))
	plan.addNames(sourceScript, src.run.scripts)
	plan.addNames(sourceWorkflow, src.run.overlay)
	plan.add(sourceRequestCapture, nil)
	plan.add(sourceRequest, requestEntries(src.req, src.sec))
	plan.add(sourceRuntimeGlobal, globalEntries(src.globals))
	plan.add(sourceDocumentGlobal, docGlobalEntries(src.doc, src.sec))
	plan.add(sourceRuntimeFile, e.runtimeFileEntries(src.doc, src.env, src.sec))
	plan.add(sourceFile, docVarEntries(src.doc, src.sec))
	plan.add(sourceEnvironment, sortedEntries(src.env.Values()))
	return plan
}

// add preserves declaration order so the last form of a name wins within a source.
func (p *variablePlan) add(source variableSource, entries []variableEntry) {
	var vals vars.NameMap[string]
	for _, e := range entries {
		vals.Set(e.name, e.value)
	}
	p.appendLayer(source, vals)
}

// addNames snapshots an already-normalized source.
func (p *variablePlan) addNames(source variableSource, vals vars.NameMap[string]) {
	p.appendLayer(source, vals.Clone())
}

func (p *variablePlan) appendLayer(source variableSource, vals vars.NameMap[string]) {
	p.layers = append(p.layers, variableLayer{source: source, vals: vals})
}

func (p *variablePlan) overlay(source variableSource, vals vars.NameMap[string]) {
	for i := range p.layers {
		if p.layers[i].source == source {
			p.layers[i].vals.Merge(vals)
			return
		}
	}
}

// providers exposes each layer to template resolution. Authored values use
// template providers so nested placeholders still expand.
func (p variablePlan) providers() []vars.Provider {
	out := make([]vars.Provider, 0, len(p.layers)+1)
	for _, l := range p.layers {
		t := l.source.traits()
		if t.template {
			out = append(out, vars.NewNameMapTemplateProvider(t.label, l.vals))
			continue
		}
		out = append(out, vars.NewNameMapProvider(t.label, l.vals))
	}
	// Process environment values are not enumerable, so they remain the final
	// fallback outside the plan.
	return append(out, vars.EnvProvider{})
}

// values flattens the plan for RTS and JavaScript while preserving provider precedence.
func (p variablePlan) values() map[string]string {
	var out vars.NameMap[string]
	for _, l := range p.layers {
		if l.source.traits().hidden {
			continue
		}
		for name, value := range l.vals.All() {
			if out.Has(name) {
				continue
			}
			out.Set(name, value)
		}
	}
	return out.Map()
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

func globalEntries(globs vars.Globals) []variableEntry {
	out := make([]variableEntry, 0, globs.Len())
	for name, g := range globs.Sorted() {
		if g.Delete {
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
		out = append(out, variableEntry{name: storedName(v.Name, key), value: v.Value})
	}
	return out
}

func sortedEntries(src map[string]string) []variableEntry {
	out := make([]variableEntry, 0, len(src))
	for _, name := range slices.Sorted(maps.Keys(src)) {
		out = append(out, variableEntry{name: name, value: src[name]})
	}
	return out
}
