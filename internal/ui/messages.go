package ui

import (
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/update"
)

type statusPulseMsg struct {
	seq int
}
type profileNextIterationMsg struct{}
type updateTickMsg struct{}

type statusLevel int

const (
	statusInfo statusLevel = iota
	statusWarn
	statusError
	statusSuccess
)

type responseMsg struct {
	response    *httpclient.Response
	grpc        *grpcclient.Response
	err         error
	tests       []scripts.TestResult
	scriptErr   error
	executed    *restfile.Request
	requestText string
	environment string
	skipped     bool
	skipReason  string
}

type statusMsg struct {
	text  string
	level statusLevel
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
