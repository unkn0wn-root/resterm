package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/tracebudget"
)

func (m *Model) cancelInFlightSend(status string) {
	if m.sendCancel != nil {
		m.sendCancel()
	}
	if strings.TrimSpace(status) != "" {
		m.setStatusMessage(statusMsg{text: status, level: statusInfo})
	}
}

func (m *Model) cancelStatus() string {
	if state := m.profileRun; state != nil {
		return "Canceling profile run..."
	}
	if state := m.workflowRun; state != nil {
		name := strings.TrimSpace(state.workflow.Name)
		if name == "" {
			name = "workflow"
		}
		return fmt.Sprintf("Canceling %s...", name)
	}
	if m.compareRun != nil {
		return "Canceling compare run..."
	}
	if m.sending {
		return "Canceling in-progress request..."
	}
	if m.responseLoading {
		return "Canceling response formatting..."
	}
	if m.hasReflowPending() {
		return "Canceling response reflow..."
	}
	return "Canceling..."
}

func (m *Model) hasActiveRun() bool {
	return m.sending || m.profileRun != nil || m.workflowRun != nil || m.compareRun != nil
}

func (m Model) hasReflowPending() bool {
	for i := range m.responsePanes {
		pane := &m.responsePanes[i]
		if pane.reflow == nil {
			continue
		}
		for _, state := range pane.reflow {
			if reflowStateLive(pane, state) {
				return true
			}
		}
	}
	return false
}

func (m Model) spinnerActive() bool {
	return m.sending || m.responseLoading || m.hasReflowPending()
}

func (m *Model) cancelActiveRuns() tea.Cmd {
	if !m.hasActiveRun() && !m.responseLoading && !m.hasReflowPending() {
		return nil
	}
	return m.cancelRuns(m.cancelStatus())
}

func (m *Model) cancelRuns(status string) tea.Cmd {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "Canceling..."
	}
	hadRun := m.profileRun != nil || m.workflowRun != nil || m.compareRun != nil

	m.stopSending()
	m.stopStatusPulse()

	var cmds []tea.Cmd
	if cmd := m.cancelProfileRun(status); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := m.cancelWorkflowRun(status); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if cmd := m.cancelCompareRun(status); cmd != nil {
		cmds = append(cmds, cmd)
	}
	cancelText := status
	if hadRun && !m.hasActiveRun() && !m.responseLoading && !m.hasReflowPending() {
		cancelText = ""
	}
	m.cancelInFlightSend(cancelText)
	if m.responseLoading {
		if cmd := m.cancelResponseFormatting(""); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if cmd := m.cancelResponseReflow(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return batchCmds(cmds)
}

func (m *Model) cancelProfileRun(reason string) tea.Cmd {
	state := m.profileRun
	if state == nil {
		return nil
	}
	state.canceled = true
	if strings.TrimSpace(state.cancelReason) == "" {
		state.cancelReason = reason
	}
	if state.current == nil {
		return m.finalizeProfileRun(responseMsg{}, state)
	}
	return nil
}

func (m *Model) cancelWorkflowRun(reason string) tea.Cmd {
	state := m.workflowRun
	if state == nil {
		return nil
	}
	state.canceled = true
	if strings.TrimSpace(state.cancelReason) == "" {
		state.cancelReason = reason
	}
	if state.current == nil {
		return m.finalizeWorkflowRun(state)
	}
	return nil
}

func (m *Model) cancelCompareRun(reason string) tea.Cmd {
	state := m.compareRun
	if state == nil {
		return nil
	}
	state.canceled = true
	if strings.TrimSpace(state.cancelReason) == "" {
		state.cancelReason = reason
	}
	if state.current == nil {
		return m.finalizeCompareRun(state)
	}
	return nil
}

func (m *Model) cancelResponseReflow() tea.Cmd {
	canceled := false
	for i := range m.responsePanes {
		pane := &m.responsePanes[i]
		if len(pane.reflow) == 0 {
			continue
		}
		wasActive := m.reflowActiveForPane(pane)
		for key, state := range pane.reflow {
			markReflowCanceled(pane, key, state.snapshotID)
		}
		clearReflowAll(pane)
		canceled = true

		if wasActive {
			m.showReflowCanceled(pane)
		}
	}
	if !canceled {
		return nil
	}
	m.respSpinStop()
	if !m.hasActiveRun() && !m.responseLoading && !m.hasReflowPending() {
		m.setStatusMessage(statusMsg{})
	}
	return nil
}

func (m *Model) showReflowCanceled(pane *responsePaneState) {
	if pane == nil {
		return
	}
	tab := pane.activeTab
	if tab == responseTabHistory {
		return
	}

	_, ww, h := paneDims(pane, tab)
	sr, sid := paneSnap(pane)
	m.applyReflowCanceled(pane, tab, ww, h, sr, sid)
}

type activeReqExec struct {
	doc  *restfile.Document
	req  *restfile.Request
	opts httpclient.Options
	wrap func(tea.Cmd) tea.Cmd
}

func (m *Model) prepareActiveRequestExec() (*activeReqExec, tea.Cmd) {
	content := m.editor.Value()
	doc := parser.Parse(m.currentFile, []byte(content))
	cursorLine := currentCursorLine(m.editor)
	req, _ := m.requestAtCursor(doc, content, cursorLine)
	if req == nil {
		return nil, func() tea.Msg {
			return statusMsg{text: "No request at cursor", level: statusWarn}
		}
	}

	rc := m.restorePane(paneRegionResponse)
	wrap := func(cmd tea.Cmd) tea.Cmd {
		return batchCommands(rc, cmd)
	}

	m.doc = doc
	m.syncRequestList(doc)
	m.setActiveRequest(req)
	m.syncAllGlobals(doc)

	cloned := cloneRequest(req)
	m.currentRequest = cloned
	m.testResults = nil
	m.scriptError = nil

	opts := m.cfg.HTTPOptions
	if opts.BaseDir == "" && m.currentFile != "" {
		opts.BaseDir = filepath.Dir(m.currentFile)
	}

	return &activeReqExec{doc: doc, req: cloned, opts: opts, wrap: wrap}, nil
}

func (m *Model) sendActiveRequest() tea.Cmd {
	if cmd := m.cancelActiveRuns(); cmd != nil {
		return cmd
	}
	st, cmd := m.prepareActiveRequestExec()
	if cmd != nil {
		return cmd
	}

	if st.req.Metadata.ForEach != nil {
		if spec := m.compareSpecForRequest(st.req); spec != nil {
			m.setStatusMessage(
				statusMsg{level: statusWarn, text: "@compare cannot run alongside @for-each"},
			)
			return st.wrap(nil)
		}
		if st.req.Metadata.Profile != nil {
			m.setStatusMessage(
				statusMsg{level: statusWarn, text: "@profile cannot run alongside @for-each"},
			)
			return st.wrap(nil)
		}
		if st.req.Metadata.Trace != nil && st.req.Metadata.Trace.Enabled {
			st.opts.Trace = true
			if budget, ok := tracebudget.FromSpec(st.req.Metadata.Trace); ok {
				st.opts.TraceBudget = &budget
			}
		}
		return st.wrap(m.startForEachRun(st.doc, st.req, st.opts))
	}

	if spec := m.compareSpecForRequest(st.req); spec != nil {
		if st.req.Metadata.Profile != nil {
			m.setStatusMessage(
				statusMsg{level: statusWarn, text: "@compare cannot run alongside @profile"},
			)
			return st.wrap(nil)
		}
		return st.wrap(m.startCompareRun(st.doc, st.req, spec, st.opts))
	}

	if st.req.Metadata.Trace != nil && st.req.Metadata.Trace.Enabled {
		st.opts.Trace = true
		if budget, ok := tracebudget.FromSpec(st.req.Metadata.Trace); ok {
			st.opts.TraceBudget = &budget
		}
	}

	if st.req.Metadata.Profile != nil {
		if st.req.GRPC != nil {
			m.setStatusMessage(
				statusMsg{text: "Profiling is not supported for gRPC requests", level: statusWarn},
			)
		} else {
			return st.wrap(m.startProfileRun(st.doc, st.req, st.opts))
		}
	}

	if st.req.WebSocket != nil && len(st.req.WebSocket.Steps) == 0 {
		spin := m.startSending()
		target := m.statusRequestTarget(st.doc, st.req, "")
		base := "Sending"
		if trimmed := strings.TrimSpace(target); trimmed != "" {
			base = fmt.Sprintf("Sending %s", trimmed)
		}
		m.statusPulseBase = base
		m.statusPulseFrame = -1
		m.setStatusMessage(statusMsg{text: base, level: statusInfo})

		execCmd := m.executeRequest(st.doc, st.req, st.opts, "", nil)
		pulse := m.startStatusPulse()
		return st.wrap(batchCmds([]tea.Cmd{execCmd, pulse, spin}))
	}

	spin := m.startSending()
	target := m.statusRequestTarget(st.doc, st.req, "")
	base := "Sending"
	if trimmed := strings.TrimSpace(target); trimmed != "" {
		base = fmt.Sprintf("Sending %s", trimmed)
	}
	m.statusPulseBase = base
	m.statusPulseFrame = -1
	m.setStatusMessage(statusMsg{text: base, level: statusInfo})

	execCmd := m.execRunReq(st.doc, st.req, st.opts, "", nil)
	pulse := m.startStatusPulse()
	return st.wrap(batchCmds([]tea.Cmd{execCmd, pulse, spin}))
}

func (m *Model) explainActiveRequest() tea.Cmd {
	if cmd := m.cancelActiveRuns(); cmd != nil {
		return cmd
	}

	st, cmd := m.prepareActiveRequestExec()
	if cmd != nil {
		return cmd
	}

	spin := m.startSending()
	target := m.statusRequestTarget(st.doc, st.req, "")
	base := "Preparing explain preview"
	if trimmed := strings.TrimSpace(target); trimmed != "" {
		base = fmt.Sprintf("Preparing explain for %s", trimmed)
	}
	m.statusPulseBase = base
	m.statusPulseFrame = -1
	m.setStatusMessage(statusMsg{text: base, level: statusInfo})

	execCmd := m.executeExplain(st.doc, st.req, st.opts, "", nil)
	pulse := m.startStatusPulse()
	return st.wrap(batchCmds([]tea.Cmd{execCmd, pulse, spin}))
}

// Allow CLI-level compare flags to kick off a sweep even when the request lacks
// @compare metadata so users can reuse the same editor workflow while honoring
// --compare selections.
func (m *Model) startConfigCompareFromEditor() tea.Cmd {
	content := m.editor.Value()
	doc := parser.Parse(m.currentFile, []byte(content))
	cursorLine := currentCursorLine(m.editor)
	req, _ := m.requestAtCursor(doc, content, cursorLine)
	if req == nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "No request at cursor"})
		return nil
	}

	if req.Metadata.ForEach != nil {
		m.setStatusMessage(
			statusMsg{level: statusWarn, text: "@compare cannot run alongside @for-each"},
		)
		return nil
	}
	if req.Metadata.Profile != nil {
		m.setStatusMessage(statusMsg{level: statusWarn, text: "@profile cannot run during compare"})
		return nil
	}

	spec := buildConfigCompareSpec(m.cfg.CompareTargets, m.cfg.CompareBase)
	if spec == nil && req.Metadata.Compare != nil {
		spec = cloneCompareSpec(req.Metadata.Compare)
	}
	if spec == nil {
		m.setStatusMessage(statusMsg{
			level: statusWarn,
			text:  "No compare targets configured. Use --compare or add @compare.",
		})
		return nil
	}

	m.doc = doc
	m.syncRequestList(doc)
	m.setActiveRequest(req)
	m.syncAllGlobals(doc)

	cloned := cloneRequest(req)
	m.currentRequest = cloned
	m.testResults = nil
	m.scriptError = nil

	options := m.cfg.HTTPOptions
	if options.BaseDir == "" && m.currentFile != "" {
		options.BaseDir = filepath.Dir(m.currentFile)
	}
	if cloned.Metadata.Trace != nil && cloned.Metadata.Trace.Enabled {
		options.Trace = true
		if budget, ok := tracebudget.FromSpec(cloned.Metadata.Trace); ok {
			options.TraceBudget = &budget
		}
	}

	return m.startCompareRun(doc, cloned, spec, options)
}

const (
	statusPulseInterval = 1 * time.Second
	tabSpinInterval     = 100 * time.Millisecond
)

func (m *Model) startSending() tea.Cmd {
	m.sending = true
	return m.startTabSpin()
}

func (m *Model) stopSending() {
	m.sending = false
	m.stopTabSpinIfIdle()
}

func (m *Model) stopStatusPulse() {
	m.statusPulseOn = false
	m.statusPulseBase = ""
	m.statusPulseFrame = 0
}

func (m *Model) stopStatusPulseIfIdle() {
	if m.hasActiveRun() {
		return
	}
	m.stopStatusPulse()
}

func (m *Model) scheduleStatusPulse() tea.Cmd {
	if !m.statusPulseOn || !m.hasActiveRun() {
		return nil
	}
	seq := m.statusPulseSeq
	return tea.Tick(statusPulseInterval, func(time.Time) tea.Msg {
		return statusPulseMsg{seq: seq}
	})
}

func (m *Model) startStatusPulse() tea.Cmd {
	if m.statusPulseOn {
		return nil
	}
	m.statusPulseOn = true
	m.statusPulseSeq++
	m.statusPulseFrame = 0
	return m.scheduleStatusPulse()
}

func (m *Model) stopTabSpin() {
	m.tabSpinOn = false
	m.tabSpinIdx = 0
}

func (m *Model) stopTabSpinIfIdle() {
	if m.spinnerActive() {
		return
	}
	m.stopTabSpin()
}

func (m *Model) scheduleTabSpin() tea.Cmd {
	if !m.tabSpinOn || !m.spinnerActive() || len(tabSpinFrames) == 0 {
		return nil
	}
	seq := m.tabSpinSeq
	return tea.Tick(tabSpinInterval, func(time.Time) tea.Msg {
		return tabSpinMsg{seq: seq}
	})
}

func (m *Model) startTabSpin() tea.Cmd {
	if m.tabSpinOn || !m.spinnerActive() || len(tabSpinFrames) == 0 {
		return nil
	}
	m.tabSpinOn = true
	m.tabSpinSeq++
	m.tabSpinIdx = 0
	return m.scheduleTabSpin()
}

func (m *Model) handleTabSpin(msg tabSpinMsg) tea.Cmd {
	if msg.seq != m.tabSpinSeq {
		return nil
	}
	if !m.tabSpinOn || !m.spinnerActive() || len(tabSpinFrames) == 0 {
		m.stopTabSpin()
		return nil
	}
	m.tabSpinIdx++
	if m.tabSpinIdx >= len(tabSpinFrames) {
		m.tabSpinIdx = 0
	}
	return m.scheduleTabSpin()
}

func (m *Model) handleStatusPulse(msg statusPulseMsg) tea.Cmd {
	if msg.seq != m.statusPulseSeq {
		return nil
	}
	if !m.statusPulseOn || !m.hasActiveRun() {
		m.stopStatusPulse()
		return nil
	}

	m.statusPulseFrame++
	if m.statusPulseFrame >= 3 {
		m.statusPulseFrame = 0
	}

	base := strings.TrimSpace(m.statusPulseBase)
	if base == "" {
		base = "Sending"
	}

	dots := strings.Repeat(".", m.statusPulseFrame+1)
	m.setStatusMessage(statusMsg{text: base + dots, level: statusInfo})
	return m.scheduleStatusPulse()
}

func isCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

func batchCmds(cmds []tea.Cmd) tea.Cmd {
	switch len(cmds) {
	case 0:
		return nil
	case 1:
		return cmds[0]
	default:
		return tea.Batch(cmds...)
	}
}
