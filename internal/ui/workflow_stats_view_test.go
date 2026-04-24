package ui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestNewWorkflowStatsViewSelectsFirstFailure(t *testing.T) {
	state := &workflowState{
		workflow: restfile.Workflow{Name: "wf"},
		start:    time.Now(),
		steps: []workflowStepRuntime{
			{step: restfile.WorkflowStep{Name: "Auth"}},
			{step: restfile.WorkflowStep{Name: "Verify"}},
			{step: restfile.WorkflowStep{Name: "Cleanup"}},
		},
		results: []workflowStepResult{
			{Step: restfile.WorkflowStep{Name: "Auth"}, Success: true},
			{Step: restfile.WorkflowStep{Name: "Verify"}, Success: false},
			{Step: restfile.WorkflowStep{Name: "Cleanup"}, Canceled: true},
		},
	}

	view := newWorkflowStatsView(state)
	if view.selected != 1 {
		t.Fatalf("expected first failure to be selected, got %d", view.selected)
	}
}

func TestNewWorkflowStatsViewSelectsFirstStepWhenAllPass(t *testing.T) {
	state := &workflowState{
		workflow: restfile.Workflow{Name: "wf"},
		start:    time.Now(),
		steps: []workflowStepRuntime{
			{step: restfile.WorkflowStep{Name: "Auth"}},
			{step: restfile.WorkflowStep{Name: "Verify"}},
		},
		results: []workflowStepResult{
			{Step: restfile.WorkflowStep{Name: "Auth"}, Success: true},
			{Step: restfile.WorkflowStep{Name: "Verify"}, Success: true},
		},
	}

	view := newWorkflowStatsView(state)
	if view.selected != 0 {
		t.Fatalf("expected first step to be selected, got %d", view.selected)
	}
}

func TestWorkflowStatsRenderSplitListAndSelectedDetail(t *testing.T) {
	view := workflowStatsTestView()
	view.selected = 1

	render := view.render(120, 18)
	plain := stripANSIEscape(render.content)

	for _, want := range []string{
		"Workflow wf",
		"Steps",
		"Selected Step",
		"2. Verify",
		"FAIL",
		"nope",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected rendered workflow view to contain %q, got %q", want, plain)
		}
	}
	if strings.Contains(plain, "[+]") || strings.Contains(plain, "[-]") {
		t.Fatalf("did not expect accordion markers in split workflow view: %q", plain)
	}
}

func TestWorkflowStatsEmptyViewIsUnknown(t *testing.T) {
	view := &workflowStatsView{name: "empty"}

	if got := view.overallStatus(); got != "UNKNOWN" {
		t.Fatalf("expected empty workflow status UNKNOWN, got %q", got)
	}
	plain := stripANSIEscape(view.render(80, 10).content)
	if !strings.Contains(plain, "UNKNOWN") {
		t.Fatalf("expected rendered empty workflow to show UNKNOWN, got %q", plain)
	}
	if !strings.Contains(plain, "No workflow steps captured") {
		t.Fatalf("expected empty workflow message, got %q", plain)
	}
}

func TestWorkflowStatsRenderShowsSelectedDetailOnly(t *testing.T) {
	view := workflowStatsTestView()
	view.entries[0].result.HTTP.Body = []byte(`{"selected":"alpha-body"}`)
	view.entries[1].result.HTTP.Body = []byte(`{"hidden":"beta-body"}`)
	view.selected = 0

	render := view.render(120, 18)
	plain := stripANSIEscape(render.content)
	if !strings.Contains(plain, "alpha-body") {
		t.Fatalf("expected selected step detail, got %q", plain)
	}
	if strings.Contains(plain, "beta-body") {
		t.Fatalf("did not expect unselected response body in detail pane, got %q", plain)
	}
}

func TestWorkflowStatsDetailScrollKeepsSelection(t *testing.T) {
	view := workflowStatsTestView()
	view.selected = 0
	view.entries[0].result.HTTP.Body = []byte(strings.Repeat("line\n", 40))

	if !view.scrollDetail(120, 14, 5) {
		t.Fatal("expected detail scroll to move")
	}
	if view.selected != 0 {
		t.Fatalf("expected selected step to remain stable, got %d", view.selected)
	}
	if view.detailOffset == 0 {
		t.Fatal("expected detail offset to advance")
	}
}

func TestWorkflowStatsMoveResetsDetailScroll(t *testing.T) {
	view := workflowStatsTestView()
	view.entries[0].result.HTTP.Body = []byte(strings.Repeat("line\n", 40))
	if !view.scrollDetail(120, 14, 5) {
		t.Fatal("expected detail scroll to move")
	}

	if !view.move(1) {
		t.Fatal("expected selection to move")
	}
	if view.selected != 1 {
		t.Fatalf("expected second step selected, got %d", view.selected)
	}
	if view.detailOffset != 0 {
		t.Fatalf("expected detail offset reset, got %d", view.detailOffset)
	}
}

func TestWorkflowStatsDetailFocusLetsJScrollDetail(t *testing.T) {
	model := New(Config{})
	model.focus = focusResponse
	model.responsePaneFocus = responsePanePrimary
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane")
	}
	pane.viewport = viewport.New(120, 14)
	pane.activeTab = responseTabStats

	view := workflowStatsTestView()
	view.entries[0].result.HTTP.Body = []byte(strings.Repeat("line\n", 40))
	pane.snapshot = &responseSnapshot{
		id:            "wf-focus",
		stats:         "workflow stats",
		statsKind:     statsReportKindWorkflow,
		workflowStats: view,
		ready:         true,
	}

	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		_ = cmd()
	}
	if !view.detailFocus {
		t.Fatal("expected enter to focus selected step detail")
	}
	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}); cmd != nil {
		_ = cmd()
	}
	if view.selected != 0 {
		t.Fatalf("expected j to keep selected step while detail is focused, got %d", view.selected)
	}
	if view.detailOffset == 0 {
		t.Fatal("expected j to scroll selected detail when focused")
	}
	if cmd := model.handleKey(tea.KeyMsg{Type: tea.KeyEsc}); cmd != nil {
		_ = cmd()
	}
	if view.detailFocus {
		t.Fatal("expected esc to return focus to step list")
	}
}

func TestActivateWorkflowStatsViewFocusesResponsePane(t *testing.T) {
	model := New(Config{})
	model.focus = focusWorkflows
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatal("expected response pane")
	}
	view := workflowStatsTestView()
	snapshot := &responseSnapshot{
		id:            "wf-focus",
		stats:         "workflow stats",
		statsKind:     statsReportKindWorkflow,
		workflowStats: view,
		ready:         true,
	}
	pane.snapshot = snapshot
	pane.activeTab = responseTabPretty

	if cmd := model.activateWorkflowStatsView(snapshot); cmd != nil {
		_ = cmd()
	}

	if model.focus != focusResponse {
		t.Fatalf("expected focusResponse after workflow completion, got %v", model.focus)
	}
	if pane.activeTab != responseTabStats {
		t.Fatalf("expected Workflow stats tab active, got %v", pane.activeTab)
	}
}

func TestWorkflowStatsRenderNarrowLayout(t *testing.T) {
	view := workflowStatsTestView()
	view.selected = 1

	render := view.render(60, 18)
	plain := stripANSIEscape(render.content)
	if !strings.Contains(plain, "Steps") || !strings.Contains(plain, "Selected Step") {
		t.Fatalf("expected narrow layout to stack list and detail, got %q", plain)
	}
	if !strings.Contains(plain, "nope") {
		t.Fatalf("expected selected detail in narrow layout, got %q", plain)
	}
}

func TestWorkflowStatsCanceledAndSkippedDetails(t *testing.T) {
	view := &workflowStatsView{
		name:       "wf",
		started:    time.Now(),
		ended:      time.Now(),
		totalSteps: 2,
		entries: []workflowStatsEntry{
			{
				index: 0,
				result: workflowStepResult{
					Step:     restfile.WorkflowStep{Name: "One"},
					Canceled: true,
				},
			},
			{
				index: 1,
				result: workflowStepResult{
					Step:    restfile.WorkflowStep{Name: "Two"},
					Skipped: true,
					Message: "condition was false",
				},
			},
		},
		selected: 0,
	}

	plain := stripANSIEscape(view.render(100, 16).content)
	if !strings.Contains(plain, "Canceled before response capture") {
		t.Fatalf("expected canceled placeholder, got %q", plain)
	}
	view.selected = 1
	plain = stripANSIEscape(view.render(100, 16).content)
	if !strings.Contains(plain, "condition was false") {
		t.Fatalf("expected skipped reason, got %q", plain)
	}
}

func TestWorkflowStatsSyncKeepsOuterViewportStable(t *testing.T) {
	width := 100
	height := 16
	view := workflowStatsTestView()
	snapshot := &responseSnapshot{
		stats:         "workflow stats",
		statsKind:     statsReportKindWorkflow,
		workflowStats: view,
		ready:         true,
	}

	vp := viewport.New(width, height)
	pane := newResponsePaneState(vp, false)
	pane.activeTab = responseTabStats
	pane.snapshot = snapshot
	pane.viewport.SetContent(strings.Repeat("x\n", 40))
	pane.viewport.SetYOffset(12)

	model := &Model{
		responsePaneFocus: responsePanePrimary,
		theme:             theme.DefaultTheme(),
	}
	model.responsePanes[responsePanePrimary] = pane

	if cmd := model.syncWorkflowStatsPane(
		&model.responsePanes[responsePanePrimary],
		width,
		height,
		snapshot,
	); cmd != nil {
		t.Fatalf("expected syncWorkflowStatsPane to be immediate")
	}
	if got := model.responsePanes[responsePanePrimary].viewport.YOffset; got != 0 {
		t.Fatalf("expected workflow stats viewport offset 0, got %d", got)
	}
}

func workflowStatsTestView() *workflowStatsView {
	return &workflowStatsView{
		name:       "wf",
		started:    time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
		ended:      time.Date(2024, time.January, 1, 0, 0, 1, 0, time.UTC),
		totalSteps: 2,
		entries: []workflowStatsEntry{
			{
				index: 0,
				result: workflowStepResult{
					Step:     restfile.WorkflowStep{Name: "Auth"},
					Success:  true,
					Status:   "200 OK",
					Duration: 7 * time.Millisecond,
					HTTP: &httpclient.Response{
						Status:     "200 OK",
						StatusCode: 200,
						Headers:    http.Header{"Content-Type": []string{"application/json"}},
						Body:       []byte(`{"token":"abc"}`),
					},
				},
			},
			{
				index: 1,
				result: workflowStepResult{
					Step:     restfile.WorkflowStep{Name: "Verify"},
					Success:  false,
					Status:   "500 Internal Server Error",
					Duration: 12 * time.Millisecond,
					Message:  "expected 200",
					HTTP: &httpclient.Response{
						Status:     "500 Internal Server Error",
						StatusCode: 500,
						Headers:    http.Header{"Content-Type": []string{"application/json"}},
						Body:       []byte(`{"error":"nope"}`),
					},
				},
			},
		},
		selected: 0,
	}
}
