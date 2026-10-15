package rtshost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/stdlib"
	"github.com/unkn0wn-root/resterm/internal/scriptapi"
)

const testDoc = "dir/api.http"

func testBlock(name string, line int, body ...string) restfile.ScriptBlock {
	b := restfile.ScriptBlock{
		Kind:       restfile.ScriptKindTest,
		Lang:       restfile.ScriptLangRTS,
		Name:       name,
		Body:       strings.Join(body, "\n"),
		SourcePath: testDoc,
	}
	for i := range body {
		b.Lines = append(b.Lines, restfile.ScriptLine{Line: line + i, Col: 3})
	}
	return b
}

func runTests(t *testing.T, rt Runtime, blocks ...restfile.ScriptBlock) ([]scriptapi.TestResult, error) {
	t.Helper()
	return RunTests(t.Context(), NewEngine(stdlib.New), Tests{
		Doc:     &restfile.Document{Path: testDoc},
		Scripts: blocks,
		Runtime: rt,
	})
}

type result struct {
	name   string
	passed bool
	msg    string
}

func results(rs []scriptapi.TestResult) []result {
	out := make([]result, len(rs))
	for i, r := range rs {
		out[i] = result{r.Name, r.Passed, r.Message}
	}
	return out
}

func writeModule(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRunTestsReportsOneResultPerBlock(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "checks.rts", `module checks
export fn stop(t, msg) {
  t.fatal(msg)
}
export fn each(t, items) {
  for let _, it range items {
    t.error("bad", it)
  }
}
`)
	rt := testRuntime(t)
	rt.BaseDir = dir
	rt.Uses = []rts.Use{{Path: "checks.rts"}}

	tests := []struct {
		name  string
		block restfile.ScriptBlock
		want  result
	}{
		{
			"no calls",
			testBlock("", 2, "let ok = response.json().ok"),
			result{"@rts test", true, ""},
		},
		{
			"named",
			testBlock("orders are valid", 2, `if typeof(t) != "t" {`, "  t.error(typeof(t))", "}"),
			result{"orders are valid", true, ""},
		},
		{
			"error keeps going",
			testBlock("", 2, `t.error("first")`, `t.error("second")`),
			result{"@rts test", false, "api.http:2: first\napi.http:3: second"},
		},
		{
			"fatal stops",
			testBlock("", 2, `t.fatal("stop")`, `t.error("after")`),
			result{"@rts test", false, "api.http:2: stop"},
		},
		{
			"try does not catch fatal",
			testBlock("", 2, `let r = try t.fatal("stop")`, `t.error("after")`),
			result{"@rts test", false, "api.http:2: stop"},
		},
		{
			"fatal in a module",
			testBlock("", 2, `checks.stop(t, "bad")`, `t.error("after")`),
			result{"@rts test", false, "checks.rts:3: bad"},
		},
		{
			"module helper",
			testBlock("", 2, `checks.each(t, [1, "x"])`),
			result{"@rts test", false, "checks.rts:7: bad 1\nchecks.rts:7: bad x"},
		},
		{
			"fatal in a list callback",
			testBlock("", 2,
				"fn f(x) {",
				`  t.fatal("item", x)`,
				"}",
				"let r = try rts.list.map([1, 2], f)",
				`t.error("after")`,
			),
			result{"@rts test", false, "api.http:3: item 1"},
		},
		{
			"t is a value",
			testBlock("", 2, "let tt = t", `tt.error("x")`),
			result{"@rts test", false, "api.http:3: x"},
		},
		{
			"arguments",
			testBlock("", 2, `t.error("got", 2, "want", 3, [1], true)`),
			result{"@rts test", false, "api.http:2: got 2 want 3 [1] true"},
		},
		{
			"no arguments",
			testBlock("", 2, "t.error()", "t.fatal()"),
			result{"@rts test", false, "api.http:2\napi.http:3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runTests(t, rt, tt.block)
			if err != nil {
				t.Fatalf("RunTests: %v", err)
			}
			if want := []result{tt.want}; !slices.Equal(results(got), want) {
				t.Fatalf("results = %+v, want %+v", results(got), want)
			}
		})
	}
}

func TestRunTestsReportsJSONDiff(t *testing.T) {
	rt := testRuntime(t)
	resp, err := NewResponse("200 OK", 200, nil, []byte(`{"user":{"id":7,"tags":["a","b"]},"extra":true}`), "")
	if err != nil {
		t.Fatalf("NewResponse: %v", err)
	}
	rt.Response = resp
	got, err := runTests(t, rt, testBlock("user", 2,
		`let want = {user: {id: 8, tags: ["a"]}, "display.name": "Ada"}`,
		"for let _, d range json.diff(response.json(), want) {",
		"  t.error(d)",
		"}",
		`if json.equal(response.json().user.tags, ["a", "b"]) {`,
		`  t.error("tags match")`,
		"}",
	))
	if err != nil {
		t.Fatalf("RunTests: %v", err)
	}
	want := []result{{"user", false, strings.Join([]string{
		`api.http:4: $["display.name"]: missing, want "Ada"`,
		"api.http:4: $.extra: unexpected true",
		"api.http:4: $.user.id: got 7, want 8",
		`api.http:4: $.user.tags[1]: unexpected "b"`,
		"api.http:7: tags match",
	}, "\n")}}
	if !slices.Equal(results(got), want) {
		t.Fatalf("results = %+v, want %+v", results(got), want)
	}
}

func TestRunTestsSkipsOtherBlocks(t *testing.T) {
	got, err := runTests(t, testRuntime(t),
		restfile.ScriptBlock{Kind: restfile.ScriptKindTest, Lang: restfile.ScriptLangJS, Body: "not rts"},
		restfile.ScriptBlock{Kind: restfile.ScriptKindPreRequest, Lang: restfile.ScriptLangRTS, Body: "t.error()"},
		testBlock("blank", 2, "  "),
		testBlock("", 3, `t.error("ran")`),
	)
	if err != nil {
		t.Fatalf("RunTests: %v", err)
	}
	if want := []result{{"@rts test", false, "api.http:3: ran"}}; !slices.Equal(results(got), want) {
		t.Fatalf("results = %+v, want %+v", results(got), want)
	}
}

func TestRunTestsErrorEndsOnlyItsBlock(t *testing.T) {
	doc := &restfile.Document{
		Path: testDoc,
		Raw:  []byte("GET https://example.test\n> t.error(\"first\")\n> let x = missing.value\n"),
	}
	blocks := []restfile.ScriptBlock{
		testBlock("broken", 2, `t.error("first")`, "let x = missing.value"),
		testBlock("loop", 4, "for {", "}"),
		testBlock("read only", 6, `vars.set("token", "b")`),
		testBlock("", 7, `vars.global.set("shared", "no")`),
		testBlock("", 8, `request.setHeader("X-Test", "one")`),
		testBlock("", 9, "let t = 1"),
		{Kind: restfile.ScriptKindTest, Lang: restfile.ScriptLangRTS, Name: "file", FilePath: "missing.rts"},
		testBlock("last", 10, `t.error("ran")`),
	}
	rt := testRuntime(t)
	rt.BaseDir = t.TempDir()
	got, err := RunTests(t.Context(), NewEngine(stdlib.New), Tests{
		Doc:     doc,
		Scripts: blocks,
		Runtime: rt,
	})
	want := []result{
		{"broken", false, "api.http:2: first"},
		{"loop", false, ""},
		{"read only", false, ""},
		{"@rts test", false, ""},
		{"@rts test", false, ""},
		{"@rts test", false, ""},
		{"file", false, ""},
		{"last", false, "api.http:10: ran"},
	}
	if !slices.Equal(results(got), want) {
		t.Fatalf("results = %+v, want %+v", results(got), want)
	}
	for _, msg := range []string{
		`undefined name "missing"`,
		"step limit exceeded",
		"vars is read-only",
		"vars.global is read-only",
		"request is read-only",
		`name already defined: "t"`,
		"rts test script 7:",
	} {
		if err == nil || !strings.Contains(err.Error(), msg) {
			t.Fatalf("error = %v, want one holding %q", err, msg)
		}
	}

	rep := diag.ReportOf(err)
	if len(rep.Items) != 7 {
		t.Fatalf("report has %d items, want one per failed block: %+v", len(rep.Items), rep.Items)
	}
	for i, site := range []string{"@rts test broken", "@rts test loop", `@rts test "read only"`, "@rts test"} {
		frames := rep.Items[i].Frames
		if len(frames) == 0 || frames[0].Name != site {
			t.Fatalf("item %d frames = %+v, want %s first", i, frames, site)
		}
	}
	first := rep.Items[0]
	if first.Span.Start.Line != 3 || first.Span.Start.Path != testDoc {
		t.Fatalf("first error at %+v, want %s:3", first.Span.Start, testDoc)
	}
	if out := diag.Render(err); !strings.Contains(out, `   3 | > let x = missing.value`) {
		t.Fatalf("rendered error has no source line:\n%s", out)
	}
}

func TestRunTestsBindsTOverLocals(t *testing.T) {
	rt := testRuntime(t)
	rt.Locals = rts.Local("t", rts.Str("item"))
	got, err := runTests(t, rt, testBlock("", 2, `t.error("x")`))
	if err != nil {
		t.Fatalf("RunTests: %v", err)
	}
	if want := []result{{"@rts test", false, "api.http:2: x"}}; !slices.Equal(results(got), want) {
		t.Fatalf("results = %+v, want %+v", results(got), want)
	}
}

func TestRunTestsRejectsTFromAnEndedBlock(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "keep.rts", `module keep
let saved = null
let fatal = null
let report = null
export fn hold(t) {
  saved = t
  fatal = t.fatal
  report = t.error
}
export fn useT() {
  saved.fatal("stale")
}
export fn useFatal() {
  fatal("stale")
}
export fn useError() {
  report("stale")
}
export fn tryT() {
  let r = try saved.fatal("stale")
}
export fn tryFatal() {
  let r = try fatal("stale")
}
export fn tryError() {
  let r = try report("stale")
}
`)
	const ended = "t belongs to a test block that has ended"
	for _, use := range []string{"useT", "useFatal", "useError", "tryT", "tryFatal", "tryError"} {
		t.Run(use, func(t *testing.T) {
			eng := NewEngine(stdlib.New)
			rt := testRuntime(t)
			rt.BaseDir = dir
			rt.Uses = []rts.Use{{Path: "keep.rts"}}
			got, err := RunTests(t.Context(), eng, Tests{
				Doc: &restfile.Document{Path: testDoc},
				Scripts: []restfile.ScriptBlock{
					testBlock("", 2, "keep.hold(t)"),
					testBlock("", 3, "keep."+use+"()", `t.error("ran")`),
				},
				Runtime: rt,
			})
			if err == nil || !strings.Contains(err.Error(), ended) {
				t.Fatalf("error = %v, want the ended block", err)
			}
			want := []result{{"@rts test", true, ""}, {"@rts test", false, ""}}
			if !slices.Equal(results(got), want) {
				t.Fatalf("results = %+v, want %+v", results(got), want)
			}

			err = RunPreRequest(t.Context(), eng, PreRequest{
				Scripts: []restfile.ScriptBlock{
					{Kind: restfile.ScriptKindPreRequest, Lang: restfile.ScriptLangRTS, Body: "keep." + use + "()"},
				},
				BaseDir: dir,
				Runtime: func() (Runtime, error) { return rt, nil },
			})
			if err == nil || !strings.Contains(err.Error(), ended) || errors.Is(err, rts.ErrHalt) {
				t.Fatalf("pre-request error = %v, want the ended block", err)
			}
		})
	}
}

func TestPreRequestHasNoT(t *testing.T) {
	run := func(body string) error {
		return RunPreRequest(t.Context(), NewEngine(stdlib.New), PreRequest{
			Scripts: []restfile.ScriptBlock{
				{Kind: restfile.ScriptKindPreRequest, Lang: restfile.ScriptLangRTS, Body: body},
			},
			Runtime: func() (Runtime, error) { return testRuntime(t), nil },
		})
	}
	if err := run("let t = 1"); err != nil {
		t.Fatalf("let t in a pre-request block: %v", err)
	}
	if err := run(`t.error("x")`); err == nil || !strings.Contains(err.Error(), `undefined name "t"`) {
		t.Fatalf("t in a pre-request block: error = %v, want it undefined", err)
	}
}

func TestRunTestsStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := RunTests(ctx, NewEngine(stdlib.New), Tests{
		Scripts: []restfile.ScriptBlock{testBlock("", 2, "let x = 1")},
		Runtime: testRuntime(t),
	})
	if !errors.Is(err, context.Canceled) || len(got) != 0 {
		t.Fatalf("results = %+v, error = %v, want none and context.Canceled", got, err)
	}
}

func stepRuntime(t *testing.T) Runtime {
	t.Helper()
	dir := t.TempDir()
	writeModule(t, dir, "work.rts", `module work
export fn spin(n) {
  for let i = 0; i < n; i = i + 1 {
  }
}
`)
	rt := testRuntime(t)
	rt.BaseDir = dir
	rt.Uses = []rts.Use{{Path: "work.rts"}}
	return rt
}

func TestBlocksGetBlockStepLimit(t *testing.T) {
	rt := stepRuntime(t)
	const fits, over = "work.spin(6250)", "work.spin(18750)"

	pre := func(body string) error {
		return RunPreRequest(t.Context(), NewEngine(stdlib.New), PreRequest{
			Scripts: []restfile.ScriptBlock{
				{Kind: restfile.ScriptKindPreRequest, Lang: restfile.ScriptLangRTS, Body: body},
			},
			Runtime: func() (Runtime, error) { return rt, nil },
		})
	}
	if err := pre(fits); err != nil {
		t.Fatalf("pre-request block of 50,000 steps: %v", err)
	}
	if err := pre(over); err == nil || !strings.Contains(err.Error(), "step limit exceeded") {
		t.Fatalf("pre-request block of 150,000 steps: error = %v, want the step limit", err)
	}

	got, err := runTests(t, rt,
		testBlock("fits", 2, fits),
		testBlock("over", 3, over),
		testBlock("next", 4, `t.error("ran")`),
	)
	want := []result{{"fits", true, ""}, {"over", false, ""}, {"next", false, "api.http:4: ran"}}
	if !slices.Equal(results(got), want) {
		t.Fatalf("results = %+v, want %+v", results(got), want)
	}
	if err == nil || !strings.Contains(err.Error(), "step limit exceeded") {
		t.Fatalf("error = %v, want the step limit", err)
	}
}

func TestExpressionsKeepEngineStepLimit(t *testing.T) {
	rt := stepRuntime(t)
	eng := NewEngine(stdlib.New)
	const spin = "work.spin(2500)"
	if _, err := eng.EvalAssertion(t.Context(), rt, spin+" == null", testPos); err == nil ||
		!strings.Contains(err.Error(), "step limit exceeded") {
		t.Fatalf("assertion of 20,000 steps: error = %v, want the step limit", err)
	}
	if _, err := eng.EvalStr(t.Context(), rt, spin, testPos); err == nil ||
		!strings.Contains(err.Error(), "step limit exceeded") {
		t.Fatalf("template of 20,000 steps: error = %v, want the step limit", err)
	}
}

func TestModuleLoadKeepsEngineStepLimit(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir, "heavy.rts", "module heavy\nfor let i = 0; i < 6250; i = i + 1 {\n}\n")
	rt := testRuntime(t)
	rt.BaseDir = dir
	rt.Uses = []rts.Use{{Path: "heavy.rts"}}
	got, err := runTests(t, rt, testBlock("", 2, "let x = 1"))
	if want := []result{{"@rts test", false, ""}}; !slices.Equal(results(got), want) {
		t.Fatalf("results = %+v, want %+v", results(got), want)
	}
	if err == nil || !strings.Contains(err.Error(), "step limit exceeded") {
		t.Fatalf("error = %v, want the step limit", err)
	}
}
