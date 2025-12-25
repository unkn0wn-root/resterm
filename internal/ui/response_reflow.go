package ui

import (
	"fmt"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	respReflowDelay     = 100 * time.Millisecond
	responseReflowLimit = rawHeavyLimit
)

type responseReflowReq struct {
	token      string
	paneID     responsePaneID
	tab        responseTab
	width      int
	content    string
	snapshotID string
	mode       rawViewMode
	headers    headersViewMode
}

type responseReflowStartMsg struct {
	req responseReflowReq
}

type responseReflowDoneMsg struct {
	req     responseReflowReq
	wrapped string
}

var responseReflowSeq uint64

func nextResponseReflowToken() string {
	id := atomic.AddUint64(&responseReflowSeq, 1)
	return fmt.Sprintf("reflow-%d", id)
}

func shouldReflow(tab responseTab, mode rawViewMode, snap *responseSnapshot) bool {
	if tab != responseTabRaw || snap == nil || !snap.ready {
		return false
	}
	if mode != rawViewHex && mode != rawViewBase64 {
		return false
	}
	return rawHeavy(len(snap.body))
}

func reflowDelay(
	pane *responsePaneState,
	tab responseTab,
	width int,
	mode rawViewMode,
) time.Duration {
	if pane == nil {
		return 0
	}
	if tab == responseTabRaw {
		if pane.rawWrapCache != nil {
			if cache := pane.rawWrapCache[mode]; cache.valid && cache.width != width {
				return respReflowDelay
			}
		}
		return 0
	}
	if cache := pane.wrapCache[tab]; cache.valid && cache.width != width {
		return respReflowDelay
	}
	return 0
}

func (m *Model) scheduleResponseReflow(
	pane *responsePaneState,
	req responseReflowReq,
	delay time.Duration,
) tea.Cmd {
	if pane == nil {
		return nil
	}
	if reflowPending(pane, req) {
		return nil
	}
	tok := nextResponseReflowToken()
	pane.reflow = responseReflowState{
		token:      tok,
		tab:        req.tab,
		width:      req.width,
		mode:       req.mode,
		headers:    req.headers,
		snapshotID: req.snapshotID,
	}
	req.token = tok
	if delay < 0 {
		delay = 0
	}
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return responseReflowStartMsg{req: req}
	})
}

func reflowPending(pane *responsePaneState, req responseReflowReq) bool {
	if pane == nil {
		return false
	}
	state := pane.reflow
	return state.token != "" &&
		state.tab == req.tab &&
		state.width == req.width &&
		state.mode == req.mode &&
		state.headers == req.headers &&
		state.snapshotID == req.snapshotID
}

func (m *Model) handleResponseReflowStart(msg responseReflowStartMsg) tea.Cmd {
	req := msg.req
	pane := m.pane(req.paneID)
	if !reflowReqValid(pane, req) {
		return nil
	}
	return responseReflowCmd(req)
}

func responseReflowCmd(req responseReflowReq) tea.Cmd {
	return func() tea.Msg {
		wrapped := wrapContentForTab(req.tab, req.content, req.width)
		return responseReflowDoneMsg{req: req, wrapped: wrapped}
	}
}

func (m *Model) handleResponseReflowDone(msg responseReflowDoneMsg) tea.Cmd {
	req := msg.req
	pane := m.pane(req.paneID)
	if !reflowReqValid(pane, req) {
		return nil
	}
	snap := pane.snapshot
	if snap == nil {
		return nil
	}
	if req.tab == responseTabRaw {
		pane.ensureRawWrapCache()
		pane.rawWrapCache[req.mode] = cachedWrap{
			width:   req.width,
			content: msg.wrapped,
			base:    req.content,
			valid:   true,
		}
	} else {
		pane.wrapCache[req.tab] = cachedWrap{
			width:   req.width,
			content: msg.wrapped,
			base:    req.content,
			valid:   true,
		}
	}
	pane.reflow = responseReflowState{}
	if pane.activeTab == req.tab {
		m.applyPaneContent(pane, req.tab, msg.wrapped, req.width, snap.ready, snap.id)
	}
	return nil
}

func reflowReqValid(pane *responsePaneState, req responseReflowReq) bool {
	if pane == nil {
		return false
	}
	state := pane.reflow
	if state.token == "" || state.token != req.token {
		return false
	}
	if state.tab != req.tab || state.width != req.width || state.mode != req.mode ||
		state.headers != req.headers ||
		state.snapshotID != req.snapshotID {
		return false
	}
	if pane.snapshot == nil || !pane.snapshot.ready || pane.snapshot.id != req.snapshotID {
		return false
	}
	if req.tab == responseTabRaw && pane.snapshot.rawMode != req.mode {
		return false
	}
	if req.tab == responseTabHeaders && pane.headersView != req.headers {
		return false
	}
	return true
}
