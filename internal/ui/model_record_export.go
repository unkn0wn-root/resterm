package ui

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/files"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/recorder"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type recordExportMsg struct {
	session  *recorder.Session
	path     string
	revision uint64
	plan     *recorder.Plan
	text     string
	err      error
	commit   *exportCommit
}

func (msg recordExportMsg) abort() {
	if msg.commit != nil {
		msg.commit.Abort()
	}
}

// Cleanup and the UI may finish an export concurrently. Keep or remove
// its fixtures exactly once.
type exportCommit struct {
	mu   sync.Mutex
	stop func() bool
	done bool
	undo func()
}

func newExportCommit(ctx context.Context, undo func()) *exportCommit {
	c := &exportCommit{undo: undo}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stop = context.AfterFunc(ctx, c.Abort)
	return c
}

func (c *exportCommit) Commit() { c.finish(true) }

func (c *exportCommit) Abort() { c.finish(false) }

func (c *exportCommit) finish(keep bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return
	}
	c.done = true
	if c.stop != nil {
		c.stop()
	}
	if !keep {
		c.undo()
	}
}

// The editor may change during export. Check its revision before insertion.
type exportRequest struct {
	session  *recorder.Session
	ctx      context.Context
	mode     recorder.Mode
	path     string
	text     string
	revision uint64
	entries  []recorder.Entry
	existing *restfile.Document
	mocks    mock.Sources
}

func (m *Model) exportRecording(mode recorder.Mode, args []string) tea.Cmd {
	s := m.record.session
	switch {
	case s == nil:
		return statusCmd(statusWarn, "No recorder session is available")
	case m.record.exporting:
		return statusCmd(statusWarn, "A recording export is already running")
	case m.currentFile == "" || !files.IsRequest(m.currentFile):
		return statusCmd(statusWarn, "Save As a .http or .rest file before exporting recordings")
	}

	entries, warn := selectRecordings(s, args)
	if warn != nil {
		return warn
	}
	req := exportRequest{
		session:  s,
		ctx:      m.record.ctx,
		mode:     mode,
		path:     m.currentFile,
		text:     m.editor.Value(),
		revision: m.editor.Revision(),
		entries:  entries,
		mocks:    m.captureSources(),
	}

	existing, err := recorder.ParseOverlay(req.path, req.text)
	if err != nil {
		return statusCmd(statusWarn, err.Error())
	}
	req.existing = existing
	m.record.exporting = true
	return req.run
}

func selectRecordings(s *recorder.Session, args []string) ([]recorder.Entry, tea.Cmd) {
	entries := s.Snapshot(0)
	if len(args) == 1 && args[0] != "all" {
		id, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil || id == 0 {
			return nil, statusCmd(statusWarn, "Capture ID must be a positive integer")
		}
		entries = slices.DeleteFunc(entries, func(e recorder.Entry) bool { return e.ID != id })
	}
	if len(entries) == 0 {
		return nil, statusCmd(statusWarn, "No matching completed recordings")
	}
	return entries, nil
}

func (r exportRequest) run() tea.Msg {
	msg := recordExportMsg{session: r.session, path: r.path, revision: r.revision}

	p, err := recorder.Build(r.entries, recorder.ExportOptions{
		Mode:     r.mode,
		Path:     r.path,
		Upstream: r.session.Upstream(),
		Existing: r.existing,
	})
	if err != nil {
		msg.err = err
		return msg
	}
	msg.plan = p

	cleanup, err := p.PublishFixtures(r.ctx)
	if err != nil {
		msg.err = err
		return msg
	}
	msg.commit = newExportCommit(r.ctx, cleanup)
	msg.text, err = recorder.AppendText(r.path, r.text, p.Text)
	if err == nil {
		err = r.verify(msg.text)
	}
	if err != nil {
		msg.commit.Abort()
		msg.err = err
	}
	return msg
}

func (r exportRequest) verify(text string) error {
	overlay, err := recorder.ParseOverlay(r.path, text)
	if err != nil {
		return err
	}
	if r.mode.Wants(recorder.Mocks) {
		if err := recorder.ValidateMocks(r.mocks, overlay); err != nil {
			return err
		}
	}
	return r.ctx.Err()
}

func (m *Model) handleRecordExport(msg recordExportMsg) tea.Cmd {
	if msg.session != m.record.session {
		msg.abort()
		return nil
	}
	m.record.exporting = false

	if msg.err != nil {
		return statusCmd(statusWarn, "Recording export: "+oneLine(msg.err.Error()))
	}
	if msg.path != m.currentFile || msg.revision != m.editor.Revision() {
		msg.abort()
		return statusCmd(statusWarn, "Document changed during recording export. Retry the export")
	}

	m.rememberRecordNotes(msg.plan)
	if msg.plan.Text == "" {
		msg.abort()
		return statusCmd(statusWarn, recordExportSummary(msg.plan))
	}

	m.insertRecordExport(msg)
	msg.commit.Commit()
	return batchCommands(
		m.setFocus(focusEditor),
		m.scheduleMockReload(0),
		statusCmd(statusInfo, recordExportSummary(msg.plan)),
	)
}

func (m *Model) insertRecordExport(msg recordExportMsg) {
	start := strings.Count(m.editor.Value(), "\n") + 1
	m.editor.pushUndoSnapshot()
	m.editor.SetValue(msg.text)
	m.refreshCurrentDocument([]byte(m.editor.Value()))
	m.markDirty()
	m.revealLineRangeInEditor(restfile.LineRange{Start: start, End: strings.Count(msg.text, "\n") + 1})

	for _, export := range msg.plan.Exported {
		m.record.pending = append(
			m.record.pending,
			recordInsertion{id: export.ID, path: msg.path, names: export.Names},
		)
	}
}

func (m *Model) rememberRecordNotes(p *recorder.Plan) {
	for _, x := range p.Excluded {
		m.addRecordNote(x.ID, string(x.Mode)+" not exported: "+string(x.Reason))
	}
	for _, s := range p.Shadowed {
		m.addRecordNote(s.ID, shadowNote(s))
	}
}

func (m *Model) addRecordNote(id uint64, note string) {
	if m.record.notes == nil {
		m.record.notes = make(map[uint64][]string)
	}
	if !slices.Contains(m.record.notes[id], note) {
		m.record.notes[id] = append(m.record.notes[id], note)
	}
}

func shadowNote(s recorder.Shadow) string {
	if s.By == 0 {
		return "mock matches the same requests as a scenario already in the document"
	}
	return fmt.Sprintf("mock matches the same requests as record %d", s.By)
}

func recordExportSummary(p *recorder.Plan) string {
	text := fmt.Sprintf(
		"Exported %d recordings, %d values redacted, %d exports skipped",
		len(p.Exported), p.Redactions, len(p.Excluded),
	)
	if len(p.Shadowed) > 0 {
		text += fmt.Sprintf(". %d mocks match an earlier scenario", len(p.Shadowed))
	}
	if len(p.Excluded) > 0 {
		x := p.Excluded[0]
		text += fmt.Sprintf(". Record %d: %s (see :record list)", x.ID, x.Reason)
	}
	return text
}
