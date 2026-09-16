package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/recorder"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const recordTickInterval = 250 * time.Millisecond

// Exports may continue after :record stop. Cancel ctx only when clearing
// the session or closing the app.
type recordState struct {
	session *recorder.Session
	ctx     context.Context
	cancel  context.CancelFunc

	stopping  bool
	exporting bool
	pending   []recordInsertion
	saved     map[uint64]bool
	notes     map[uint64][]string

	showList bool
	viewport *viewport.Model
}

// Track generated names to detect insertions undone before the file is saved.
type recordInsertion struct {
	id    uint64
	path  string
	names []string
}

func (s *recordState) reset() {
	if s.cancel != nil {
		s.cancel()
	}
	*s = recordState{}
}

type recordTickMsg struct{ session *recorder.Session }

type recordClosedMsg struct {
	session *recorder.Session
	err     error
}

func (m *Model) executeRecordCommand(args []string) tea.Cmd {
	if len(args) == 0 {
		return statusCmd(statusInfo, m.recordStatus())
	}

	command := strings.ToLower(args[0])
	args = args[1:]
	def, ok := exCommands.Record(command)
	if !ok {
		return statusCmd(
			statusWarn,
			"Unknown :record command (use "+strings.Join(subNames(exCommands.record), ", ")+")",
		)
	}
	if def.tooManyArgs(len(args)) {
		return statusCmd(statusWarn, "Usage: :record "+def.usage())
	}

	switch command {
	case "start":
		return m.startRecorder(args)
	case "list":
		return m.openRecordList()
	case "stop":
		return m.stopRecorder()
	case "clear":
		return m.clearRecordings()
	case "as-request":
		return m.exportRecording(recorder.Requests, args)
	case "as-mock":
		return m.exportRecording(recorder.Mocks, args)
	default:
		return statusCmd(statusInfo, m.recordStatus())
	}
}

func (m *Model) startRecorder(args []string) tea.Cmd {
	if m.record.session != nil {
		if m.record.session.Stats().Running || m.record.stopping {
			return statusCmd(statusWarn, "Recorder is already running or stopping")
		}
		return statusCmd(
			statusWarn,
			"Export and save recordings, then use :record clear before starting another session",
		)
	}

	cfg, cmd := recordStartConfig(args)
	if cmd != nil {
		return cmd
	}
	ctx, cancel := context.WithCancel(context.Background())
	s, err := recorder.Start(ctx, cfg)
	if err != nil {
		cancel()
		return statusCmd(statusWarn, "Recorder: "+oneLine(err.Error()))
	}

	m.record = recordState{
		session: s,
		ctx:     ctx,
		cancel:  cancel,
		saved:   make(map[uint64]bool),
		notes:   make(map[uint64][]string),
	}
	msg := m.recordStatus()
	if !mock.IsLoopbackAddr(s.Addr()) {
		msg += ". Listening beyond localhost"
	}
	return batchCommands(statusCmd(statusInfo, msg), recordTick(s))
}

func recordStartConfig(args []string) (recorder.Config, tea.Cmd) {
	fs := cli.NewFlagSet("record start")
	flags := cli.NewRecordFlags()
	flags.Bind(fs)

	if err := fs.Parse(args); err != nil {
		def, _ := exCommands.Record("start")
		return recorder.Config{}, statusCmd(statusWarn, "Usage: :record "+def.usage())
	}
	if len(fs.Args()) != 0 {
		return recorder.Config{}, statusCmd(statusWarn, "Record start accepts flags only")
	}
	cfg, err := flags.Resolve()
	if err != nil {
		return recorder.Config{}, statusCmd(statusWarn, err.Error())
	}
	return cfg, nil
}

func (m *Model) stopRecorder() tea.Cmd {
	s := m.record.session
	if s == nil || m.record.stopping || !s.Stats().Running {
		return statusCmd(statusInfo, m.recordStatus())
	}

	m.record.stopping = true
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), recorder.ShutdownTimeout)
		defer cancel()
		return recordClosedMsg{session: s, err: s.Close(ctx)}
	}
}

func (m *Model) clearRecordings() tea.Cmd {
	s := m.record.session
	if s != nil && (s.Stats().Running || m.record.stopping || m.record.exporting) {
		return statusCmd(statusWarn, "Stop recording and finish exporting before clearing captures")
	}
	m.record.reset()
	return statusCmd(statusInfo, "Recordings cleared")
}

func recordTick(s *recorder.Session) tea.Cmd {
	return tea.Tick(recordTickInterval, func(time.Time) tea.Msg { return recordTickMsg{s} })
}

func (m *Model) handleRecordTick(msg recordTickMsg) tea.Cmd {
	if msg.session != m.record.session {
		return nil
	}
	if m.record.showList {
		m.syncRecordList()
	}
	if msg.session.Stats().Running {
		return recordTick(msg.session)
	}
	return nil
}

func (m *Model) handleRecordClosed(msg recordClosedMsg) tea.Cmd {
	if msg.session != m.record.session {
		return nil
	}
	m.record.stopping = false
	if msg.err != nil {
		return statusCmd(statusWarn, "Recorder stopped: "+oneLine(msg.err.Error()))
	}
	return statusCmd(statusInfo, m.recordStatus())
}

func (m *Model) recordStatus() string {
	s := m.record.session
	if s == nil {
		return "Recorder is stopped. Use :record start --upstream <origin>"
	}

	stats := s.Stats()
	state := "stopped"
	if stats.Running {
		state = "listening on http://" + s.Addr()
	}
	if m.record.stopping {
		state = "stopping"
	}
	text := fmt.Sprintf(
		"Recorder %s. %d recorded, %d excluded, %d active",
		state, stats.Entries, stats.Excluded, stats.ActiveCaptures,
	)
	if stats.Limit != "" {
		text += ". " + string(stats.Limit) + ", forwarding continues"
	}
	return text
}

func (m *Model) hasUnexportedRecordings() bool {
	s := m.record.session
	if s == nil {
		return false
	}
	stats := s.Stats()
	if stats.Running || m.record.stopping || m.record.exporting {
		return true
	}
	// Shutdown publishes the captures that are still in flight.
	return stats.Entries+stats.ActiveCaptures > len(m.record.saved)
}

func (m *Model) settleRecordExports(path string, doc *restfile.Document) {
	if len(m.record.pending) == 0 {
		return
	}
	declared := recorder.DeclaredNames(doc)
	m.record.pending = slices.DeleteFunc(m.record.pending, func(in recordInsertion) bool {
		if in.path != path {
			return false
		}
		if declaresAll(declared, in.names) {
			m.record.saved[in.id] = true
		}
		return true
	})
}

func declaresAll(declared map[string]struct{}, names []string) bool {
	for _, name := range names {
		if _, ok := declared[name]; !ok {
			return false
		}
	}
	return true
}

func (m *Model) closeRecorder() error {
	if m.record.cancel != nil {
		m.record.cancel()
	}
	if m.record.session == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), recorder.ShutdownTimeout)
	defer cancel()
	err := m.record.session.Close(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
