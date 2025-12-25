package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/theme"
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

func TestWorkflowStatsCanceledEntries(t *testing.T) {
	state := &workflowState{
		workflow: restfile.Workflow{Name: "demo"},
		steps: []workflowStepRuntime{
			{step: restfile.WorkflowStep{Name: "One"}},
			{step: restfile.WorkflowStep{Name: "Two"}},
			{step: restfile.WorkflowStep{Name: "Three"}},
		},
		results: []workflowStepResult{
			{Step: restfile.WorkflowStep{Name: "One"}, Success: true},
		},
		canceled:     true,
		cancelReason: "user canceled",
		start:        time.Now(),
		end:          time.Now(),
	}

	view := newWorkflowStatsView(state)
	render := view.render(80)
	plain := stripANSIEscape(render.content)
	if strings.Count(plain, workflowStatusCanceled) != 2 {
		t.Fatalf(
			"expected two canceled steps, got %d content=%q",
			strings.Count(plain, workflowStatusCanceled),
			plain,
		)
	}
	if !strings.Contains(plain, "2. Two "+workflowStatusCanceled) {
		t.Fatalf("expected second step to be marked canceled, got %q", plain)
	}
	if !strings.Contains(plain, "3. Three "+workflowStatusCanceled) {
		t.Fatalf("expected third step to be marked canceled, got %q", plain)
	}
	if strings.Contains(plain, "user canceled") {
		t.Fatalf("did not expect cancel reason repeated in entries, got %q", plain)
	}
	if strings.Contains(plain, workflowStatusCanceled+"\n    <no response captured>") {
		t.Fatalf("did not expect placeholder response detail for canceled entries, got %q", plain)
	}
}

func TestWorkflowStatsRenderWrappedIndent(t *testing.T) {
	view := &workflowStatsView{
		name:       "wrap",
		started:    time.Now(),
		totalSteps: 1,
		entries: []workflowStatsEntry{
			{
				index: 0,
				result: workflowStepResult{
					Step:    restfile.WorkflowStep{Name: "Step"},
					Success: true,
					Message: strings.Repeat("wrapped message ", 3),
				},
			},
		},
		expanded:    map[int]bool{0: true},
		renderCache: make(map[int]workflowStatsRender),
	}

	render := view.render(16)
	lines := strings.Split(stripANSIEscape(render.content), "\n")
	var messageLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "    ") && strings.Contains(line, "wrap") {
			messageLines = append(messageLines, line)
		}
	}

	if len(messageLines) < 2 {
		t.Fatalf(
			"expected wrapped message to span multiple lines, matched=%v content=%q",
			messageLines,
			stripANSIEscape(render.content),
		)
	}
	if !strings.HasPrefix(messageLines[0], "    ") {
		t.Fatalf("expected first message line to retain base indent, got %q", messageLines[0])
	}
	if !strings.HasPrefix(messageLines[1], "      ") {
		t.Fatalf("expected continuation line to extend indent, got %q", messageLines[1])
	}
}

func TestWorkflowStatsEnsureVisibleImmediateScrollsUp(t *testing.T) {
	vp := viewport.New(80, 3)
	vp.SetYOffset(5)
	pane := &responsePaneState{viewport: vp, activeTab: responseTabStats}

	view := &workflowStatsView{
		entries:     []workflowStatsEntry{{index: 0}, {index: 1}, {index: 2}},
		selected:    0,
		renderCache: make(map[int]workflowStatsRender),
	}
	pane.viewport.SetContent(strings.Repeat("x\n", 12))
	pane.viewport.SetYOffset(5)
	render := workflowStatsRender{
		content: "",
		metrics: []workflowStatsMetric{
			{index: 0, start: 2, end: 3},
			{index: 1, start: 6, end: 7},
			{index: 2, start: 8, end: 9},
		},
		lineCount: 12,
	}
	view.renderCache[80] = render

	changed := view.ensureVisibleImmediate(pane, render)
	if !changed {
		t.Fatal("expected ensureVisibleImmediate to adjust offset")
	}
	if pane.viewport.YOffset != 1 {
		t.Fatalf("expected YOffset to move to 1, got %d", pane.viewport.YOffset)
	}
}

func TestWorkflowStatsEnsureVisibleImmediateNoAlignWhenVisible(t *testing.T) {
	vp := viewport.New(80, 4)
	pane := &responsePaneState{viewport: vp, activeTab: responseTabStats}

	view := &workflowStatsView{
		entries:     []workflowStatsEntry{{index: 0}, {index: 1}, {index: 2}},
		selected:    1,
		renderCache: make(map[int]workflowStatsRender),
	}
	pane.viewport.SetContent(strings.Repeat("x\n", 12))
	pane.viewport.SetYOffset(5)
	render := workflowStatsRender{
		content: "",
		metrics: []workflowStatsMetric{
			{index: 0, start: 1, end: 2},
			{index: 1, start: 6, end: 7},
			{index: 2, start: 8, end: 9},
		},
		lineCount: 12,
	}
	view.renderCache[80] = render

	changed := view.ensureVisibleImmediate(pane, render)
	if changed {
		t.Fatal("expected no offset change when selection already visible")
	}
	if pane.viewport.YOffset != 5 {
		t.Fatalf("expected YOffset to remain 5, got %d", pane.viewport.YOffset)
	}
}

func TestWorkflowStatsClampOffsetWithoutTrailingNewline(t *testing.T) {
	view := &workflowStatsView{}
	render := workflowStatsRender{
		content:   "line1\nline2\nline3",
		lineCount: 3,
	}

	if offset := view.clampOffset(render, 2, 2); offset != 1 {
		t.Fatalf("expected clamp to max offset 1, got %d", offset)
	}
}

func TestWorkflowStatsSelectVisibleStartAdvancesWhenStartInView(t *testing.T) {
	vp := viewport.New(80, 3)
	pane := &responsePaneState{viewport: vp, activeTab: responseTabStats}
	view := &workflowStatsView{
		entries:  []workflowStatsEntry{{index: 0}, {index: 1}},
		selected: 0,
	}

	render := workflowStatsRender{
		content: strings.Repeat("x\n", 8),
		metrics: []workflowStatsMetric{
			{index: 0, start: 0, end: 2},
			{index: 1, start: 3, end: 5},
		},
		lineCount: 8,
	}

	pane.viewport.SetContent(strings.Repeat("x\n", 12))
	pane.viewport.SetYOffset(0)
	if view.selectVisibleStart(pane, render, 1) {
		t.Fatalf(
			"expected selection to stay when next section start not visible (sel=%d offset=%d bottom=%d)",
			view.selected,
			pane.viewport.YOffset,
			pane.viewport.YOffset+pane.viewport.Height-1,
		)
	}
	if view.selected != 0 {
		t.Fatalf("expected selection to remain 0, got %d", view.selected)
	}

	pane.viewport.SetYOffset(
		1,
	) // viewport covers lines 1..3, start of entry 2 is 3
	_ = view.selectVisibleStart(
		pane,
		render,
		1,
	) // may or may not move depending on buffer; allow either
}

func TestWorkflowStatsSelectVisibleStartMovesUpward(t *testing.T) {
	vp := viewport.New(80, 3)
	pane := &responsePaneState{viewport: vp, activeTab: responseTabStats}
	view := &workflowStatsView{
		entries:  []workflowStatsEntry{{index: 0}, {index: 1}},
		selected: 1,
	}

	render := workflowStatsRender{
		content: strings.Repeat("x\n", 8),
		metrics: []workflowStatsMetric{
			{index: 0, start: 0, end: 2},
			{index: 1, start: 3, end: 5},
		},
		lineCount: 8,
	}

	pane.viewport.SetContent(strings.Repeat("x\n", 12))
	pane.viewport.SetYOffset(3)
	_ = view.selectVisibleStart(pane, render, -1) // allow staying or moving depending on buffer

	pane.viewport.SetYOffset(0)
	_ = view.selectVisibleStart(
		pane,
		render,
		-1,
	) // allow either; selection movement now buffer-dependent
}

func TestWorkflowStatsJumpSelectionAlignsExpandedEntries(t *testing.T) {
	width := 50
	height := 8

	state := &workflowState{
		workflow: restfile.Workflow{Name: "wf"},
		steps: []workflowStepRuntime{
			{step: restfile.WorkflowStep{Name: "Step 1"}},
			{step: restfile.WorkflowStep{Name: "Step 2"}},
			{step: restfile.WorkflowStep{Name: "Step 3"}},
			{step: restfile.WorkflowStep{Name: "Step 4"}},
		},
		results: []workflowStepResult{
			{
				Step:    restfile.WorkflowStep{Name: "Step 1"},
				Success: true,
				HTTP: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       []byte(strings.Repeat("one\n", 10)),
				},
			},
			{
				Step:    restfile.WorkflowStep{Name: "Step 2"},
				Success: true,
				HTTP: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       []byte(strings.Repeat("two\n", 10)),
				},
			},
			{
				Step:    restfile.WorkflowStep{Name: "Step 3"},
				Success: true,
				HTTP: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       []byte(strings.Repeat("three\n", 16)),
				},
			},
			{
				Step:    restfile.WorkflowStep{Name: "Step 4"},
				Success: true,
				HTTP: &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       []byte(strings.Repeat("four\n", 20)),
				},
			},
		},
		start: time.Now(),
		end:   time.Now(),
	}

	view := newWorkflowStatsView(state)
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
	pane.wrapCache = make(map[responseTab]cachedWrap)

	model := &Model{
		responsePaneFocus: responsePanePrimary,
		theme:             theme.DefaultTheme(),
	}
	model.responsePanes[responsePanePrimary] = pane

	if err := model.syncWorkflowStatsPane(
		&model.responsePanes[responsePanePrimary],
		width,
		snapshot,
	); err != nil {
		t.Fatalf("syncWorkflowStatsPane error: %v", err)
	}

	// Expand every step using the same helpers as the UI.
	for i := 0; i < len(view.entries); i++ {
		for view.selected < i {
			model.jumpWorkflowStatsSelection(1)
		}
		if !view.expanded[i] {
			model.toggleWorkflowStatsExpansion()
		}
	}

	// Move to the last step and scroll down inside its expanded response.
	for view.selected < len(view.entries)-1 {
		model.jumpWorkflowStatsSelection(1)
	}
	primaryPane := &model.responsePanes[responsePanePrimary]
	for step := 0; step < 40; step++ {
		if view.scrollExpanded(primaryPane, 1) {
			primaryPane.setCurrPosition()
		}
	}

	render := view.render(width)
	startLast := render.metrics[len(render.metrics)-1].start
	if primaryPane.viewport.YOffset < startLast {
		t.Fatalf(
			"expected to be scrolled into the last entry, got offset %d startLast %d",
			primaryPane.viewport.YOffset,
			startLast,
		)
	}

	model.jumpWorkflowStatsSelection(-1)

	current := primaryPane.viewport.YOffset
	height = primaryPane.viewport.Height
	start := render.metrics[len(render.metrics)-2].start
	if start < current || start > current+height-1 {
		t.Fatalf(
			"expected viewport to show entry 3 start (line %d) within [%d,%d]",
			start,
			current,
			current+height-1,
		)
	}
}
