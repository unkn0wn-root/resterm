package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestWorkflowStatsRenderIndicators(t *testing.T) {
	view := &workflowStatsView{
		name:       "Sample",
		started:    time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC),
		totalSteps: 2,
		entries: []workflowStatsEntry{
			{
				index: 0,
				result: workflowStepResult{
					Step:     restfile.WorkflowStep{Name: "Auth"},
					Success:  true,
					Status:   "200 OK",
					Duration: 1500 * time.Millisecond,
					HTTP: &httpclient.Response{
						Status:     "200 OK",
						StatusCode: 200,
						Body:       []byte(`{"token": "abc"}`),
					},
				},
			},
			{
				index: 1,
				result: workflowStepResult{
					Step:    restfile.WorkflowStep{Name: "Cleanup"},
					Success: false,
					Message: "request failed",
				},
			},
		},
		selected:    0,
		expanded:    make(map[int]bool),
		renderCache: make(map[int]workflowStatsRender),
	}

	render := view.render(80)
	if !strings.Contains(render.content, "[+] 1. Auth") {
		t.Fatalf("expected collapsed indicator for first entry, got %q", render.content)
	}
	if strings.Contains(render.content, "token") {
		t.Fatalf("did not expect response body when collapsed")
	}

	view.toggle()
	render = view.render(80)
	if !strings.Contains(render.content, "token") {
		t.Fatalf("expected expanded detail to include response body, got %q", render.content)
	}

	if !strings.Contains(render.content, "[ ] 2. Cleanup") {
		t.Fatalf("expected placeholder indicator for second entry, got %q", render.content)
	}
	if !strings.Contains(render.content, "<no response captured>") {
		t.Fatalf("expected placeholder detail for entry without response")
	}
}
