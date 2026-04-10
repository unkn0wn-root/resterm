package ui

import (
	"fmt"
	"strings"
	"time"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
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
	HTTP       *httpclient.Response
	GRPC       *grpcclient.Response
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

func displayStepName(step restfile.WorkflowStep) string {
	name := strings.TrimSpace(step.Name)
	if name != "" {
		return name
	}
	switch step.Kind {
	case restfile.WorkflowStepKindIf:
		return "@if"
	case restfile.WorkflowStepKindSwitch:
		return "@switch"
	case restfile.WorkflowStepKindForEach:
		if step.Using != "" {
			return step.Using
		}
		return "@for-each"
	default:
		return step.Using
	}
}

func workflowStepLabel(step restfile.WorkflowStep, branch string, iter, total int) string {
	label := displayStepName(step)
	if label == "" {
		label = "step"
	}
	if branch != "" {
		label = fmt.Sprintf("%s -> %s", label, branch)
	}
	if iter > 0 && total > 0 {
		label = fmt.Sprintf("%s (%d/%d)", label, iter, total)
	}
	return label
}
