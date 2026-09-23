package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type profileState struct {
	id       string
	short    string
	req      *restfile.Request
	env      vars.Environment
	target   runTarget
	acc      *core.ProfileAccumulator
	view     *profileStatsView
	live     *responseSnapshot
	prev     *responseSnapshot
	last     responseMsg
	canceled bool
	latGen   int
}

func (m *Model) startProfileRun(
	doc *restfile.Document,
	req *restfile.Request,
	options httpx.Options,
) tea.Cmd {
	if cmd := m.runBlocked(); cmd != nil {
		return cmd
	}
	env := m.ws.active
	pl, err := core.PrepareProfile(doc, req, core.RunMeta{
		ID:  fmt.Sprintf("%d", time.Now().UnixNano()),
		Env: env,
	})
	if err != nil {
		m.setStatusMessage(statusMsg{text: err.Error(), level: statusError})
		return nil
	}
	rq := m.runRequestSvc(options)
	if rq == nil {
		return nil
	}

	title, short := m.statusRunTitles(doc, req)
	acc := core.NewProfileAccumulator(pl)
	view := &profileStatsView{title: title, env: env.Label(), snap: acc.Snapshot()}
	live := newTextSnapshot(fmt.Sprintf("Profiling %s. Results are in the Profile tab.", title), env.Label())
	live.profile = view
	st := &profileState{
		id:     pl.Run.ID,
		short:  short,
		req:    req.Clone(),
		env:    env,
		target: paneRunTarget(m.responseTargetPane()),
		acc:    acc,
		view:   view,
		live:   live,
		prev:   m.responseLatest,
		latGen: m.latencySeries.generation(),
	}
	m.profileRun = st
	m.applyRunSnapshot(st.live, nil, nil)
	m.showProfileProgress(st)

	spin := m.startSending()
	pulse := m.startStatusPulse()
	ch := m.runMsgChan
	worker := m.startRunWorker(st.id, func(ctx context.Context) error {
		_ = core.RunProfile(ctx, rq, runSink(ch), pl)
		return nil
	})
	return batchCmds([]tea.Cmd{m.activateProfileStatsTab(st.live), worker, pulse, spin})
}

func (m *Model) handleProfileRunEvt(evt core.Evt) tea.Cmd {
	st := m.profileRun
	if st == nil || runIDMismatch(st.id, core.MetaOf(evt).Run.ID) {
		return nil
	}
	st.acc.Apply(evt)
	switch v := evt.(type) {
	case core.ProIterStart:
		m.currentRequest = v.Request.Clone()
		m.showProfileProgress(st)
		return batchCmds([]tea.Cmd{m.startSending(), m.startStatusPulse()})
	case core.ProIterDone:
		msg := m.responseMsgFromRunState(v.Result, false)
		msg.latGen = st.latGen
		m.recordHeaderTelemetry(msg)
		st.last = msg
		st.view.snap.Progress = st.acc.Progress()
		m.invalidateStatsCaches(st.live)
		m.showProfileProgress(st)
		return m.syncResponsePanes()
	case core.RunDone:
		return m.finalizeProfileRun(st)
	}
	return nil
}

// Measured counts completed requests, so the display adds one for the current request.
func (m *Model) showProfileProgress(st *profileState) {
	p := st.acc.Progress()
	label := fmt.Sprintf("Profiling %s run %d/%d", st.short, min(p.Measured+1, p.Count), p.Count)
	if p.WarmupDone < p.Warmup {
		label = fmt.Sprintf("Profiling %s warmup %d/%d", st.short, p.WarmupDone+1, p.Warmup)
	}
	if st.canceled {
		label = "Canceling profile run"
	}
	m.statusPulseBase = label
	m.setStatusMessage(statusMsg{text: label, level: statusInfo})
}

// Keep the user's selected tab when the final response replaces the live view.
func (m *Model) finalizeProfileRun(st *profileState) tea.Cmd {
	m.profileRun = nil
	m.sendCancel = nil
	m.stopSending()
	m.stopStatusPulseIfIdle()

	snap := st.acc.Snapshot()
	st.view.snap = snap
	onProfile := m.panesShowingTab(st.live, responseTabStats)

	var cmd tea.Cmd
	status := snap.Progress.Status
	hasResponse := st.last.response != nil || st.last.err != nil
	if hasResponse && status != core.ProfileCanceled && status != core.ProfileSkipped {
		cmd = m.showProfileResponse(st)
	} else {
		m.showProfileSummary(st, snap.Summary())
	}
	for _, id := range onProfile {
		if pane := m.pane(id); pane != nil && pane.snapshot == m.responseLatest {
			pane.setActiveTab(responseTabStats)
		}
	}

	m.recordProfileHistory(st, snap)
	m.setStatusMessage(statusMsg{text: snap.Summary(), level: profileStatusLevel(snap), noModal: true})
	return batchCmds([]tea.Cmd{cmd, m.syncResponsePanes()})
}

func (m *Model) showProfileResponse(st *profileState) tea.Cmd {
	msg := st.last
	msg.target = st.target
	// The live placeholder is not a response. Restore the previous response
	// before the normal handler moves the latest response into responsePrevious.
	m.responseLatest = st.prev
	var cmd tea.Cmd
	if msg.err != nil {
		cmd = m.consumeRequestError(msg)
	} else {
		cmd = m.consumeHTTPResponse(msg)
	}
	if latest := m.responseLatest; latest != nil {
		latest.profile = st.view
	}
	return cmd
}

func (m *Model) showProfileSummary(st *profileState, body string) {
	st.live.pretty, st.live.raw, st.live.headers, st.live.requestHeaders = body, body, body, body
	for _, id := range m.visiblePaneIDs() {
		if pane := m.pane(id); pane != nil && pane.snapshot == st.live {
			pane.invalidateCaches()
		}
	}
}

func (m *Model) panesShowingTab(snap *responseSnapshot, tab responseTab) []responsePaneID {
	var ids []responsePaneID
	for _, id := range m.visiblePaneIDs() {
		if pane := m.pane(id); pane != nil && pane.snapshot == snap && pane.activeTab == tab {
			ids = append(ids, id)
		}
	}
	return ids
}

func profileStatusLevel(snap core.ProfileSnapshot) statusLevel {
	switch snap.Progress.Status {
	case core.ProfileFail, core.ProfileError:
		return statusError
	case core.ProfileCanceled, core.ProfileSkipped:
		return statusWarn
	}
	if snap.Progress.WarmupFailed > 0 {
		return statusWarn
	}
	return statusInfo
}

// @no-log does not skip profile history because profile entries store no body.
func (m *Model) recordProfileHistory(st *profileState, snap core.ProfileSnapshot) {
	hs := m.historyStore()
	if hs == nil {
		return
	}
	secrets := m.secretValuesForRedaction(st.req)
	ent := snap.HistoryEntry(st.req, st.env, m.historyFilePath(), func(text string, mask bool) string {
		return redactHistoryText(text, secrets, mask)
	})
	if err := hs.Append(ent); err != nil {
		m.setStatusMessage(statusMsg{text: fmt.Sprintf("history error: %v", err), level: statusWarn})
		return
	}
	m.historySelectedID = ent.ID
	m.syncHistory()
}

// Wait for RunDone after cancellation so the result has final counts.
func (m *Model) cancelProfileRun() {
	if st := m.profileRun; st != nil && !st.canceled {
		st.canceled = true
		m.showProfileProgress(st)
	}
}

func profileHistoryView(ent history.Entry) *profileStatsView {
	snap := core.ProfileSnapshotFromHistory(ent)
	title := strings.TrimSpace(strings.ToUpper(ent.Method) + " " + ent.RequestName)
	return &profileStatsView{title: title, env: ent.Environment, snap: snap}
}
