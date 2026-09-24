package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

type profileRun struct {
	t    *testing.T
	m    *Model
	spec restfile.ProfileSpec
	meta core.RunMeta
	req  *restfile.Request
	at   time.Time
}

func startTestProfile(t *testing.T, m *Model, spec restfile.ProfileSpec, meta restfile.RequestMetadata) *profileRun {
	t.Helper()
	meta.Name = "ProfileItems"
	meta.Profile = &spec
	req := &restfile.Request{Method: "GET", URL: "https://example.com/profile", Metadata: meta}
	if cmd := m.startProfileRun(&restfile.Document{}, req, httpx.Options{}); cmd == nil {
		t.Fatal("startProfileRun() = nil")
	}
	if m.profileRun == nil {
		t.Fatal("profile run did not start")
	}
	r := &profileRun{
		t:    t,
		m:    m,
		spec: spec,
		meta: core.RunMeta{ID: m.profileRun.id, Mode: core.ModeProfile},
		req:  req,
		at:   time.Unix(10, 0),
	}
	r.emit(core.RunStart{Meta: core.NewMeta(r.meta, r.at)})
	return r
}

func (r *profileRun) emit(evt core.Evt) {
	r.t.Helper()
	applyRunEvt(r.t, r.m, evt)
}

func (r *profileRun) iter(i int) core.IterMeta {
	it := core.IterMeta{
		Index:       i,
		Total:       r.spec.Count + r.spec.Warmup,
		Warmup:      i < r.spec.Warmup,
		WarmupTotal: r.spec.Warmup,
		RunTotal:    r.spec.Count,
	}
	if it.Warmup {
		it.WarmupIndex = i + 1
	} else {
		it.RunIndex = i - r.spec.Warmup + 1
	}
	return it
}

func (r *profileRun) start(i int) {
	r.emit(core.ProIterStart{Meta: core.NewMeta(r.meta, r.at), Iter: r.iter(i), Request: r.req.Clone()})
}

func (r *profileRun) done(i int, res engine.RequestResult) {
	r.at = r.at.Add(10 * time.Millisecond)
	res.Executed = r.req.Clone()
	r.emit(core.ProIterDone{Meta: core.NewMeta(r.meta, r.at), Iter: r.iter(i), Result: res})
}

func (r *profileRun) ok(i int) {
	r.start(i)
	r.done(i, engine.RequestResult{
		Response: testHTTPResp("https://example.com/profile", 200, `{"ok":true}`, 10*time.Millisecond),
	})
}

func (r *profileRun) finish(done core.RunDone) {
	done.Meta = core.NewMeta(r.meta, r.at)
	r.emit(done)
}

func TestProfileRunShowsLiveProgress(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	r := startTestProfile(
		t,
		&m,
		restfile.ProfileSpec{Count: 2, Warmup: 1, Delay: time.Second},
		restfile.RequestMetadata{},
	)
	if !strings.Contains(m.statusMessage.text, "warmup 1/1") {
		t.Fatalf("status = %q, want warmup progress", m.statusMessage.text)
	}
	pane := m.pane(responsePanePrimary)
	if pane.activeTab != responseTabStats || pane.snapshot.profile == nil {
		t.Fatalf("tab = %v, want the live Profile tab", pane.activeTab)
	}

	r.ok(0)
	if m.profileRun == nil {
		t.Fatal("profile run ended after the warmup")
	}
	if !strings.Contains(m.statusMessage.text, "run 1/2") || !strings.Contains(m.statusPulseBase, "run 1/2") {
		t.Fatalf("status = %q, pulse = %q, want measured progress", m.statusMessage.text, m.statusPulseBase)
	}
	content, _ := m.paneContentBase(responsePanePrimary, responseTabStats, 100)
	if !strings.Contains(content, "RUNNING") || !strings.Contains(content, "0/2") {
		t.Fatalf("live dashboard = %q", content)
	}

	r.ok(1)
	r.start(2)
	r.done(2, engine.RequestResult{
		Response: testHTTPResp("https://example.com/profile", 500, `{}`, 10*time.Millisecond),
	})
	if m.profileRun == nil {
		t.Fatal("profile run ended before RunDone")
	}
	content, _ = m.paneContentBase(responsePanePrimary, responseTabStats, 100)
	for _, want := range []string{"2/2", "10ms", "200 ×1 · 500 ×1", "Run 3: HTTP 500"} {
		if !strings.Contains(ansi.Strip(content), want) {
			t.Fatalf("live dashboard is missing %q:\n%s", want, ansi.Strip(content))
		}
	}
}

func TestProfileRunCancelFinalizesOnRunDone(t *testing.T) {
	tests := []struct {
		name string
		run  func(*profileRun)
		want string
	}{
		{
			name: "during request",
			run: func(r *profileRun) {
				r.start(0)
				r.m.cancelActiveRuns()
				r.done(0, engine.RequestResult{Err: context.Canceled})
			},
			want: "Profiling canceled after 0/2 runs (0/2 measured)",
		},
		{
			name: "during delay",
			run: func(r *profileRun) {
				r.ok(0)
				r.m.cancelActiveRuns()
			},
			want: "Profiling canceled after 1/2 runs (1/2 measured)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := newOrchTestModel(t, Config{})
			r := startTestProfile(t, &m, restfile.ProfileSpec{Count: 2, Delay: time.Second}, restfile.RequestMetadata{})
			test.run(r)
			if m.profileRun == nil || !m.profileRun.canceled {
				t.Fatal("cancel finished the run before RunDone")
			}
			r.finish(core.RunDone{Canceled: true})

			if m.profileRun != nil {
				t.Fatal("RunDone did not finish the run")
			}
			snap := m.responseLatest
			if snap == nil || snap.profile == nil || snap.profile.snap.Progress.Status != core.ProfileCanceled {
				t.Fatalf("latest = %+v, want a canceled profile", snap)
			}
			if !strings.Contains(snap.pretty, test.want) || !strings.Contains(m.statusMessage.text, test.want) {
				t.Fatalf("pretty = %q, status = %q, want %q", snap.pretty, m.statusMessage.text, test.want)
			}
		})
	}
}

func TestProfileRunCancelRefreshesPrettyTab(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	r := startTestProfile(t, &m, restfile.ProfileSpec{Count: 2}, restfile.RequestMetadata{})
	pane := m.pane(responsePanePrimary)
	pane.setActiveTab(responseTabPretty)
	collectMsgs(m.syncResponsePanes())
	if view := ansi.Strip(pane.viewport.View()); !strings.Contains(view, "Profiling GET ProfileItems") {
		t.Fatalf("pretty before cancel = %q", view)
	}

	r.start(0)
	m.cancelActiveRuns()
	r.done(0, engine.RequestResult{Err: context.Canceled})
	r.finish(core.RunDone{Canceled: true})
	collectMsgs(m.syncResponsePanes())
	if view := ansi.Strip(pane.viewport.View()); !strings.Contains(view, "Profiling canceled") {
		t.Fatalf("pretty after cancel = %q, want the summary", view)
	}
}

func TestProfileRunFinishKeepsUserTab(t *testing.T) {
	for _, stay := range []bool{true, false} {
		m := newOrchTestModel(t, Config{})
		prev := newTextSnapshot("previous", "")
		m.applyRunSnapshot(prev, nil, nil)
		r := startTestProfile(t, &m, restfile.ProfileSpec{Count: 1}, restfile.RequestMetadata{})
		pane := m.pane(responsePanePrimary)
		if !stay {
			pane.setActiveTab(responseTabPretty)
		}
		r.ok(0)
		r.finish(core.RunDone{Success: true})

		if m.responseLatest == nil || m.responseLatest.profile == nil || m.responseLatest == r.m.responsePrevious {
			t.Fatalf("latest = %+v, want the final response with the profile", m.responseLatest)
		}
		if m.responsePrevious != prev {
			t.Fatalf("previous = %+v, want the response from before the run", m.responsePrevious)
		}
		if got := pane.activeTab == responseTabStats; got != stay {
			t.Fatalf("stay=%t: tab = %v", stay, pane.activeTab)
		}
		if m.responseLatest.profile.snap.Progress.Status != core.ProfilePass {
			t.Fatalf("status = %s, want pass", m.responseLatest.profile.snap.Progress.Status)
		}
	}
}

func TestProfileRunKeepsLastTestResults(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	r := startTestProfile(t, &m, restfile.ProfileSpec{Count: 1}, restfile.RequestMetadata{})
	r.start(0)
	r.done(0, engine.RequestResult{
		Response: testHTTPResp("https://example.com/profile", 200, `{"ok":true}`, 10*time.Millisecond),
		Tests:    []scripts.TestResult{{Name: "ok", Passed: true}, {Name: "slow", Passed: false}},
	})
	r.finish(core.RunDone{})

	if got, _, ok := m.headerTestStatus(); !ok || got != "1 fail" {
		t.Fatalf("headerTestStatus() = %q, %t, want the last run's tests", got, ok)
	}
}

func TestProfileRunErrorShowsOnce(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	r := startTestProfile(t, &m, restfile.ProfileSpec{Count: 2}, restfile.RequestMetadata{})
	id := m.profileRun.id
	r.start(0)
	err := errors.New("executor gone")
	r.finish(core.RunDone{Err: err})

	if m.profileRun != nil {
		t.Fatal("an errored run stayed active")
	}
	if !strings.Contains(m.statusMessage.text, "executor gone") {
		t.Fatalf("status = %q, want the error", m.statusMessage.text)
	}
	if cmd := m.handleRunWorkerDone(runWorkerDoneMsg{runID: id}); cmd != nil {
		t.Fatal("worker done reported the run again")
	}
	if !strings.Contains(m.statusMessage.text, "executor gone") {
		t.Fatalf("status after worker done = %q, want the error", m.statusMessage.text)
	}
}

func TestProfileRunRejectsGRPC(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	req := &restfile.Request{
		GRPC:     &restfile.GRPCRequest{},
		Metadata: restfile.RequestMetadata{Profile: &restfile.ProfileSpec{Count: 2}},
	}
	if cmd := m.startProfileRun(&restfile.Document{}, req, httpx.Options{}); cmd != nil {
		t.Fatal("startProfileRun() started a gRPC profile")
	}
	if m.profileRun != nil || !strings.Contains(m.statusMessage.text, "gRPC") {
		t.Fatalf("status = %q, want the gRPC rejection", m.statusMessage.text)
	}
}

func TestProfileRunInteractiveWebSocketUsesCorePath(t *testing.T) {
	m := newOrchTestModel(t, Config{})
	req := &restfile.Request{
		Method:    "GET",
		URL:       "wss://example.com/chat",
		WebSocket: &restfile.WebSocketRequest{},
		Metadata:  restfile.RequestMetadata{Name: "chat", Profile: &restfile.ProfileSpec{Count: 1}},
	}
	if cmd := m.startProfileRun(&restfile.Document{}, req, httpx.Options{}); cmd == nil || m.profileRun == nil {
		t.Fatal("expected the core profile path")
	}
}

func TestProfileHistoryReopensDashboard(t *testing.T) {
	store := histdb.New(filepath.Join(t.TempDir(), "history.db"))
	m := New(Config{History: store})
	m.ready, m.width, m.height = true, 120, 40
	if cmd := m.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}
	r := startTestProfile(t, &m, restfile.ProfileSpec{Count: 2}, restfile.RequestMetadata{NoLog: true})
	r.ok(0)
	r.start(1)
	r.done(1, engine.RequestResult{
		Response: testHTTPResp("https://example.com/profile", 500, `{}`, 10*time.Millisecond),
	})
	r.finish(core.RunDone{})

	entries, err := store.Entries()
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %d, err = %v, want one entry with @no-log", len(entries), err)
	}
	ent := entries[0]
	res := ent.ProfileResults
	if ent.Status != "FAIL 1/2" || res == nil || res.Status != "fail" || len(res.Failures) != 1 {
		t.Fatalf("entry = %q %+v", ent.Status, res)
	}

	m.pane(responsePanePrimary).setActiveTab(responseTabPretty)
	if cmd := m.presentHistoryEntry(ent, nil); cmd != nil {
		collectMsgs(cmd)
	}
	pane := m.pane(responsePanePrimary)
	if pane.activeTab != responseTabStats || m.responseLatest.profile == nil {
		t.Fatalf("tab = %v, want the Profile tab", pane.activeTab)
	}
	content, _ := m.paneContentBase(responsePanePrimary, responseTabStats, 100)
	for _, want := range []string{"FAIL", "Run 2: HTTP 500"} {
		if !strings.Contains(content, want) {
			t.Fatalf("dashboard missing %q:\n%s", want, content)
		}
	}
}

func TestProfileRunComparesWithPreviousRun(t *testing.T) {
	store := histdb.New(filepath.Join(t.TempDir(), "history.db"))
	m := New(Config{History: store})
	m.ready, m.width, m.height = true, 120, 40
	if cmd := m.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}
	for _, d := range []time.Duration{10 * time.Millisecond, 20 * time.Millisecond} {
		r := startTestProfile(t, &m, restfile.ProfileSpec{Count: 1}, restfile.RequestMetadata{})
		r.start(0)
		r.done(0, engine.RequestResult{Response: testHTTPResp("https://example.com/profile", 200, `{}`, d)})
		r.finish(core.RunDone{Success: true})
	}
	content, _ := m.paneContentBase(responsePanePrimary, responseTabStats, 100)
	if plain := ansi.Strip(content); !strings.Contains(plain, "vs run at") || !strings.Contains(plain, "+10ms") {
		t.Fatalf("second run has no comparison:\n%s", plain)
	}

	entries, err := store.Entries()
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries = %d, err = %v, want two profile runs", len(entries), err)
	}
	for _, ent := range entries {
		if cmd := m.presentHistoryEntry(ent, nil); cmd != nil {
			collectMsgs(cmd)
		}
		content, _ := m.paneContentBase(responsePanePrimary, responseTabStats, 100)
		later := ent.ProfileResults.Latency.Max == 20*time.Millisecond
		if got := strings.Contains(ansi.Strip(content), "+10ms"); got != later {
			t.Fatalf("entry %s comparison = %t, want %t:\n%s", ent.ID, got, later, ansi.Strip(content))
		}
	}
}
