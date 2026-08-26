package rtshost

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/stdlib"
)

var testPos = rts.Pos{Path: "test", Line: 1, Col: 1}

func testRuntime(t *testing.T) Runtime {
	t.Helper()
	scope, err := NewScope(ScopeInput{
		EnvName: "dev",
		Env:     map[string]string{"NAME": "service", "meta": "value"},
		Groups:  map[string]string{"region": "eu"},
		Globals: map[string]string{"shared": "yes"},
	}, map[string]string{"token": "abc"})
	if err != nil {
		t.Fatalf("NewScope: %v", err)
	}
	req, err := NewRequest(
		"POST",
		"https://example.test/p?a=1&a=2&b=3&empty=",
		map[string][]string{
			"X-Empty": {},
			"X-One":   {"one"},
			"X-Test":  {"one", "two"},
		},
		map[string][]string{
			"a":     {"1", "2"},
			"b":     {"3"},
			"empty": {""},
			"none":  {},
		},
	)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := NewResponse(
		"200 OK",
		200,
		map[string][]string{
			"Content-Type": {"application/json"},
			"Set-Cookie":   {"a=1", "b=2"},
		},
		[]byte(`{"ok":true}`),
		"https://example.test/p",
	)
	if err != nil {
		t.Fatalf("NewResponse: %v", err)
	}
	return Runtime{Scope: scope, Request: req, Last: resp, Response: resp}
}

func evalHost(t *testing.T, eng *Engine, rt Runtime, src string) rts.Value {
	t.Helper()
	v, err := eng.Eval(t.Context(), rt, src, testPos)
	if err != nil {
		t.Fatalf("Eval(%s): %v", src, err)
	}
	return v
}

func TestRuntimeSeparatesValuesFromEnvironmentMetadata(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	tests := map[string]string{
		`env.meta.name`:          "dev",
		`env.meta.groups.region`: "eu",
		`env.NAME`:               "service",
		`env.get("meta")`:        "value",
		`vars.token`:             "abc",
		`vars.global.shared`:     "yes",
	}
	for src, want := range tests {
		v := evalHost(t, eng, rt, src)
		if v.K != rts.VStr || v.S != want {
			t.Errorf("%s = %+v, want %q", src, v, want)
		}
	}
}

func TestEnvironmentMetadataGroupsAndRequire(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	tests := map[string]string{
		`env.meta.groups.get("REGION")`:     "eu",
		`env.meta.groups.require("region")`: "eu",
		`env.require("name")`:               "service",
		`vars.require("TOKEN")`:             "abc",
		`vars.global.require("shared")`:     "yes",
	}
	for src, want := range tests {
		v := evalHost(t, eng, rt, src)
		if v.K != rts.VStr || v.S != want {
			t.Errorf("%s = %+v, want %q", src, v, want)
		}
	}

	_, err := eng.Eval(
		context.Background(),
		rt,
		`env.meta.groups.require("missing", "profile missing")`,
		testPos,
	)
	if err == nil || !strings.Contains(err.Error(), "profile missing") {
		t.Fatalf("custom require error = %v, want profile missing", err)
	}
}

func TestRuntimeExposesCardinalityBasedHeadersAndQuery(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	tests := map[string]string{
		`request.headers["x-test"][0]`:      "one",
		`request.headers["x-test"][1]`:      "two",
		`request.headers["x-one"]`:          "one",
		`request.query.a[0]`:                "1",
		`request.query.a[1]`:                "2",
		`request.query.b`:                   "3",
		`request.query.empty`:               "",
		`response.headers["content-type"]`:  "application/json",
		`response.headers["set-cookie"][1]`: "b=2",
		`response.json("ok")`:               "true",
	}
	for src, want := range tests {
		v := evalHost(t, eng, rt, `str(`+src+`)`)
		if v.K != rts.VStr || v.S != want {
			t.Errorf("%s = %+v, want %q", src, v, want)
		}
	}

	shapes := []struct {
		name string
		src  string
		kind rts.VKind
		n    int
	}{
		{name: "one header", src: `request.headers["x-one"]`, kind: rts.VStr},
		{name: "multiple headers", src: `request.headers["x-test"]`, kind: rts.VList, n: 2},
		{name: "zero headers", src: `request.headers["x-empty"]`, kind: rts.VList},
		{name: "one query value", src: `request.query.b`, kind: rts.VStr},
		{name: "multiple query values", src: `request.query.a`, kind: rts.VList, n: 2},
		{name: "zero query values", src: `request.query.none`, kind: rts.VList},
	}
	for _, tt := range shapes {
		t.Run(tt.name, func(t *testing.T) {
			v := evalHost(t, eng, rt, tt.src)
			if v.K != tt.kind || len(v.L) != tt.n {
				t.Fatalf("%s = %+v, want kind %v and len %d", tt.src, v, tt.kind, tt.n)
			}
		})
	}
}

func TestRuntimeRejectsAmbiguousNamesAtConstruction(t *testing.T) {
	_, err := NewScope(ScopeInput{}, map[string]string{"Token": "a", " token ": "b"})
	if err == nil || !strings.Contains(err.Error(), "same name") {
		t.Fatalf("NewScope error = %v, want a collision", err)
	}
	if _, err := NewRequest("GET", "", map[string][]string{
		"X-Test": {"a"},
		"x-test": {"b"},
	}, nil); err == nil || !strings.Contains(err.Error(), "same HTTP header") {
		t.Fatalf("NewRequest error = %v, want a collision", err)
	}
}

func TestRuntimeNativeFunctionsAreStrict(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	for _, src := range []string{
		`request.header(1)`,
		`request.setMethod(1)`,
		`vars.set("n", 1)`,
		`vars.global.set("n", true)`,
	} {
		if _, err := eng.Eval(context.Background(), rt, src, testPos); err == nil {
			t.Fatalf("%s: expected a type error", src)
		}
	}
}

func TestRuntimeResponseAndAssertionBindings(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	tests := map[string]string{
		`str(response.statusCode)`:        "200",
		`response.statusText`:             "200 OK",
		`response.url`:                    "https://example.test/p",
		`response.header("Content-Type")`: "application/json",
		`response.text()`:                 `{"ok":true}`,
	}
	for src, want := range tests {
		v := evalHost(t, eng, rt, src)
		if v.K != rts.VStr || v.S != want {
			t.Errorf("%s = %+v, want %q", src, v, want)
		}
	}

	for _, src := range []string{
		`status == 200`,
		`statusCode == 200`,
		`statusCode in [199, 200, 201]`,
		`response.statusCode in [200]`,
		`response.statusCode not in [400, 500]`,
		`statusText == "200 OK"`,
		`header("Content-Type") == "application/json"`,
		`text() == "{\"ok\":true}"`,
	} {
		v, err := eng.EvalAssertion(t.Context(), rt, src, testPos)
		if err != nil {
			t.Errorf("EvalAssertion(%s): %v", src, err)
			continue
		}
		if v.K != rts.VBool || !v.B {
			t.Errorf("EvalAssertion(%s) = %+v, want true", src, v)
		}
	}

	if _, err := eng.Eval(context.Background(), rt, "status", testPos); err == nil ||
		!strings.Contains(err.Error(), `undefined name "status"`) {
		t.Fatalf("status outside assertion: error = %v, want undefined", err)
	}
}

func TestEnvironmentMetadataHonorsStringLimitsThroughEveryAccess(t *testing.T) {
	eng := NewEngine(stdlib.New)
	eng.Core().Lim.MaxStr = 2
	rt := testRuntime(t)
	for _, src := range []string{`env.meta.name`, `env.meta["name"]`} {
		if _, err := eng.Eval(context.Background(), rt, src, testPos); err == nil ||
			!strings.Contains(err.Error(), "string too long") {
			t.Fatalf("%s: error = %v, want a string-limit error", src, err)
		}
	}
}

func TestRequestQueryReportsInvalidEscapingWhenRead(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	req, err := NewRequest("GET", "https://example.test/?bad=%zz", nil, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	rt.Request = req

	_, err = eng.Eval(context.Background(), rt, "request.query", testPos)
	if err == nil || !strings.Contains(err.Error(), "request query") {
		t.Fatalf("Eval error = %v, want an invalid-query error", err)
	}
}

func TestUnboundResponseRejectsReadsEvenBehindTry(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	rt.Response = nil
	for _, src := range []string{
		`response`,
		`not response`,
		`response == null`,
		`response ?? last`,
		`response.statusCode`,
		`response["statusCode"]`,
		`try response`,
		`(try response.statusCode).value`,
	} {
		_, err := eng.Eval(context.Background(), rt, src, testPos)
		if err == nil || !strings.Contains(err.Error(), "use last") {
			t.Fatalf("%s: error = %v, want an unbound-response error", src, err)
		}
	}
}

func TestCachedModuleRequestReadsAreEvaluationLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "request_view.rts")
	src := []byte("module requestView\nexport fn method() { return request.method }\n")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatalf("write module: %v", err)
	}

	eng := NewEngine(stdlib.New)
	base := testRuntime(t)
	base.BaseDir = dir
	base.Uses = []rts.Use{{Path: "request_view.rts"}}

	methods := []string{"GET", "POST", "PATCH", "DELETE"}
	var wg sync.WaitGroup
	errCh := make(chan error, len(methods)*8)
	for range 8 {
		for _, method := range methods {
			wg.Go(func() {
				rt := base
				req, err := NewRequest(method, "https://example.test", nil, nil)
				if err != nil {
					errCh <- err
					return
				}
				rt.Request = req
				v, err := eng.Eval(
					context.Background(),
					rt,
					"requestView.method()",
					testPos,
				)
				if err != nil {
					errCh <- err
					return
				}
				if v.K != rts.VStr || v.S != method {
					errCh <- &methodError{got: v.S, want: method}
				}
			})
		}
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

type methodError struct {
	got  string
	want string
}

func (e *methodError) Error() string {
	return "request method = " + e.got + ", want " + e.want
}

func TestReadOnlyRuntimeRefusesEveryWrite(t *testing.T) {
	eng := NewEngine(stdlib.New)
	rt := testRuntime(t)
	if rt.Mutator != nil {
		t.Fatal("testRuntime is not read-only")
	}
	tests := map[string]string{
		`request.setURL("https://example.test")`: "request is read-only",
		`request.setHeader("X-Test", "one")`:     "request is read-only",
		`request.setBody("body")`:                "request is read-only",
		`vars.set("token", "b")`:                 "vars is read-only",
		`vars.global.set("shared", "no")`:        "vars.global is read-only",
		`vars.global.delete("shared")`:           "vars.global is read-only",
	}
	for src, want := range tests {
		_, err := eng.Eval(context.Background(), rt, src, testPos)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Eval(%s) error = %v, want one holding %q", src, err, want)
		}
	}
}

func TestPreparedScopeValidatesLayersAndRebindsVars(t *testing.T) {
	rejects := map[string]func() error{
		"env": func() error {
			_, err := PrepareScope(ScopeInput{Env: map[string]string{"A": "1", " a ": "2"}})
			return err
		},
		"groups": func() error {
			_, err := PrepareScope(ScopeInput{Groups: map[string]string{"R": "1", "r": "2"}})
			return err
		},
		"globals": func() error {
			_, err := PrepareScope(ScopeInput{Globals: map[string]string{"G": "1", "g": "2"}})
			return err
		},
		"vars": func() error {
			p, err := PrepareScope(ScopeInput{})
			if err != nil {
				return err
			}
			_, err = p.BindVars(map[string]string{"V": "1", " v ": "2"})
			return err
		},
	}
	for layer, reject := range rejects {
		if err := reject(); err == nil || !strings.Contains(err.Error(), "same name") {
			t.Errorf("%s error = %v, want a collision", layer, err)
		}
	}

	// Prepared layers are validated before vars, so the global error wins when
	// both inputs are invalid.
	_, err := NewScope(
		ScopeInput{Globals: map[string]string{"G": "1", "g": "2"}},
		map[string]string{"V": "1", "v": "2"},
	)
	if err == nil || !strings.Contains(err.Error(), "vars.global") {
		t.Fatalf("NewScope error = %v, want the globals collision", err)
	}

	prep, err := PrepareScope(ScopeInput{
		EnvName: "dev",
		Env:     map[string]string{"NAME": "service"},
		Groups:  map[string]string{"region": "eu"},
		Globals: map[string]string{"shared": "yes"},
	})
	if err != nil {
		t.Fatalf("PrepareScope: %v", err)
	}
	eng := NewEngine(stdlib.New)
	for _, want := range []string{"first", "second"} {
		scope, err := prep.BindVars(map[string]string{"token": want})
		if err != nil {
			t.Fatalf("BindVars: %v", err)
		}
		rt := Runtime{Scope: scope}
		if v := evalHost(t, eng, rt, "vars.token"); v.S != want {
			t.Errorf("vars.token = %+v, want %q", v, want)
		}
		if v := evalHost(t, eng, rt, `env.NAME + env.meta.name + vars.global.shared`); v.S != "servicedevyes" {
			t.Errorf("prepared layers = %+v, want them intact", v)
		}
	}
}

// The shorthand is bound before the assertion runs, so it cannot apply the
// string limit the way response.statusText does when read: an eager check
// would fail assertions that never mention statusText. This pins that choice.
func TestAssertionShorthandBindsStatusTextAsGiven(t *testing.T) {
	eng := NewEngine(stdlib.New)
	eng.Core().Lim = rts.Limits{MaxStr: 2, MaxList: 100, MaxDict: 100, MaxSteps: 1000, MaxCall: 10}
	resp, err := NewResponse("long", 200, nil, nil, "")
	if err != nil {
		t.Fatalf("NewResponse: %v", err)
	}
	rt := Runtime{Response: resp, Last: resp}

	// An assertion that never reads statusText is unaffected by its length.
	for _, src := range []string{"statusCode == 200", "response.statusCode == 200"} {
		v, err := eng.EvalAssertion(context.Background(), rt, src, testPos)
		if err != nil || !v.B {
			t.Errorf("EvalAssertion(%s) = %+v, err = %v, want true", src, v, err)
		}
	}

	// The shorthand carries the host status as given.
	v, err := eng.EvalAssertion(context.Background(), rt, "statusText", testPos)
	if err != nil || v.S != "long" {
		t.Errorf("EvalAssertion(statusText) = %+v, err = %v, want \"long\"", v, err)
	}

	// Reading it through the response applies the limit.
	if _, err := eng.EvalAssertion(context.Background(), rt, "response.statusText", testPos); err == nil ||
		!strings.Contains(err.Error(), "string too long") {
		t.Errorf("EvalAssertion(response.statusText) error = %v, want the string limit", err)
	}
}
