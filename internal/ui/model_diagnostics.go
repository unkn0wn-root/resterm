package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

const diagnosticDebounce = 250 * time.Millisecond

type diagnosticAction uint8

const (
	diagnosticNone diagnosticAction = iota
	diagnosticHelp
	diagnosticNext
	diagnosticPrevious
	diagnosticList
	diagnosticStatusMessage
)

type diagnosticPhase uint8

// idle: the snapshot is current or its parse is running. pending: waiting for
// the debounce or for insert mode to end. due: parse as soon as the slot is free.
const (
	diagnosticIdle diagnosticPhase = iota
	diagnosticPending
	diagnosticDue
)

type diagnosticKey struct {
	revision uint64
	path     string
}

// Any change here closes the popup and cancels a deferred action.
type diagnosticContext struct {
	key   diagnosticKey
	caret cursorPosition
	focus paneFocus
}

type diagnosticIntent struct {
	action diagnosticAction
	diagnosticContext
}

// Snapshots are immutable once delivered and are the only data actions use.
type diagnosticSnapshot struct {
	key     diagnosticKey
	report  diag.Report
	overlay *diag.Overlay
}

func newDiagnosticSnapshot(key diagnosticKey, rep diag.Report) *diagnosticSnapshot {
	return &diagnosticSnapshot{key: key, report: rep, overlay: diag.NewOverlay(rep)}
}

// visible is render-only and may outlive current while a newer parse is pending.
type diagnosticState struct {
	disabled  bool
	key       diagnosticKey
	active    bool
	inserting bool
	phase     diagnosticPhase
	ticket    uint64
	running   uint64
	current   *diagnosticSnapshot
	visible   *diag.Overlay
	intent    diagnosticIntent
	popup     diagnosticPopup
}

type diagnosticsInitMsg struct{}

type diagnosticsTickMsg struct{ ticket uint64 }

type diagnosticsResultMsg struct {
	ticket   uint64
	snapshot *diagnosticSnapshot
}

func (m *Model) diagnosticKey() diagnosticKey {
	return diagnosticKey{revision: m.editor.Revision(), path: m.currentFile}
}

func (m *Model) diagnosticContext() diagnosticContext {
	return diagnosticContext{key: m.diagnosticKey(), caret: m.editor.caretPosition(), focus: m.focus}
}

func (m *Model) diagnosticContextCurrent(ctx diagnosticContext) bool {
	return m.editorIdle() && ctx == m.diagnosticContext()
}

func (m *Model) diagnosticsActive() bool {
	return !m.diagnostics.disabled && (m.currentFile == "" || files.IsRequest(m.currentFile))
}

func (m *Model) editorIdle() bool {
	return !m.editorInsertMode && !m.mouseModalActive()
}

func (m *Model) diagnosticParseAllowed() bool {
	return m.diagnosticsActive() && !m.editorInsertMode && m.diagnostics.key == m.diagnosticKey()
}

func (m *Model) currentDiagnostics() *diagnosticSnapshot {
	s := m.diagnostics.current
	if !m.diagnosticsActive() || s == nil || s.key != m.diagnosticKey() {
		return nil
	}
	return s
}

// visibleDiagnostics may carry counts and decorations from an older parse.
// Interactive actions must use currentDiagnostics.
func (m *Model) visibleDiagnostics() *diag.Overlay {
	o := m.diagnostics.visible
	if !m.diagnosticsActive() || o == nil || m.diagnostics.key.path != m.currentFile {
		return nil
	}
	return o
}

// syncDiagnostics runs after every Update, including modal paths and
// programmatic edits. It never replaces the execution document or its registry.
func (m *Model) syncDiagnostics() tea.Cmd {
	s := &m.diagnostics
	key, active := m.diagnosticKey(), m.diagnosticsActive()
	leftInsert := s.inserting && !m.editorInsertMode
	s.inserting = m.editorInsertMode
	if s.intent.action != diagnosticNone && !m.diagnosticContextCurrent(s.intent.diagnosticContext) {
		s.intent = diagnosticIntent{}
	}
	if s.key != key || s.active != active {
		m.beginDiagnosticRefresh(key, active)
	} else if !leftInsert || s.phase == diagnosticIdle {
		return nil
	}
	if !active || m.editorInsertMode {
		return nil
	}
	// Textarea normalization can change the bytes without changing the document revision.
	if m.docMatchesEditor() && string(m.doc.Raw) == m.editor.Value() {
		m.publishDiagnostics(newDiagnosticSnapshot(key, parser.Diagnostics(m.doc)))
		s.phase = diagnosticIdle
		return nil
	}
	if leftInsert {
		s.phase = diagnosticDue
		return m.startDiagnosticParse()
	}
	ticket := s.ticket
	return tea.Tick(diagnosticDebounce, func(time.Time) tea.Msg { return diagnosticsTickMsg{ticket: ticket} })
}

func (m *Model) handleDiagnosticsTick(msg diagnosticsTickMsg) tea.Cmd {
	s := &m.diagnostics
	if msg.ticket != s.ticket || s.phase == diagnosticIdle || !m.diagnosticParseAllowed() {
		return nil
	}
	s.phase = diagnosticDue
	return m.startDiagnosticParse()
}

func (m *Model) startDiagnosticParse() tea.Cmd {
	s := &m.diagnostics
	if s.running != 0 || s.phase != diagnosticDue || !m.diagnosticParseAllowed() {
		return nil
	}
	ticket, key, source := s.ticket, s.key, m.editor.Value()
	s.running, s.phase = ticket, diagnosticIdle
	// Capture input before dispatch; the background parse must not read mutable model state.
	return func() tea.Msg {
		rep := parser.Diagnostics(parser.Parse(key.path, []byte(source)))
		return diagnosticsResultMsg{ticket: ticket, snapshot: newDiagnosticSnapshot(key, rep)}
	}
}

func (m *Model) handleDiagnosticsResult(msg diagnosticsResultMsg) tea.Cmd {
	s := &m.diagnostics
	if msg.ticket != s.running {
		return nil
	}
	// Only completion releases the slot, even for obsolete jobs, to prevent overlapping parses.
	s.running = 0
	if msg.ticket != s.ticket {
		return m.startDiagnosticParse()
	}
	if m.editorInsertMode {
		s.phase = diagnosticPending
		return nil
	}
	m.publishDiagnostics(msg.snapshot)
	intent := s.intent
	s.intent = diagnosticIntent{}
	var actionCmd tea.Cmd
	if intent.action != diagnosticNone && m.diagnosticContextCurrent(intent.diagnosticContext) {
		actionCmd = m.performDiagnosticAction(intent.action)
	}
	return batchCommands(actionCmd, m.startDiagnosticParse())
}

func (m *Model) beginDiagnosticRefresh(key diagnosticKey, active bool) {
	s := &m.diagnostics
	keep := active && s.active && s.key.path == key.path && !m.editorInsertMode
	if !keep {
		s.visible = nil
	} else if s.visible != nil {
		s.visible = s.visible.Retain(&m.editor)
	}
	s.key, s.active = key, active
	s.ticket++
	s.current = nil
	s.popup = diagnosticPopup{}
	s.intent = diagnosticIntent{}
	s.phase = diagnosticIdle
	if active {
		s.phase = diagnosticPending
	}
	m.editor.setDiagnosticOverlay(s.visible)
}

func (m *Model) publishDiagnostics(snapshot *diagnosticSnapshot) {
	m.diagnostics.current = snapshot
	m.diagnostics.visible = snapshot.overlay
	m.editor.setDiagnosticOverlay(snapshot.overlay)
}

func (m *Model) requestDiagnostics(action diagnosticAction) tea.Cmd {
	if !m.diagnosticsActive() {
		return statusCmdQuiet(
			statusInfo,
			"Editor diagnostics are off or unavailable for this file. Use :diagnostics on in a request file",
		)
	}
	// Synchronization may start an InsertLeave parse; its command must still be dispatched.
	syncCmd := m.syncDiagnostics()
	if m.currentDiagnostics() != nil {
		return m.performDiagnosticAction(action)
	}
	s := &m.diagnostics
	s.intent = diagnosticIntent{action: action, diagnosticContext: m.diagnosticContext()}
	if s.phase == diagnosticPending {
		s.phase = diagnosticDue
	}
	if s.running != 0 {
		return syncCmd
	}
	return m.startDiagnosticParse()
}

func (m *Model) performDiagnosticAction(action diagnosticAction) tea.Cmd {
	s := m.currentDiagnostics()
	if s == nil {
		return nil
	}
	switch action {
	case diagnosticHelp:
		if m.openDiagnosticPopup() {
			return nil
		}
		return m.showContextDocumentation()
	case diagnosticList:
		if len(s.report.Items) == 0 {
			return statusCmdQuiet(statusInfo, "No editor diagnostics")
		}
		m.openDiagnosticList(s)
	case diagnosticStatusMessage:
		if len(s.report.Items) > 0 {
			m.openDiagnosticList(s)
			return nil
		}
		m.openCurrentStatusModal()
	case diagnosticNext, diagnosticPrevious:
		caret := m.editor.caretPosition()
		at, ok := s.overlay.Next(diag.Cell{Line: caret.Line, Col: caret.Column}, action == diagnosticPrevious)
		if !ok {
			return statusCmdQuiet(statusInfo, "No editor diagnostics")
		}
		focusCmd := m.setFocus(focusEditor)
		m.editor.ClearSelection()
		m.editor.moveCursorTo(at.Line, at.Col)
		m.openDiagnosticPopup()
		return focusCmd
	}
	return nil
}

func (m *Model) openDiagnosticList(s *diagnosticSnapshot) {
	level := statusWarn
	if errs, _ := s.overlay.Counts(); errs > 0 {
		level = statusError
	}
	var lines []string
	for _, item := range s.report.Items {
		lines = append(
			lines,
			fmt.Sprintf("%s %s: %s", strings.ToUpper(string(item.Severity)), item.Span.Start, item.Message),
		)
	}
	m.diagnostics.popup = diagnosticPopup{}
	m.openStatusModal(level, strings.Join(lines, "\n\n"))
}

func (m *Model) executeDiagnosticsCommand(args []string) tea.Cmd {
	switch strings.Join(args, " ") {
	case "":
		return m.requestDiagnostics(diagnosticList)
	case "on", "off":
		m.diagnostics.disabled = args[0] == "off"
		cmd := m.syncDiagnostics()
		return batchCommands(cmd, statusCmdQuiet(statusInfo, "Editor diagnostics "+args[0]+" (this session)"))
	case "next":
		return m.requestDiagnostics(diagnosticNext)
	case "prev":
		return m.requestDiagnostics(diagnosticPrevious)
	default:
		return statusCmdQuiet(statusWarn, "Usage: :diagnostics [on|off|next|prev]")
	}
}

// shortcutAvailable gates bindings that only make sense in editor normal mode.
func (m *Model) shortcutAvailable(action bindings.ActionID) bool {
	switch action {
	case bindings.ActionNextDiagnostic, bindings.ActionPreviousDiagnostic:
		return m.focus == focusEditor && !m.editorInsertMode && m.diagnosticsActive()
	default:
		return true
	}
}
