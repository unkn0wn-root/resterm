package request

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rivo/uniseg"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/engine"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const leakedSecret = "s3cr3t-value"

func secretLeakRequest(scripts ...restfile.ScriptBlock) *restfile.Request {
	return &restfile.Request{
		Method:   http.MethodGet,
		URL:      "http://example.test",
		Metadata: restfile.RequestMetadata{Scripts: scripts},
	}
}

func assertExplainMasksSecret(t *testing.T, rep *xplain.Report) {
	t.Helper()

	if rep == nil {
		t.Fatal("expected an explain report")
	}
	rendered := fmt.Sprintf("%+v", *rep)
	if rep.Final != nil {
		rendered += fmt.Sprintf("\n%+v", *rep.Final)
	}
	if strings.Contains(rendered, leakedSecret) {
		t.Fatalf("explain report contains the plaintext secret:\n%s", rendered)
	}
}

func assertSecretStaysRedactable(t *testing.T, res engine.RequestResult) {
	t.Helper()

	if !slices.Contains(res.RuntimeSecrets, leakedSecret) {
		t.Fatalf("RuntimeSecrets = %#v, want the exposed secret recorded", res.RuntimeSecrets)
	}
	assertExplainMasksSecret(t, res.Explain)

	rendered := map[string]string{
		"Err":              errText(res.Err),
		"Err diagnostic":   errRender(res.Err),
		"ScriptErr":        errText(res.ScriptErr),
		"ScriptErr report": errRender(res.ScriptErr),
	}
	for _, tc := range res.Tests {
		rendered["test "+tc.Name] = tc.Name + " " + tc.Message
	}
	for field, text := range rendered {
		if strings.Contains(text, leakedSecret) {
			t.Fatalf("%s contains the plaintext secret: %s", field, text)
		}
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func errRender(err error) string {
	if err == nil {
		return ""
	}
	return diag.Render(err)
}

func TestExplainMasksDeletedSecretGlobal(t *testing.T) {
	scripts := map[string]restfile.ScriptBlock{
		"rts": rtsPre(`vars.global.set("token", "` + leakedSecret + `", true)
request.setHeader("X-Leak", vars.global.get("token"))
vars.global.delete("token")`),
		"js": jsPre(`vars.global.set("token", "` + leakedSecret + `", true);
request.setHeader("X-Leak", vars.global.get("token"));
vars.global.delete("token");`),
	}

	for lang, script := range scripts {
		t.Run(lang+" send", func(t *testing.T) {
			eng, st := newStubEngine(t)
			res, err := eng.ExecuteWith(nil, secretLeakRequest(script), testEnv(""), ExecOptions{})
			if err != nil || res.Err != nil {
				t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
			}
			if st.wire == nil {
				t.Fatal("request was never sent")
			}
			if got := st.wire.Header.Get("X-Leak"); got != leakedSecret {
				t.Fatalf("X-Leak = %q, want the secret on the wire", got)
			}
			assertSecretStaysRedactable(t, res)
		})

		t.Run(lang+" preview", func(t *testing.T) {
			eng, _ := newStubEngine(t)
			res, err := eng.ExecuteWith(nil, secretLeakRequest(script), testEnv(""), ExecOptions{
				Mode: ExecModePreview,
			})
			if err != nil || res.Err != nil {
				t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
			}
			assertExplainMasksSecret(t, res.Explain)
		})
	}
}

func TestExplainMasksSecretGlobalDeletedFromStorage(t *testing.T) {
	scripts := map[string]restfile.ScriptBlock{
		"rts": rtsPre(`request.setHeader("X-Leak", vars.global.get("token"))
vars.global.delete("token")`),
		"js": jsPre(`request.setHeader("X-Leak", vars.global.get("token"));
vars.global.delete("token");`),
	}

	for lang, script := range scripts {
		t.Run(lang, func(t *testing.T) {
			eng, st := newStubEngine(t)
			env := testEnv("")
			eng.rt.Globals().Set(env.Scope(), "token", leakedSecret, true)

			res, err := eng.ExecuteWith(nil, secretLeakRequest(script), env, ExecOptions{})
			if err != nil || res.Err != nil {
				t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
			}
			if st.wire == nil || st.wire.Header.Get("X-Leak") != leakedSecret {
				t.Fatalf("the script never copied the secret onto the wire: %v", st.wire)
			}
			assertSecretStaysRedactable(t, res)
		})
	}
}

func TestExplainMasksSecretGlobalDeletedByLaterScript(t *testing.T) {
	req := secretLeakRequest(
		rtsPre(`vars.global.set("token", "`+leakedSecret+`", true)
request.setHeader("X-Leak", vars.global.get("token"))`),
		jsPre(`vars.global.delete("token");`),
	)

	eng, _ := newStubEngine(t)
	res, err := eng.ExecuteWith(nil, req, testEnv(""), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	assertSecretStaysRedactable(t, res)
}

func TestExplainMasksSecretGlobalOverwrittenAsPublic(t *testing.T) {
	req := secretLeakRequest(rtsPre(`vars.global.set("token", "` + leakedSecret + `", true)
request.setHeader("X-Leak", vars.global.get("token"))
vars.global.set("token", "public", false)`))

	eng, _ := newStubEngine(t)
	res, err := eng.ExecuteWith(nil, req, testEnv(""), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	assertSecretStaysRedactable(t, res)
}

func TestFailedProducerKeepsSecretRedactable(t *testing.T) {
	capture := func(scope directive.Scope, name, expr string, secret bool) restfile.CaptureSpec {
		return restfile.CaptureSpec{
			Scope:      scope,
			Name:       name,
			Expression: expr,
			Mode:       restfile.CaptureExprModeRTS,
			Secret:     secret,
		}
	}

	cases := map[string]*restfile.Request{
		"rts pre-request": secretLeakRequest(rtsPre(`vars.global.set("token", "` + leakedSecret + `", true)
fail(vars.global.get("token"))`)),
		"js pre-request": secretLeakRequest(jsPre(`vars.global.set("token", "` + leakedSecret + `", true);
throw new Error(vars.global.get("token"));`)),
		"js test": secretLeakRequest(restfile.ScriptBlock{
			Kind: "test",
			Lang: "js",
			Body: `vars.global.set("token", "` + leakedSecret + `", true);
throw new Error(vars.global.get("token"));`,
		}),
		"capture batch": {
			Method: http.MethodGet,
			URL:    "http://example.test",
			Metadata: restfile.RequestMetadata{
				Captures: []restfile.CaptureSpec{
					capture(directive.ScopeGlobal, "token", `"`+leakedSecret+`"`, true),
					capture(directive.ScopeRequest, "boom", `fail(vars.get("token"))`, false),
				},
			},
		},
	}

	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			eng, _ := newStubEngine(t)
			res, err := eng.ExecuteWith(nil, req, testEnv(""), ExecOptions{})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if res.Err == nil && res.ScriptErr == nil {
				t.Fatal("expected the producer to fail")
			}
			assertSecretStaysRedactable(t, res)
		})
	}
}

func TestFailedProducerKeepsStoredSecretRedactable(t *testing.T) {
	scripts := map[string]restfile.ScriptBlock{
		"rts": rtsPre(`fail(vars.global.get("token"))`),
		"js":  jsPre(`throw new Error(vars.global.get("token"));`),
	}

	for lang, script := range scripts {
		t.Run(lang, func(t *testing.T) {
			eng, _ := newStubEngine(t)
			env := testEnv("")
			eng.rt.Globals().Set(env.Scope(), "token", leakedSecret, true)

			res, err := eng.ExecuteWith(nil, secretLeakRequest(script), env, ExecOptions{})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if res.Err == nil {
				t.Fatal("expected the script to fail")
			}
			assertSecretStaysRedactable(t, res)
		})
	}
}

func TestRTSTestBlockKeepsStoredSecretRedactable(t *testing.T) {
	eng, _ := newStubEngine(t)
	env := testEnv("")
	eng.rt.Globals().Set(env.Scope(), "token", leakedSecret, true)
	req := secretLeakRequest(restfile.ScriptBlock{
		Kind: "test",
		Lang: "rts",
		Body: `t.error("token", vars.global.get("token"))
fail(vars.global.get("token"))`,
	})

	res, err := eng.ExecuteWith(nil, req, env, ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if len(res.Tests) != 1 || res.Tests[0].Passed || !strings.Contains(res.Tests[0].Message, "token "+redact.Mask) {
		t.Fatalf("Tests = %#v, want one failed test that read the secret", res.Tests)
	}
	if res.ScriptErr == nil {
		t.Fatal("expected the block to fail")
	}
	assertSecretStaysRedactable(t, res)
}

func TestFailedAssertKeepsSecretRedactable(t *testing.T) {
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{{
				Kind: "test",
				Lang: "js",
				Body: `vars.global.set("token", "` + leakedSecret + `", true);
tests.assert(false, "leaked " + vars.global.get("token"));`,
			}},
		},
	}

	eng, _ := newStubEngine(t)
	res, err := eng.ExecuteWith(nil, req, testEnv(""), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if len(res.Tests) != 1 || res.Tests[0].Passed {
		t.Fatalf("Tests = %#v, want one failed test", res.Tests)
	}
	assertSecretStaysRedactable(t, res)
}

func TestEnvironmentRefValueStaysRedactable(t *testing.T) {
	key := "RESTERM_REDACTION_ENV_REF"
	t.Setenv(key, leakedSecret)
	env := envWith(t, "dev", map[string]string{"auth.token": "env:" + key})
	req := secretLeakRequest()
	req.Headers = http.Header{"X-Custom": []string{"{{auth.token}}"}}

	eng, st := newStubEngine(t)
	res, err := eng.ExecuteWith(nil, req, env, ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	if st.wire == nil || st.wire.Header.Get("X-Custom") != leakedSecret {
		t.Fatalf("mapped secret did not reach the wire: %v", st.wire)
	}
	assertSecretStaysRedactable(t, res)
}

func TestSecretExcerptKeepsTheFailingLine(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(strconv.Quote(eol), func(t *testing.T) {
			t.Setenv("RESTERM_REDACTION_KEY", "key-line-1"+eol+"key-line-2")
			t.Setenv("RESTERM_REDACTION_TOKEN", leakedSecret)
			env := envWith(t, "dev", map[string]string{
				"key":   "env:RESTERM_REDACTION_KEY",
				"token": "env:RESTERM_REDACTION_TOKEN",
			})
			source := "POST http://example.test" + eol + eol +
				"key-line-1" + eol + "key-line-2 " + leakedSecret + " {{missing}}" + eol
			doc := parser.Parse("secrets.http", []byte(source))

			eng := New(engine.Config{}, nil)
			res, err := eng.ExecuteWith(doc, doc.Requests[0], env, ExecOptions{})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if res.Err == nil {
				t.Fatal("expected the undefined variable to fail the request")
			}

			at := strings.Index(source, "{{missing}}")
			want := diag.Pos{
				Path: "secrets.http",
				Line: strings.Count(source[:at], "\n") + 1,
				Col:  at - strings.LastIndex(source[:at], "\n"),
			}
			rep := diag.ReportOf(res.Err)
			if got := rep.Items[0].Span.Start; got != want {
				t.Errorf("span at %+v, want %+v", got, want)
			}

			rendered := diag.RenderReport(rep)
			if strings.Contains(rendered, leakedSecret) {
				t.Fatalf("diagnostic kept the secret:\n%s", rendered)
			}
			var excerpt, caret string
			for _, line := range diag.Lines(rep) {
				switch line.Kind {
				case diag.LineSrc:
					excerpt = line.Text
				case diag.LineMark:
					caret = line.Text
				}
			}
			before, _, ok := strings.Cut(excerpt, "{{missing}}")
			if !ok {
				t.Fatalf("excerpt does not show the placeholder:\n%s", rendered)
			}
			pad, _, _ := strings.Cut(caret, "^")
			if uniseg.StringWidth(pad) != uniseg.StringWidth(before) {
				t.Errorf("caret is not under the placeholder:\n%s", rendered)
			}
		})
	}
}

type sourceDiagError struct {
	msg string
	rep diag.Report
}

func (e *sourceDiagError) Error() string           { return e.msg }
func (e *sourceDiagError) Diagnostic() diag.Report { return e.rep }

func sourceOnlySecretErr(secret string) error {
	return &sourceDiagError{
		msg: "undefined variable: missing",
		rep: diag.Report{
			Path:   "secret.http",
			Source: []byte("GET http://example.test\n\n" + secret + " trailing\n"),
			Items: []diag.Diagnostic{{
				Class:    diag.ClassScript,
				Severity: diag.SeverityError,
				Message:  "undefined variable: missing",
				Span: diag.Span{
					Start: diag.Pos{Line: 3, Col: 1},
					End:   diag.Pos{Line: 3, Col: 1},
				},
			}},
		},
	}
}

func TestExecuteWithMasksWarningsAndSkipReason(t *testing.T) {
	for _, val := range []string{"plain", leakedSecret} {
		src := fmt.Sprintf("### one\n# @name one\n# @request-secret tok %s\n# @request val %s\n"+
			"# @script pre-request\n> request.setHeader(\"X-Note\", \"{{\" + vars.get(\"val\") + \"}}\");\n"+
			"GET http://example.test\n\n"+
			"### two\n# @name two\n# @request-secret tok %s\n# @when \"%s\" == \"other\"\nGET http://example.test\n",
			leakedSecret, val, leakedSecret, val)
		doc := parser.Parse("mask.http", []byte(src))
		eng, _ := newStubEngine(t)
		warn, err := eng.ExecuteWith(doc, doc.Requests[0], testEnv(""), ExecOptions{})
		if err != nil {
			t.Fatal(err)
		}
		skip, err := eng.ExecuteWith(doc, doc.Requests[1], testEnv(""), ExecOptions{})
		if err != nil {
			t.Fatal(err)
		}
		shown := map[string]string{"warnings": strings.Join(warn.Warnings, "\n"), "skip reason": skip.SkipReason}
		for field, text := range shown {
			if val == leakedSecret && (strings.Contains(text, leakedSecret) || !strings.Contains(text, "•••")) {
				t.Errorf("%s shows the secret: %q", field, text)
			}
			if val != leakedSecret && (!strings.Contains(text, val) || strings.Contains(text, "•••")) {
				t.Errorf("%s = %q, want %q unmasked", field, text, val)
			}
		}
	}
}

func TestSecretSourcesSecretsLongestFirst(t *testing.T) {
	got := SecretSources{Extra: []string{"ab", "abcd", "abc"}}.Secrets()
	if want := []string{"abcd", "abc", "ab"}; !slices.Equal(got, want) {
		t.Errorf("Secrets() = %q, want %q", got, want)
	}
}
