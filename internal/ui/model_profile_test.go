package ui

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

func TestHandleProfileResponseUpdatesState(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 42
	model.frameWidth = model.width + 2
	model.frameHeight = model.height + 2
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Profile: &restfile.ProfileSpec{Count: 1},
		},
	}

	state := &profileState{
		base:        cloneRequest(req),
		doc:         &restfile.Document{Requests: []*restfile.Request{req}},
		options:     httpclient.Options{},
		spec:        restfile.ProfileSpec{Count: 1},
		total:       1,
		warmup:      0,
		successes:   make([]time.Duration, 0, 1),
		failures:    make([]profileFailure, 0, 1),
		current:     req,
		messageBase: "Profiling " + requestBaseTitle(req),
		start:       time.Now(),
	}
	model.profileRun = state

	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Headers:      http.Header{"Content-Type": []string{"application/json"}},
		Body:         []byte(`{"ok":true}`),
		Duration:     25 * time.Millisecond,
		EffectiveURL: "https://example.com/profile",
	}

	msg := responseMsg{
		response: resp,
		tests: []scripts.TestResult{
			{Name: "status", Passed: true},
		},
		executed: req,
	}

	cmd := model.handleProfileResponse(msg)
	if cmd == nil {
		t.Fatalf("expected profile response handler to schedule render command")
	}
	drainResponseCommands(t, &model, cmd)

	if len(model.testResults) != 1 {
		t.Fatalf("expected test results to be recorded, got %d", len(model.testResults))
	}
	if model.scriptError != nil {
		t.Fatalf("expected script error to be nil, got %v", model.scriptError)
	}
	if model.lastError != nil {
		t.Fatalf("expected lastError to be cleared, got %v", model.lastError)
	}
	if model.responseLatest == nil {
		t.Fatalf("expected latest response snapshot to be populated")
	}
	if strings.TrimSpace(model.responseLatest.stats) == "" {
		t.Fatalf("expected stats report to be populated after profiling run")
	}
}

