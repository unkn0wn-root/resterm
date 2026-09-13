package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/recorder"
)

func recordTestTraffic(t *testing.T, m *Model) {
	t.Helper()
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "### literal\n{{= never() }}\n")
	}))
	t.Cleanup(up.Close)
	_ = m.executeExCommand("record start --listen 127.0.0.1:0 --upstream " + up.URL)
	s := m.record.session
	if s == nil {
		t.Fatal("recorder did not start")
	}
	r, err := http.Get("http://" + s.Addr() + "/users")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()
	deadline := time.After(5 * time.Second)
	for len(s.Snapshot(0)) == 0 {
		select {
		case <-s.Updates():
		case <-deadline:
			t.Fatal("capture timeout")
		}
	}
	cmd := m.executeExCommand("record stop")
	msg := cmd().(recordClosedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	_ = m.handleRecordClosed(msg)
}

func TestTUIRecordExportAndSave(t *testing.T) {
	for name, undone := range map[string]bool{"saved": false, "undone": true} {
		t.Run(name, func(t *testing.T) {
			m := newMockTestModel(t, "")
			recordTestTraffic(t, m)
			status, ok := statusMsgFromCmd(m.executeExCommand("record as-mock 1 2"))
			if !ok || status.level != statusWarn {
				t.Fatal("export accepted extra arguments")
			}
			msg := m.executeExCommand("record as-mock")().(recordExportMsg)
			if msg.err != nil {
				t.Fatal(msg.err)
			}
			_ = m.handleRecordExport(msg)
			if !m.dirty || len(m.doc.Mocks) != 1 || !m.hasUnexportedRecordings() || len(msg.plan.Fixtures) != 1 {
				t.Fatal("export must insert an unsaved mock with a literal fixture")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(m.currentFile), msg.plan.Fixtures[0].Path)); err != nil {
				t.Fatal(err)
			}
			if undone {
				m.editor, _ = m.editor.UndoLastChange()
			}
			outcome, _ := m.saveFileWithOutcome()
			if outcome != saveFileOutcomeSaved || m.hasUnexportedRecordings() != undone || len(m.record.pending) != 0 {
				t.Fatal("saved export state does not match the document")
			}
			_ = m.executeExCommand("record clear")
			if m.record.session != nil {
				t.Fatal("clear retained the stopped session")
			}
		})
	}
}

func TestRecordQuitGuards(t *testing.T) {
	m := newMockTestModel(t, "")
	const content = "### kept\nGET http://example.invalid/\n"
	m.editor.SetValue(content)
	m.markDirty()
	if !commandHasQuit(m.quitApp(nil)) {
		t.Fatal("Ctrl+Q no longer discards a dirty buffer without recordings")
	}
	recordTestTraffic(t, m)
	m.showHelp = true
	status, ok := statusMsgFromCmd(m.handleHelpKey(tea.KeyMsg{Type: tea.KeyCtrlQ}))
	if !ok || status.level != statusWarn || m.showHelp {
		t.Fatal("quit must dismiss the modal and warn about unsaved captures")
	}
	if commandHasQuit(m.executeExCommand("wq")) {
		t.Fatal("write-quit left the app with unsaved captures")
	}
	saved, err := os.ReadFile(m.currentFile)
	if err != nil || string(saved) != content || m.dirty || !m.hasUnexportedRecordings() {
		t.Fatalf("write-quit must save the file and retain unsaved captures: %q, %v", saved, err)
	}
}

func TestTUIRecordRejectsStaleExport(t *testing.T) {
	m := newMockTestModel(t, "")
	recordTestTraffic(t, m)
	msg := m.exportRecording(recorder.Mocks, nil)().(recordExportMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	m.editor.SetValue("### changed\nGET http://example.invalid/\n")
	_ = m.handleRecordExport(msg)
	if strings.Contains(m.editor.Value(), "@mock") || m.record.exporting {
		t.Fatal("stale export changed editor")
	}
	for _, fixture := range msg.plan.Fixtures {
		if _, err := os.Stat(filepath.Join(filepath.Dir(m.currentFile), fixture.Path)); !os.IsNotExist(err) {
			t.Fatal("aborted export left a fixture")
		}
	}
}

func TestModalPrecedenceMatchesRender(t *testing.T) {
	m := newMockTestModel(t, "")
	m.showMockLogs = true
	_ = m.openRecordList()

	_ = m.update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.showMockLogs {
		t.Fatal("Esc did not reach the modal that View renders")
	}
}

func TestRecordQuitGuardsSaveAs(t *testing.T) {
	m := newMockTestModel(t, "")
	recordTestTraffic(t, m)
	_ = m.openTemporaryDocument()
	m.editor.SetValue("GET http://example.invalid/\n")
	m.markDirty()

	if commandHasQuit(m.executeExCommand("wq")) || !m.showNewFileModal {
		t.Fatal("write-quit on an unnamed buffer must open Save As")
	}
	m.newFileInput.SetValue("saved")
	if commandHasQuit(m.submitNewFile()) {
		t.Fatal("Save As quit with unsaved captures")
	}
	if !m.hasUnexportedRecordings() {
		t.Fatal("captures were marked exported by Save As")
	}
}

func TestRecordQuitGuardsDrainingCapture(t *testing.T) {
	m := newMockTestModel(t, "")
	entered, release := make(chan struct{}), make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer up.Close()
	defer close(release)

	_ = m.executeExCommand("record start --listen 127.0.0.1:0 --upstream " + up.URL)
	s := m.record.session
	if s == nil {
		t.Fatal("recorder did not start")
	}
	go func() {
		res, err := http.Get("http://" + s.Addr() + "/pending")
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}
	}()
	<-entered

	stop := m.stopRecorder()
	go stop()
	// Close marks the session stopped before it drains the capture.
	for s.Stats().Running {
		select {
		case <-s.Updates():
		case <-time.After(5 * time.Second):
			t.Fatal("recorder did not stop")
		}
	}
	if stats := s.Stats(); stats.Entries != 0 || stats.ActiveCaptures != 1 {
		t.Fatalf("capture is no longer in flight: %+v", stats)
	}
	if commandHasQuit(m.executeExCommand("q")) {
		t.Fatal("quit discarded a capture that shutdown will publish")
	}
}
