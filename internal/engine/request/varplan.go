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
	// declared identifies sources allowed to contain env: references.
	declared bool
}

var sourceTable = [...]sourceTraits{
	sourceUnknown:        {},
	sourceConst:          {label: "const", template: true, hidden: true, declared: true},
	sourceScript:         {label: "script"},
	sourceWorkflow:       {label: "workflow"},
	sourceRequestCapture: {label: "request"},
	sourceRequest:        {label: "request", template: true, declared: true},
	sourceRuntimeGlobal:  {label: "global"},
	sourceDocumentGlobal: {label: "document-global", template: true, declared: true},
	sourceRuntimeFile:    {label: "file"},
	sourceFile:           {label: "file", template: true, declared: true},
	sourceEnvironment:    {label: "environment", declared: true},
}

func (s variableSource) traits() sourceTraits { return sourceTable[s] }

type variableEntry struct {
	name string
	val  vars.Value
}

type variableLayer struct {
	source variableSource
	vals   vars.NameMap[vars.Value]
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
	env     vars.ResolvedEnv
	globals vars.Globals
	sec     secrecy
	run     runVars
}

// Variable plans keep template and script lookups at the same precedence. Each
// plan includes every source layer because resolver memoization uses provider
// positions.
func (e *Engine) buildVariablePlan(src varSources) variablePlan {
	env := src.env
	if src.sec == omitSecrets {
		env = env.WithoutRefValues()
	}
	refs := env.Refs()

	plan := variablePlan{layers: make([]variableLayer, 0, len(sourceTable))}
	plan.add(sourceConst, constEntries(src.doc, refs))
	plan.addLiteral(sourceScript, src.run.scripts)
	plan.addLiteral(sourceWorkflow, src.run.overlay)
	plan.add(sourceRequestCapture, nil)
	plan.add(sourceRequest, requestEntries(src.req, src.sec, refs))
	plan.add(sourceRuntimeGlobal, literalEntries(globalValues(src.globals)))
	plan.add(sourceDocumentGlobal, docGlobalEntries(src.doc, src.sec, refs))
	plan.add(sourceRuntimeFile, literalEntries(e.runtimeFileValues(src.doc, env, src.sec)))
	plan.add(sourceFile, docVarEntries(src.doc, src.sec, refs))
	plan.add(sourceEnvironment, valueEntries(env.ProviderValues()))
	return plan
}

// add preserves declaration order so the last form of a name wins within a source.
func (p *variablePlan) add(source variableSource, entries []variableEntry) {
	var vals vars.NameMap[vars.Value]
	for _, e := range entries {
		vals.Set(e.name, e.val)
	}
	p.layers = append(p.layers, variableLayer{source: source, vals: vals})
}

func (p *variablePlan) addLiteral(source variableSource, vals vars.NameMap[string]) {
	var out vars.NameMap[vars.Value]
	for name, v := range vals.All() {
		out.Set(name, vars.Value{Text: v})
	}
	p.layers = append(p.layers, variableLayer{source: source, vals: out})
}

func (p *variablePlan) overlay(source variableSource, vals vars.NameMap[string]) {
	for i := range p.layers {
		if p.layers[i].source != source {
			continue
		}
		for name, v := range vals.All() {
			p.layers[i].vals.Set(name, vars.Value{Text: v})
		}
		return
	}
}

// providers exposes each layer to template resolution. Authored values use
// template providers so nested placeholders still expand.
func (p variablePlan) providers() []vars.Provider {
	out := make([]vars.Provider, 0, len(p.layers)+1)
	for _, l := range p.layers {
		t := l.source.traits()
		if t.template {
			out = append(out, vars.NewValueMapTemplateProvider(t.label, l.vals))
			continue
		}
		out = append(out, vars.NewValueMapProvider(t.label, l.vals))
	}
	// Process environment values are not enumerable, so they remain the final
	// fallback outside the plan.
	return append(out, vars.EnvProvider{})
}

// Scripts should see ordinary nested references resolved just as templates do.
// Dynamic helpers and expressions are evaluated later, after pre-request
// scripts have run.
func (p variablePlan) values() map[string]string {
	var seen vars.NameMap[vars.Value]
	var res *vars.Resolver
	for _, l := range p.layers {
		t := l.source.traits()
		if t.hidden {
			continue
		}
		for name, val := range l.vals.All() {
			if seen.Has(name) {
				continue
			}
			if !val.Missing && !val.Final && t.template && vars.HasPlaceholder(val.Text) {
				if res == nil {
					res = vars.NewResolver(p.providers()...)
				}
				// Leave values that need runtime data or the expression evaluator unchanged.
				if expanded, err := res.ExpandTemplatesStatic(val.Text); err == nil {
					val.Text = expanded
				}
			}
			seen.Set(name, val)
		}
	}

	out := make(map[string]string, seen.Len())
	for name, val := range seen.All() {
		if val.Missing {
			continue
		}
		out[name] = val.Text
	}
	return out
}

// Only declarations may choose the OS variable in a templated env: reference.
func declaredNames(doc *restfile.Document, req *restfile.Request, env vars.Environment) *vars.Resolver {
	out := make([]vars.Provider, 0, len(sourceTable))
	for i, t := range sourceTable {
		if !t.declared {
			continue
		}
		out = append(
			out,
			vars.NewValueMapTemplateProvider(t.label, declaredText(variableSource(i), doc, req, env)),
		)
	}
	return vars.NewResolver(out...)
}

func declaredText(
	source variableSource,
	doc *restfile.Document,
	req *restfile.Request,
	env vars.Environment,
) vars.NameMap[vars.Value] {
	var out vars.NameMap[vars.Value]
	set := func(name, text string) { out.Set(name, vars.Value{Text: text}) }
	switch source {
	case sourceConst:
		if doc != nil {
			for _, c := range doc.Constants {
				set(c.Name, c.Value)
			}
		}
	case sourceRequest:
		if req != nil {
			for _, v := range req.Variables {
				set(v.Name, v.Value)
			}
		}
	case sourceDocumentGlobal:
		if doc != nil {
			for _, v := range doc.Globals {
				set(v.Name, v.Value)
			}
		}
	case sourceFile:
		if doc != nil {
			for _, v := range doc.Variables {
				set(v.Name, v.Value)
			}
		}
	case sourceEnvironment:
		for _, name := range slices.Sorted(maps.Keys(env.Values())) {
			set(name, env.Values()[name])
		}
	}
	return out
}

func constEntries(doc *restfile.Document, refs *vars.EnvRefs) []variableEntry {
	if doc == nil {
		return nil
	}
	out := make([]variableEntry, 0, len(doc.Constants))
	for _, c := range doc.Constants {
		out = append(out, variableEntry{name: c.Name, val: declaredValue(refs, c.Authored, c.Value)})
	}
	return out
}

func requestEntries(req *restfile.Request, sec secrecy, refs *vars.EnvRefs) []variableEntry {
	if req == nil {
		return nil
	}
	return declaredEntries(req.Variables, sec, refs)
}

func docGlobalEntries(doc *restfile.Document, sec secrecy, refs *vars.EnvRefs) []variableEntry {
	if doc == nil {
		return nil
	}
	return declaredEntries(doc.Globals, sec, refs)
}

func docVarEntries(doc *restfile.Document, sec secrecy, refs *vars.EnvRefs) []variableEntry {
	if doc == nil {
		return nil
	}
	return declaredEntries(doc.Variables, sec, refs)
}

// Unlike -secret values, withheld env: references still shadow lower sources.
func declaredEntries(xs []restfile.Variable, sec secrecy, refs *vars.EnvRefs) []variableEntry {
	out := make([]variableEntry, 0, len(xs))
	for _, v := range xs {
		if sec == omitSecrets && v.Secret && !namesProcessVar(v.Authored, v.Value) {
			continue
		}
		out = append(out, variableEntry{name: v.Name, val: declaredValue(refs, v.Authored, v.Value)})
	}
	return out
}

func declaredValue(refs *vars.EnvRefs, authored bool, text string) vars.Value {
	if !authored {
		return vars.Value{Text: text}
	}
	return refs.Declared(text)
}

func namesProcessVar(authored bool, text string) bool {
	if !authored {
		return false
	}
	_, ok := vars.EnvRefKey(text)
	return ok
}

func globalValues(globs vars.Globals) map[string]string {
	out := make(map[string]string, globs.Len())
	for name, g := range globs.Sorted() {
		if g.Delete {
			continue
		}
		out[name] = g.Value
	}
	return out
}

func (e *Engine) runtimeFileValues(
	doc *restfile.Document,
	env vars.ResolvedEnv,
	sec secrecy,
) map[string]string {
	fs := e.rt.Files()
	if fs == nil {
		return nil
	}
	snap := fs.Snapshot(env.Scope(), e.filePath(doc))
	out := make(map[string]string, len(snap))
	for key, v := range snap {
		if sec == omitSecrets && v.Secret {
			continue
		}
		out[storedName(v.Name, key)] = v.Value
	}
	return out
}

func literalEntries(src map[string]string) []variableEntry {
	out := make([]variableEntry, 0, len(src))
	for _, name := range slices.Sorted(maps.Keys(src)) {
		out = append(out, variableEntry{name: name, val: vars.Value{Text: src[name]}})
	}
	return out
}

func valueEntries(src vars.NameMap[vars.Value]) []variableEntry {
	out := make([]variableEntry, 0, src.Len())
	for name, val := range src.Sorted() {
		out = append(out, variableEntry{name: name, val: val})
	}
	return out
}
