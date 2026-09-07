package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/unkn0wn-root/resterm/internal/bindings"
	"github.com/unkn0wn-root/resterm/internal/config"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

func newDiagnosticModel(t *testing.T, source string) Model {
	t.Helper()
	m := New(Config{WorkspaceRoot: t.TempDir()})
	m.width, m.height, m.ready = 160, 40, true
	_ = m.applyLayout()
	_ = m.setFocus(focusEditor)
	m.editor.SetValue(source)
	m.editor.moveCursorTo(0, 0)
	m.setDocument(parser.Parse("", []byte(m.editor.Value())))
	_ = m.syncDiagnostics()
	return m
}

func updateDiagnosticsModel(t *testing.T, m *Model, msg tea.Msg) tea.Cmd {
	t.Helper()
	model, cmd := m.Update(msg)
	*m = model.(Model)
	return cmd
}

// deliverDiagnosticAction runs cmd and feeds the diagnostics messages it
// produces back into the model until a deferred action has run.
func deliverDiagnosticAction(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, child := range msg {
			deliverDiagnosticAction(t, m, child)
		}
	case diagnosticsResultMsg, diagnosticsTickMsg:
		deliverDiagnosticAction(t, m, updateDiagnosticsModel(t, m, msg))
	}
}

func completeDiagnosticParse(t *testing.T, m *Model) {
	t.Helper()
	cmd := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: m.diagnostics.ticket})
	if cmd == nil {
		t.Fatal("expected a parse command")
	}
	updateDiagnosticsModel(t, m, cmd())
	if m.currentDiagnostics() == nil {
		t.Fatal("parse did not install a current snapshot")
	}
}

func TestDiagnosticsDebounceAndStaleCompletion(t *testing.T) {
	m := newDiagnosticModel(t, "GET http://x")
	doc := m.doc
	m.editor.SetValue("# @nmae typo\nGET http://x")
	m.markDirty()
	updateDiagnosticsModel(t, &m, diagnosticsInitMsg{})
	oldTicket := m.diagnostics.ticket
	job := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: oldTicket})
	if job == nil || m.diagnostics.running == 0 {
		t.Fatal("parse not started")
	}
	m.editor.SetValue("# @sse max-event=5\nGET http://x")
	updateDiagnosticsModel(t, &m, diagnosticsInitMsg{})
	if m.currentDiagnostics() != nil {
		t.Fatal("stale snapshot remained visible")
	}
	if cmd := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: oldTicket}); cmd != nil {
		t.Fatal("obsolete timer started work")
	}
	updateDiagnosticsModel(t, &m, job())
	if m.diagnostics.running != 0 || m.diagnostics.phase != diagnosticPending || m.currentDiagnostics() != nil {
		t.Fatalf("completion ignored new debounce: %+v", m.diagnostics)
	}
	completeDiagnosticParse(t, &m)
	if got := m.currentDiagnostics().report.Items[0].Message; !strings.Contains(got, "max-event") {
		t.Fatalf("latest finding = %q", got)
	}
	if m.doc != doc || !m.dirty {
		t.Fatal("diagnostics changed execution document or dirty state")
	}
}

func TestDiagnosticsCoalesceChangesWhileBusy(t *testing.T) {
	for _, action := range []string{"edit", "toggle", "switch"} {
		t.Run(action, func(t *testing.T) {
			m := newDiagnosticModel(t, "GET http://x")
			m.editor.SetValue("# @nmae typo")
			_ = m.syncDiagnostics()
			job := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: m.diagnostics.ticket})
			running := m.diagnostics.running
			switch action {
			case "edit":
				for _, value := range []string{"# @nmae b", "# @nmae c"} {
					m.editor.SetValue(value)
					_ = m.syncDiagnostics()
					if cmd := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: m.diagnostics.ticket}); cmd != nil {
						t.Fatal("overlapping parse")
					}
				}
			case "toggle":
				m.executeDiagnosticsCommand([]string{"off"})
				m.executeDiagnosticsCommand([]string{"on"})
			case "switch":
				m.currentFile = "different.http"
				_ = m.syncDiagnostics()
			}
			if m.diagnostics.running != running {
				t.Fatal("lost running slot")
			}
			if cmd := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: m.diagnostics.ticket}); cmd != nil {
				t.Fatal("overlapping replacement parse")
			}
			cmd := m.handleDiagnosticsResult(job().(diagnosticsResultMsg))
			if cmd == nil || m.diagnostics.running != m.diagnostics.ticket || m.currentDiagnostics() != nil {
				t.Fatal("stale result was installed or latest job was lost")
			}
			updateDiagnosticsModel(t, &m, cmd())
			if s := m.currentDiagnostics(); s == nil || string(s.report.Source) != m.editor.Value() ||
				s.report.Path != m.currentFile {
				t.Fatal("replacement snapshot does not match the current buffer")
			}
		})
	}
}

func TestDiagnosticsReplacingSameContentKeepsResult(t *testing.T) {
	m := newDiagnosticModel(t, "GET http://x")
	m.editor.SetValue("# @nmae typo")
	_ = m.syncDiagnostics()
	job := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: m.diagnostics.ticket})
	m.replaceEditorContent(m.editor.Value(), editorContentOptions{})
	_ = m.syncDiagnostics()
	if cmd := m.handleDiagnosticsResult(job().(diagnosticsResultMsg)); cmd != nil || m.currentDiagnostics() == nil {
		t.Fatal("same content lost its parse")
	}
}

func TestDiagnosticsEarlyHelpAndCancellation(t *testing.T) {
	for _, action := range []string{"stay", "move", "escape", "modal"} {
		t.Run(action, func(t *testing.T) {
			m := newDiagnosticModel(t, "GET http://x")
			m.editor.SetValue("# @sse max-event=5\nGET http://x")
			m.editor.moveCursorTo(0, 0)
			cmd := m.showContextHelp()
			if cmd == nil || m.diagnostics.running == 0 {
				t.Fatal("explicit help didn't bypass debounce")
			}
			switch action {
			case "move":
				updateDiagnosticsModel(t, &m, keyMsgFor("j"))
			case "escape":
				updateDiagnosticsModel(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
			case "modal":
				m.openStatusModal(statusInfo, "another interaction")
			}
			updateDiagnosticsModel(t, &m, cmd())
			if m.diagnostics.popup.open != (action == "stay") {
				t.Fatalf("popup state = %t", m.diagnostics.popup.open)
			}
		})
	}
}

func TestDiagnosticsRefreshAfterEditorMutations(t *testing.T) {
	m := newDiagnosticModel(t, "# \n# @nmae typo")
	doc := m.doc
	updateDiagnosticsModel(t, &m, keyMsgFor("A"))
	for _, r := range "@moc" {
		updateDiagnosticsModel(t, &m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if !m.editorInsertMode || m.diagnostics.running != 0 || m.diagnostics.current != nil ||
			m.diagnostics.phase == diagnosticIdle || !m.dirty {
			t.Fatal("typing published or started diagnostics")
		}
		if cmd := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: m.diagnostics.ticket}); cmd != nil {
			t.Fatal("pause in insert mode started diagnostics")
		}
	}
	if !m.editor.completion.active {
		t.Fatal("expected directive completion to be open")
	}
	updateDiagnosticsModel(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	if !m.editorInsertMode || m.diagnostics.running != 0 {
		t.Fatal("dismissing completion refreshed diagnostics before InsertLeave")
	}
	cmd := updateDiagnosticsModel(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.editorInsertMode || m.diagnostics.running == 0 || cmd == nil {
		t.Fatal("InsertLeave did not immediately start diagnostics")
	}
	// Esc returns finite commands, so executing this batch cannot start a recurring timer.
	var deliver func(tea.Cmd)
	deliver = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, child := range msg {
				deliver(child)
			}
		case diagnosticsResultMsg:
			updateDiagnosticsModel(t, &m, msg)
		case diagnosticsTickMsg:
			t.Fatal("InsertLeave was debounced")
		}
	}
	deliver(cmd)
	s := m.currentDiagnostics()
	if s == nil {
		t.Fatal("InsertLeave did not install diagnostics")
	}
	if _, warns := s.overlay.Counts(); warns != 2 {
		t.Fatal("missing completed directive warning")
	}
	if m.editor.Value() != "# @moc\n# @nmae typo" {
		t.Fatal("diagnostics changed source")
	}
	updateDiagnosticsModel(t, &m, keyMsgFor("u"))
	if m.currentDiagnostics() != nil || m.diagnostics.phase == diagnosticIdle {
		t.Fatal("undo did not invalidate diagnostics")
	}
	completeDiagnosticParse(t, &m)
	if m.doc != doc {
		t.Fatal("background parse replaced execution document")
	}
	m.openStatusModal(statusInfo, "notice")
	m.editor.SetValue("GET http://x")
	updateDiagnosticsModel(t, &m, diagnosticsInitMsg{})
	if m.currentDiagnostics() != nil || m.diagnostics.phase == diagnosticIdle {
		t.Fatal("modal early return skipped synchronization")
	}
	completeDiagnosticParse(t, &m)
	if len(m.currentDiagnostics().report.Items) != 0 {
		t.Fatal("corrected buffer retained findings")
	}
}

func TestDiagnosticsDeferInFlightResultDuringInsert(t *testing.T) {
	for _, edit := range []bool{false, true} {
		m := newDiagnosticModel(t, "GET http://x")
		m.editor.SetValue("# @moc")
		_ = m.syncDiagnostics()
		job := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: m.diagnostics.ticket})
		updateDiagnosticsModel(t, &m, keyMsgFor("i"))
		if edit {
			m.editor.SetValue("# @nmae")
			_ = m.syncDiagnostics()
		}
		updateDiagnosticsModel(t, &m, job())
		if m.currentDiagnostics() != nil || m.diagnostics.running != 0 || m.diagnostics.phase == diagnosticIdle {
			t.Fatalf("edit=%t: result changed diagnostics during insert: %+v", edit, m.diagnostics)
		}
		_ = m.setInsertMode(false, false)
		cmd := m.syncDiagnostics()
		if cmd == nil {
			t.Fatal("deferred result lost its refresh")
		}
		updateDiagnosticsModel(t, &m, cmd())
		if s := m.currentDiagnostics(); s == nil || string(s.report.Source) != m.editor.Value() {
			t.Fatal("InsertLeave failed to install current diagnostics")
		}
	}
}

func TestDiagnosticsEnteringInsertWithoutEditsRetainsSnapshot(t *testing.T) {
	m := newDiagnosticModel(t, "# @moc")
	snapshot := m.currentDiagnostics()
	updateDiagnosticsModel(t, &m, keyMsgFor("i"))
	if m.currentDiagnostics() != snapshot {
		t.Fatal("entering insert cleared unchanged diagnostics")
	}
	updateDiagnosticsModel(t, &m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.currentDiagnostics() != snapshot || m.diagnostics.running != 0 {
		t.Fatal("unchanged buffer was reanalyzed")
	}
}

func TestDiagnosticsHelpAfterInsertDispatchesPendingParse(t *testing.T) {
	m := newDiagnosticModel(t, "GET http://x")
	m.editor.SetValue("# @moc")
	_ = m.syncDiagnostics()
	ticket := m.diagnostics.ticket
	updateDiagnosticsModel(t, &m, keyMsgFor("i"))
	if cmd := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: ticket}); cmd != nil {
		t.Fatal("normal-mode timer started a job after InsertEnter")
	}
	_ = m.setInsertMode(false, false)
	cmd := m.showContextHelp()
	if cmd == nil || m.diagnostics.running == 0 {
		t.Fatal("help lost the parse command started by InsertLeave")
	}
	updateDiagnosticsModel(t, &m, cmd())
	if !m.diagnostics.popup.open {
		t.Fatal("pending help didn't show completed diagnostics")
	}
}

func TestDiagnosticsUseNormalizedBufferAndEligibleFiles(t *testing.T) {
	m := newDiagnosticModel(t, "GET http://x")
	raw := "\t# @nmae typo\r\nGET http://x"
	m.editor.SetValue(raw)
	m.setDocument(parser.Parse("", []byte(raw)))
	_ = m.syncDiagnostics()
	if m.currentDiagnostics() != nil {
		t.Fatal("raw disk bytes reused for normalized buffer")
	}
	completeDiagnosticParse(t, &m)
	if string(m.currentDiagnostics().report.Source) != m.editor.Value() {
		t.Fatal("wrong source analyzed")
	}
	for _, path := range []string{"file.rts", "settings.json", "settings.toml"} {
		m.currentFile = path
		if cmd := m.syncDiagnostics(); cmd != nil || m.currentDiagnostics() != nil {
			t.Fatalf("analyzed %s", path)
		}
	}
}

func TestDiagnosticsSettingsAndSessionOverride(t *testing.T) {
	enabled := false
	m := New(Config{
		WorkspaceRoot: t.TempDir(),
		Settings:      config.Settings{Editor: config.EditorSettings{Diagnostics: &enabled}},
	})
	if m.diagnosticsActive() {
		t.Fatal("explicit false ignored")
	}
	m.executeDiagnosticsCommand([]string{"on"})
	if !m.diagnosticsActive() || *m.cfg.Settings.Editor.Diagnostics {
		t.Fatal("session override changed persisted settings")
	}
	m.executeDiagnosticsCommand([]string{"off"})
	if m.diagnosticsActive() || m.diagnostics.popup.open {
		t.Fatal("off did not clear feature")
	}
}

func TestDiagnosticNavigationAndShortcutEligibility(t *testing.T) {
	m := newDiagnosticModel(t, "# @nmae a\nGET http://x\n### next\n# @sse max-event=5\nGET http://x")
	m.editor.moveCursorTo(0, 2)
	updateDiagnosticsModel(t, &m, keyMsgFor("]"))
	updateDiagnosticsModel(t, &m, keyMsgFor("d"))
	if m.editor.Line() != 3 || !m.diagnostics.popup.open {
		t.Fatal("next didn't navigate/open details")
	}
	m.performDiagnosticAction(diagnosticNext)
	if m.editor.Line() != 0 {
		t.Fatal("next didn't wrap")
	}
	m.performDiagnosticAction(diagnosticPrevious)
	if m.editor.Line() != 3 {
		t.Fatal("previous didn't wrap")
	}
	m.diagnostics.popup = diagnosticPopup{}
	m.focus = focusResponse
	if m.canStartChord(keyMsgFor("]"), "]") {
		t.Fatal("editor prefix consumed outside editor")
	}
	m.focus = focusEditor
	_ = m.setInsertMode(true, false)
	if m.canStartChord(keyMsgFor("["), "[") {
		t.Fatal("prefix consumed while typing")
	}
	if m.shortcutAvailable(bindings.ActionNextDiagnostic) {
		t.Fatal("action enabled while typing")
	}
}

func TestDiagnosticsStatusDoesNotReplaceRunProgress(t *testing.T) {
	m := newDiagnosticModel(t, "# @sse max-event=5\n# @use\nGET http://x")
	m.statusMessage = statusMsg{text: "Sending GET /x", level: statusInfo}
	section, ok := m.statusBarWarningSection(statusBarPalette(m.theme.StatusBarPalette))
	if !ok || section.text != "ERR 1 · WARN 1" {
		t.Fatalf("section = %+v", section)
	}
	m.openStatusMessageModal()
	if !m.showStatusModal || !strings.Contains(m.statusModalMessage, "ERROR") ||
		!strings.Contains(m.statusModalMessage, "WARNING") {
		t.Fatal("full list omitted a severity")
	}
	if m.statusMessage.text != "Sending GET /x" {
		t.Fatal("diagnostics replaced progress")
	}
}

func TestDiagnosticsUndoRedoRetainsDisplayWhileRefreshing(t *testing.T) {
	source := "GET http://x\n# @mock method=GET path=/x"
	m := newDiagnosticModel(t, source)
	doc := m.doc
	s := m.currentDiagnostics()
	if s == nil || !strings.Contains(s.report.Items[0].Message, "must start a new block") {
		t.Fatal("expected the mock block error")
	}
	if errs, _ := s.overlay.Counts(); errs != 1 {
		t.Fatal("expected one error")
	}
	m.editor.pushUndoSnapshot()
	m.editor.SetValue(source + "x")
	m.markDirty()
	_ = m.syncDiagnostics()
	completeDiagnosticParse(t, &m)

	for _, key := range []tea.KeyMsg{keyMsgFor("u"), {Type: tea.KeyCtrlR}, keyMsgFor("u")} {
		updateDiagnosticsModel(t, &m, key)
		if m.currentDiagnostics() != nil || m.diagnostics.phase != diagnosticPending {
			t.Fatal("undo/redo must still invalidate current diagnostics and debounce")
		}
		section, ok := m.statusBarWarningSection(statusBarPalette(m.theme.StatusBarPalette))
		if !ok || section.text != "ERR 1" {
			t.Fatalf("pending status = %+v, want ERR 1", section)
		}
		styles := m.editor.styler.StylesForLine(m.editor.LineRunes(1), 1)
		if styles[2].GetForeground() != m.theme.EditorDiagnosticError.GetForeground() || !styles[2].GetUnderline() {
			t.Fatal("unchanged @mock lost its error decoration during refresh")
		}
		m.editor.moveCursorTo(1, 2)
		if m.openDiagnosticPopup() {
			t.Fatal("pending display opened a stale popup")
		}
		before := m.editor.caretPosition()
		m.performDiagnosticAction(diagnosticNext)
		if m.editor.caretPosition() != before {
			t.Fatal("pending display allowed stale navigation")
		}
		completeDiagnosticParse(t, &m)
	}

	m.editor.SetValue("GET http://x")
	_ = m.syncDiagnostics()
	completeDiagnosticParse(t, &m)
	if _, ok := m.statusBarWarningSection(statusBarPalette(m.theme.StatusBarPalette)); ok {
		t.Fatal("fresh clean result retained the error count")
	}
	o := m.editor.styler.overlay
	if o == nil || len(o.Ranges(0))+len(o.Ranges(1)) != 0 {
		t.Fatal("fresh clean result retained error decorations")
	}
	if _, marked := o.Mark(1); marked {
		t.Fatal("fresh clean result retained the gutter mark")
	}
	if m.doc != doc || !m.dirty {
		t.Fatal("diagnostic refresh changed execution document or dirty state")
	}
}

func TestDiagnosticsRetainedDisplayLifecycle(t *testing.T) {
	for _, action := range []string{"insert edit", "switch request", "switch non-request", "disable", "theme"} {
		t.Run(action, func(t *testing.T) {
			m := newDiagnosticModel(t, "GET http://x\n# @mock path=/x")
			m.editor.SetValue(m.editor.Value() + "x")
			_ = m.syncDiagnostics()
			job := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: m.diagnostics.ticket})
			if job == nil || m.visibleDiagnostics() == nil {
				t.Fatal("missing pending display or parse")
			}
			switch action {
			case "insert edit":
				updateDiagnosticsModel(t, &m, keyMsgFor("i"))
				m.editor.SetValue(m.editor.Value() + "y")
			case "switch request":
				m.currentFile = "other.http"
			case "switch non-request":
				m.currentFile = "other.rts"
			case "disable":
				m.diagnostics.disabled = true
			case "theme":
				display, ticket := m.visibleDiagnostics(), m.diagnostics.ticket
				m.updateEditorStyler(m.currentFile)
				if m.editor.styler.overlay != display || m.diagnostics.ticket != ticket {
					t.Fatal("theme rebuild discarded pending display or restarted diagnostics")
				}
				if m.currentDiagnostics() != nil {
					t.Fatal("theme rebuild made stale diagnostics current")
				}
				return
			}
			if action != "insert edit" && m.visibleDiagnostics() != nil {
				t.Fatal("display leaked across file or enabled-state change before synchronization")
			}
			_ = m.syncDiagnostics()
			updateDiagnosticsModel(t, &m, job())
			if m.visibleDiagnostics() != nil || m.editor.styler.overlay != nil || m.currentDiagnostics() != nil {
				t.Fatal("obsolete completion restored a cleared display")
			}
		})
	}
}

func TestDiagnosticsStaleCompletionPreservesPendingDisplay(t *testing.T) {
	m := newDiagnosticModel(t, "GET http://x\n# @mock path=/x")
	m.editor.SetValue("# @nmae typo\nGET http://x")
	_ = m.syncDiagnostics()
	job := m.handleDiagnosticsTick(diagnosticsTickMsg{ticket: m.diagnostics.ticket})
	if job == nil {
		t.Fatal("missing parse command")
	}
	m.editor.SetValue("GET http://y")
	_ = m.syncDiagnostics()
	display := m.visibleDiagnostics()
	updateDiagnosticsModel(t, &m, job())
	if m.visibleDiagnostics() != display || display == nil || m.currentDiagnostics() != nil {
		t.Fatal("stale warning result replaced the retained error display")
	}
	if errs, _ := display.Counts(); errs != 1 {
		t.Fatal("retained display lost its error count")
	}
	completeDiagnosticParse(t, &m)
	current, display := m.currentDiagnostics(), m.visibleDiagnostics()
	if current == nil || display == nil || display != current.overlay {
		t.Fatal("fresh clean result did not replace both snapshots")
	}
	if errs, _ := display.Counts(); errs != 0 {
		t.Fatal("fresh clean result kept the error count")
	}
}
