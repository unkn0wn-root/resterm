package request

import (
	"context"
	"testing"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

const namedRefTarget = "RESTERM_NAMED_TARGET"

func TestRefNameComesFromDeclarationsOnly(t *testing.T) {
	decl := `# @file picked ` + namedRefTarget + `
# @file token env:{{picked}}
`

	t.Run("declaration names it", func(t *testing.T) {
		t.Setenv(namedRefTarget, "declared-value")
		doc, req := parseDoc(t, decl+`
### one
# @name one
GET http://example.test
X-Token: {{token}}
`)
		req.Metadata.Scripts = append(req.Metadata.Scripts, rtsPre(
			`request.setHeader("X-Vars", vars.get("token") ?? "absent")`,
		))
		sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
		for _, name := range []string{"X-Token", "X-Vars"} {
			if got := sent.wire.Header.Get(name); got != "declared-value" {
				t.Fatalf("%s = %q, want %q", name, got, "declared-value")
			}
		}
	})

	t.Run("script write cannot name it", func(t *testing.T) {
		t.Setenv(namedRefTarget, "declared-value")
		t.Setenv("RESTERM_NAMED_HIJACK", "hijacked-value")
		doc, req := parseDoc(t, decl+`
### one
# @name one
GET http://example.test
X-Token: {{token}}
`)
		req.Metadata.Scripts = append(req.Metadata.Scripts, rtsPre(
			`vars.set("picked", "RESTERM_NAMED_HIJACK")`,
		))
		sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
		if got := sent.wire.Header.Get("X-Token"); got != "declared-value" {
			t.Fatalf("X-Token = %q, want the declared name to stand", got)
		}
	})

	t.Run("workflow value cannot name it", func(t *testing.T) {
		t.Setenv(namedRefTarget, "declared-value")
		t.Setenv("RESTERM_NAMED_HIJACK", "hijacked-value")
		doc, req := parseDoc(t, decl+`
### one
# @name one
GET http://example.test
X-Token: {{token}}
`)
		sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{
			Extra: map[string]string{"picked": "RESTERM_NAMED_HIJACK"},
		})
		if got := sent.wire.Header.Get("X-Token"); got != "declared-value" {
			t.Fatalf("X-Token = %q, want the declared name to stand", got)
		}
	})

	t.Run("capture cannot name it", func(t *testing.T) {
		t.Setenv(namedRefTarget, "declared-value")
		t.Setenv("RESTERM_NAMED_HIJACK", "hijacked-value")
		doc, req := parseDoc(t, decl+`
### one
# @name one
# @capture request picked {{response.body}}
GET http://example.test
`)
		eng, st := newStubEngine(t)
		st.body = "RESTERM_NAMED_HIJACK"
		if _, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{}); err != nil {
			t.Fatalf("ExecuteWith() error = %v", err)
		}
		vv := eng.CollectVariables(doc, req, envWith(t, "dev", nil), nil)
		if vv["token"] != "declared-value" {
			t.Fatalf("token = %q, want the declared name to stand", vv["token"])
		}
	})
}

func TestRefNameWorksInEnvironmentCatalog(t *testing.T) {
	t.Setenv(namedRefTarget, "catalog-value")
	doc, req := parseDoc(t, `# @file picked `+namedRefTarget+`

### one
# @name one
GET http://example.test
X-Env: {{alias}}
`)

	env := envWith(t, "dev", map[string]string{"alias": "env:{{picked}}"})
	sent := sendRequest(t, doc, req, env, ExecOptions{})
	if got := sent.wire.Header.Get("X-Env"); got != "catalog-value" {
		t.Fatalf("X-Env = %q, want the catalog reference resolved", got)
	}

	snap := ResolveEnvironment(env, doc, req)
	if got := snap.Values()["alias"]; got != "catalog-value" {
		t.Fatalf("env host alias = %q, want %q", got, "catalog-value")
	}
	if secrets := snap.Secrets(); len(secrets) != 1 {
		t.Fatalf("Secrets() = %#v, want one entry for one process variable", secrets)
	}
}

func TestRefNameKeepsGlobalHostInAgreement(t *testing.T) {
	t.Setenv(namedRefTarget, "global-value")
	doc, req := parseDoc(t, `# @file picked `+namedRefTarget+`
# @global token env:{{picked}}

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

func TestRefNameOnUnusedConstIsStillRedactable(t *testing.T) {
	t.Setenv(namedRefTarget, leakedSecret)
	doc, req := parseDoc(t, `# @file picked `+namedRefTarget+`
# @const unused env:{{picked}}

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

func TestRefNameIsWithheldFromDisplay(t *testing.T) {
	t.Setenv(namedRefTarget, "private")
	t.Setenv("TOKEN", "ambient-value")
	doc, req := parseDoc(t, `# @file picked `+namedRefTarget+`
# @file token env:{{picked}}

### one
# @name one
GET http://example.test
`)

	eng := New(engcfg.Config{}, nil)
	res := eng.DisplayResolver(context.Background(), doc, req, envWith(t, "dev", nil), "", rts.Locals{})
	for _, name := range []string{"token", "file.token"} {
		if got, err := res.ExpandTemplates("{{" + name + "}}"); err == nil {
			t.Fatalf("{{%s}} resolved to %q, want the reference withheld", name, got)
		}
	}
}

func TestRefNameWithNoDeclarationStaysUndefined(t *testing.T) {
	doc, req := parseDoc(t, `# @file token env:{{picked}}

### one
# @name one
GET http://example.test
X-Token: {{token}}
`)

	eng, st := newStubEngine(t)
	res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err == nil {
		t.Fatal("an unresolvable reference name produced a value")
	}
	if st.wire != nil {
		t.Fatal("request with an unresolved reference was sent")
	}
}

func TestOnlyDocumentSourcesMayNameAReference(t *testing.T) {
	runtime := []variableSource{
		sourceScript,
		sourceWorkflow,
		sourceRequestCapture,
		sourceRuntimeGlobal,
		sourceRuntimeFile,
	}
	for _, s := range runtime {
		if s.traits().declared {
			t.Fatalf("%s is marked declared; a value produced while a request runs must not name a reference",
				s.traits().label)
		}
	}

	declared := []variableSource{
		sourceConst,
		sourceRequest,
		sourceDocumentGlobal,
		sourceFile,
		sourceEnvironment,
	}
	for _, s := range declared {
		if !s.traits().declared {
			t.Fatalf("%s is not marked declared; its values would stop naming references", s.traits().label)
		}
		if len(declarations(s, nil, nil)) != 0 {
			t.Fatalf("%s produced declarations from an empty document", s.traits().label)
		}
	}
}

func TestCapturedNameCannotBeUsedByALaterRequest(t *testing.T) {
	t.Setenv(namedRefTarget, "declared-value")
	t.Setenv("RESTERM_NAMED_HIJACK", "hijacked-value")

	doc := parser.Parse("two.http", []byte(`# @file picked `+namedRefTarget+`
# @file token env:{{picked}}

### grab
# @name grab
# @capture file picked {{response.body}}
GET http://example.test/grab

### use
# @name use
GET http://example.test/use
X-Token: {{token}}
`))
	if len(doc.Requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(doc.Requests))
	}

	eng, st := newStubEngine(t)
	st.body = "RESTERM_NAMED_HIJACK"
	if _, err := eng.ExecuteWith(doc, doc.Requests[0], envWith(t, "dev", nil), ExecOptions{}); err != nil {
		t.Fatalf("first request: %v", err)
	}
	for _, v := range doc.Variables {
		if v.Name == "picked" && v.Authored {
			t.Fatal("the capture left the declaration flag set")
		}
	}

	// The capture removed the only declaration, so the name is undefined.
	st.body = "ok"
	st.wire = nil
	res, err := eng.ExecuteWith(doc, doc.Requests[1], envWith(t, "dev", nil), ExecOptions{})
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	if res.Err == nil {
		t.Fatalf("the reference resolved to %q", st.wire.Header.Get("X-Token"))
	}
	if st.wire != nil {
		t.Fatalf("request was sent with X-Token = %q", st.wire.Header.Get("X-Token"))
	}
}
