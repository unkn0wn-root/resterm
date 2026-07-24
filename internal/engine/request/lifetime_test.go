package request

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// The old version parked a goroutine per session waiting on the parent. Calling
// cancel did not stop it, so a long lived parent leaked one per stream.
func TestSessionContextDoesNotParkAWatcher(t *testing.T) {
	before := runtime.NumGoroutine()

	cancels := make([]context.CancelFunc, 0, 50)
	for range 50 {
		_, cancel, _ := sessionContext(context.Background())
		cancels = append(cancels, cancel)
	}
	for _, cancel := range cancels {
		cancel()
	}

	var after int
	for range 100 {
		runtime.Gosched()
		if after = runtime.NumGoroutine(); after <= before+5 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("goroutines grew from %d to %d across 50 sessions", before, after)
}

func TestSessionContextFollowsParentUntilDetached(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	ctx, cancel, detach := sessionContext(parent)
	defer cancel()
	defer detach()

	stop()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("session context did not follow parent cancellation")
	}
}

func TestSessionContextOutlivesParentAfterDetach(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	ctx, cancel, detach := sessionContext(parent)
	detach()
	stop()

	select {
	case <-ctx.Done():
		t.Fatal("detached session context followed parent cancellation")
	case <-time.After(20 * time.Millisecond):
	}

	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("session context did not honor explicit cancellation")
	}
}
