package headless

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestEvaluateStepStatusCode(t *testing.T) {
	res := wfStepRes{
		step: restfile.WorkflowStep{
			Expect: map[string]string{
				workflowExpectStatusCode: "404",
			},
		},
		http: &httpclient.Response{
			Status:     http.StatusText(http.StatusNotFound),
			StatusCode: http.StatusNotFound,
			Duration:   1500 * time.Millisecond,
		},
	}

	got := evaluateStep(res)
	if !got.ok {
		t.Fatalf("expected step to pass, got %+v", got)
	}
	if got.msg != "" {
		t.Fatalf("expected empty message, got %q", got.msg)
	}
	if got.status != http.StatusText(http.StatusNotFound) {
		t.Fatalf("expected status %q, got %q", http.StatusText(http.StatusNotFound), got.status)
	}
	if got.dur != 1500*time.Millisecond {
		t.Fatalf("expected duration %s, got %s", 1500*time.Millisecond, got.dur)
	}
}

func TestEvaluateStepScriptErr(t *testing.T) {
	res := wfStepRes{
		http: &httpclient.Response{
			Status:     http.StatusText(http.StatusOK),
			StatusCode: http.StatusOK,
			Duration:   250 * time.Millisecond,
		},
		sErr: errors.New("script crashed"),
	}

	got := evaluateStep(res)
	if got.ok {
		t.Fatalf("expected step to fail, got %+v", got)
	}
	if got.msg != "script crashed" {
		t.Fatalf("expected script error message, got %q", got.msg)
	}
	if got.status != http.StatusText(http.StatusOK) {
		t.Fatalf("expected transport status to be preserved, got %q", got.status)
	}
	if got.dur != 250*time.Millisecond {
		t.Fatalf("expected duration %s, got %s", 250*time.Millisecond, got.dur)
	}
}
