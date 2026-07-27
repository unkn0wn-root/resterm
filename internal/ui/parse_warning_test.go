package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// A run in flight owns the status message. Parse warnings live in their own
// segment, so nothing may queue them into the message and take the progress
// text with it.
func TestRunWarningQueueLeavesParseWarningsAlone(t *testing.T) {
	doc := parser.Parse(
		"warn.http",
		[]byte("### r\n# @sse max-event=5\nGET http://x\n"),
	)
	if len(doc.Warnings) == 0 {
		t.Fatal("expected the document to carry a warning")
	}

	model := New(Config{})
	model.width = 140
	model.doc = doc
	model.sending = true
	model.statusPulseOn = true
	// The run starters set the pulse base directly, the way startWorkflowRun does.
	model.statusPulseBase = "Sending GET /x"
	model.setStatusMessage(statusMsg{text: "Sending GET /x", level: statusInfo})

	rq := &uiRequestEngine{Engine: nil, model: &model}
	_, _ = rq.ExecuteWith(doc, nil, "", rqeng.ExecOptions{})

	select {
	case queued := <-model.runMsgChan:
		t.Fatalf("parse warning must not reach the status message: %#v", queued)
	default:
	}
	if model.statusPulseBase != "Sending GET /x" {
		t.Fatalf("pulse base = %q, want the progress text kept", model.statusPulseBase)
	}
	if text, _ := model.statusBarMessage(); text != "Sending GET /x" {
		t.Fatalf("status message = %q, want the progress text kept", text)
	}
	if !strings.Contains(stripANSIEscape(model.renderStatusBar()), "WARN line 2") {
		t.Fatalf("segment lost the warning:\n%s", stripANSIEscape(model.renderStatusBar()))
	}
}

// An engine warning still goes to the message on purpose. It describes the run
// rather than the file, and it has no segment of its own.
func TestRunWarningQueueStillCarriesEngineWarnings(t *testing.T) {
	model := New(Config{})
	rq := &uiRequestEngine{model: &model}

	rq.queueWarning(string(rqeng.WarningSSHHostKeyVerificationDisabled))
	rq.queueWarning(string(rqeng.WarningSSHHostKeyVerificationDisabled))

	msg := receiveRunWarning(t, model.runMsgChan)
	if msg.text != string(rqeng.WarningSSHHostKeyVerificationDisabled) {
		t.Fatalf("warning = %q", msg.text)
	}
	select {
	case extra := <-model.runMsgChan:
		t.Fatalf("warning repeated within one run: %#v", extra)
	default:
	}
}

// A run's completion status replaces whatever the status bar message was, and
// the status bar never clears itself, so the warning gets its own segment.
func TestStatusBarWarningSectionSurvivesTheRunStatus(t *testing.T) {
	model := New(Config{})
	model.statusMessage = statusMsg{}
	model.width = 200
	model.doc = parser.Parse(
		"warn.http",
		[]byte("### r\n# @websocket compresion=true\nWS wss://x\n"),
	)
	if len(model.doc.Warnings) == 0 {
		t.Fatal("expected the document to carry a warning")
	}
	palette := statusBarPalette(model.theme.StatusBarPalette)

	section, ok := model.statusBarWarningSection(palette)
	if !ok {
		t.Fatal("expected a warning section")
	}
	if section.text != "WARN line 2" {
		t.Fatalf("section text = %q, want %q", section.text, "WARN line 2")
	}

	// The result claims the message text, which is why the warning is not there.
	model.statusMessage = statusMsg{text: "200 OK 12ms", level: statusSuccess}
	if text, _ := model.statusBarMessage(); text != "200 OK 12ms" {
		t.Fatalf("message = %q, want the completion text", text)
	}
	if _, ok := model.statusBarWarningSection(palette); !ok {
		t.Fatal("warning section must outlive the run status")
	}
	if !strings.Contains(model.renderStatusBar(), "WARN line 2") {
		t.Fatalf("rendered status bar dropped the warning:\n%s", model.renderStatusBar())
	}
}

// A startup message such as "Using environment" claims the message text, so the
// segment has to be independent of it.
func TestStatusBarWarningSectionShowsAtStartup(t *testing.T) {
	src := "### r\n# @websocket compresion=true\nWS wss://x\n"
	for _, cfg := range []Config{
		{FilePath: "warn.http", InitialContent: src},
		{FilePath: "warn.http", InitialContent: src, EnvironmentFallback: "dev"},
	} {
		model := New(cfg)
		model.width = 200
		if !strings.Contains(model.renderStatusBar(), "WARN line 2") {
			t.Fatalf("startup status bar omits the warning:\n%s", model.renderStatusBar())
		}
	}
}

func TestStatusBarWarningLabelCountsTheRest(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "clean file",
			src:  "### r\nGET http://x\n",
			want: "",
		},
		{
			name: "one warning",
			src:  "### r\n# @websocket compresion=true\nWS wss://x\n",
			want: "WARN line 2",
		},
		{
			name: "several warnings",
			src:  "### r\n# @websocket compresion=true\n# @ssh host=h userr=bob\nWS wss://x\n",
			want: "WARN line 2 +1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := parser.Parse("warn.http", []byte(tt.src))
			if got := parseWarningLabel(doc); got != tt.want {
				t.Fatalf("parseWarningLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusBarWarningLabelHandlesNilDocument(t *testing.T) {
	if got := parseWarningLabel(nil); got != "" {
		t.Fatalf("parseWarningLabel(nil) = %q, want empty", got)
	}
}

// Every step's report repeats the document warnings. Labelling each copy with a
// step name would make one typo look like several problems.
func TestWorkflowExplainCarriesDocumentWarningsOnce(t *testing.T) {
	warn := `api.http:2: unknown @sse option "max-event"`
	state := &workflowState{
		workflow: restfile.Workflow{Name: "demo"},
		warnings: []string{warn},
		results: []workflowStepResult{
			{
				Step:    restfile.WorkflowStep{Name: "One", Using: "One"},
				Explain: &xplain.Report{Warnings: []string{warn}},
			},
			{
				Step:    restfile.WorkflowStep{Name: "Two", Using: "Two"},
				Explain: &xplain.Report{Warnings: []string{warn, "step only warning"}},
			},
		},
	}

	got := state.explainWarnings(buildWorkflowStatsEntries(state))

	count := 0
	for _, item := range got {
		if strings.Contains(item, "max-event") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("document warning appears %d times in %v, want once", count, got)
	}
	if got[0] != warn {
		t.Fatalf("first warning = %q, want the unlabelled document warning %q", got[0], warn)
	}
	if !slices.Contains(got, "Two: step only warning") {
		t.Fatalf("step scoped warning lost its label: %v", got)
	}
}

// A workflow where nothing executed has no step reports at all, so the run level
// copy is the only thing that can carry the warning.
func TestWorkflowExplainCarriesWarningsWithoutAnyStepReport(t *testing.T) {
	warn := `api.http:2: unknown @sse option "max-event"`
	state := &workflowState{
		workflow: restfile.Workflow{Name: "demo"},
		warnings: []string{warn},
		results: []workflowStepResult{{
			Step:    restfile.WorkflowStep{Name: "One", Using: "One"},
			Skipped: true,
		}},
	}

	rep := state.explainReport()
	if rep == nil {
		t.Fatal("expected an explain report")
	}
	if !slices.Contains(rep.Warnings, warn) {
		t.Fatalf("warnings = %v, want %q", rep.Warnings, warn)
	}
}

func receiveRunWarning(t *testing.T, ch chan tea.Msg) runWarningMsg {
	t.Helper()
	select {
	case queued := <-ch:
		msg, ok := queued.(runWarningMsg)
		if !ok {
			t.Fatalf("queued message type = %T, want runWarningMsg", queued)
		}
		return msg
	default:
		t.Fatal("expected a queued warning")
		return runWarningMsg{}
	}
}

// The top level Warnings list is not the only place a warning can surface. Every
// step's report also carries the document warnings, and the step notes used to
// repeat them once per step.
func TestWorkflowExplainReportDoesNotRepeatWarningsInStepNotes(t *testing.T) {
	warn := `api.http:2: unknown @sse option "max-event"`
	state := &workflowState{
		workflow: restfile.Workflow{Name: "demo"},
		warnings: []string{warn},
		results: []workflowStepResult{
			{
				Step:    restfile.WorkflowStep{Name: "One", Using: "One"},
				Status:  "200 OK",
				Explain: &xplain.Report{Warnings: []string{warn}},
			},
			{
				Step:   restfile.WorkflowStep{Name: "Two", Using: "Two"},
				Status: "200 OK",
				Explain: &xplain.Report{
					Warnings: []string{warn, "step only warning"},
				},
			},
			{
				Step:    restfile.WorkflowStep{Name: "Three", Using: "Three"},
				Status:  "200 OK",
				Explain: &xplain.Report{Warnings: []string{warn}},
			},
		},
	}

	rep := state.explainReport()
	if rep == nil {
		t.Fatal("expected an explain report")
	}

	total := countOccurrences(rep.Warnings, "max-event")
	for _, stage := range rep.Stages {
		total += countOccurrences(stage.Notes, "max-event")
	}
	if total != 1 {
		t.Fatalf("document warning appears %d times across the report, want once", total)
	}

	// A warning that really belongs to one step keeps its place in that step.
	stepOnly := 0
	for _, stage := range rep.Stages {
		stepOnly += countOccurrences(stage.Notes, "step only warning")
	}
	if stepOnly != 1 {
		t.Fatalf("step scoped warning appears %d times, want once", stepOnly)
	}
}

// Editing does not reparse, so the segment must not describe text that is no
// longer in the buffer, and must not stay silent about one that now is.
func TestStatusBarWarningSectionTracksTheEditorRevision(t *testing.T) {
	warned := "### r\n# @sse max-event=5\nGET http://x\n"
	clean := "### r\nGET http://x\n"

	t.Run("stale warning is hidden after an edit", func(t *testing.T) {
		model := New(Config{FilePath: "warn.http", InitialContent: warned})
		model.width = 200
		if !strings.Contains(stripANSIEscape(model.renderStatusBar()), "WARN line 2") {
			t.Fatal("expected the warning before editing")
		}

		model.editor.SetValue(clean)
		model.markDirty()

		if strings.Contains(stripANSIEscape(model.renderStatusBar()), "WARN") {
			t.Fatalf("stale warning survived the edit:\n%s", stripANSIEscape(model.renderStatusBar()))
		}
	})

	t.Run("a new warning is claimed only after a reparse", func(t *testing.T) {
		model := New(Config{FilePath: "warn.http", InitialContent: clean})
		model.width = 200

		model.editor.SetValue(warned)
		model.markDirty()
		if strings.Contains(stripANSIEscape(model.renderStatusBar()), "WARN") {
			t.Fatal("segment must not describe an unparsed buffer")
		}

		model.refreshCurrentDocument([]byte(model.editor.Value()))
		if !strings.Contains(stripANSIEscape(model.renderStatusBar()), "WARN line 2") {
			t.Fatalf("reparse did not restore the warning:\n%s", stripANSIEscape(model.renderStatusBar()))
		}
	})

	// Running an unsaved buffer reparses it, so the segment is accurate again
	// even though the file is still dirty.
	t.Run("running an unsaved buffer refreshes it", func(t *testing.T) {
		model := New(Config{FilePath: "warn.http", InitialContent: clean})
		model.width = 200
		model.editor.SetValue(warned)
		model.markDirty()

		model.setDocument(parser.Parse("warn.http", []byte(model.editor.Value())))

		if !model.dirty {
			t.Fatal("expected the buffer to still be dirty")
		}
		if !strings.Contains(stripANSIEscape(model.renderStatusBar()), "WARN line 2") {
			t.Fatalf("segment stayed hidden after a run:\n%s", stripANSIEscape(model.renderStatusBar()))
		}
	})
}

func countOccurrences(items []string, want string) int {
	n := 0
	for _, item := range items {
		if strings.Contains(item, want) {
			n++
		}
	}
	return n
}
