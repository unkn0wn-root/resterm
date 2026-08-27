package ui

import (
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	xexec "github.com/unkn0wn-root/resterm/internal/exec"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/update"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type statusPulseMsg struct {
	seq int
}

type repeatProgressMsg struct {
	progress xexec.RepeatProgress
}

type tabSpinMsg struct {
	seq int
}

type latAnimMsg struct{}

type updateTickMsg struct{}

type statusLevel int

const (
	statusInfo statusLevel = iota
	statusWarn
	statusError
	statusSuccess
)

type responseMsg struct {
	response       *httpx.Response
	grpc           *grpcx.Response
	stream         *scripts.StreamInfo
	streamID       string
	transcript     []byte
	err            error
	tests          []scripts.TestResult
	scriptErr      error
	executed       *restfile.Request
	requestText    string
	runtimeSecrets []string
	environment    string
	selection      vars.Selection
	skipped        bool
	skipReason     string
	preview        bool
	explain        *xplain.Report
	historyDone    bool
	latGen         int
	target         runTarget
	elapsed        time.Duration
}

// runTarget remembers which response pane a run was launched from so a late
// response lands there instead of wherever focus has drifted to. The zero
// value means the run never captured one.
type runTarget struct {
	pane responsePaneID
	set  bool
}

func paneRunTarget(id responsePaneID) runTarget {
	return runTarget{pane: id, set: true}
}

// resolve reports the pane the run was launched from and whether it is still a
// destination worth honouring. The pane is always usable: an unset target, or a
// secondary pane after the split closed, resolves to the primary pane.
func (t runTarget) resolve(split bool) (responsePaneID, bool) {
	if !t.set {
		return responsePanePrimary, false
	}
	if t.pane == responsePaneSecondary && !split {
		return responsePanePrimary, false
	}
	return t.pane, true
}

type statusMsg struct {
	text        string
	level       statusLevel
	testSummary string
	testLevel   statusLevel
	noModal     bool // error shown elsewhere (e.g. response pane); don't also pop the modal
}

func (s statusMsg) then(next statusMsg) statusMsg {
	if next.text == "" {
		return s
	}
	if s.text == "" {
		return next
	}
	s.text += ". " + next.text
	if next.level == statusWarn || next.level == statusError {
		s.level = next.level
	}
	return s
}

type updateCheckMsg struct {
	res *update.Result
	err error
}

type streamEventMsg struct {
	sessionID string
	events    []*stream.Event
}

type streamStateMsg struct {
	sessionID string
	state     stream.State
	err       error
}

type streamCompleteMsg struct {
	sessionID string
}

// streamAttachMsg hands a session opened on the engine goroutine to the update
// loop, which owns every map and pane it has to be recorded in. gen is the
// stream generation the run started under, so an attachment that arrives after
// a workspace change can be dropped instead of writing into the new workspace.
type streamAttachMsg struct {
	session *stream.Session
	req     *restfile.Request
	sender  *httpx.WebSocketSender
	baseDir string
	pane    responsePaneID
	gen     uint64
}

type wsConsoleResultMsg struct {
	err     error
	status  string
	mode    websocketConsoleMode
	payload string
}

type rawDumpLoadedMsg struct {
	snapshot *responseSnapshot
	mode     rawViewMode
	content  string
}

type externalEditorMsg struct {
	path string
	err  error
}

type runReqMsg struct {
	res    engine.RequestResult
	err    error
	latGen int
	target runTarget
}

type runEvtMsg struct {
	evt core.Evt
}

type runWorkerDoneMsg struct {
	runID string
	err   error
}

// A warning the engine raised while running. The engine works off the UI
// goroutine, so it hands these to the run queue instead of touching the model.
type runWarningMsg struct {
	text string
}
