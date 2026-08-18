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
	hidden       bool
	allowsEnvRef bool
}

var sourceTable = [...]sourceTraits{
	sourceUnknown:        {},
	sourceConst:          {label: "const", template: true, hidden: true, allowsEnvRef: true},
	sourceScript:         {label: "script"},
	sourceWorkflow:       {label: "workflow"},
	sourceRequestCapture: {label: "request"},
	sourceRequest:        {label: "request", template: true, allowsEnvRef: true},
	sourceRuntimeGlobal:  {label: "global"},
	sourceDocumentGlobal: {label: "document-global", template: true, allowsEnvRef: true},
	sourceRuntimeFile:    {label: "file"},
	sourceFile:           {label: "file", template: true, allowsEnvRef: true},
	sourceEnvironment:    {label: "environment", allowsEnvRef: true},
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
	plan.add(sourceConst, entries(sourceConst, src, refs))
	plan.addLiteral(sourceScript, src.run.scripts)
	plan.addLiteral(sourceWorkflow, src.run.overlay)
	plan.add(sourceRequestCapture, nil)
	plan.add(sourceRequest, entries(sourceRequest, src, refs))
	plan.add(sourceRuntimeGlobal, literalEntries(globalValues(src.globals)))
	plan.add(sourceDocumentGlobal, entries(sourceDocumentGlobal, src, refs))
	plan.add(sourceRuntimeFile, literalEntries(e.runtimeFileValues(src.doc, env, src.sec)))
	plan.add(sourceFile, entries(sourceFile, src, refs))
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
		out = append(out, layerProvider(l.source.traits(), l.vals))
	}
	// Process environment values are not enumerable, so they remain the final
	// fallback outside the plan.
	return append(out, vars.EnvProvider{})
}

func layerProvider(t sourceTraits, vals vars.NameMap[vars.Value]) vars.Provider {
	if t.template {
		return vars.NewValueMapTemplateProvider(t.label, vals)
	}
	return vars.NewValueMapProvider(t.label, vals)
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
// A name from response data would let a server pick what a request sends.
func declaredNames(doc *restfile.Document, req *restfile.Request, env vars.Environment) *vars.Resolver {
	out := make([]vars.Provider, 0, len(sourceTable))
	for i, t := range sourceTable {
		if !t.allowsEnvRef {
			continue
		}
		var vals vars.NameMap[vars.Value]
		if variableSource(i) == sourceEnvironment {
			// An environment file has no runtime writer.
			for name, text := range env.Values() {
				vals.Set(name, vars.Value{Text: text})
			}
		}
		for _, d := range declarations(variableSource(i), doc, req) {
			if d.authored {
				vals.Set(d.name, vars.Value{Text: d.text})
			}
		}
		out = append(out, layerProvider(t, vals))
	}
	return vars.NewResolver(out...)
}

type declaration struct {
	name string
	text string
	// authored is false once a capture or a script replaces what the document
	// wrote. Such a value is data: it never reads as a reference nor names one.
	authored bool
	secret   bool
}

// declarations is the one reading of a source, so the variable plan and the
// reference-name lookup cannot disagree about which values a run produced.
func declarations(
	source variableSource,
	doc *restfile.Document,
	req *restfile.Request,
) []declaration {
	var xs []restfile.Variable
	switch source {
	case sourceConst:
		if doc == nil {
			return nil
		}
		out := make([]declaration, 0, len(doc.Constants))
		for _, c := range doc.Constants {
			out = append(out, declaration{name: c.Name, text: c.Value, authored: c.Authored})
		}
		return out
	case sourceRequest:
		if req == nil {
			return nil
		}
		xs = req.Variables
	case sourceDocumentGlobal:
		if doc == nil {
			return nil
		}
		xs = doc.Globals
	case sourceFile:
		if doc == nil {
			return nil
		}
		xs = doc.Variables
	default:
		return nil
	}

	out := make([]declaration, 0, len(xs))
	for _, v := range xs {
		out = append(out, declaration{
			name:     v.Name,
			text:     v.Value,
			authored: v.Authored,
			secret:   v.Secret,
		})
	}
	return out
}

// Unlike a -secret value, a withheld env: reference keeps shadowing lower
// sources, so it stays in the layer.
func entries(source variableSource, src varSources, refs *vars.EnvRefs) []variableEntry {
	ds := declarations(source, src.doc, src.req)
	out := make([]variableEntry, 0, len(ds))
	for _, d := range ds {
		if src.sec == omitSecrets && d.secret && !isEnvRef(d.authored, d.text) {
			continue
		}
		out = append(out, variableEntry{name: d.name, val: declaredValue(refs, d.authored, d.text)})
	}
	return out
}

func declaredValue(refs *vars.EnvRefs, authored bool, text string) vars.Value {
	if !authored {
		return vars.Value{Text: text}
	}
	return refs.ResolveDeclared(text)
}

func isEnvRef(authored bool, text string) bool {
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
