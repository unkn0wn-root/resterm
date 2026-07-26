package request

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// The old version parked a goroutine per session waiting on the parent. Calling
// cancel did not stop it, so a long lived parent leaked one per stream.
func TestSessionLifetimeDoesNotParkAWatcher(t *testing.T) {
	before := runtime.NumGoroutine()

	lts := make([]*sessionLifetime, 0, 50)
	for range 50 {
		lts = append(lts, newSessionLifetime(context.Background()))
	}
	for _, lt := range lts {
		lt.close()
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

func TestSessionLifetimeFollowsParentUntilDetached(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	lt := newSessionLifetime(parent)
	defer lt.close()

	stop()
	select {
	case <-lt.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("session context did not follow parent cancellation")
	}
}

func TestSessionLifetimeOutlivesParentAfterDetach(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	lt := newSessionLifetime(parent)
	if !lt.detach() {
		t.Fatal("detach lost against a live parent")
	}
	stop()

	select {
	case <-lt.ctx.Done():
		t.Fatal("detached session context followed parent cancellation")
	case <-time.After(20 * time.Millisecond):
	}

	lt.cancel()
	select {
	case <-lt.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("session context did not honor explicit cancellation")
	}
}

// Losing the race has to be reported, and the context has to be done by then so
// the caller does not have to poll for it.
func TestSessionLifetimeDetachReportsLostRace(t *testing.T) {
	parent, stop := context.WithCancel(context.Background())
	lt := newSessionLifetime(parent)
	stop()

	if lt.detach() {
		t.Fatal("detach won against a canceled parent")
	}
	if err := lt.ctx.Err(); err == nil {
		t.Fatal("session context was still live after a lost detach")
	}
}

// Whichever side wins, the two outcomes stay consistent: a won detach keeps the
// session alive, a lost one leaves it canceled.
func TestSessionLifetimeDetachRacesParentCancel(t *testing.T) {
	for range 200 {
		parent, stop := context.WithCancel(context.Background())
		lt := newSessionLifetime(parent)

		var wg sync.WaitGroup
		wg.Add(1)
		var won bool
		go func() {
			defer wg.Done()
			won = lt.detach()
		}()
		stop()
		wg.Wait()

		switch err := lt.ctx.Err(); {
		case won && err != nil:
			t.Fatalf("detach won but the session context was canceled: %v", err)
		case !won && err == nil:
			t.Fatal("detach lost but the session context was still live")
		}
		lt.close()
	}
}
