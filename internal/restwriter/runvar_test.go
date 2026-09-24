package restwriter

import (
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRenderRunVarsRoundTrip(t *testing.T) {
	wfVars := []restfile.RunVar{
		{Name: "suffix", Value: "{{$fake.word}}"},
		{Name: "eq", Value: "=starts with equals"},
	}
	reqVars := []restfile.RunVar{
		{Name: "id", Value: "{{$uuid}}"},
		{Name: "colon", Value: ":starts with colon"},
		{Name: "empty"},
	}
	wf := testWorkflow("flow")
	wf.RunVars = wfVars
	req := testRequest()
	req.RunVars = reqVars
	doc := &restfile.Document{Requests: []*restfile.Request{req}, Workflows: []restfile.Workflow{wf}}

	out, err := Render(doc, Options{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	back := parser.Parse("run.http", []byte(out))
	if len(back.Errors) != 0 {
		t.Fatalf("rendered document does not parse: %v\n%s", back.Errors, out)
	}
	if got := nameValues(back.Workflows[0].RunVars); !slices.Equal(got, nameValues(wfVars)) {
		t.Fatalf("workflow run vars = %v, want %v\n%s", got, nameValues(wfVars), out)
	}
	if got := nameValues(back.Requests[0].RunVars); !slices.Equal(got, nameValues(reqVars)) {
		t.Fatalf("request run vars = %v, want %v\n%s", got, nameValues(reqVars), out)
	}

	again, err := Render(back, Options{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if again != out {
		t.Fatalf("render is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
}

func nameValues(vs []restfile.RunVar) []string {
	out := make([]string, 0, 2*len(vs))
	for _, v := range vs {
		out = append(out, v.Name, v.Value)
	}
	return out
}
