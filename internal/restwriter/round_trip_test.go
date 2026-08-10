package restwriter

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func testWorkflow(name string) restfile.Workflow {
	return restfile.Workflow{
		Name: name,
		Steps: []restfile.WorkflowStep{{
			Name:  "one",
			Using: "Req",
			Kind:  restfile.WorkflowStepKindRequest,
		}},
	}
}

func testMock() *restfile.Mock {
	return &restfile.Mock{
		Name:   "m1",
		Method: "GET",
		Path:   "/thing",
		Responses: []restfile.MockResponse{{
			Status: 200,
			Body:   restfile.BodySource{Text: `{"ok":true}`},
		}},
	}
}

func testRequest() *restfile.Request {
	return &restfile.Request{
		Method:   "POST",
		URL:      "http://x/api",
		Metadata: restfile.RequestMetadata{Name: "Req"},
		Body:     restfile.BodySource{Text: `{"a":1}`},
	}
}

// A mock block runs until the next separator, so a workflow rendered straight
// after one used to be read back as part of the mock body and disappear.
func TestRenderRoundTripsWorkflowsAfterEveryBlock(t *testing.T) {
	tests := []struct {
		name string
		doc  *restfile.Document
	}{
		{
			name: "mock then workflow",
			doc: &restfile.Document{
				Mocks:     []*restfile.Mock{testMock()},
				Workflows: []restfile.Workflow{testWorkflow("flow")},
			},
		},
		{
			name: "request then workflow",
			doc: &restfile.Document{
				Requests:  []*restfile.Request{testRequest()},
				Workflows: []restfile.Workflow{testWorkflow("flow")},
			},
		},
		{
			name: "request and mock then workflows",
			doc: &restfile.Document{
				Requests:  []*restfile.Request{testRequest()},
				Mocks:     []*restfile.Mock{testMock()},
				Workflows: []restfile.Workflow{testWorkflow("a"), testWorkflow("b")},
			},
		},
		{
			name: "workflows only",
			doc: &restfile.Document{
				Workflows: []restfile.Workflow{testWorkflow("a"), testWorkflow("b")},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Render(tt.doc, Options{})
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			back := parser.Parse("round-trip.http", []byte(out))
			if len(back.Errors) != 0 {
				t.Fatalf("rendered document does not parse: %v\n%s", back.Errors, out)
			}
			if got, want := len(back.Workflows), len(tt.doc.Workflows); got != want {
				t.Fatalf("workflows = %d, want %d\n%s", got, want, out)
			}
			if got, want := len(back.Mocks), len(tt.doc.Mocks); got != want {
				t.Fatalf("mocks = %d, want %d\n%s", got, want, out)
			}
			if got, want := len(back.Requests), len(tt.doc.Requests); got != want {
				t.Fatalf("requests = %d, want %d\n%s", got, want, out)
			}
			for i, wf := range back.Workflows {
				if wf.Name != tt.doc.Workflows[i].Name {
					t.Fatalf("workflow %d name = %q, want %q", i, wf.Name, tt.doc.Workflows[i].Name)
				}
				if len(wf.Steps) != 1 {
					t.Fatalf("workflow %q steps = %d, want 1", wf.Name, len(wf.Steps))
				}
			}
			for _, mock := range back.Mocks {
				for _, res := range mock.Responses {
					if strings.Contains(res.Body.Text, "@workflow") {
						t.Fatalf("mock body swallowed the workflow: %q", res.Body.Text)
					}
				}
			}
		})
	}
}

func TestRenderCaptureTemplateKeepsLiteralDelimiters(t *testing.T) {
	src := `### r
# @capture request label prefix[{{response.status}}
# @capture request frag anchor#{{response.status}}
GET https://example.com/
`
	doc := parser.Parse("c.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse: %v", doc.Errors)
	}
	got, err := Render(doc, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{
		"# @capture request label prefix[{{response.status}}\n",
		"# @capture request frag anchor#{{response.status}}\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered document lost %q:\n%s", want, got)
		}
	}
}

func TestRenderCaptureSpanningTemplateIsIdempotent(t *testing.T) {
	src := `### r
# @capture request label prefix[{{
#   response.status
# }}
GET https://example.com/
`
	out := src
	for i := range 3 {
		doc := parser.Parse("c.http", []byte(out))
		if len(doc.Errors) != 0 {
			t.Fatalf("round %d did not parse: %v\n%s", i, doc.Errors, out)
		}
		got, err := Render(doc, Options{})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if i > 0 && got != out {
			t.Fatalf("round %d changed the document:\nbefore:\n%s\nafter:\n%s", i, out, got)
		}
		out = got
	}
	if !strings.Contains(out, "#   response.status") {
		t.Fatalf("continuation lost its indent:\n%s", out)
	}
}

func TestRenderCaptureSpanningExpressionIsIdempotent(t *testing.T) {
	src := `### r
# @capture request total sum(
#   response.json.a,
#   response.json.b
# )
GET https://example.com/
`
	out := src
	for i := range 3 {
		doc := parser.Parse("c.http", []byte(out))
		if len(doc.Errors) != 0 {
			t.Fatalf("round %d did not parse: %v\n%s", i, doc.Errors, out)
		}
		got, err := Render(doc, Options{})
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		if i > 0 && got != out {
			t.Fatalf("round %d changed the document:\nbefore:\n%s\nafter:\n%s", i, out, got)
		}
		out = got
	}
	if !strings.Contains(out, "#   response.json.a,") {
		t.Fatalf("continuation lost its indent:\n%s", out)
	}
}

func TestRenderBlockCommentCaptureRoundTripsExactly(t *testing.T) {
	doc := parser.Parse("c.http", []byte(`/*
@capture request label prefix {{
value
}} suffix
*/
GET https://example.com/
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse: %v", doc.Errors)
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(doc.Requests))
	}
	if len(doc.Requests[0].Metadata.Captures) != 1 {
		t.Fatalf("captures = %d, want 1", len(doc.Requests[0].Metadata.Captures))
	}
	want := doc.Requests[0].Metadata.Captures[0].Expression
	if want != "prefix {{\n value\n }} suffix" {
		t.Fatalf("initial expression = %q", want)
	}

	out := mustRender(t, doc)
	again := parser.Parse("c.http", []byte(out))
	if len(again.Errors) != 0 {
		t.Fatalf("rendered document did not parse: %v\n%s", again.Errors, out)
	}
	if len(again.Requests) != 1 {
		t.Fatalf("round-trip requests = %d, want 1", len(again.Requests))
	}
	if len(again.Requests[0].Metadata.Captures) != 1 {
		t.Fatalf("round-trip captures = %d, want 1", len(again.Requests[0].Metadata.Captures))
	}
	got := again.Requests[0].Metadata.Captures[0].Expression
	if got != want {
		t.Fatalf("expression changed after round trip:\nwant %q\ngot  %q\n%s", want, got, out)
	}
	if next := mustRender(t, again); next != out {
		t.Fatalf("rendering was not idempotent:\nfirst:\n%s\nsecond:\n%s", out, next)
	}
}

func TestRenderRoundTripsRequestDirectives(t *testing.T) {
	src := `# @patch file auth { headers: { "X-Tenant": "acme" } }
# @patch global trace { headers: { "X-Trace": "on" } }

### r
# @when (
#   env == "dev"
#   and 1 == 1
# )
# @for-each [1, 2] as n
# @apply use=auth, use=trace
# @apply { query: { "n": n } }
# @capture request token response.json.token
# @assert status == 200 => "healthy"
# @assert (
#   status == 200
#   and statusText == "200 OK"
# ) => "still healthy"
GET https://example.com/
`
	doc := parser.Parse("r.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("source did not parse: %v", doc.Errors)
	}

	out := mustRender(t, doc)
	back := parser.Parse("r.http", []byte(out))
	if len(back.Errors) != 0 {
		t.Fatalf("rendered document did not parse: %v\n%s", back.Errors, out)
	}

	if len(back.Patches) != len(doc.Patches) {
		t.Fatalf("patches = %d, want %d\n%s", len(back.Patches), len(doc.Patches), out)
	}
	for i, want := range doc.Patches {
		got := back.Patches[i]
		if got.Scope != want.Scope || got.Name != want.Name || got.Expression != want.Expression {
			t.Fatalf("patch %d = %+v, want %+v\n%s", i, got, want, out)
		}
	}

	if len(back.Requests) != 1 {
		t.Fatalf("requests = %d, want 1\n%s", len(back.Requests), out)
	}
	got, want := back.Requests[0].Metadata, doc.Requests[0].Metadata

	if got.When == nil || want.When == nil || *got.When != *want.When {
		t.Fatalf("@when = %+v, want %+v\n%s", got.When, want.When, out)
	}
	if got.ForEach == nil || want.ForEach == nil ||
		got.ForEach.Expression != want.ForEach.Expression || got.ForEach.Var != want.ForEach.Var {
		t.Fatalf("@for-each = %+v, want %+v\n%s", got.ForEach, want.ForEach, out)
	}
	if len(got.Applies) != len(want.Applies) {
		t.Fatalf("applies = %d, want %d\n%s", len(got.Applies), len(want.Applies), out)
	}
	for i := range want.Applies {
		if strings.Join(got.Applies[i].Uses, ",") != strings.Join(want.Applies[i].Uses, ",") ||
			got.Applies[i].Expression != want.Applies[i].Expression {
			t.Fatalf("apply %d = %+v, want %+v\n%s", i, got.Applies[i], want.Applies[i], out)
		}
	}
	if len(got.Captures) != 1 || got.Captures[0].Expression != want.Captures[0].Expression {
		t.Fatalf("captures = %+v, want %+v\n%s", got.Captures, want.Captures, out)
	}
	if len(got.Asserts) != len(want.Asserts) {
		t.Fatalf("asserts = %d, want %d\n%s", len(got.Asserts), len(want.Asserts), out)
	}
	for i := range want.Asserts {
		if got.Asserts[i].Expression != want.Asserts[i].Expression ||
			got.Asserts[i].Message != want.Asserts[i].Message {
			t.Fatalf("assert %d = %+v, want %+v\n%s", i, got.Asserts[i], want.Asserts[i], out)
		}
	}

	if again := mustRender(t, back); again != out {
		t.Fatalf("render is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
}

func TestRenderRoundTripsSkipIfAndQuotedAssertMessage(t *testing.T) {
	src := `### r
# @skip-if env == "prod"
# @assert status == 200 => "a \"quoted\" note"
GET https://example.com/
`
	doc := parser.Parse("r.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("source did not parse: %v", doc.Errors)
	}
	want := doc.Requests[0].Metadata

	out := mustRender(t, doc)
	if !strings.Contains(out, directive.SkipIf.Comment()+" ") {
		t.Fatalf("negated condition lost its @skip-if spelling:\n%s", out)
	}

	back := parser.Parse("r.http", []byte(out))
	if len(back.Errors) != 0 {
		t.Fatalf("rendered document did not parse: %v\n%s", back.Errors, out)
	}
	got := back.Requests[0].Metadata
	if got.When == nil || !got.When.Negate || got.When.Expression != want.When.Expression {
		t.Fatalf("@skip-if = %+v, want %+v\n%s", got.When, want.When, out)
	}
	if got.Asserts[0].Message != want.Asserts[0].Message {
		t.Fatalf("message = %q, want %q\n%s", got.Asserts[0].Message, want.Asserts[0].Message, out)
	}
	if again := mustRender(t, back); again != out {
		t.Fatalf("render is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
}

func TestRenderRoundTripsAssertMessageWhitespace(t *testing.T) {
	src := "### r\n# @assert status == 200 => \" padded \"\nGET https://example.com/\n"
	doc := parser.Parse("r.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("source did not parse: %v", doc.Errors)
	}
	want := doc.Requests[0].Metadata.Asserts[0].Message
	if want != " padded " {
		t.Fatalf("parsed message = %q, want %q", want, " padded ")
	}

	out := mustRender(t, doc)
	back := parser.Parse("r.http", []byte(out))
	if len(back.Errors) != 0 {
		t.Fatalf("rendered document did not parse: %v\n%s", back.Errors, out)
	}
	if got := back.Requests[0].Metadata.Asserts[0].Message; got != want {
		t.Fatalf("message = %q, want %q\n%s", got, want, out)
	}
	if again := mustRender(t, back); again != out {
		t.Fatalf("render is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
}
