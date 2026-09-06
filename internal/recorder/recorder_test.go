package recorder

import (
	"context"
	"testing"
	"time"
)

func startTest(t *testing.T, upstream string, configure func(*Config)) *Session {
	t.Helper()
	c := DefaultConfig()
	c.Listen = "127.0.0.1:0"
	c.Upstream = upstream
	if configure != nil {
		configure(&c)
	}
	s, err := Start(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
		defer cancel()
		_ = s.Close(ctx)
	})
	return s
}

func entriesAfter(t *testing.T, s *Session, n int) []Entry {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		entries := s.Snapshot(0)
		if len(entries) >= n {
			return entries
		}
		select {
		case <-s.Updates():
		case <-ctx.Done():
			t.Fatalf("recordings: got %d, want %d; stats %+v", len(entries), n, s.Stats())
		}
	}
}
