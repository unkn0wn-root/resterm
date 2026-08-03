package ui

import (
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
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
}

type statusMsg struct {
	text        string
	level       statusLevel
	testSummary string
	testLevel   statusLevel
	noModal     bool // error shown elsewhere (e.g. response pane); don't also pop the modal
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

type streamReadyMsg struct {
	sessionID string
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
