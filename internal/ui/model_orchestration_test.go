package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/history"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestCompareRunProgressionPinsReferenceAndBuildsBundle(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{}
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/items",
		Metadata: restfile.RequestMetadata{
			Name: "Items",
		},
	}
	spec := &restfile.CompareSpec{
		Environments: []string{"dev", "stage"},
		Baseline:     "dev",
	}

	cmd := m.startCompareRun(doc, req, spec, httpclient.Options{})
	if cmd == nil {
		t.Fatal("expected compare run to return command")
	}
	if m.compareRun == nil {
		t.Fatal("expected compare run state")
	}
	if !m.compareRun.core {
		t.Fatal("expected standard compare run to use core events")
	}
	if !m.responseSplit {
		t.Fatal("expected compare to enable response split")
	}
	if got := m.compareRun.currentEnv; got != "dev" {
		t.Fatalf("expected first env dev before first event, got %q", got)
	}
	if m.compareRun.current == nil {
		t.Fatal("expected first compare request to be active before first event")
	}
	run := core.RunMeta{ID: m.compareRun.id, Mode: core.ModeCompare}
	at := time.Unix(1, 0)

	applyRunEvt(t, &m, core.RunStart{Meta: core.NewMeta(run, at)})
	applyRunEvt(t, &m, core.CmpRowStart{
		Meta: core.NewMeta(run, at),
		Row:  core.RowMeta{Index: 0, Env: "dev", Base: true, Total: 2},
		Doc:  doc,
		Request: &restfile.Request{
			Method: "GET",
			URL:    "https://example.com/items",
			Metadata: restfile.RequestMetadata{
				Name: "Items",
			},
		},
	})
	if got := m.compareRun.currentEnv; got != "dev" {
		t.Fatalf("expected first env dev after row start, got %q", got)
	}
	first := cloneRequest(m.compareRun.current)
	applyRunEvt(t, &m, core.CmpRowDone{
		Meta: core.NewMeta(run, at.Add(10*time.Millisecond)),
		Row:  core.RowMeta{Index: 0, Env: "dev", Base: true, Total: 2},
		Result: engine.RequestResult{
			Response:    testHTTPResp("https://example.com/items", 200, `{"env":"dev"}`, 10*time.Millisecond),
			Executed:    first,
			RequestText: renderRequestText(first),
			Environment: "dev",
		},
	})

	if m.compareRun == nil {
		t.Fatal("expected compare run to continue after first row")
	}
	if m.compareRun.current != nil {
		t.Fatal("expected no active compare request between row events")
	}
	applyRunEvt(t, &m, core.CmpRowStart{
		Meta: core.NewMeta(run, at.Add(11*time.Millisecond)),
		Row:  core.RowMeta{Index: 1, Env: "stage", Total: 2},
		Doc:  doc,
		Request: &restfile.Request{
			Method: "GET",
			URL:    "https://example.com/items",
			Metadata: restfile.RequestMetadata{
				Name: "Items",
			},
		},
	})
	if got := m.compareRun.currentEnv; got != "stage" {
		t.Fatalf("expected second env stage, got %q", got)
	}
	if m.compareRun.current == nil {
		t.Fatal("expected second compare request to be active")
	}
	devSnap := m.compareSnapshot("dev")
	if devSnap == nil {
		t.Fatal("expected dev snapshot to be stored after first row")
	}
	secondary := m.pane(responsePaneSecondary)
	if secondary == nil {
		t.Fatal("expected secondary pane")
	}
	if secondary.followLatest {
		t.Fatal("expected secondary pane to stay pinned during compare")
	}
	if secondary.snapshot != devSnap {
		t.Fatal("expected secondary pane to pin first row snapshot")
	}

	second := cloneRequest(m.compareRun.current)
	applyRunEvt(t, &m, core.CmpRowDone{
		Meta: core.NewMeta(run, at.Add(23*time.Millisecond)),
		Row:  core.RowMeta{Index: 1, Env: "stage", Total: 2},
		Result: engine.RequestResult{
			Response: testHTTPResp(
				"https://example.com/items",
				200,
				`{"env":"stage"}`,
				12*time.Millisecond,
			),
			Executed:    second,
			RequestText: renderRequestText(second),
			Environment: "stage",
		},
	})

	if m.compareRun != nil {
		t.Fatal("expected compare run to finalize after last row")
	}
	if m.compareBundle == nil {
		t.Fatal("expected compare bundle to be built")
	}
	if got := m.compareBundle.Baseline; got != "dev" {
		t.Fatalf("expected compare baseline dev, got %q", got)
	}
	if got := len(m.compareBundle.Rows); got != 2 {
		t.Fatalf("expected 2 compare rows, got %d", got)
	}
	if got := m.compareSelectedEnv; got != "dev" {
		t.Fatalf("expected selected env dev, got %q", got)
	}
	if got := m.compareFocusedEnv; got != "dev" {
		t.Fatalf("expected focused env dev, got %q", got)
	}
	if got := m.compareRowIndex; got != 0 {
		t.Fatalf("expected compare row index 0, got %d", got)
	}
	if m.compareSnapshot("stage") == nil {
		t.Fatal("expected stage snapshot to be stored after final row")
	}
	if !secondary.followLatest {
		t.Fatal("expected secondary pane to follow latest after compare finalizes")
	}
	if secondary.snapshot != m.responseLatest {
		t.Fatal("expected secondary pane to track latest snapshot after finalize")
	}
}

func TestCompareRunCancelAfterFirstRowPreservesCanceledState(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{}
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/items",
		Metadata: restfile.RequestMetadata{
			Name: "Items",
		},
	}
	spec := &restfile.CompareSpec{
		Environments: []string{"dev", "stage"},
		Baseline:     "dev",
	}

	cmd := m.startCompareRun(doc, req, spec, httpclient.Options{})
	if cmd == nil {
		t.Fatal("expected compare run to return command")
	}
	if m.compareRun == nil || !m.compareRun.core {
		t.Fatal("expected core compare run state")
	}

	run := core.RunMeta{ID: m.compareRun.id, Mode: core.ModeCompare}
	at := time.Unix(2, 0)
	applyRunEvt(t, &m, core.RunStart{Meta: core.NewMeta(run, at)})
	applyRunEvt(t, &m, core.CmpRowStart{
		Meta: core.NewMeta(run, at),
		Row:  core.RowMeta{Index: 0, Env: "dev", Base: true, Total: 2},
		Doc:  doc,
		Request: &restfile.Request{
			Method: "GET",
			URL:    "https://example.com/items",
			Metadata: restfile.RequestMetadata{
				Name: "Items",
			},
		},
	})
	if m.compareRun.current == nil {
		t.Fatal("expected active compare request before cancel")
	}

	m.cancelActiveRuns()
	if m.compareRun == nil || !m.compareRun.canceled {
		t.Fatal("expected compare run to be marked canceled")
	}

	cur := cloneRequest(m.compareRun.current)
	applyRunEvt(t, &m, core.CmpRowDone{
		Meta: core.NewMeta(run, at.Add(time.Millisecond)),
		Row:  core.RowMeta{Index: 0, Env: "dev", Base: true, Total: 2},
		Result: engine.RequestResult{
			Err:         context.Canceled,
			Executed:    cur,
			RequestText: renderRequestText(cur),
			Environment: "dev",
		},
	})
	if m.compareRun != nil {
		t.Fatal("expected compare run to finalize after canceled row")
	}
	if m.compareBundle == nil {
		t.Fatal("expected compare bundle after cancel")
	}
	if got := len(m.compareBundle.Rows); got != 1 {
		t.Fatalf("expected one compare row after cancel, got %d", got)
	}
	if got := m.compareBundle.Rows[0].Summary; got != "canceled" {
		t.Fatalf("expected canceled compare summary, got %q", got)
	}
	if !strings.Contains(m.statusMessage.text, "canceled") {
		t.Fatalf("expected canceled compare status, got %q", m.statusMessage.text)
	}
}

func TestProfileRunWarmupProgressAdvancesToMeasuredStage(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{}
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Name: "ProfileItems",
			Profile: &restfile.ProfileSpec{
				Count:  2,
				Warmup: 1,
				Delay:  time.Second,
			},
		},
	}

	cmd := m.startProfileRun(doc, req, httpclient.Options{})
	if cmd == nil {
		t.Fatal("expected profile run to return command")
	}
	if m.profileRun == nil || m.profileRun.current == nil {
		t.Fatal("expected profile run to start first iteration")
	}
	if !m.profileRun.core {
		t.Fatal("expected standard profile run to use core events")
	}
	if !strings.Contains(m.statusMessage.text, "warmup 1/1") {
		t.Fatalf("expected warmup status, got %q", m.statusMessage.text)
	}

	run := core.RunMeta{ID: m.profileRun.id, Mode: core.ModeProfile}
	at := time.Unix(3, 0)
	applyRunEvt(t, &m, core.RunStart{Meta: core.NewMeta(run, at)})
	cur := cloneRequest(m.profileRun.current)
	applyRunEvt(t, &m, core.ProIterStart{
		Meta: core.NewMeta(run, at),
		Iter: core.IterMeta{
			Index:       0,
			Total:       3,
			Warmup:      true,
			WarmupIndex: 1,
			WarmupTotal: 1,
			RunTotal:    2,
			Delay:       time.Second,
		},
		Doc:     doc,
		Request: cur,
	})
	applyRunEvt(t, &m, core.ProIterDone{
		Meta: core.NewMeta(run, at.Add(15*time.Millisecond)),
		Iter: core.IterMeta{
			Index:       0,
			Total:       3,
			Warmup:      true,
			WarmupIndex: 1,
			WarmupTotal: 1,
			RunTotal:    2,
			Delay:       time.Second,
		},
		Result: engine.RequestResult{
			Response:    testHTTPResp("https://example.com/profile", 200, `{"ok":true}`, 15*time.Millisecond),
			Executed:    cur,
			RequestText: renderRequestText(cur),
		},
	})

	if m.profileRun == nil {
		t.Fatal("expected profile run to remain active")
	}
	if got := m.profileRun.index; got != 1 {
		t.Fatalf("expected profile index 1 after warmup, got %d", got)
	}
	if m.profileRun.current != nil {
		t.Fatal("expected no active iteration while waiting for profile delay")
	}
	if !strings.Contains(m.statusMessage.text, "run 1/2") {
		t.Fatalf("expected measured progress status, got %q", m.statusMessage.text)
	}
	if !strings.Contains(m.statusPulseBase, "run 1/2") {
		t.Fatalf("expected pulse base to follow measured progress, got %q", m.statusPulseBase)
	}
}

func TestProfileRunCancelWhileActiveFinalizesSummary(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{}
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Name: "ProfileItems",
			Profile: &restfile.ProfileSpec{
				Count: 2,
			},
		},
	}

	cmd := m.startProfileRun(doc, req, httpclient.Options{})
	if cmd == nil {
		t.Fatal("expected profile run to return command")
	}
	if m.profileRun == nil || m.profileRun.current == nil {
		t.Fatal("expected active profile iteration")
	}
	if !m.profileRun.core {
		t.Fatal("expected standard profile run to use core events")
	}

	run := core.RunMeta{ID: m.profileRun.id, Mode: core.ModeProfile}
	at := time.Unix(4, 0)
	applyRunEvt(t, &m, core.RunStart{Meta: core.NewMeta(run, at)})
	cur := cloneRequest(m.profileRun.current)
	applyRunEvt(t, &m, core.ProIterStart{
		Meta: core.NewMeta(run, at),
		Iter: core.IterMeta{
			Index:    0,
			Total:    2,
			RunIndex: 1,
			RunTotal: 2,
		},
		Doc:     doc,
		Request: cur,
	})
	m.cancelActiveRuns()
	if m.profileRun == nil || !m.profileRun.canceled {
		t.Fatal("expected profile run to be marked canceled")
	}

	applyRunEvt(t, &m, core.ProIterDone{
		Meta: core.NewMeta(run, at.Add(5*time.Millisecond)),
		Iter: core.IterMeta{
			Index:    0,
			Total:    2,
			RunIndex: 1,
			RunTotal: 2,
		},
		Result: engine.RequestResult{
			Err:         context.Canceled,
			Executed:    cur,
			RequestText: renderRequestText(cur),
		},
	})

	if m.profileRun != nil {
		t.Fatal("expected profile run to finalize after canceled response")
	}
	if m.responseLatest == nil {
		t.Fatal("expected profile snapshot after cancel")
	}
	if got := m.responseLatest.statsKind; got != statsReportKindProfile {
		t.Fatalf("expected profile stats kind, got %v", got)
	}
	if !strings.Contains(m.responseLatest.pretty, "Profiling canceled after 1/2 runs") {
		t.Fatalf("expected canceled summary in profile snapshot, got %q", m.responseLatest.pretty)
	}
	if !strings.Contains(m.statusMessage.text, "Profiling canceled after 1/2 runs") {
		t.Fatalf("expected canceled summary status, got %q", m.statusMessage.text)
	}
}

func TestProfileRunCancelWhileIdleBetweenRunsFinalizesImmediately(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{}
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Name: "ProfileItems",
			Profile: &restfile.ProfileSpec{
				Count:  2,
				Warmup: 1,
				Delay:  time.Second,
			},
		},
	}

	cmd := m.startProfileRun(doc, req, httpclient.Options{})
	if cmd == nil {
		t.Fatal("expected profile run command")
	}
	if m.profileRun == nil || !m.profileRun.core {
		t.Fatal("expected core profile run state")
	}

	run := core.RunMeta{ID: m.profileRun.id, Mode: core.ModeProfile}
	at := time.Unix(5, 0)
	cur := cloneRequest(m.profileRun.current)
	applyRunEvt(t, &m, core.ProIterDone{
		Meta: core.NewMeta(run, at.Add(10*time.Millisecond)),
		Iter: core.IterMeta{
			Index:       0,
			Total:       3,
			Warmup:      true,
			WarmupIndex: 1,
			WarmupTotal: 1,
			RunTotal:    2,
			Delay:       time.Second,
		},
		Result: engine.RequestResult{
			Response:    testHTTPResp("https://example.com/profile", 200, `{"ok":true}`, 10*time.Millisecond),
			Executed:    cur,
			RequestText: renderRequestText(cur),
		},
	})
	if m.profileRun == nil {
		t.Fatal("expected profile run to remain active during delay")
	}
	if m.profileRun.current != nil {
		t.Fatal("expected no active request while waiting for next delayed profile run")
	}

	m.cancelActiveRuns()

	if m.profileRun != nil {
		t.Fatal("expected idle profile cancel to finalize immediately")
	}
	if m.responseLatest == nil {
		t.Fatal("expected profile snapshot after idle cancel")
	}
	if !strings.Contains(m.responseLatest.pretty, "Profiling canceled after 1/3 runs") {
		t.Fatalf("expected idle canceled summary in profile snapshot, got %q", m.responseLatest.pretty)
	}
	if !strings.Contains(m.statusMessage.text, "Profiling canceled after 1/3 runs") {
		t.Fatalf("expected idle canceled summary status, got %q", m.statusMessage.text)
	}
}

func TestProfileRunInteractiveWebSocketFallsBackToUIDrivenPath(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{}
	req := &restfile.Request{
		Method:    "GET",
		URL:       "wss://example.com/chat",
		WebSocket: &restfile.WebSocketRequest{},
		Metadata: restfile.RequestMetadata{
			Name: "chat",
			Profile: &restfile.ProfileSpec{
				Count: 1,
			},
		},
	}

	cmd := m.startProfileRun(doc, req, httpclient.Options{})
	if cmd == nil {
		t.Fatal("expected profile run command")
	}
	if m.profileRun == nil {
		t.Fatal("expected profile run state")
	}
	if m.profileRun.core {
		t.Fatal("expected interactive websocket profile to use UI-driven TUI path")
	}
	if m.profileRun.current == nil {
		t.Fatal("expected UI-driven profile path to start interactive request immediately")
	}
}

func TestWorkflowRunBranchAndLoopProgression(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{
		Variables: []restfile.Variable{{
			Name:  "items",
			Value: `["a","b"]`,
		}},
		Requests: []*restfile.Request{
			{
				Method: "GET",
				URL:    "https://example.com/branch",
				Metadata: restfile.RequestMetadata{
					Name: "branch",
				},
			},
			{
				Method: "GET",
				URL:    "https://example.com/items/{{vars.request.item}}",
				Metadata: restfile.RequestMetadata{
					Name: "loop",
				},
			},
		},
	}
	wf := restfile.Workflow{
		Name: "demo",
		Steps: []restfile.WorkflowStep{
			{
				Kind: restfile.WorkflowStepKindIf,
				Name: "Choose",
				If: &restfile.WorkflowIf{
					Then: restfile.WorkflowIfBranch{
						Cond: "true",
						Run:  "branch",
					},
				},
			},
			{
				Kind:  restfile.WorkflowStepKindForEach,
				Name:  "Each",
				Using: "loop",
				ForEach: &restfile.WorkflowForEach{
					Expr: `json.parse(vars.require("items"))`,
					Var:  "item",
				},
			},
		},
	}
	pl, err := core.PrepareWorkflow(doc, wf, core.RunMeta{ID: "wf-1", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}
	m.workflowRun = workflowStateFromPlan(pl, httpclient.Options{}, true)
	at := time.Unix(10, 0)

	applyRunEvt(t, &m, core.RunStart{Meta: core.NewMeta(pl.Run, at)})
	applyRunEvt(t, &m, core.WfStepStart{
		Meta: core.NewMeta(pl.Run, at),
		Step: core.StepMeta{
			Index:  0,
			Name:   "Choose",
			Kind:   restfile.WorkflowStepKindIf,
			Branch: "branch",
		},
		Doc:     doc,
		Request: doc.Requests[0],
	})
	applyRunEvt(t, &m, core.ReqStart{
		Meta: core.NewMeta(pl.Run, at),
		Req: core.ReqMeta{
			Index: 1,
			Label: "Choose -> branch",
			Env:   "dev",
		},
		Doc:     doc,
		Request: doc.Requests[0],
	})
	if m.workflowRun == nil || m.workflowRun.current == nil {
		t.Fatal("expected workflow to start first request")
	}
	if got := m.workflowRun.currentBranch; got != "branch" {
		t.Fatalf("expected selected branch request, got %q", got)
	}

	first := engine.RequestResult{
		Response:    testHTTPResp("https://example.com/branch", 200, `{"ok":true}`, 8*time.Millisecond),
		Executed:    cloneRequest(doc.Requests[0]),
		RequestText: renderRequestText(doc.Requests[0]),
		Environment: "dev",
	}
	applyRunEvt(t, &m, core.ReqDone{
		Meta:   core.NewMeta(pl.Run, at.Add(8*time.Millisecond)),
		Req:    core.ReqMeta{Index: 1, Label: "Choose -> branch", Env: "dev"},
		Result: first,
	})
	applyRunEvt(t, &m, core.WfStepDone{
		Meta: core.NewMeta(pl.Run, at.Add(8*time.Millisecond)),
		Step: core.StepMeta{
			Index:  0,
			Name:   "Choose",
			Kind:   restfile.WorkflowStepKindIf,
			Branch: "branch",
		},
		Result: first,
	})
	applyRunEvt(t, &m, core.WfStepStart{
		Meta: core.NewMeta(pl.Run, at.Add(9*time.Millisecond)),
		Step: core.StepMeta{
			Index: 1,
			Name:  "Each",
			Kind:  restfile.WorkflowStepKindForEach,
			Iter:  1,
			Total: 2,
		},
		Doc:     doc,
		Request: doc.Requests[1],
	})
	applyRunEvt(t, &m, core.ReqStart{
		Meta: core.NewMeta(pl.Run, at.Add(9*time.Millisecond)),
		Req: core.ReqMeta{
			Index: 2,
			Label: "Each (1/2)",
			Env:   "dev",
		},
		Doc:     doc,
		Request: doc.Requests[1],
	})

	if m.workflowRun == nil {
		t.Fatal("expected workflow to continue after branch step")
	}
	if got := len(m.workflowRun.results); got != 1 {
		t.Fatalf("expected first workflow result to be recorded, got %d", got)
	}
	if got := m.workflowRun.results[0].Branch; got != "branch" {
		t.Fatalf("expected workflow result to record chosen branch, got %q", got)
	}
	if m.workflowRun.loop == nil {
		t.Fatal("expected workflow to enter for-each loop")
	}
	if m.workflowRun.current == nil {
		t.Fatal("expected first loop iteration to start immediately")
	}

	second := engine.RequestResult{
		Response:    testHTTPResp("https://example.com/items/a", 200, `{"item":"a"}`, 9*time.Millisecond),
		Executed:    cloneRequest(doc.Requests[1]),
		RequestText: "GET https://example.com/items/a\n",
		Environment: "dev",
	}
	applyRunEvt(t, &m, core.ReqDone{
		Meta:   core.NewMeta(pl.Run, at.Add(18*time.Millisecond)),
		Req:    core.ReqMeta{Index: 2, Label: "Each (1/2)", Env: "dev"},
		Result: second,
	})
	applyRunEvt(t, &m, core.WfStepDone{
		Meta: core.NewMeta(pl.Run, at.Add(18*time.Millisecond)),
		Step: core.StepMeta{
			Index: 1,
			Name:  "Each",
			Kind:  restfile.WorkflowStepKindForEach,
			Iter:  1,
			Total: 2,
		},
		Result: second,
	})
	applyRunEvt(t, &m, core.WfStepStart{
		Meta: core.NewMeta(pl.Run, at.Add(19*time.Millisecond)),
		Step: core.StepMeta{
			Index: 1,
			Name:  "Each",
			Kind:  restfile.WorkflowStepKindForEach,
			Iter:  2,
			Total: 2,
		},
		Doc:     doc,
		Request: doc.Requests[1],
	})
	applyRunEvt(t, &m, core.ReqStart{
		Meta: core.NewMeta(pl.Run, at.Add(19*time.Millisecond)),
		Req: core.ReqMeta{
			Index: 3,
			Label: "Each (2/2)",
			Env:   "dev",
		},
		Doc:     doc,
		Request: doc.Requests[1],
	})

	if m.workflowRun == nil || m.workflowRun.current == nil {
		t.Fatal("expected workflow to continue to second loop iteration")
	}
	if got := len(m.workflowRun.results); got != 2 {
		t.Fatalf("expected two workflow results after first loop iteration, got %d", got)
	}
	if got := m.workflowRun.results[1].Iteration; got != 1 {
		t.Fatalf("expected first loop result iteration 1, got %d", got)
	}
	if got := m.workflowRun.results[1].Total; got != 2 {
		t.Fatalf("expected loop total 2, got %d", got)
	}

	third := engine.RequestResult{
		Response:    testHTTPResp("https://example.com/items/b", 200, `{"item":"b"}`, 11*time.Millisecond),
		Executed:    cloneRequest(doc.Requests[1]),
		RequestText: "GET https://example.com/items/b\n",
		Environment: "dev",
	}
	applyRunEvt(t, &m, core.ReqDone{
		Meta:   core.NewMeta(pl.Run, at.Add(30*time.Millisecond)),
		Req:    core.ReqMeta{Index: 3, Label: "Each (2/2)", Env: "dev"},
		Result: third,
	})
	applyRunEvt(t, &m, core.WfStepDone{
		Meta: core.NewMeta(pl.Run, at.Add(30*time.Millisecond)),
		Step: core.StepMeta{
			Index: 1,
			Name:  "Each",
			Kind:  restfile.WorkflowStepKindForEach,
			Iter:  2,
			Total: 2,
		},
		Result: third,
	})
	applyRunEvt(t, &m, core.RunDone{
		Meta:    core.NewMeta(pl.Run, at.Add(31*time.Millisecond)),
		Success: true,
	})

	if m.workflowRun != nil {
		t.Fatal("expected workflow to finalize after last loop iteration")
	}
	if m.responseLatest == nil || m.responseLatest.workflowStats == nil {
		t.Fatal("expected workflow stats snapshot after finalize")
	}
	view := m.responseLatest.workflowStats
	if got := len(view.entries); got != 3 {
		t.Fatalf("expected three workflow stats entries, got %d", got)
	}
	if got := view.entries[0].result.Branch; got != "branch" {
		t.Fatalf("expected stats to retain branch label, got %q", got)
	}
	if got := view.entries[1].result.Iteration; got != 1 {
		t.Fatalf("expected first loop stats entry iteration 1, got %d", got)
	}
	if got := view.entries[2].result.Iteration; got != 2 {
		t.Fatalf("expected second loop stats entry iteration 2, got %d", got)
	}
	if !strings.Contains(m.statusMessage.text, "completed: 3/3 steps passed") {
		t.Fatalf("expected workflow completion summary, got %q", m.statusMessage.text)
	}
}

func TestWorkflowRunCancelMarksRemainingStepsCanceled(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{
		Requests: []*restfile.Request{
			{
				Method: "GET",
				URL:    "https://example.com/one",
				Metadata: restfile.RequestMetadata{
					Name: "one",
				},
			},
			{
				Method: "GET",
				URL:    "https://example.com/two",
				Metadata: restfile.RequestMetadata{
					Name: "two",
				},
			},
		},
	}
	wf := restfile.Workflow{
		Name: "demo",
		Steps: []restfile.WorkflowStep{
			{Name: "One", Using: "one"},
			{Name: "Two", Using: "two"},
		},
	}
	pl, err := core.PrepareWorkflow(doc, wf, core.RunMeta{ID: "wf-2", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}
	m.workflowRun = workflowStateFromPlan(pl, httpclient.Options{}, true)
	at := time.Unix(20, 0)

	applyRunEvt(t, &m, core.RunStart{Meta: core.NewMeta(pl.Run, at)})
	applyRunEvt(t, &m, core.WfStepStart{
		Meta: core.NewMeta(pl.Run, at),
		Step: core.StepMeta{
			Index: 0,
			Name:  "One",
			Kind:  restfile.WorkflowStepKindRequest,
		},
		Doc:     doc,
		Request: doc.Requests[0],
	})
	applyRunEvt(t, &m, core.ReqStart{
		Meta: core.NewMeta(pl.Run, at),
		Req: core.ReqMeta{
			Index: 1,
			Label: "One",
			Env:   "dev",
		},
		Doc:     doc,
		Request: doc.Requests[0],
	})
	if m.workflowRun == nil || m.workflowRun.current == nil {
		t.Fatal("expected active workflow request")
	}

	m.cancelActiveRuns()
	if m.workflowRun == nil || !m.workflowRun.canceled {
		t.Fatal("expected workflow run to be marked canceled")
	}

	res := engine.RequestResult{
		Err:         context.Canceled,
		Executed:    cloneRequest(doc.Requests[0]),
		Environment: "dev",
	}
	applyRunEvt(t, &m, core.ReqDone{
		Meta:   core.NewMeta(pl.Run, at.Add(time.Millisecond)),
		Req:    core.ReqMeta{Index: 1, Label: "One", Env: "dev"},
		Result: res,
	})
	applyRunEvt(t, &m, core.WfStepDone{
		Meta: core.NewMeta(pl.Run, at.Add(time.Millisecond)),
		Step: core.StepMeta{
			Index: 0,
			Name:  "One",
			Kind:  restfile.WorkflowStepKindRequest,
		},
		Result: res,
	})
	applyRunEvt(t, &m, core.RunDone{
		Meta:     core.NewMeta(pl.Run, at.Add(2*time.Millisecond)),
		Canceled: true,
	})

	if m.workflowRun != nil {
		t.Fatal("expected workflow to finalize after canceled response")
	}
	if m.responseLatest == nil || m.responseLatest.workflowStats == nil {
		t.Fatal("expected workflow stats after cancel")
	}
	view := m.responseLatest.workflowStats
	if got := len(view.entries); got != 2 {
		t.Fatalf("expected canceled workflow to show two steps, got %d", got)
	}
	for i, entry := range view.entries {
		if !entry.result.Canceled {
			t.Fatalf("expected entry %d to be marked canceled", i)
		}
	}
	if !strings.Contains(m.statusMessage.text, "canceled at step 1/2") {
		t.Fatalf("expected canceled workflow summary, got %q", m.statusMessage.text)
	}
}

func TestWorkflowRunKeepsSpinnerActiveUntilRunDone(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{
		Requests: []*restfile.Request{{
			Method: "GET",
			URL:    "https://example.com/one",
			Metadata: restfile.RequestMetadata{
				Name: "one",
			},
		}},
	}
	wf := restfile.Workflow{
		Name: "demo",
		Steps: []restfile.WorkflowStep{{
			Name:  "One",
			Using: "one",
		}},
	}
	pl, err := core.PrepareWorkflow(doc, wf, core.RunMeta{ID: "wf-spin", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}
	m.workflowRun = workflowStateFromPlan(pl, httpclient.Options{}, true)
	at := time.Unix(25, 0)

	applyRunEvt(t, &m, core.RunStart{Meta: core.NewMeta(pl.Run, at)})
	applyRunEvt(t, &m, core.WfStepStart{
		Meta: core.NewMeta(pl.Run, at),
		Step: core.StepMeta{
			Index: 0,
			Name:  "One",
			Kind:  restfile.WorkflowStepKindRequest,
		},
		Doc:     doc,
		Request: doc.Requests[0],
	})
	applyRunEvt(t, &m, core.ReqStart{
		Meta: core.NewMeta(pl.Run, at),
		Req: core.ReqMeta{
			Index: 1,
			Label: "One",
			Env:   "dev",
		},
		Doc:     doc,
		Request: doc.Requests[0],
	})
	if !m.sending {
		t.Fatal("expected workflow spinner to start with the request")
	}

	res := engine.RequestResult{
		Response:    testHTTPResp("https://example.com/one", 200, `{"ok":true}`, 9*time.Millisecond),
		Executed:    cloneRequest(doc.Requests[0]),
		RequestText: "GET https://example.com/one\n",
		Environment: "dev",
	}
	applyRunEvt(t, &m, core.ReqDone{
		Meta:   core.NewMeta(pl.Run, at.Add(9*time.Millisecond)),
		Req:    core.ReqMeta{Index: 1, Label: "One", Env: "dev"},
		Result: res,
	})
	if !m.sending {
		t.Fatal("expected workflow spinner to remain active after request completion")
	}
	if m.responseLatest == nil {
		t.Fatal("expected workflow response snapshot before finalize")
	}

	applyRunEvt(t, &m, core.WfStepDone{
		Meta: core.NewMeta(pl.Run, at.Add(9*time.Millisecond)),
		Step: core.StepMeta{
			Index: 0,
			Name:  "One",
			Kind:  restfile.WorkflowStepKindRequest,
		},
		Result: res,
	})
	if !m.sending {
		t.Fatal("expected workflow spinner to stay active until run done")
	}

	applyRunEvt(t, &m, core.RunDone{
		Meta:    core.NewMeta(pl.Run, at.Add(10*time.Millisecond)),
		Success: true,
	})
	if m.workflowRun != nil {
		t.Fatal("expected workflow to finalize")
	}
	if m.sending {
		t.Fatal("expected workflow spinner to stop after run done")
	}
}

func TestWorkflowUIDrivenResponseKeepsSpinnerActiveBetweenSteps(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{
		Requests: []*restfile.Request{
			{
				Method: "GET",
				URL:    "https://example.com/one",
				Metadata: restfile.RequestMetadata{
					Name: "one",
				},
			},
			{
				Method: "GET",
				URL:    "https://example.com/two",
				Metadata: restfile.RequestMetadata{
					Name: "two",
				},
			},
		},
	}
	wf := restfile.Workflow{
		Name: "demo",
		Steps: []restfile.WorkflowStep{
			{Name: "One", Using: "one"},
			{Name: "Two", Using: "two"},
		},
	}
	pl, err := core.PrepareWorkflow(doc, wf, core.RunMeta{ID: "wf-ui-spin", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}
	m.doc = doc
	m.workflowRun = workflowStateFromPlan(pl, httpclient.Options{}, false)
	m.workflowRun.current = cloneRequest(doc.Requests[0])
	m.workflowRun.stepStart = time.Unix(26, 0)
	m.sending = true

	res := engine.RequestResult{
		Response:    testHTTPResp("https://example.com/one", 200, `{"ok":true}`, 11*time.Millisecond),
		Executed:    cloneRequest(doc.Requests[0]),
		RequestText: "GET https://example.com/one\n",
		Environment: "dev",
		Explain:     testRunExplain(doc.Requests[0], "dev", xplain.StatusReady, "HTTP request sent"),
	}
	_ = m.handleWorkflowUIDrivenResponse(m.responseMsgFromRunState(res, false))

	if m.workflowRun == nil {
		t.Fatal("expected workflow to continue after first UI-driven response")
	}
	if !m.sending {
		t.Fatal("expected UI-driven workflow spinner to remain active between steps")
	}
	if got := m.workflowRun.index; got != 1 {
		t.Fatalf("expected workflow to advance to second step, got %d", got)
	}
	if len(m.workflowRun.results) != 1 || m.workflowRun.results[0].Explain == nil {
		t.Fatal("expected UI-driven workflow result to retain explain report")
	}
}

func TestWorkflowRunFinalExplainAggregatesAllSteps(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{
		Requests: []*restfile.Request{
			{
				Method: "GET",
				URL:    "https://example.com/one",
				Metadata: restfile.RequestMetadata{
					Name: "one",
				},
			},
			{
				Method: "GET",
				URL:    "https://example.com/two",
				Metadata: restfile.RequestMetadata{
					Name: "two",
				},
			},
		},
	}
	wf := restfile.Workflow{
		Name: "demo",
		Steps: []restfile.WorkflowStep{
			{Name: "One", Using: "one"},
			{Name: "Two", Using: "two"},
		},
	}
	pl, err := core.PrepareWorkflow(doc, wf, core.RunMeta{ID: "wf-explain", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareWorkflow: %v", err)
	}
	m.workflowRun = workflowStateFromPlan(pl, httpclient.Options{}, true)
	at := time.Unix(27, 0)

	applyRunEvt(t, &m, core.RunStart{Meta: core.NewMeta(pl.Run, at)})
	applyRunEvt(t, &m, core.WfStepStart{
		Meta: core.NewMeta(pl.Run, at),
		Step: core.StepMeta{
			Index: 0,
			Name:  "One",
			Kind:  restfile.WorkflowStepKindRequest,
		},
		Doc:     doc,
		Request: doc.Requests[0],
	})
	applyRunEvt(t, &m, core.ReqStart{
		Meta: core.NewMeta(pl.Run, at),
		Req:  core.ReqMeta{Index: 1, Label: "One", Env: "dev"},
		Doc:  doc, Request: doc.Requests[0],
	})
	first := engine.RequestResult{
		Response:    testHTTPResp("https://example.com/one", 200, `{"one":true}`, 8*time.Millisecond),
		Executed:    cloneRequest(doc.Requests[0]),
		RequestText: "GET https://example.com/one\n",
		Environment: "dev",
		Explain:     testRunExplain(doc.Requests[0], "dev", xplain.StatusReady, "HTTP request sent"),
	}
	applyRunEvt(t, &m, core.ReqDone{
		Meta:   core.NewMeta(pl.Run, at.Add(8*time.Millisecond)),
		Req:    core.ReqMeta{Index: 1, Label: "One", Env: "dev"},
		Result: first,
	})
	applyRunEvt(t, &m, core.WfStepDone{
		Meta: core.NewMeta(pl.Run, at.Add(8*time.Millisecond)),
		Step: core.StepMeta{
			Index: 0,
			Name:  "One",
			Kind:  restfile.WorkflowStepKindRequest,
		},
		Result: first,
	})
	applyRunEvt(t, &m, core.WfStepStart{
		Meta: core.NewMeta(pl.Run, at.Add(9*time.Millisecond)),
		Step: core.StepMeta{
			Index: 1,
			Name:  "Two",
			Kind:  restfile.WorkflowStepKindRequest,
		},
		Doc:     doc,
		Request: doc.Requests[1],
	})
	applyRunEvt(t, &m, core.ReqStart{
		Meta: core.NewMeta(pl.Run, at.Add(9*time.Millisecond)),
		Req:  core.ReqMeta{Index: 2, Label: "Two", Env: "dev"},
		Doc:  doc, Request: doc.Requests[1],
	})
	second := engine.RequestResult{
		Response:    testHTTPResp("https://example.com/two", 200, `{"two":true}`, 9*time.Millisecond),
		Executed:    cloneRequest(doc.Requests[1]),
		RequestText: "GET https://example.com/two\n",
		Environment: "dev",
		Explain:     testRunExplain(doc.Requests[1], "dev", xplain.StatusReady, "HTTP request sent"),
	}
	applyRunEvt(t, &m, core.ReqDone{
		Meta:   core.NewMeta(pl.Run, at.Add(18*time.Millisecond)),
		Req:    core.ReqMeta{Index: 2, Label: "Two", Env: "dev"},
		Result: second,
	})
	applyRunEvt(t, &m, core.WfStepDone{
		Meta: core.NewMeta(pl.Run, at.Add(18*time.Millisecond)),
		Step: core.StepMeta{
			Index: 1,
			Name:  "Two",
			Kind:  restfile.WorkflowStepKindRequest,
		},
		Result: second,
	})
	applyRunEvt(t, &m, core.RunDone{
		Meta:    core.NewMeta(pl.Run, at.Add(19*time.Millisecond)),
		Success: true,
	})

	if m.responseLatest == nil || m.responseLatest.explain.report == nil {
		t.Fatal("expected final workflow snapshot to carry explain report")
	}
	out := renderExplainReport(m.responseLatest.explain.report)
	if !strings.Contains(out, "Pipeline: 2 ok") {
		t.Fatalf("expected workflow explain to aggregate both steps, got %q", out)
	}
	if !strings.Contains(out, "One / HTTP Request [ok]") ||
		!strings.Contains(out, "Two / HTTP Request [ok]") {
		t.Fatalf("expected workflow explain to include both step labels, got %q", out)
	}
}

func TestWorkflowExplainReportCarriesPerStepChangesAndVars(t *testing.T) {
	state := &workflowState{
		workflow: restfile.Workflow{Name: "demo"},
		steps: []workflowStepRuntime{{
			step: restfile.WorkflowStep{Name: "Authenticate"},
		}},
		results: []workflowStepResult{{
			Step:    restfile.WorkflowStep{Name: "Authenticate"},
			Success: true,
			Status:  "200 OK",
			Explain: &xplain.Report{
				Env: "dev",
				Vars: []xplain.Var{{
					Name:   "base_url",
					Source: "env",
					Value:  "https://httpbin.org",
					Uses:   1,
				}},
				Stages: []xplain.Stage{
					{
						Name:    explainStageAuth,
						Status:  xplain.StageOK,
						Summary: "auth prepared",
						Changes: []xplain.Change{{
							Field:  "header.Authorization",
							Before: "",
							After:  "•••",
						}},
					},
					{
						Name:    explainStageHTTPPrepare,
						Status:  xplain.StageOK,
						Summary: "HTTP request sent",
						Changes: []xplain.Change{{
							Field:  "url",
							Before: "{{base_url}}/status/200",
							After:  "https://httpbin.org/status/200",
						}},
					},
				},
			},
		}},
	}

	out := renderExplainReport(workflowExplainReport(state))
	if !strings.Contains(out, "Pipeline: 2 ok") {
		t.Fatalf("expected flattened workflow pipeline, got %q", out)
	}
	if !strings.Contains(out, "Variables: 1 resolved") {
		t.Fatalf("expected workflow explain to merge request vars, got %q", out)
	}
	if !strings.Contains(out, "Authenticate / Authentication [ok]") {
		t.Fatalf("expected authentication stage in workflow explain, got %q", out)
	}
	if !strings.Contains(out, "Authenticate / HTTP Request [ok]") {
		t.Fatalf("expected HTTP stage in workflow explain, got %q", out)
	}
	if !strings.Contains(out, "set header Authorization = •••") {
		t.Fatalf("expected auth header change in workflow explain, got %q", out)
	}
	if !strings.Contains(out, "change url: {{base_url}}/status/200 -> https://httpbin.org/status/200") {
		t.Fatalf("expected URL expansion change in workflow explain, got %q", out)
	}
}

func TestWorkflowRunInteractiveWebSocketFallsBackToUIDrivenPath(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{
		Requests: []*restfile.Request{{
			Method:    "GET",
			URL:       "wss://example.com/chat",
			WebSocket: &restfile.WebSocketRequest{},
			Metadata: restfile.RequestMetadata{
				Name: "chat",
			},
		}},
	}
	wf := restfile.Workflow{
		Name: "chat-flow",
		Steps: []restfile.WorkflowStep{{
			Name:  "Chat",
			Using: "chat",
		}},
	}

	cmd := m.startWorkflowRun(doc, wf, httpclient.Options{})
	if cmd == nil {
		t.Fatal("expected workflow run command")
	}
	if m.workflowRun == nil {
		t.Fatal("expected workflow run state")
	}
	if m.workflowRun.core {
		t.Fatal("expected interactive websocket workflow to use UI-driven TUI path")
	}
	if m.workflowRun.current == nil {
		t.Fatal("expected UI-driven workflow path to start interactive request immediately")
	}
}

func TestCompareRunInteractiveWebSocketFallsBackToUIDrivenPath(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	doc := &restfile.Document{}
	req := &restfile.Request{
		Method:    "GET",
		URL:       "wss://example.com/chat",
		WebSocket: &restfile.WebSocketRequest{},
		Metadata: restfile.RequestMetadata{
			Name: "chat",
		},
	}
	spec := &restfile.CompareSpec{
		Environments: []string{"dev", "stage"},
		Baseline:     "dev",
	}

	cmd := m.startCompareRun(doc, req, spec, httpclient.Options{})
	if cmd == nil {
		t.Fatal("expected compare run command")
	}
	if m.compareRun == nil {
		t.Fatal("expected compare run state")
	}
	if m.compareRun.core {
		t.Fatal("expected interactive websocket compare to use UI-driven TUI path")
	}
	if got := m.compareRun.currentEnv; got != "dev" {
		t.Fatalf("expected UI-driven compare path to start first env immediately, got %q", got)
	}
	if m.compareRun.current == nil {
		t.Fatal("expected UI-driven compare path to start interactive request immediately")
	}
}

func TestForEachRunRecordsPerRequestHistory(t *testing.T) {
	store := histdb.New(filepath.Join(t.TempDir(), "history.db"))
	t.Cleanup(func() { _ = store.Close() })

	m := newOrchTestModel(t, Config{History: store, EnvironmentName: "dev"})
	doc := &restfile.Document{
		Variables: []restfile.Variable{{
			Name:  "items",
			Value: `["a","b"]`,
		}},
		Requests: []*restfile.Request{
			{
				Method: "GET",
				URL:    "https://example.com/items/{{vars.request.item}}",
				Metadata: restfile.RequestMetadata{
					Name: "each",
					ForEach: &restfile.ForEachSpec{
						Expression: `json.parse(vars.require("items"))`,
						Var:        "item",
					},
				},
			},
		},
	}
	req := doc.Requests[0]
	m.doc = doc
	pl, err := core.PrepareForEach(doc, req, core.RunMeta{ID: "each-1", Env: "dev"})
	if err != nil {
		t.Fatalf("PrepareForEach: %v", err)
	}
	m.workflowRun = workflowStateFromPlan(pl, httpclient.Options{}, true)
	at := time.Unix(30, 0)

	applyRunEvt(t, &m, core.RunStart{Meta: core.NewMeta(pl.Run, at)})
	applyRunEvt(t, &m, core.WfStepStart{
		Meta: core.NewMeta(pl.Run, at),
		Step: core.StepMeta{
			Index: 0,
			Name:  pl.Workflow.Name,
			Kind:  restfile.WorkflowStepKindRequest,
			Iter:  1,
			Total: 2,
		},
		Doc:     doc,
		Request: req,
	})
	applyRunEvt(t, &m, core.ReqStart{
		Meta: core.NewMeta(pl.Run, at),
		Req: core.ReqMeta{
			Index: 1,
			Label: "GET each (1/2)",
			Env:   "dev",
		},
		Doc:     doc,
		Request: req,
	})
	if m.workflowRun == nil || m.workflowRun.current == nil {
		t.Fatal("expected first for-each iteration to start")
	}

	first := engine.RequestResult{
		Response:    testHTTPResp("https://example.com/items/a", 200, `{"item":"a"}`, 7*time.Millisecond),
		Executed:    cloneRequest(req),
		RequestText: "GET https://example.com/items/a\n",
		Environment: "dev",
	}
	applyRunEvt(t, &m, core.ReqDone{
		Meta:   core.NewMeta(pl.Run, at.Add(7*time.Millisecond)),
		Req:    core.ReqMeta{Index: 1, Label: "GET each (1/2)", Env: "dev"},
		Result: first,
	})
	applyRunEvt(t, &m, core.WfStepDone{
		Meta: core.NewMeta(pl.Run, at.Add(7*time.Millisecond)),
		Step: core.StepMeta{
			Index: 0,
			Name:  pl.Workflow.Name,
			Kind:  restfile.WorkflowStepKindRequest,
			Iter:  1,
			Total: 2,
		},
		Result: first,
	})
	applyRunEvt(t, &m, core.WfStepStart{
		Meta: core.NewMeta(pl.Run, at.Add(8*time.Millisecond)),
		Step: core.StepMeta{
			Index: 0,
			Name:  pl.Workflow.Name,
			Kind:  restfile.WorkflowStepKindRequest,
			Iter:  2,
			Total: 2,
		},
		Doc:     doc,
		Request: req,
	})
	applyRunEvt(t, &m, core.ReqStart{
		Meta: core.NewMeta(pl.Run, at.Add(8*time.Millisecond)),
		Req: core.ReqMeta{
			Index: 2,
			Label: "GET each (2/2)",
			Env:   "dev",
		},
		Doc:     doc,
		Request: req,
	})

	if m.workflowRun == nil || m.workflowRun.current == nil {
		t.Fatal("expected second for-each iteration to start")
	}

	second := engine.RequestResult{
		Response:    testHTTPResp("https://example.com/items/b", 200, `{"item":"b"}`, 9*time.Millisecond),
		Executed:    cloneRequest(req),
		RequestText: "GET https://example.com/items/b\n",
		Environment: "dev",
	}
	applyRunEvt(t, &m, core.ReqDone{
		Meta:   core.NewMeta(pl.Run, at.Add(17*time.Millisecond)),
		Req:    core.ReqMeta{Index: 2, Label: "GET each (2/2)", Env: "dev"},
		Result: second,
	})
	applyRunEvt(t, &m, core.WfStepDone{
		Meta: core.NewMeta(pl.Run, at.Add(17*time.Millisecond)),
		Step: core.StepMeta{
			Index: 0,
			Name:  pl.Workflow.Name,
			Kind:  restfile.WorkflowStepKindRequest,
			Iter:  2,
			Total: 2,
		},
		Result: second,
	})
	applyRunEvt(t, &m, core.RunDone{
		Meta:    core.NewMeta(pl.Run, at.Add(18*time.Millisecond)),
		Success: true,
	})

	if m.workflowRun != nil {
		t.Fatal("expected for-each run to finalize")
	}

	es, err := store.Entries()
	if err != nil {
		t.Fatalf("entries: %v", err)
	}
	if got := len(es); got != 2 {
		t.Fatalf("expected one history entry per item, got %d", got)
	}
	if hasHistoryMethod(es, restfile.HistoryMethodWorkflow) {
		t.Fatalf("did not expect workflow history entry, got %+v", es)
	}
	if hasHistoryMethod(es, restfile.HistoryMethodCompare) {
		t.Fatalf("did not expect compare history entry, got %+v", es)
	}
	if !hasRequestText(es, "/items/a") || !hasRequestText(es, "/items/b") {
		t.Fatalf("expected per-item request text in history, got %+v", es)
	}
}

func newOrchTestModel(t *testing.T, cfg Config) Model {
	t.Helper()

	m := New(cfg)
	m.ready = true
	m.width = 120
	m.height = 40
	m.frameWidth = 120
	m.frameHeight = 40
	if cmd := m.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}
	return m
}

func testHTTPResp(url string, code int, body string, dur time.Duration) *httpclient.Response {
	return &httpclient.Response{
		Status:       httpStatus(code),
		StatusCode:   code,
		Body:         []byte(body),
		Duration:     dur,
		EffectiveURL: url,
	}
}

func httpStatus(code int) string {
	switch code {
	case 200:
		return "200 OK"
	case 201:
		return "201 Created"
	case 500:
		return "500 Internal Server Error"
	default:
		return "200 OK"
	}
}

func hasHistoryMethod(es []history.Entry, method string) bool {
	for _, e := range es {
		if e.Method == method {
			return true
		}
	}
	return false
}

func hasRequestText(es []history.Entry, want string) bool {
	for _, e := range es {
		if strings.Contains(e.RequestText, want) {
			return true
		}
	}
	return false
}

func applyRunEvt(t *testing.T, m *Model, evt core.Evt) {
	t.Helper()

	next, _ := m.Update(runEvtMsg{evt: evt})
	nm, ok := next.(Model)
	if !ok {
		t.Fatalf("unexpected model type %T", next)
	}
	*m = nm
}
