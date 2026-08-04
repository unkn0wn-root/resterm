package ui

import (
	"time"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
)

type workflowStepResult struct {
	Step       restfile.WorkflowStep
	Success    bool
	Canceled   bool
	Skipped    bool
	Status     string
	Duration   time.Duration
	Message    string
	Iteration  int
	Total      int
	Branch     string
	Src        *restfile.Request
	Req        *restfile.Request
	HTTP       *httpx.Response
	GRPC       *grpcx.Response
	Stream     *scripts.StreamInfo
	Transcript []byte
	Tests      []scripts.TestResult
	ScriptErr  error
	Err        error
	Explain    *xplain.Report
}

const (
	workflowStatusPass     = "[PASS]"
	workflowStatusFail     = "[FAIL]"
	workflowStatusCanceled = "[CANCELED]"
	workflowStatusSkipped  = "[SKIPPED]"
)
