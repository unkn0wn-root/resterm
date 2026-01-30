package ui

import (
	"context"
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
	ctx        context.Context
	paneID     responsePaneID
	tab        responseTab
	width      int
	content    string
	snapshotID string
	mode       rawViewMode
	headers    headersViewMode
}

type responseReflowKey struct {
	tab     responseTab
	mode    rawViewMode
	headers headersViewMode
}

func reflowKey(tab responseTab, mode rawViewMode, headers headersViewMode) responseReflowKey {
	if tab != responseTabRaw {
		mode = 0
	}
	if tab != responseTabHeaders {
		headers = 0
	}
	return responseReflowKey{tab: tab, mode: mode, headers: headers}
}

func reflowKeyForReq(req responseReflowReq) responseReflowKey {
	return reflowKey(req.tab, req.mode, req.headers)
}

func reflowKeyForPane(pane *responsePaneState) (responseReflowKey, bool) {
	if pane == nil {
		return responseReflowKey{}, false
	}
	tab := pane.activeTab
	mode := rawViewMode(0)
	headers := headersViewMode(0)
	switch tab {
	case responseTabRaw:
		if pane.snapshot == nil {
			return responseReflowKey{}, false
		}
		mode = pane.snapshot.rawMode
	case responseTabHeaders:
		headers = pane.headersView
	}
	return reflowKey(tab, mode, headers), true
}

func reflowStateLive(pane *responsePaneState, state responseReflowState) bool {
	if pane == nil || state.token == "" || state.snapshotID == "" {
		return false
	}
	snap := pane.snapshot
	if snap == nil || !snap.ready || snap.id != state.snapshotID {
		return false
	}
	return true
}

type responseReflowStartMsg struct {
	req responseReflowReq
}

type responseReflowDoneMsg struct {
	req   responseReflowReq
	cache cachedWrap
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
	cache := pane.cacheForTab(tab, mode, pane.headersView)
	if cache.valid && cache.width != width {
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
	if pane.reflow == nil {
		pane.reflow = make(map[responseReflowKey]responseReflowState)
	}
	key := reflowKeyForReq(req)
	if st, ok := pane.reflow[key]; ok && st.cancel != nil {
		st.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	req.ctx = ctx
	pane.reflow[key] = responseReflowState{
		token:      tok,
		tab:        req.tab,
		width:      req.width,
		mode:       req.mode,
		headers:    req.headers,
		snapshotID: req.snapshotID,
		cancel:     cancel,
	}
	req.token = tok
	if delay < 0 {
		delay = 0
	}
	cmd := tea.Tick(delay, func(time.Time) tea.Msg {
		return responseReflowStartMsg{req: req}
	})
	return m.respCmd(cmd)
}

func reflowPending(pane *responsePaneState, req responseReflowReq) bool {
	if pane == nil {
		return false
	}
	key := reflowKeyForReq(req)
	state, ok := pane.reflow[key]
	if !ok {
		return false
	}
	if state.token == "" || state.tab != req.tab || state.width != req.width ||
		state.snapshotID != req.snapshotID {
		return false
	}
	if req.tab == responseTabRaw && state.mode != req.mode {
		return false
	}
	if req.tab == responseTabHeaders && state.headers != req.headers {
		return false
	}
	return true
}

func (m *Model) handleResponseReflowStart(msg responseReflowStartMsg) tea.Cmd {
	req := msg.req
	pane := m.pane(req.paneID)
	if !reflowReqValid(pane, req) {
		clearReflow(pane, req)
		m.respSpinStop()
		return nil
	}
	return m.respRflCmd(req)
}

func (m *Model) handleResponseReflowDone(msg responseReflowDoneMsg) tea.Cmd {
	req := msg.req
	pane := m.pane(req.paneID)
	if !reflowReqValid(pane, req) {
		clearReflow(pane, req)
		m.respSpinStop()
		return nil
	}
	snap := pane.snapshot
	if snap == nil {
		return nil
	}
	pane.setCacheForTab(req.tab, req.mode, req.headers, msg.cache)
	clearReflow(pane, req)
	if pane.activeTab == req.tab {
		if req.tab == responseTabRaw && snap.rawMode != req.mode {
			// keep cache updated, skip viewport apply
		} else if req.tab == responseTabHeaders && pane.headersView != req.headers {
			// keep cache updated, skip viewport apply
		} else {
			m.applyPaneContent(pane, req.tab, msg.cache.content, req.width, snap.ready, snap.id)
		}
	}
	m.respSpinStop()
	return nil
}

func reflowReqValid(pane *responsePaneState, req responseReflowReq) bool {
	if pane == nil {
		return false
	}
	key := reflowKeyForReq(req)
	state, ok := pane.reflow[key]
	if !ok {
		return false
	}
	if state.token == "" || state.token != req.token {
		return false
	}
	if state.tab != req.tab || state.width != req.width || state.snapshotID != req.snapshotID {
		return false
	}
	if req.tab == responseTabRaw && state.mode != req.mode {
		return false
	}
	if req.tab == responseTabHeaders && state.headers != req.headers {
		return false
	}
	if pane.snapshot == nil || !pane.snapshot.ready || pane.snapshot.id != req.snapshotID {
		return false
	}
	return true
}

func clearReflow(pane *responsePaneState, req responseReflowReq) {
	if pane == nil || pane.reflow == nil {
		return
	}
	key := reflowKeyForReq(req)
	state, ok := pane.reflow[key]
	if !ok || state.token != req.token {
		return
	}
	if state.cancel != nil {
		state.cancel()
	}
	delete(pane.reflow, key)
}

func clearReflowAll(pane *responsePaneState) {
	if pane == nil || pane.reflow == nil {
		return
	}
	for key, state := range pane.reflow {
		if state.cancel != nil {
			state.cancel()
		}
		delete(pane.reflow, key)
	}
}

func shouldAsyncWrap(tab responseTab, content string) bool {
	if content == "" || !tabAllowsAsyncWrap(tab) {
		return false
	}
	return len(content) > responseWrapAsyncLimit
}

func shouldInlineWrap(tab responseTab, content string) bool {
	return !shouldAsyncWrap(tab, content)
}
