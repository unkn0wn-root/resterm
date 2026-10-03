package request

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/parser"
)

func TestVarsInterpolateBuildsRequestValues(t *testing.T) {
	t.Setenv("RESTERM_INTERPOLATE_TOKEN", "os-token")
	for _, tc := range []struct {
		name  string
		block string
	}{
		{
			name: "js",
			block: "# @script pre-request\n" +
				"> request.setURL(vars.interpolate(\"{{base}}/users/{{id}}\"));\n" +
				"> request.setHeader(\"Authorization\", vars.interpolate(\"Bearer {{token}}\"));\n",
		},
		{
			name: "rts",
			block: "# @rts pre-request\n" +
				"> request.setURL(vars.interpolate(\"{{base}}/users/{{id}}\"))\n" +
				"> request.setHeader(\"Authorization\", vars.interpolate(\"Bearer {{token}}\"))\n",
		},
		{
			name: "apply",
			block: "# @apply {url: vars.interpolate(\"{{base}}/users/{{id}}\"), " +
				"headers: {\"Authorization\": vars.interpolate(\"Bearer {{token}}\")}}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, req := parseDoc(t, "# @file token env:RESTERM_INTERPOLATE_TOKEN\n\n"+
				"### one\n# @name one\n# @request id 42\n"+tc.block+"GET http://example.test\n")
			if len(doc.Warnings) != 0 {
				t.Fatalf("parse warnings = %+v", doc.Warnings)
			}
			eng, st := newStubEngine(t)
			env := envWith(t, "dev", map[string]string{"base": "http://example.test/api"})
			res, err := eng.ExecuteWith(doc, req, env, ExecOptions{})
			if err != nil || res.Err != nil {
				t.Fatalf("ExecuteWith() = %v, %v", err, res.Err)
			}
			if len(res.Warnings) != 0 {
				t.Fatalf("warnings = %q", res.Warnings)
			}
			got := wireText(t, sentRequest{wire: st.wire})
			for _, want := range []string{"path: /api/users/42", "Authorization: Bearer os-token"} {
				if !strings.Contains(got, want) {
					t.Fatalf("sent request does not contain %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestVarsInterpolateInsertsCapturedValuesAsWritten(t *testing.T) {
	const secret = "secret-value-9f3a"
	src := "# @file-secret api.token " + secret + "\n\n" +
		"### a\n# @name a\n# @capture global note {{response.json.note}}\nGET http://example.test/a\n\n" +
		"### b\n# @name b\n# @script pre-request\n" +
		"> request.setHeader(\"X-Note\", vars.interpolate(\"note: {{note}}\"));\n" +
		"GET http://example.test/b\n"
	doc := parser.Parse("interpolate.http", []byte(src))
	eng, st := newStubEngine(t)
	env := envWith(t, "dev", nil)
	st.body = `{"note": "{{api.token}} {{= 1 + 1}}"}`
	sendWith(t, eng, st, doc, doc.Requests[0], env, ExecOptions{})

	sent := sendWith(t, eng, st, doc, doc.Requests[1], env, ExecOptions{})
	if got := sent.wire.Header.Get("X-Note"); got != "note: {{api.token}} {{= 1 + 1}}" {
		t.Fatalf("X-Note = %q, want the captured text as written", got)
	}
	if got := wireText(t, sent); strings.Contains(got, secret) {
		t.Fatalf("captured text read a secret:\n%s", got)
	}
}

func TestVarsInterpolateReadsOnlyScriptVariables(t *testing.T) {
	t.Setenv("RESTERM_INTERPOLATE_LEAK", "leaked")
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			name: "unmapped OS variable",
			src: "### one\n# @name one\n# @script pre-request\n" +
				"> request.setHeader(\"X\", vars.interpolate(\"{{RESTERM_INTERPOLATE_LEAK}}\"));\n" +
				"GET http://example.test\n",
			want: "vars.interpolate: undefined variable: RESTERM_INTERPOLATE_LEAK",
		},
		{
			name: "constant",
			src: "# @const c constant\n\n### one\n# @name one\n# @rts pre-request\n" +
				"> request.setHeader(\"X\", vars.interpolate(\"{{c}}\"))\n" +
				"GET http://example.test\n",
			want: "vars.interpolate: undefined variable: c",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, req := parseDoc(t, tc.src)
			eng, st := newStubEngine(t)
			res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if res.Err == nil || !strings.Contains(res.Err.Error(), tc.want) {
				t.Fatalf("result error = %v, want %q", res.Err, tc.want)
			}
			if st.wire != nil {
				t.Fatal("request was sent")
			}
		})
	}
}

func TestVarsInterpolateSelfReferenceIsACycle(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @request x {{= vars.interpolate(vars.get("tpl"))}}
# @apply {vars: {tpl: "{" + "{x}" + "}"}}
GET http://example.test/{{x}}
`)
	eng, st := newStubEngine(t)
	res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "variable cycle") {
		t.Fatalf("result error = %v, want a variable cycle", res.Err)
	}
	if st.wire != nil {
		t.Fatal("request was sent")
	}
}
