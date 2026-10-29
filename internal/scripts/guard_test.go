package scripts

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/dop251/goja"

	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func endlessScript(kind restfile.ScriptKind) []restfile.ScriptBlock {
	return []restfile.ScriptBlock{{Kind: kind, Lang: "js", Body: "while (true) {}"}}
}

func TestRunTestsStopsAtTheScriptTimeout(t *testing.T) {
	t.Parallel()

	runner := NewRunner(nil)
	runner.timeout = 50 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		_, _, err := runner.RunTests(
			context.Background(),
			endlessScript("test"),
			TestInput{Response: &Response{}},
		)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("RunTests() = nil, want the script time limit")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunTests did not stop at its time limit")
	}
}

func TestRunTestsStopsWhenTheCallerCancels(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	runner := NewRunner(nil)

	done := make(chan error, 1)
	go func() {
		_, _, err := runner.RunTests(ctx, endlessScript("test"), TestInput{Response: &Response{}})
		done <- err
	}()

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunTests() = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunTests did not stop when the caller cancelled")
	}
}

func TestRunTestsReportsTheStopCause(t *testing.T) {
	why := errors.New("response checks timed out")
	expired, cancel := context.WithDeadlineCause(t.Context(), time.Now(), why)
	defer cancel()
	test := []restfile.ScriptBlock{{Kind: "test", Lang: "js", Body: "1 + 1;"}}
	others := []restfile.ScriptBlock{
		{Kind: "pre-request", Lang: "js", Body: "1 + 1;"},
		{Kind: "test", Lang: "rts", Body: "1"},
	}
	tests := []struct {
		name    string
		ctx     context.Context
		scripts []restfile.ScriptBlock
		want    error
	}{
		{"expired", expired, test, why},
		{"expired without a JavaScript test", expired, others, nil},
		{"live", t.Context(), test, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, _, err := NewRunner(nil).RunTests(tt.ctx, tt.scripts, TestInput{Response: &Response{}})
			if !errors.Is(err, tt.want) || len(results) != 0 {
				t.Fatalf("RunTests() = %+v, %v, want no results and %v", results, err, tt.want)
			}
		})
	}
}

func TestPreRequestStopsAtTheScriptTimeout(t *testing.T) {
	t.Parallel()

	runner := NewRunner(nil)
	runner.timeout = 50 * time.Millisecond
	req := &restfile.Request{Method: "GET", URL: "https://example.com"}

	done := make(chan error, 1)
	go func() {
		_, err := runner.RunPreRequest(
			endlessScript("pre-request"),
			prerequest.Input{Request: req},
		)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("RunPreRequest() = nil, want the script time limit")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunPreRequest did not stop at its time limit")
	}
}

func TestGuardReportsWhatStoppedTheScript(t *testing.T) {
	why := errors.New("response checks timed out")
	soon, cancel := context.WithTimeoutCause(t.Context(), 20*time.Millisecond, why)
	defer cancel()
	late, cancelLate := context.WithTimeoutCause(t.Context(), time.Minute, why)
	defer cancelLate()
	stopped, stop := context.WithCancelCause(t.Context())
	stop(why)
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"context deadline first", soon, "response checks timed out"},
		{"budget first", late, "script exceeded the 20ms time limit"},
		{"canceled context", stopped, "response checks timed out"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := goja.New()
			release := guardVM(tt.ctx, vm, 20*time.Millisecond)
			defer release()
			_, err := vm.RunString("while (true) {}")
			if got := stopReason(tt.ctx, err); got == nil || got.Error() != tt.want {
				t.Fatalf("stopReason() = %v, want %q", got, tt.want)
			}
		})
	}
}

func TestGuardReleasesItsWatcher(t *testing.T) {
	ctx := t.Context()

	before := runtime.NumGoroutine()
	runner := NewRunner(nil)
	for range 50 {
		if _, _, err := runner.RunTests(
			ctx,
			[]restfile.ScriptBlock{{Kind: "test", Lang: "js", Body: "1 + 1;"}},
			TestInput{Response: &Response{}},
		); err != nil {
			t.Fatalf("RunTests: %v", err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+5 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if leaked := runtime.NumGoroutine() - before; leaked > 5 {
		t.Fatalf("%d goroutines outlived their scripts", leaked)
	}
}
