package request

import "testing"

func TestEnvRefWithNoNameStaysUndefined(t *testing.T) {
	for _, decl := range []string{
		"# @file token env:",
		"# @global token env:",
		"# @request token env:",
		"# @file token env:   ",
	} {
		t.Run(decl, func(t *testing.T) {
			t.Setenv("TOKEN", "ambient-value")
			doc, req := parseDoc(t, `### one
# @name one
`+decl+`
GET http://example.test
X-Token: {{token}}
`)
			req.Metadata.Scripts = append(req.Metadata.Scripts, rtsPre(
				`request.setHeader("X-Vars", vars.get("token") ?? "absent")`,
			))

			eng, st := newStubEngine(t)
			res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if res.Err == nil {
				t.Fatal("a reference with no name produced a value")
			}
			if st.wire != nil {
				t.Fatal("request with an unusable reference was sent")
			}

			if len(doc.Errors) == 0 {
				t.Fatal("the malformed reference was not reported")
			}
		})
	}
}

func TestValuesThatOnlyLookLikeReferencesStayText(t *testing.T) {
	doc, req := parseDoc(t, `# @file a environment:X
# @file b envs:X
# @file c env

### one
# @name one
GET http://example.test
X-A: {{a}}
X-B: {{b}}
X-C: {{c}}
`)

	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	for name, want := range map[string]string{
		"X-A": "environment:X",
		"X-B": "envs:X",
		"X-C": "env",
	} {
		if got := sent.wire.Header.Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	if len(doc.Errors) != 0 {
		t.Fatalf("errors = %#v, want none", doc.Errors)
	}
}
