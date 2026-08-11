package request

import (
	"context"
	"net/http"
	"testing"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type varSource int

const (
	srcConst varSource = iota
	srcScript
	srcWorkflow
	srcRequest
	srcRuntimeGlobal
	srcDocGlobal
	srcRuntimeFile
	srcFileVar
	srcEnvironment
)

var varSourceOrder = []varSource{
	srcConst,
	srcScript,
	srcWorkflow,
	srcRequest,
	srcRuntimeGlobal,
	srcDocGlobal,
	srcRuntimeFile,
	srcFileVar,
	srcEnvironment,
}

func (s varSource) String() string {
	switch s {
	case srcConst:
		return "const"
	case srcScript:
		return "script"
	case srcWorkflow:
		return "workflow"
	case srcRequest:
		return "request"
	case srcRuntimeGlobal:
		return "runtime-global"
	case srcDocGlobal:
		return "document-global"
	case srcRuntimeFile:
		return "runtime-file"
	case srcFileVar:
		return "file"
	case srcEnvironment:
		return "environment"
	default:
		return "unknown"
	}
}

func (s varSource) hidden() bool { return s == srcConst }

type varDecl struct {
	src   varSource
	name  string
	value string
}

type planFixture struct {
	eng   *Engine
	doc   *restfile.Document
	req   *restfile.Request
	env   vars.Environment
	globs map[string]vars.GlobalMutation
	run   runVars
}

func newPlanFixture(t *testing.T, decls ...varDecl) planFixture {
	t.Helper()

	f := planFixture{
		eng:   New(engcfg.Config{}, nil),
		doc:   &restfile.Document{Path: "plan.http"},
		req:   &restfile.Request{Method: http.MethodGet, URL: "http://example.test"},
		globs: make(map[string]vars.GlobalMutation),
	}
	envValues := make(map[string]string)
	scriptVars := make(map[string]string)
	loopVars := make(map[string]string)
	var runtimeFiles []varDecl

	for _, d := range decls {
		switch d.src {
		case srcConst:
			f.doc.Constants = append(f.doc.Constants, restfile.Constant{Name: d.name, Value: d.value})
		case srcScript:
			scriptVars[d.name] = d.value
		case srcWorkflow:
			loopVars[d.name] = d.value
		case srcRequest:
			f.req.Variables = append(f.req.Variables, restfile.Variable{Name: d.name, Value: d.value})
		case srcRuntimeGlobal:
			f.globs[vars.NameKey(d.name)] = vars.GlobalMutation{Name: d.name, Value: d.value}
		case srcDocGlobal:
			f.doc.Globals = append(f.doc.Globals, restfile.Variable{Name: d.name, Value: d.value})
		case srcRuntimeFile:
			runtimeFiles = append(runtimeFiles, d)
		case srcFileVar:
			f.doc.Variables = append(f.doc.Variables, restfile.Variable{Name: d.name, Value: d.value})
		case srcEnvironment:
			envValues[d.name] = d.value
		default:
			t.Fatalf("unknown variable source %d", d.src)
		}
	}

	cat, err := vars.NewCatalog(vars.EnvironmentSet{"dev": envValues})
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}
	sel, err := cat.Select("dev", nil)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	f.env, err = cat.Resolve(sel)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	for _, d := range runtimeFiles {
		f.eng.rt.Files().Set(f.env.Scope(), f.doc.Path, d.name, d.value, false)
	}

	f.run = runVars{
		scripts: vars.CollectNames(scriptVars),
		overlay: vars.CollectNames(loopVars),
	}
	return f
}

func (f planFixture) values() map[string]string {
	return f.eng.buildVariablePlan(varSources{
		doc:     f.doc,
		req:     f.req,
		env:     f.env,
		globals: f.globs,
		sec:     keepSecrets,
		run:     f.run,
	}).values()
}

func (f planFixture) expand(t *testing.T, input string) string {
	t.Helper()

	res := f.eng.buildResolver(
		context.Background(),
		f.doc,
		f.req,
		f.env,
		"",
		f.globs,
		rts.Locals{},
		f.run,
	)
	out, err := res.ExpandTemplates(input)
	if err != nil {
		t.Fatalf("ExpandTemplates(%q) error = %v", input, err)
	}
	return out
}

func TestVariablePlanPrecedenceMatrix(t *testing.T) {
	for i, high := range varSourceOrder {
		for _, low := range varSourceOrder[i+1:] {
			t.Run(high.String()+"_over_"+low.String(), func(t *testing.T) {
				f := newPlanFixture(t,
					varDecl{src: high, name: "token", value: "high"},
					varDecl{src: low, name: "token", value: "low"},
				)

				if got := f.expand(t, "{{token}}"); got != "high" {
					t.Fatalf("{{token}} = %q, want %q", got, "high")
				}

				// Constants resolve in templates but are hidden from script variables.
				want := "high"
				if high.hidden() {
					want = "low"
				}
				if got := f.values()["token"]; got != want {
					t.Fatalf("values()[token] = %q, want %q", got, want)
				}
				if got := f.expand(t, `{{= vars.get("token") }}`); got != want {
					t.Fatalf(`{{= vars.get("token") }} = %q, want %q`, got, want)
				}
			})
		}
	}
}

func TestVariablePlanCollapsesCaseVariantsAcrossLayers(t *testing.T) {
	f := newPlanFixture(t,
		varDecl{src: srcFileVar, name: "token", value: "file"},
		varDecl{src: srcEnvironment, name: "TOKEN", value: "environment"},
	)

	vv := f.values()
	if len(vv) != 1 {
		t.Fatalf("values() = %v, want a single entry", vv)
	}
	if got := vv["token"]; got != "file" {
		t.Fatalf("values()[token] = %q, want %q", got, "file")
	}
	if got := f.expand(t, "{{TOKEN}}"); got != "file" {
		t.Fatalf("{{TOKEN}} = %q, want %q", got, "file")
	}
}

func TestVariablePlanLastDeclarationWinsWithinLayer(t *testing.T) {
	f := newPlanFixture(t,
		varDecl{src: srcFileVar, name: "Token", value: "first"},
		varDecl{src: srcFileVar, name: "token", value: "second"},
	)

	vv := f.values()
	if len(vv) != 1 {
		t.Fatalf("values() = %v, want a single entry", vv)
	}
	if got := vv["token"]; got != "second" {
		t.Fatalf("values()[token] = %q, want %q", got, "second")
	}
	if got := f.expand(t, "{{Token}}"); got != "second" {
		t.Fatalf("{{Token}} = %q, want %q", got, "second")
	}
}

func TestVariablePlanReadsTheSuppliedRuntimeGlobals(t *testing.T) {
	f := newPlanFixture(t, varDecl{src: srcRuntimeGlobal, name: "token", value: "preview"})

	if got := f.values()["token"]; got != "preview" {
		t.Fatalf("values()[token] = %q, want %q", got, "preview")
	}
	if got := f.expand(t, `{{= vars.get("token") }}`); got != "preview" {
		t.Fatalf(`{{= vars.get("token") }} = %q, want %q`, got, "preview")
	}
}

func TestVariablePlanKeepsProcessEnvironmentOutOfVars(t *testing.T) {
	t.Setenv("RESTERM_PLAN_TOKEN", "shell")
	f := newPlanFixture(t)

	if got := f.expand(t, "{{RESTERM_PLAN_TOKEN}}"); got != "shell" {
		t.Fatalf("{{RESTERM_PLAN_TOKEN}} = %q, want %q", got, "shell")
	}
	if _, ok := f.values()["RESTERM_PLAN_TOKEN"]; ok {
		t.Fatalf("values() = %v, want no process environment entry", f.values())
	}
	if got := f.expand(t, `{{= vars.get("RESTERM_PLAN_TOKEN") ?? "missing" }}`); got != "missing" {
		t.Fatalf(`{{= vars.get("RESTERM_PLAN_TOKEN") }} = %q, want %q`, got, "missing")
	}
}
