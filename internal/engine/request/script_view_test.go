package request

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/parser"
)

// Scripts read authored values with references expanded and write data, so a
// read, change, and write keeps the expanded value.
func TestScriptsReadAuthoredValuesExpanded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block string
		check func(sentRequest) (string, string)
	}{
		{
			name:  "js url",
			block: "# @script pre-request\n> request.setURL(request.getURL() + \"?debug=1\");\nGET {{base}}/users/{{id}}\n",
			check: wireURL("http://example.test/users/42?debug=1"),
		},
		{
			name:  "rts url",
			block: "# @rts pre-request\n> request.setURL(request.url + \"?debug=1\")\nGET {{base}}/users/{{id}}\n",
			check: wireURL("http://example.test/users/42?debug=1"),
		},
		{
			name:  "apply url",
			block: "# @apply {url: request.url + \"?debug=1\"}\nGET {{base}}/users/{{id}}\n",
			check: wireURL("http://example.test/users/42?debug=1"),
		},
		{
			name:  "const url",
			block: "# @script pre-request\n> request.setURL(request.getURL() + \"/x\");\nGET {{root}}\n",
			check: wireURL("http://example.test/v1/x"),
		},
		{
			name: "js header copy",
			block: "# @script pre-request\n> request.setHeader(\"Authorization\", request.getHeader(\"Authorization\"));\n" +
				"GET http://example.test\nAuthorization: Bearer {{token}}\n",
			check: wireHeader("Authorization", "Bearer actual-token"),
		},
		{
			name: "rts header and query",
			block: "# @rts pre-request\n> request.setHeader(\"X-Seen\", request.header(\"X-Id\") + \" \" + request.query.id)\n" +
				"GET http://example.test?id={{id}}\nX-Id: {{id}}\n",
			check: wireHeader("X-Seen", "42 42"),
		},
		{
			name:  "js var set first",
			block: "# @script pre-request\n> vars.set(\"id\", \"7\");\n> request.setURL(request.getURL() + \"/x\");\nGET {{base}}/{{id}}\n",
			check: wireURL("http://example.test/7/x"),
		},
		{
			name:  "js signs the sent url",
			block: "# @script pre-request\n> request.setHeader(\"X-Signed\", request.getURL());\nGET {{base}}/v1\n",
			check: wireHeader("X-Signed", "http://example.test/v1"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "# @file base http://example.test\n# @file id 42\n# @file token actual-token\n" +
				"# @const root http://example.test/v1\n\n### one\n# @name one\n" + tc.block
			doc, req := parseDoc(t, src)
			sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
			if got, want := tc.check(sent); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

// A value from a response stays data when a script reads and writes it back,
// whether it reached the request through a template or an earlier write.
func TestScriptsReadRuntimeValuesAsData(t *testing.T) {
	src := "# @file-secret api.token secret-value-9f3a\n\n" +
		"### a\n# @name a\n# @capture global item {{response.json.item}}\nGET http://example.test/a\n\n" +
		"### b\n# @name b\n# @apply {headers: {\"X-Applied\": vars.get(\"item\")}}\n" +
		"# @rts pre-request\n> request.setHeader(\"X-Rts\", request.header(\"X-Applied\") + request.header(\"X-Item\"))\n" +
		"# @script pre-request\n> request.setHeader(\"X-Js\", request.getHeader(\"X-Rts\") + request.getHeader(\"X-Item\"));\n" +
		"GET http://example.test/b\nX-Item: {{item}}\n"
	doc := parser.Parse("view.http", []byte(src))
	eng, st := newStubEngine(t)
	env := envWith(t, "dev", nil)
	st.body = `{"item":"{{api.token}}"}`
	sendWith(t, eng, st, doc, doc.Requests[0], env, ExecOptions{})
	sent := sendWith(t, eng, st, doc, doc.Requests[1], env, ExecOptions{})

	got := wireText(t, sent)
	if strings.Contains(got, "secret-value") {
		t.Fatalf("a response value was expanded:\n%s", got)
	}
	if want := "X-Js: " + strings.Repeat("{{api.token}}", 3); !strings.Contains(got, want) {
		t.Fatalf("sent request does not contain %q:\n%s", want, got)
	}
}

// Reading a value with an undefined reference fails like sending it would.
func TestScriptGettersFailOnUndefinedReferences(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block string
		want  string
	}{
		{name: "js url", block: "# @script pre-request\n> request.getURL();\n", want: "request.getURL"},
		{name: "js header", block: "# @script pre-request\n> request.getHeader(\"X-Id\");\n", want: "request.getHeader"},
		{name: "rts url", block: "# @rts pre-request\n> let u = request.url\n", want: "request url"},
		{name: "apply url", block: "# @apply {url: request.url}\n", want: "request url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, req := parseDoc(
				t,
				"### one\n# @name one\n"+tc.block+"GET http://example.test/{{missing}}\nX-Id: {{missing}}\n",
			)
			eng, _ := newStubEngine(t)
			res, err := eng.ExecuteWith(doc, req, envWith(t, "dev", nil), ExecOptions{})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if res.Err == nil || !strings.Contains(res.Err.Error(), tc.want) ||
				!strings.Contains(res.Err.Error(), "missing") {
				t.Fatalf("error = %v, want %s to report the missing variable", res.Err, tc.want)
			}
		})
	}
}

func TestJSGetterErrorCanBeCaught(t *testing.T) {
	doc, req := parseDoc(t, `### one
# @name one
# @script pre-request
> try { request.getURL(); } catch (e) { request.setHeader("X-Caught", "yes"); request.setURL("http://example.test/ok"); }
GET http://example.test/{{missing}}
`)
	sent := sendRequest(t, doc, req, envWith(t, "dev", nil), ExecOptions{})
	if got := sent.wire.Header.Get("X-Caught"); got != "yes" {
		t.Fatalf("X-Caught = %q, want yes", got)
	}
}

func wireURL(want string) func(sentRequest) (string, string) {
	return func(s sentRequest) (string, string) { return s.wire.URL.String(), want }
}

func wireHeader(name, want string) func(sentRequest) (string, string) {
	return func(s sentRequest) (string, string) { return s.wire.Header.Get(name), want }
}
