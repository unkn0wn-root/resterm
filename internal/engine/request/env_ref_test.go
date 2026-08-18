package request

import (
	"context"
	"fmt"
	"testing"
	"time"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func absentKey(t *testing.T, label string) string {
	t.Helper()
	return fmt.Sprintf("RESTERM_ABSENT_%s_%d", label, time.Now().UnixNano())
}

func parseDoc(t *testing.T, src string) (*restfile.Document, *restfile.Request) {
	t.Helper()
	doc := parser.Parse("env_ref.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("parsed %d requests, want 1", len(doc.Requests))
	}
	return doc, doc.Requests[0]
}

func echoToken(req *restfile.Request) {
	req.Metadata.Scripts = append(req.Metadata.Scripts,
		rtsPre(`request.setHeader("X-RTS-Vars", vars.get("token") ?? "absent")
request.setHeader("X-RTS-Env", env.get("token") ?? "absent")`),
		jsPre(`request.setHeader("X-JS-Vars", vars.get("token") || "absent");`),
	)
}

func TestAuthoredEnvRefsResolveEverywhere(t *testing.T) {
	tests := []struct {
		name   string
		decl   string
		hidden bool // @const is not exposed to scripts
	}{
		{name: "file", decl: "# @file token env:RESTERM_AUTHORED_REF"},
		{name: "global", decl: "# @global token env:RESTERM_AUTHORED_REF"},
		{name: "request", decl: "# @request token env:RESTERM_AUTHORED_REF"},
		{name: "const", decl: "# @const token env:RESTERM_AUTHORED_REF", hidden: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RESTERM_AUTHORED_REF", "os-value")
			doc, req := parseDoc(t, `### one
# @name one
`+tt.decl+`
GET http://example.test
X-Template: {{token}}
`)
			echoToken(req)

			sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
			if got := sent.wire.Header.Get("X-Template"); got != "os-value" {
				t.Fatalf("{{token}} = %q, want the process value", got)
			}

			want := "os-value"
			if tt.hidden {
				want = "absent"
			}
			for _, name := range []string{"X-RTS-Vars", "X-JS-Vars"} {
				if got := sent.wire.Header.Get(name); got != want {
					t.Fatalf("%s = %q, want %q", name, got, want)
				}
			}
			if got := sent.wire.Header.Get("X-RTS-Env"); got != "absent" {
				t.Fatalf(`env.get("token") = %q, want the alias absent from env`, got)
			}
		})
	}
}

func TestAuthoredEnvRefResolvesUnderQualifiedName(t *testing.T) {
	t.Setenv("RESTERM_QUALIFIED_REF", "os-value")
	doc, req := parseDoc(t, `# @file token env:RESTERM_QUALIFIED_REF

### one
# @name one
GET http://example.test
X-Plain: {{token}}
X-Qualified: {{file.token}}
`)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	for _, name := range []string{"X-Plain", "X-Qualified"} {
		if got := sent.wire.Header.Get(name); got != "os-value" {
			t.Fatalf("%s = %q, want the process value", name, got)
		}
	}
}

func TestMissingAuthoredEnvRefBlocksLowerSources(t *testing.T) {
	key := absentKey(t, "PRECEDENCE")
	t.Setenv("TOKEN", "ambient-value")
	src := `# @file token file-value

### one
# @name one
# @request token env:` + key + `
GET http://example.test
`
	doc, req := parseDoc(t, src+"X-Used: {{token}}\n")

	eng, st := newStubEngine(t)
	res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err == nil {
		t.Fatal("a missing reference resolved through a lower source")
	}
	if st.wire != nil {
		t.Fatal("request with an unresolved reference was sent")
	}

	doc2, req2 := parseDoc(t, src+"X-Lower: {{file.token}}\n")
	echoToken(req2)
	sent := sendRequest(t, doc2, req2, envWith(t, "dev", nil), ExecOptions{})
	if got := sent.wire.Header.Get("X-Lower"); got != "file-value" {
		t.Fatalf("{{file.token}} = %q, want the file value still reachable", got)
	}
	for _, name := range []string{"X-RTS-Vars", "X-JS-Vars"} {
		if got := sent.wire.Header.Get(name); got != "absent" {
			t.Fatalf("%s = %q, want the missing reference to stay undefined", name, got)
		}
	}
}

func TestAuthoredEnvRefResolvesOnePass(t *testing.T) {
	t.Setenv("RESTERM_ONE_PASS_INNER", "inner-value")
	t.Setenv("RESTERM_ONE_PASS", "env:RESTERM_ONE_PASS_INNER")
	doc, req := parseDoc(t, `# @file token env:RESTERM_ONE_PASS

### one
# @name one
GET http://example.test
X-Template: {{token}}
`)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := sent.wire.Header.Get("X-Template"); got != "env:RESTERM_ONE_PASS_INNER" {
		t.Fatalf("{{token}} = %q, want a single resolution", got)
	}
}

func TestRuntimeValuesNeverBecomeEnvRefs(t *testing.T) {
	key := "RESTERM_RUNTIME_LITERAL"
	literal := "env:" + key

	t.Run("script write", func(t *testing.T) {
		t.Setenv(key, "must-not-leak")
		doc, req := parseDoc(t, `### one
# @name one
GET http://example.test
X-Template: {{token}}
`)
		req.Metadata.Scripts = append(req.Metadata.Scripts, rtsPre(
			`vars.set("token", "`+literal+`")
request.setHeader("X-Vars", vars.require("token"))`,
		))

		sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
		for _, name := range []string{"X-Template", "X-Vars"} {
			if got := sent.wire.Header.Get(name); got != literal {
				t.Fatalf("%s = %q, want the text unchanged", name, got)
			}
		}
	})

	t.Run("workflow overlay", func(t *testing.T) {
		t.Setenv(key, "must-not-leak")
		doc, req := parseDoc(t, `### one
# @name one
GET http://example.test
X-Template: {{token}}
`)
		sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{
			Extra: map[string]string{"token": literal},
		})
		if got := sent.wire.Header.Get("X-Template"); got != literal {
			t.Fatalf("workflow value = %q, want the text unchanged", got)
		}
	})

	t.Run("capture overwriting a declaration", func(t *testing.T) {
		t.Setenv(key, "must-not-leak")
		doc, req := parseDoc(t, `# @file token env:RESTERM_RUNTIME_LITERAL

### one
# @name one
# @capture file token {{response.body}}
GET http://example.test
`)
		eng, st := newStubEngine(t)
		st.body = literal
		if _, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{}); err != nil {
			t.Fatalf("ExecuteWith() error = %v", err)
		}
		for _, v := range doc.Variables {
			if v.Name == "token" && v.Authored {
				t.Fatal("captured value kept the declaration flag")
			}
		}
		vv := eng.CollectVariables(doc, req, envWith(t, "dev", nil), nil)
		if vv["token"] != literal {
			t.Fatalf("captured token = %q, want the response text unchanged", vv["token"])
		}
	})
}

func TestAuthoredEnvRefValueStaysRedactable(t *testing.T) {
	key := "RESTERM_AUTHORED_REDACTION"
	t.Setenv(key, leakedSecret)
	doc, req := parseDoc(t, `# @file unused env:`+key+`

### one
# @name one
GET http://example.test
`)
	req.Metadata.Scripts = append(req.Metadata.Scripts, restfileTestBlock(leakedSecret))

	eng, _ := newStubEngine(t)
	res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	assertSecretStaysRedactable(t, res)
}

func TestDisplayResolverWithholdsAuthoredEnvRefs(t *testing.T) {
	key := "RESTERM_AUTHORED_DISPLAY"
	t.Setenv(key, "private")
	t.Setenv("TOKEN", "ambient-value")
	doc, req := parseDoc(t, `# @file token env:`+key+`
# @file shown visible

### one
# @name one
GET http://example.test
`)

	eng := New(engcfg.Config{}, nil)
	res := eng.DisplayResolver(context.Background(), doc, req, envWith(t, "dev", nil), "", rts.Locals{})

	if got, err := res.ExpandTemplates("{{shown}}"); err != nil || got != "visible" {
		t.Fatalf("{{shown}} = %q, %v, want %q", got, err, "visible")
	}
	for _, name := range []string{"token", "file.token"} {
		if got, err := res.ExpandTemplates("{{" + name + "}}"); err == nil {
			t.Fatalf("{{%s}} resolved to %q, want the reference withheld", name, got)
		}
	}
	if got, err := res.ExpandTemplates(`{{= vars.get("token") ?? "withheld" }}`); err != nil ||
		got != "withheld" {
		t.Fatalf(`vars.get("token") = %q, %v, want %q`, got, err, "withheld")
	}
}

func TestAuthoredAndEnvironmentRefsShareOneSnapshot(t *testing.T) {
	key := "RESTERM_SHARED_REF"
	t.Setenv(key, "shared-value")
	doc, req := parseDoc(t, `# @file doc_token env:`+key+`

### one
# @name one
GET http://example.test
X-Doc: {{doc_token}}
X-Env: {{env_token}}
`)

	env := envWith(t, "dev", map[string]string{"env_token": "env:" + key})
	sent := sendRequest(t, doc, req, env, ExecOptions{})
	for _, name := range []string{"X-Doc", "X-Env"} {
		if got := sent.wire.Header.Get(name); got != "shared-value" {
			t.Fatalf("%s = %q, want %q", name, got, "shared-value")
		}
	}

	snap := env.Resolve()
	_ = snap
	if secrets := snap.Secrets(); len(secrets) != 1 {
		t.Fatalf("Secrets() = %#v, want one entry for one process variable", secrets)
	}
}

func TestAuthoredEnvRefIsSecretWithoutTheSecretForm(t *testing.T) {
	key := "RESTERM_IMPLICIT_SECRET"
	t.Setenv(key, "implicit")
	doc, req := parseDoc(t, `# @global token env:`+key+`

### one
# @name one
GET http://example.test
`)

	eng, _ := newStubEngine(t)
	res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	found := false
	for _, s := range res.RuntimeSecrets {
		if s == "implicit" {
			found = true
		}
	}
	if !found {
		t.Fatalf("RuntimeSecrets = %#v, want the resolved value recorded", res.RuntimeSecrets)
	}
}

func TestGlobalHostSeesResolvedEnvRef(t *testing.T) {
	key := "RESTERM_GLOBAL_HOST_REF"
	t.Setenv(key, "global-value")
	doc, req := parseDoc(t, `# @global token env:`+key+`

### one
# @name one
GET http://example.test
`)
	req.Metadata.Scripts = append(req.Metadata.Scripts, rtsPre(
		`request.setHeader("X-Global", vars.global.get("token") ?? "absent")
request.setHeader("X-Vars", vars.get("token") ?? "absent")`,
	))

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	for _, name := range []string{"X-Global", "X-Vars"} {
		if got := sent.wire.Header.Get(name); got != "global-value" {
			t.Fatalf("%s = %q, want %q", name, got, "global-value")
		}
	}
}

func TestAuthoredEnvRefNamedByPlaceholder(t *testing.T) {
	t.Setenv("RESTERM_DEFERRED_TARGET", "deferred-value")
	doc, req := parseDoc(t, `# @file picked RESTERM_DEFERRED_TARGET
# @file token env:{{picked}}

### one
# @name one
GET http://example.test
X-Template: {{token}}
`)
	req.Metadata.Scripts = append(req.Metadata.Scripts, rtsPre(
		`request.setHeader("X-Vars", vars.get("token") ?? "absent")`,
	))

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	for _, name := range []string{"X-Template", "X-Vars"} {
		if got := sent.wire.Header.Get(name); got != "deferred-value" {
			t.Fatalf("%s = %q, want %q", name, got, "deferred-value")
		}
	}
}

func restfileTestBlock(value string) restfile.ScriptBlock {
	return restfile.ScriptBlock{
		Kind: "test",
		Lang: "js",
		Body: `tests.assert(false, "leaked ` + value + `");`,
	}
}
