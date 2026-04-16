package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestWorkflowRunSubjectForEachUsesRequestBaseTitle(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/" + strings.Repeat("segment/", 12),
	}
	st := &workflowState{
		origin: workflowOriginForEach,
		workflow: restfile.Workflow{
			Name: "GET " + req.URL,
		},
		steps: []workflowStepRuntime{{
			request: req,
		}},
	}

	if got, want := workflowRunSubject(st), requestBaseTitle(req); got != want {
		t.Fatalf("workflowRunSubject() = %q, want %q", got, want)
	}
}
