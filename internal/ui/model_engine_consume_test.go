package ui

import (
	"net/http"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestResponseMsgFromRunStateUsesEngineExplainAndCarriesRuntimeSecrets(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/items",
	}
	exp := testRunExplain(req, "dev", xplain.StatusReady, "HTTP request sent")
	res := engine.RequestResult{
		Response: compareHTTPResponse("https://example.com/items", []byte(`{"ok":true}`)),
		Executed: req,
		RuntimeSecrets: []string{
			"runtime-token",
			"Bearer runtime-token",
		},
		Environment: "dev",
		Explain:     exp,
	}

	msg := model.responseMsgFromRunState(res, false)
	if msg.explain != exp {
		t.Fatalf("expected engine explain report to be preserved")
	}
	if len(msg.runtimeSecrets) != 2 {
		t.Fatalf("expected runtime secrets to be preserved, got %d", len(msg.runtimeSecrets))
	}
}

func TestHandleResponseMessageRunHTTPKeepsCurrentTabAndExposesExplain(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/items",
	}
	res := engine.RequestResult{
		Response:    compareHTTPResponse("https://example.com/items", []byte(`{"ok":true}`)),
		Executed:    req,
		Environment: "dev",
		Explain:     testRunExplain(req, "dev", xplain.StatusReady, "HTTP request sent"),
	}
	msg := model.responseMsgFromRunState(res, false)

	cmd := model.handleResponseMessage(msg)
	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary response pane")
	}
	if pane.activeTab != responseTabPretty {
		t.Fatalf("expected active tab to remain Pretty, got %v", pane.activeTab)
	}
	if model.responseLatest == nil || model.responseLatest.explain.report == nil {
		t.Fatalf("expected latest snapshot to carry explain report")
	}
	if !containsResponseTab(model.availableResponseTabs(), responseTabExplain) {
		t.Fatalf("expected explain tab to be available")
	}
	if cmd == nil {
		t.Fatalf("expected HTTP response render command")
	}
}

func TestHandleResponseMessageSkippedRunKeepsCurrentTabAndExposesExplain(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/items",
	}
	msg := model.responseMsgFromRunState(engine.RequestResult{
		Executed:    req,
		Environment: "dev",
		Skipped:     true,
		SkipReason:  "Condition evaluated to false.",
		Explain:     testRunExplain(req, "dev", xplain.StatusSkipped, "Condition evaluated to false."),
	}, false)

	cmd := model.handleResponseMessage(msg)
	collectMsgs(cmd)

	pane := model.pane(responsePanePrimary)
	if pane == nil {
		t.Fatalf("expected primary response pane")
	}
	if pane.activeTab != responseTabPretty {
		t.Fatalf("expected active tab to remain Pretty, got %v", pane.activeTab)
	}
	if model.responseLatest == nil || model.responseLatest.explain.report == nil {
		t.Fatalf("expected latest snapshot to carry explain report for skipped request")
	}
	if !containsResponseTab(model.availableResponseTabs(), responseTabExplain) {
		t.Fatalf("expected explain tab to be available for skipped request")
	}
}

func compareHTTPResponse(url string, body []byte) *httpclient.Response {
	return &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": []string{"application/json"}},
		Body:         append([]byte(nil), body...),
		EffectiveURL: url,
	}
}

func testRunExplain(
	req *restfile.Request,
	env string,
	status xplain.Status,
	decision string,
) *xplain.Report {
	rep := &xplain.Report{
		Status:   status,
		Decision: decision,
	}
	if req == nil {
		return rep
	}
	rep.Method = strings.TrimSpace(req.Method)
	rep.URL = strings.TrimSpace(req.URL)
	rep.Env = strings.TrimSpace(env)
	if st, ok := testExplainStageStatus(status); ok {
		rep.Stages = []xplain.Stage{{
			Name:    explainStageHTTPPrepare,
			Status:  st,
			Summary: explainSummaryHTTPRequestPrepared,
		}}
	}
	rep.Final = &xplain.Final{
		Mode:     "send",
		Protocol: "http",
		Method:   strings.TrimSpace(req.Method),
		URL:      strings.TrimSpace(req.URL),
	}
	return rep
}

func testExplainStageStatus(status xplain.Status) (xplain.StageStatus, bool) {
	switch status {
	case xplain.StatusReady:
		return xplain.StageOK, true
	case xplain.StatusSkipped:
		return xplain.StageSkipped, true
	case xplain.StatusError:
		return xplain.StageError, true
	default:
		return "", false
	}
}
