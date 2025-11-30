package ui

import (
	"net/http"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestWorkflowStatsSearchUsesRenderedContent(t *testing.T) {
	model := New(Config{})
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane")
	}
	pane.viewport.Width = 60
	pane.activeTab = responseTabStats

	view := &workflowStatsView{
		name:        "wf",
		started:     time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
		ended:       time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
		totalSteps:  1,
		entries:     []workflowStatsEntry{{index: 0, result: workflowStepResult{Step: restfile.WorkflowStep{Name: "Call"}, Success: true, HTTP: &httpclient.Response{Status: "200 OK", StatusCode: 200, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"hello":"world"}`)}}}},
		selected:    0,
		expanded:    map[int]bool{0: true},
		renderCache: make(map[int]workflowStatsRender),
	}

	pane.snapshot = &responseSnapshot{
		id:            "wf-1",
		statsKind:     statsReportKindWorkflow,
		workflowStats: view,
		stats:         "Workflow: wf\nStarted: 2024-01-01T00:00:00Z\nSteps: 1\n1. Call [PASS]\n",
		statsColorize: true,
		ready:         true,
	}

	status := statusFromCmd(t, model.applyResponseSearch("hello", false))
	if status == nil {
		t.Fatal("expected search status")
	}
	if status.level != statusInfo {
		t.Fatalf("expected info status, got %v", status.level)
	}
	if len(pane.search.matches) == 0 {
		t.Fatalf("expected match in rendered workflow stats view")
	}
}
