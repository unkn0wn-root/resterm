package restwriter

import (
	"strings"
	"testing"

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
