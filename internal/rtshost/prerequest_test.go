package rtshost

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts"
)

func TestRunPreRequestSkipsNonRTSPreRequestBlocks(t *testing.T) {
	eng := rts.NewEng(nil)
	calls := 0
	scripts := []restfile.ScriptBlock{
		{Kind: "pre-request", Lang: "js", Body: "not rts"},
		{Kind: "test", Lang: "rts", Body: "not pre"},
		{Kind: "pre-request", Lang: "rts", Body: "let x = 1"},
	}

	err := RunPreRequest(context.Background(), eng, PreRequest{
		Scripts: scripts,
		Runtime: func() rts.RT {
			calls++
			return rts.RT{}
		},
	})
	if err != nil {
		t.Fatalf("RunPreRequest: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one RTS pre-request execution, got %d", calls)
	}
}

func TestRunPreRequestNumbersRTSBlocksOnly(t *testing.T) {
	scripts := []restfile.ScriptBlock{
		{Kind: "pre-request", Lang: "js", Body: "not rts"},
		{Kind: "pre-request", Lang: "rts", FilePath: "missing.rts"},
	}

	err := RunPreRequest(context.Background(), rts.NewEng(nil), PreRequest{
		Scripts: scripts,
		BaseDir: t.TempDir(),
		Runtime: func() rts.RT { return rts.RT{} },
	})
	if err == nil {
		t.Fatalf("expected missing script file to fail")
	}
	if !strings.Contains(err.Error(), "rts pre-request script 1:") {
		t.Fatalf("expected the first RTS block to be reported, got %v", err)
	}
}

func TestRunPreRequestStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := RunPreRequest(ctx, rts.NewEng(nil), PreRequest{
		Scripts: []restfile.ScriptBlock{{Kind: "pre-request", Lang: "rts", Body: "let x = 1"}},
		Runtime: func() rts.RT {
			t.Fatalf("expected no execution after cancel")
			return rts.RT{}
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
