package scripts

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/prerequest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func endlessScript(kind string) []restfile.ScriptBlock {
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

func TestGuardReleasesItsWatcher(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
