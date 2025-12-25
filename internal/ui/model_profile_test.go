package ui

import (
	"context"
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

func TestHelpDoesNotBlockProfileProgress(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
	}
	state := &profileState{
		base:        cloneRequest(req),
		doc:         &restfile.Document{Requests: []*restfile.Request{req}},
		spec:        restfile.ProfileSpec{Count: 2, Delay: 10 * time.Millisecond},
		total:       2,
		warmup:      0,
		delay:       10 * time.Millisecond,
		successes:   make([]time.Duration, 0, 2),
		failures:    make([]profileFailure, 0, 1),
		current:     req,
		messageBase: "Profiling " + requestBaseTitle(req),
		start:       time.Now(),
	}

	model := New(Config{})
	model.ready = true
	model.showHelp = true
	model.helpJustOpened = true
	model.profileRun = state
	model.sending = true

	resp := &httpclient.Response{
		Status:       "200 OK",
		StatusCode:   200,
		Duration:     5 * time.Millisecond,
		EffectiveURL: req.URL,
	}

	next, cmd := model.Update(responseMsg{response: resp, executed: req})
	if cmd == nil {
		t.Fatalf("expected profile iteration to keep scheduling while help is open")
	}
	updated := next.(Model)
	if updated.profileRun == nil {
		t.Fatalf("expected profile run to remain active after response")
	}
	if updated.helpJustOpened {
		t.Fatalf("expected helpJustOpened to reset after processing help-visible message")
	}
}

func TestProfileCancelStopsRun(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Profile: &restfile.ProfileSpec{Count: 3},
		},
	}
	state := &profileState{
		base:        cloneRequest(req),
		doc:         &restfile.Document{Requests: []*restfile.Request{req}},
		options:     httpclient.Options{},
		spec:        restfile.ProfileSpec{Count: 3},
		total:       3,
		warmup:      0,
		successes:   make([]time.Duration, 0, 3),
		failures:    make([]profileFailure, 0, 1),
		current:     req,
		messageBase: "Profiling " + requestBaseTitle(req),
		start:       time.Now(),
	}

	model := New(Config{})
	model.ready = true
	model.profileRun = state
	model.sending = true

	cmd := model.handleProfileResponse(responseMsg{
		err:      context.Canceled,
		executed: req,
	})
	if cmd != nil {
		collectMsgs(cmd)
	}

	if model.profileRun != nil {
		t.Fatalf("expected profile run to clear after cancellation")
	}
	if !strings.Contains(strings.ToLower(model.statusMessage.text), "canceled") {
		t.Fatalf("expected canceled status message, got %q", model.statusMessage.text)
	}
	if model.statusMessage.level != statusWarn {
		t.Fatalf("expected warning level for cancellation, got %v", model.statusMessage.level)
	}
	if len(state.successes) != 0 || len(state.failures) != 0 {
		t.Fatalf(
			"expected no successes or failures recorded on cancel, got %d success %d failure",
			len(state.successes),
			len(state.failures),
		)
	}
	if model.responseLatest == nil {
		t.Fatalf("expected response snapshot to be populated on cancel")
	}
	if stats := strings.ToLower(model.responseLatest.stats); !strings.Contains(stats, "canceled") {
		t.Fatalf("expected canceled status in response stats, got %q", model.responseLatest.stats)
	}
	if body := strings.ToLower(model.responseLatest.pretty); !strings.Contains(body, "canceled") {
		t.Fatalf(
			"expected response body to show cancellation summary, got %q",
			model.responseLatest.pretty,
		)
	}
}

func TestProfileStartShowsProgressPlaceholder(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Name:    "profile-target",
			Profile: &restfile.ProfileSpec{Count: 2},
		},
	}
	doc := &restfile.Document{Requests: []*restfile.Request{req}}

	model := New(Config{})
	model.ready = true
	prev := &responseSnapshot{
		pretty:  "old response",
		raw:     "old response",
		headers: "old response",
		ready:   true,
	}
	model.responseLatest = prev

	cmd := model.startProfileRun(doc, req, httpclient.Options{})
	if cmd == nil {
		t.Fatalf("expected startProfileRun to schedule execution")
	}
	if model.responseLatest != prev {
		t.Fatalf("expected previous response snapshot to remain unchanged during progress")
	}
	if !strings.Contains(model.statusMessage.text, "warmup 0/1") &&
		!strings.Contains(model.statusMessage.text, "Profiling") {
		t.Fatalf(
			"expected status message to reflect profiling start, got %q",
			model.statusMessage.text,
		)
	}
}

func TestProfileCancelRefreshesStatsPane(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/profile",
		Metadata: restfile.RequestMetadata{
			Profile: &restfile.ProfileSpec{Count: 4},
			Name:    "profile-target",
		},
	}

	state := &profileState{
		base:          cloneRequest(req),
		doc:           &restfile.Document{Requests: []*restfile.Request{req}},
		spec:          restfile.ProfileSpec{Count: 4},
		total:         4,
		index:         2,
		successes:     []time.Duration{10 * time.Millisecond, 15 * time.Millisecond},
		messageBase:   "Profiling " + requestBaseTitle(req),
		start:         time.Now().Add(-time.Minute),
		measuredStart: time.Now().Add(-30 * time.Second),
		measuredEnd:   time.Now(),
		canceled:      true,
	}

	model := New(Config{})
	model.ready = true
	primary := model.pane(responsePanePrimary)
	primary.viewport.Width = 80
	primary.viewport.Height = 10
	primary.setActiveTab(responseTabStats)

	cmd := model.finalizeProfileRun(responseMsg{}, state)
	if cmd != nil {
		collectMsgs(cmd)
	}

	content := primary.viewport.View()
	if !strings.Contains(content, "Runs:") {
		t.Fatalf("expected stats pane to show profile report after cancel, got %q", content)
	}
	if primary.activeTab != responseTabStats {
		t.Fatalf("expected stats tab to remain active after cancel, got %v", primary.activeTab)
	}
}
