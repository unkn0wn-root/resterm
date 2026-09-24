package request

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestStandaloneRequestEvaluatesRunVarsPerExecution(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @run var id = {{$uuid}}
# @run var mail = user-{{id}}
GET http://example.test/{{id}}
X-Id: {{id}}
X-Mail: {{mail}}
`)
	eng, st := newStubEngine(t)
	env := envWith(t, "dev", nil)

	first := sendWith(t, eng, st, doc, req, env, ExecOptions{})
	id := first.wire.Header.Get("X-Id")
	if id == "" || first.wire.URL.Path != "/"+id {
		t.Fatalf("path = %q, X-Id = %q, want one id for both", first.wire.URL.Path, id)
	}
	if got := first.wire.Header.Get("X-Mail"); got != "user-"+id {
		t.Fatalf("X-Mail = %q, want it built from X-Id %q", got, id)
	}

	second := sendWith(t, eng, st, doc, req, env, ExecOptions{})
	if second.wire.Header.Get("X-Id") == id {
		t.Fatalf("second send reused id %q, want a fresh value", id)
	}
}

func TestRunVarsReachConditionsAndScripts(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @run var id = fixed-{{$randomInt(7, 7)}}
# @when vars.get("id") == "fixed-7"
GET http://example.test
`)
	req.Metadata.Scripts = append(req.Metadata.Scripts,
		rtsPre(`request.setHeader("X-RTS", vars.get("id"))`),
		jsPre(`request.setHeader("X-JS", vars.get("id"));`),
	)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	for _, h := range []string{"X-RTS", "X-JS"} {
		if got := sent.wire.Header.Get(h); got != "fixed-7" {
			t.Fatalf("%s = %q, want fixed-7", h, got)
		}
	}
}

func TestRunVarExpressionReadsTheValueAbove(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @run var id = {{$uuid}}
# @run var derived = {{= vars.get("id") + "-x" }}
GET http://example.test/{{derived}}
X-Id: {{id}}
`)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	id := sent.wire.Header.Get("X-Id")
	if id == "" || sent.wire.URL.Path != "/"+id+"-x" {
		t.Fatalf("path = %q, X-Id = %q, want the path built from the same id", sent.wire.URL.Path, id)
	}
}

func TestRunVarValueIsNotExpandedAgain(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @run var raw = {{payload}}
GET http://example.test
X-Raw: {{raw}}
`)
	env := envWith(t, "dev", map[string]string{"payload": "{{nope}}"})

	sent := sendRequest(t, doc, req, env, ExecOptions{})
	if got := sent.wire.Header.Get("X-Raw"); got != "{{nope}}" {
		t.Fatalf("X-Raw = %q, want the value unchanged", got)
	}
}

func TestSuppliedRunVarsSkipEvaluation(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @run var id = {{$uuid}}
GET http://example.test
X-Id: {{id}}
`)
	sc := RunScope{RunVars: vars.CollectNames(map[string]string{"id": "given"})}

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{Run: &sc})
	if got := sent.wire.Header.Get("X-Id"); got != "given" {
		t.Fatalf("X-Id = %q, want the run's value", got)
	}
}

func TestRunVarFailureSendsNothing(t *testing.T) {
	tests := []struct {
		name string
		decl string
		line int
		want string
	}{
		{"bad helper", "# @run var bad = {{$randomInt(x)}}", 3, "@run var bad: "},
		{"declared below", "# @run var a = {{b}}\n# @run var b = x", 3, "@run var a: undefined variable: b"},
		{"undefined", "# @run var a = {{missing}}", 3, "@run var a: undefined variable: missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, req := parseDoc(t, "### one\n# @name one\n"+tt.decl+"\nGET http://example.test\n")
			eng, st := newStubEngine(t)

			res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if st.wire != nil {
				t.Fatal("request was sent despite the failed @run var")
			}
			if res.Err == nil || !strings.HasPrefix(res.Err.Error(), tt.want) {
				t.Fatalf("result error = %v, want one starting with %q", res.Err, tt.want)
			}
			if got := diag.ReportOf(res.Err).Items[0].Span.Start.Line; got != tt.line {
				t.Fatalf("error points at line %d, want the declaration on line %d", got, tt.line)
			}
		})
	}
}

func TestEvalRunVarsShadowsWithoutMutatingTheRun(t *testing.T) {
	doc := &restfile.Document{
		Path:      "run.http",
		Variables: []restfile.Variable{{Name: "base", Value: "pre", Authored: true}},
	}
	sc := RunScope{RunVars: vars.CollectNames(map[string]string{"x": "old", "keep": "kept"})}
	decls := []restfile.RunVar{{Name: "x", Value: "{{base}}-new", Line: 1}}

	got, err := newTestEngine().EvalRunVars(context.Background(), doc, nil, envWith(t, "dev", nil), sc, decls)
	if err != nil {
		t.Fatalf("EvalRunVars() error = %v", err)
	}
	if v, _ := got.Get("x"); v != "pre-new" {
		t.Fatalf("x = %q, want the declaration over the run's value", v)
	}
	if v, _ := got.Get("keep"); v != "kept" {
		t.Fatalf("keep = %q, want the run's value carried over", v)
	}
	if v, _ := sc.RunVars.Get("x"); v != "old" {
		t.Fatalf("run scope changed to %q", v)
	}
}

func TestEvalRunVarsDerivesFromTheValueItShadows(t *testing.T) {
	sc := RunScope{RunVars: vars.CollectNames(map[string]string{"suffix": "base"})}
	decls := []restfile.RunVar{
		{Name: "suffix", Value: "{{suffix}}-x", Line: 1},
		{Name: "next", Value: `{{= vars.get("suffix") + "-y" }}`, Line: 2},
	}

	got, err := newTestEngine().EvalRunVars(context.Background(), nil, nil, envWith(t, "dev", nil), sc, decls)
	if err != nil {
		t.Fatalf("EvalRunVars() error = %v", err)
	}
	if v, _ := got.Get("suffix"); v != "base-x" {
		t.Fatalf("suffix = %q, want it built from the run's value", v)
	}
	if v, _ := got.Get("next"); v != "base-x-y" {
		t.Fatalf("next = %q, want it built from the new suffix", v)
	}
}

func TestEvalRunVarsReportsUndefinedAsUndefined(t *testing.T) {
	decls := []restfile.RunVar{{Name: "x", Value: "{{missing}}", Line: 4}}
	_, err := newTestEngine().EvalRunVars(context.Background(), nil, nil, envWith(t, "dev", nil), RunScope{}, decls)
	if !errors.Is(err, vars.ErrUndefinedVariable) {
		t.Fatalf("EvalRunVars() error = %v, want ErrUndefinedVariable", err)
	}
}
