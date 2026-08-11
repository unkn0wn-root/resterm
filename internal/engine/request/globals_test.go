package request

import (
	"net/http"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

var globalReaders = map[string]string{
	"X-Template": "{{token}}",
	"X-Vars":     `{{= vars.get("token") }}`,
	"X-Global":   `{{= vars.global.get("token") ?? "missing" }}`,
}

func globalReaderRequest(scripts ...restfile.ScriptBlock) *restfile.Request {
	req := &restfile.Request{
		Method:   http.MethodGet,
		URL:      "http://example.test",
		Headers:  http.Header{},
		Metadata: restfile.RequestMetadata{Scripts: scripts},
	}
	for name, tpl := range globalReaders {
		req.Headers.Set(name, tpl)
	}
	return req
}

func assertGlobalReaders(t *testing.T, header http.Header, want string) {
	t.Helper()

	for name := range globalReaders {
		if got := header.Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func docWithGlobals(vs ...restfile.Variable) *restfile.Document {
	for i := range vs {
		vs[i].Scope = directive.ScopeGlobal
	}
	return &restfile.Document{Path: "globals.http", Globals: vs}
}

func TestDocumentGlobalCaseVariantsResolveToTheLastDeclaration(t *testing.T) {
	tests := []struct {
		name  string
		first string
		last  string
	}{
		{name: "lower then upper", first: "token", last: "TOKEN"},
		{name: "upper then lower", first: "TOKEN", last: "token"},
		{name: "mixed then lower", first: "ToKeN", last: "token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := docWithGlobals(
				restfile.Variable{Name: tt.first, Value: "first"},
				restfile.Variable{Name: tt.last, Value: "second"},
			)

			sent := sendRequest(t, doc, globalReaderRequest(), testEnv(""), ExecOptions{})
			assertGlobalReaders(t, sent.wire.Header, "second")
		})
	}
}

func TestRTSGlobalCaseVariantsResolveToTheLastWrite(t *testing.T) {
	req := globalReaderRequest(rtsPre(`vars.global.set("token", "first", false)
vars.global.set("TOKEN", "second", false)`))

	sent := sendRequest(t, nil, req, testEnv(""), ExecOptions{})
	assertGlobalReaders(t, sent.wire.Header, "second")
}

func TestJSGlobalCaseVariantsResolveToTheLastWrite(t *testing.T) {
	req := globalReaderRequest(jsPre(`vars.global.set("token", "first");
		vars.global.set("TOKEN", "second");`))

	sent := sendRequest(t, nil, req, testEnv(""), ExecOptions{})
	assertGlobalReaders(t, sent.wire.Header, "second")
}

func TestRTSGlobalDeleteAndSetFollowCallOrder(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name: "delete after set",
			script: `vars.global.set("token", "written", false)
vars.global.delete("TOKEN")`,
			want: "missing",
		},
		{
			name: "set after delete",
			script: `vars.global.delete("token")
vars.global.set("TOKEN", "rewritten", false)`,
			want: "rewritten",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := globalReaderRequest(rtsPre(tt.script))
			req.Headers.Del("X-Template")
			req.Headers.Set("X-Vars", `{{= vars.get("token") ?? "missing" }}`)

			eng, st := newStubEngine(t)
			env := envWith(t, "dev", nil)
			eng.rt.Globals().Set(env.Scope(), "token", "seed", false)
			sent := sendWith(t, eng, st, nil, req, env, ExecOptions{})

			for _, name := range []string{"X-Vars", "X-Global"} {
				if got := sent.wire.Header.Get(name); got != tt.want {
					t.Fatalf("%s = %q, want %q", name, got, tt.want)
				}
			}
			snap := eng.rt.Globals().Snapshot(env.Scope())
			if tt.want == "missing" {
				if len(snap) != 0 {
					t.Fatalf("store = %#v, want the global deleted", snap)
				}
				return
			}
			if len(snap) != 1 {
				t.Fatalf("store = %#v, want one entry", snap)
			}
			if got := snap["token"].Value; got != tt.want {
				t.Fatalf("store token = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPreviewReadsGlobalsScriptsSetInMemory(t *testing.T) {
	tests := []struct {
		name   string
		script restfile.ScriptBlock
	}{
		{name: "rts", script: rtsPre(`vars.global.set("token", "in-memory", false)`)},
		{name: "js", script: jsPre(`vars.global.set("token", "in-memory");`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := New(engcfg.Config{}, nil)
			env := envWith(t, "dev", nil)

			res, err := eng.ExecuteWith(nil, globalReaderRequest(tt.script), env, ExecOptions{
				Mode: ExecModePreview,
			})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if res.Err != nil {
				t.Fatalf("ExecuteWith() result error = %v", res.Err)
			}
			if res.Explain == nil || res.Explain.Final == nil {
				t.Fatalf("expected a prepared preview, got %#v", res.Explain)
			}
			assertGlobalReaders(t, previewHeaders(res.Explain.Final.Headers), "in-memory")
			if snap := eng.rt.Globals().Snapshot(env.Scope()); len(snap) != 0 {
				t.Fatalf("store = %#v, want a preview to persist nothing", snap)
			}
		})
	}
}

func TestEveryRTSDirectiveReadsRunGlobals(t *testing.T) {
	doc := docWithGlobals(restfile.Variable{Name: "token", Value: "doc-token"})
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			When: &restfile.ConditionSpec{Expression: `vars.global.get("token") == "doc-token"`},
			Applies: []restfile.ApplySpec{{
				Expression: `{headers: {"X-Applied": vars.global.get("token")}}`,
			}},
			Captures: []restfile.CaptureSpec{{
				Scope:      directive.ScopeRequest,
				Name:       "captured",
				Expression: `vars.global.get("token")`,
				Mode:       restfile.CaptureExprModeRTS,
			}},
			Asserts: []restfile.AssertSpec{{
				Expression: `vars.global.get("token") == "doc-token"`,
				Message:    "assert reads the run's globals",
			}},
		},
	}

	eng, st := newStubEngine(t)
	res, err := eng.ExecuteWith(doc, req, testEnv(""), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err != nil {
		t.Fatalf("ExecuteWith() result error = %v", res.Err)
	}
	if res.Skipped {
		t.Fatalf("@when skipped the request: %s", res.SkipReason)
	}
	if got := st.wire.Header.Get("X-Applied"); got != "doc-token" {
		t.Fatalf("@apply header = %q, want %q", got, "doc-token")
	}
	for _, tc := range res.Tests {
		if !tc.Passed {
			t.Fatalf("@assert failed: %s", tc.Name)
		}
	}
	if len(res.Tests) != 1 {
		t.Fatalf("res.Tests = %#v, want the one assert", res.Tests)
	}

	captured := ""
	for _, v := range res.Executed.Variables {
		if vars.NameKey(v.Name) == "captured" {
			captured = v.Value
		}
	}
	if captured != "doc-token" {
		t.Fatalf("@capture value = %q, want %q", captured, "doc-token")
	}
}

func previewHeaders(hs []xplain.Header) http.Header {
	out := make(http.Header, len(hs))
	for _, h := range hs {
		out.Add(h.Name, h.Value)
	}
	return out
}

func TestGlobalCaptureReachesLaterReadersInTheSameRequest(t *testing.T) {
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{{
				Scope:      directive.ScopeGlobal,
				Name:       "token",
				Expression: `"captured"`,
				Mode:       restfile.CaptureExprModeRTS,
			}},
			Asserts: []restfile.AssertSpec{
				{Expression: `vars.global.get("token") == "captured"`},
				{Expression: `vars.get("token") == "captured"`},
			},
			Scripts: []restfile.ScriptBlock{{
				Kind: "test",
				Body: `client.test("global capture", function () {
					tests.assert(vars.global.get("token") === "captured", "test script sees the capture");
				});`,
			}},
		},
	}

	eng, st := newStubEngine(t)
	env := envWith(t, "dev", nil)
	res, err := eng.ExecuteWith(nil, req, env, ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err != nil {
		t.Fatalf("ExecuteWith() result error = %v", res.Err)
	}
	if res.ScriptErr != nil {
		t.Fatalf("ExecuteWith() script error = %v", res.ScriptErr)
	}
	seen := make(map[string]bool, len(res.Tests))
	for _, tc := range res.Tests {
		if !tc.Passed {
			t.Fatalf("%s failed: %s", tc.Name, tc.Message)
		}
		seen[tc.Name] = true
	}
	for _, name := range []string{
		`vars.global.get("token") == "captured"`,
		`vars.get("token") == "captured"`,
		"test script sees the capture",
	} {
		if !seen[name] {
			t.Fatalf("res.Tests = %#v, want a result named %q", res.Tests, name)
		}
	}
	if got := eng.rt.Globals().Snapshot(env.Scope())["token"].Value; got != "captured" {
		t.Fatalf("store token = %q, want %q", got, "captured")
	}
	if st.wire == nil {
		t.Fatal("request was never sent")
	}
}

func TestGlobalCaptureIsVisibleToLaterCaptures(t *testing.T) {
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			Captures: []restfile.CaptureSpec{
				{
					Scope:      directive.ScopeGlobal,
					Name:       "first",
					Expression: `"one"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeGlobal,
					Name:       "second",
					Expression: `vars.global.get("first") + "-two"`,
					Mode:       restfile.CaptureExprModeRTS,
				},
				{
					Scope:      directive.ScopeGlobal,
					Name:       "third",
					Expression: `{{= vars.global.get("second") }}-three`,
					Mode:       restfile.CaptureExprModeTemplate,
				},
			},
		},
	}

	eng, _ := newStubEngine(t)
	env := envWith(t, "dev", nil)
	if _, err := eng.ExecuteWith(nil, req, env, ExecOptions{}); err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}

	snap := eng.rt.Globals().Snapshot(env.Scope())
	for name, want := range map[string]string{
		"first":  "one",
		"second": "one-two",
		"third":  "one-two-three",
	} {
		if got := snap[name].Value; got != want {
			t.Fatalf("global %s = %q, want %q", name, got, want)
		}
	}
}
