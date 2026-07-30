package ui

import (
	"strings"
	"testing"

	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestExecuteRequestConflictReturnsExplainReport(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		SSH:    &restfile.SSHSpec{},
		K8s:    &restfile.K8sSpec{},
	}

	msg := runCmd(model.startRun(runSpec{req: req, sel: testSelection("")}))()
	res, ok := msg.(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg, got %T", msg)
	}
	if res.err == nil {
		t.Fatalf("expected conflict error")
	}
	if res.explain == nil {
		t.Fatalf("expected explain report on route conflict")
	}
	if res.explain.Status != xplain.StatusError {
		t.Fatalf("expected explain error status, got %q", res.explain.Status)
	}
	if len(res.explain.Stages) == 0 || res.explain.Stages[0].Name != "route" {
		t.Fatalf("expected route stage, got %#v", res.explain.Stages)
	}
}

func TestExecuteExplainReturnsPreviewWithoutSending(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.client = nil

	req := &restfile.Request{
		Method: "GET",
		URL:    "{{host}}/api",
		Variables: []restfile.Variable{
			{Name: "host", Value: "https://example.com"},
		},
	}

	msg := runCmd(model.startRun(runSpec{req: req, sel: testSelection(""), mode: rqeng.ExecModePreview}))()
	res, ok := msg.(responseMsg)
	if !ok {
		t.Fatalf("expected responseMsg, got %T", msg)
	}
	if res.err != nil {
		t.Fatalf("expected no preview error, got %v", res.err)
	}
	if !res.preview {
		t.Fatal("expected preview response")
	}
	if res.explain == nil {
		t.Fatal("expected explain report")
	}
	if res.explain.Status != xplain.StatusReady {
		t.Fatalf("expected ready explain status, got %q", res.explain.Status)
	}
	if res.explain.Final == nil {
		t.Fatal("expected final explain snapshot")
	}
	if res.explain.Final.Mode != "prepared" {
		t.Fatalf("expected prepared mode, got %q", res.explain.Final.Mode)
	}
	if res.explain.Final.Protocol != "HTTP" {
		t.Fatalf("expected HTTP protocol, got %q", res.explain.Final.Protocol)
	}
	if res.explain.Final.URL != "https://example.com/api" {
		t.Fatalf("expected expanded preview url, got %q", res.explain.Final.URL)
	}
}

func TestExplainActiveRequestUsesPreviewSpinnerLabel(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.editor.SetValue("GET https://example.com/api\n")

	cmd := model.explainActiveRequest()
	if cmd == nil {
		t.Fatal("expected explain request command")
	}
	if !model.sending {
		t.Fatal("expected explain preview to keep active-run spinner state")
	}
	if model.sendingOverlayBase != responseExplainPreviewBase {
		t.Fatalf(
			"expected explain preview overlay %q, got %q",
			responseExplainPreviewBase,
			model.sendingOverlayBase,
		)
	}
	view := model.sendingView(model.pane(responsePanePrimary), 40, 8)
	if !strings.Contains(view, responseExplainPreviewBase) {
		t.Fatalf(
			"expected explain preview spinner to use %q, got %q",
			responseExplainPreviewBase,
			view,
		)
	}
	if strings.Contains(view, responseSendingBase) {
		t.Fatalf("did not expect explain preview spinner to use %q", responseSendingBase)
	}
}

func TestHandleResponseMessagePreviewOpensExplainTab(t *testing.T) {
	t.Parallel()

	model := New(Config{})
	model.handleResponseMessage(responseMsg{
		preview:     true,
		environment: "dev",
		explain: &xplain.Report{
			Status:   xplain.StatusReady,
			Decision: "Explain preview ready. No request was sent.",
		},
	})
	pane := model.pane(responsePanePrimary)
	if pane == nil || pane.snapshot == nil {
		t.Fatal("expected preview snapshot on primary pane")
	}
	if pane.activeTab != responseTabExplain {
		t.Fatalf("expected explain tab to be active, got %v", pane.activeTab)
	}
	if pane.snapshot.explain.report == nil {
		t.Fatal("expected explain report on preview snapshot")
	}
}
