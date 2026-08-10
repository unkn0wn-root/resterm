package runner

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestSelectByTagSkipsNilRequests(t *testing.T) {
	reqs := []*restfile.Request{
		nil,
		{Metadata: restfile.RequestMetadata{Tags: []string{"slow"}}},
	}

	got, err := selectByTag(reqs, "slow")
	if err != nil {
		t.Fatalf("selectByTag: %v", err)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("selectByTag = %v, want [1]", got)
	}
}

func TestSelectByLineMatchesWorkflowContinuationLines(t *testing.T) {
	doc := parser.Parse("workflow.http", []byte(`# @workflow checkout
# @step login using=Login
# @if contains(
#   ["dev", "stage"],
#   env
# ) run=Charge
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %+v", doc.Errors)
	}

	for line := 1; line <= 6; line++ {
		got, err := selectTarget(doc, selectSpec{line: line})
		if err != nil {
			t.Fatalf("line %d: %v", line, err)
		}
		if !got.hasWorkflow() || got.workflow != 0 {
			t.Fatalf("line %d selected %+v, want workflow 0", line, got)
		}
	}
}
