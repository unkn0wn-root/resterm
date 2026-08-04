package ui

import (
	"io"
	"net/http"
	"strings"
	"testing"

	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type warningRoundTripFunc func(*http.Request) (*http.Response, error)

func (f warningRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRunCfgAllowsInteractiveOAuth(t *testing.T) {
	model := New(Config{})

	cfg := model.runCfg(httpx.Options{})
	if !cfg.AllowInteractiveOAuth {
		t.Fatalf("expected UI request engine config to allow interactive OAuth")
	}
}

func TestUIRequestEngineQueuesInsecureSSHWarningOnce(t *testing.T) {
	client := httpx.NewClientWithOptions(
		httpx.WithHTTPFactory(func(httpx.Options) (*http.Client, error) {
			return &http.Client{
				Transport: warningRoundTripFunc(
					func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							Status:     "200 OK",
							StatusCode: http.StatusOK,
							Header:     make(http.Header),
							Body:       io.NopCloser(strings.NewReader("ok")),
							Request:    req,
						}, nil
					},
				),
			}, nil
		}),
	)
	model := New(Config{Client: client})
	svc := model.runRequestSvc(httpx.Options{})
	if svc == nil {
		t.Fatal("expected UI request engine")
	}
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		SSH: &restfile.SSHSpec{
			Inline: &restfile.SSHProfile{
				Host: "jump.example.test",
				Strict: restfile.Opt[bool]{
					Val: false,
					Set: true,
				},
			},
		},
	}

	callbackCount := 0
	for range 2 {
		res, err := svc.ExecuteWith(nil, req, testEnv(""), rqeng.ExecOptions{
			OnWarning: func(rqeng.Warning) {
				callbackCount++
			},
		})

		if err != nil {
			t.Fatalf("ExecuteWith() error = %v", err)
		}
		if res.Err != nil {
			t.Fatalf("ExecuteWith() result error = %v", res.Err)
		}
	}
	if callbackCount != 2 {
		t.Fatalf("caller warning callback count = %d, want 2", callbackCount)
	}

	var msg runWarningMsg
	select {
	case queued := <-model.runMsgChan:
		var ok bool
		msg, ok = queued.(runWarningMsg)
		if !ok {
			t.Fatalf("queued message type = %T, want runWarningMsg", queued)
		}
	default:
		t.Fatal("expected queued request warning")
	}
	if msg.text != string(rqeng.WarningSSHHostKeyVerificationDisabled) {
		t.Fatalf("warning = %q", msg.text)
	}
	select {
	case extra := <-model.runMsgChan:
		t.Fatalf("unexpected duplicate warning message: %#v", extra)
	default:
	}

	next, cmd := model.Update(msg)
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() model type = %T, want Model", next)
	}
	if updated.statusMessage.level != statusWarn {
		t.Fatalf("status level = %v, want statusWarn", updated.statusMessage.level)
	}
	if updated.statusMessage.text != string(rqeng.WarningSSHHostKeyVerificationDisabled) {
		t.Fatalf("status message = %q", updated.statusMessage.text)
	}
	if cmd == nil {
		t.Fatal("expected warning update to continue the run-message subscription")
	}

	sentinel := runWorkerDoneMsg{runID: "next"}
	model.runMsgChan <- sentinel
	if queued := cmd(); queued != sentinel {
		t.Fatalf("next run message = %#v, want %#v", queued, sentinel)
	}
}
