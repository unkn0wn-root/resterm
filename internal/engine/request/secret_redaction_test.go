package request

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/engine"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
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
