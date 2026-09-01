package ui

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	xexec "github.com/unkn0wn-root/resterm/internal/exec"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
)

func TestPollTimeoutShowsLastAttemptWithoutReplacingLogicalLast(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}
	logicalLast := &httpx.Response{Status: "201 Created", StatusCode: http.StatusCreated}
	model.lastResponse = logicalLast
	lastAttempt := &httpx.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Body:       []byte(`{"status":"pending"}`),
	}
	pollErr := diag.WrapAs(diag.ClassTimeout, xexec.ErrPollTimeout, "wait for poll condition")

	cmd := model.handleResponseMessage(responseMsg{response: lastAttempt, err: pollErr})
	if cmd == nil {
		t.Fatal("poll timeout did not schedule response rendering")
	}
	if model.lastResponse != logicalLast {
		t.Fatalf("logical last response was replaced: %+v", model.lastResponse)
	}
	if got := model.headerTransport.label; got != "200" {
		t.Fatalf("header transport = %q, want the displayed polling attempt", got)
	}
	if !errors.Is(model.lastError, xexec.ErrPollTimeout) {
		t.Fatalf("last error = %v", model.lastError)
	}
	if model.responseLatest == nil {
		t.Fatal("last polling attempt was not shown")
	}
	if !strings.Contains(model.statusMessage.text, "last response shown") {
		t.Fatalf("status = %q", model.statusMessage.text)
	}
}

func TestSkippedRunPreservesHeaderTransport(t *testing.T) {
	model := New(Config{})
	model.ready = true
	model.width = 120
	model.height = 40
	if cmd := model.applyLayout(); cmd != nil {
		collectMsgs(cmd)
	}
	model.headerTransport = headerTransportStatus{label: "201", level: statusSuccess}

	model.handleResponseMessage(responseMsg{skipped: true, skipReason: "condition was false"})

	if got := model.headerTransport.label; got != "201" {
		t.Fatalf("skipped run changed header transport to %q", got)
	}
}

func TestRepeatProgressStatus(t *testing.T) {
	t.Parallel()
	status := repeatProgressStatus(xexec.RepeatProgress{
		Phase:   xexec.RepeatRetryWait,
		Attempt: 3,
		Poll:    2,
		Delay:   250 * time.Millisecond,
	})
	if !strings.Contains(status.text, "Retrying in 250ms") ||
		!strings.Contains(status.text, "attempt 3") ||
		!strings.Contains(status.text, "poll 2") {
		t.Fatalf("repeat progress status = %q", status.text)
	}
}

func TestRepeatProgressStatusOmitsTheOnlyCycle(t *testing.T) {
	t.Parallel()
	for _, phase := range []xexec.RepeatPhase{
		xexec.RepeatAttempt,
		xexec.RepeatRetryWait,
		xexec.RepeatPollWait,
	} {
		status := repeatProgressStatus(xexec.RepeatProgress{
			Phase:   phase,
			Attempt: 2,
			Poll:    1,
			Delay:   100 * time.Millisecond,
		})
		if status.text == "" || strings.Contains(status.text, "poll") {
			t.Fatalf("phase %d status = %q", phase, status.text)
		}
	}
}
