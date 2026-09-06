package recorder

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestConcurrentSnapshotsAndShutdown(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "ok")
	}))
	defer up.Close()
	s := startTest(t, up.URL, nil)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			r, err := http.Get("http://" + s.Addr() + "/")
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
			_ = s.Snapshot(0)
		})
	}
	wg.Wait()
	entries := entriesAfter(t, s, 20)
	for i, e := range entries {
		if e.ID != uint64(i+1) {
			t.Fatal("unordered publication")
		}
	}
	entries[0].Response.Body[0] = 'X'
	if string(s.Snapshot(0)[0].Response.Body) != "ok" {
		t.Fatal("snapshot aliases retained data")
	}
	for range 3 {
		wg.Go(func() {
			if err := s.Close(t.Context()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if s.Stats().Running {
		t.Fatal("still running")
	}
}

func TestCaptureSaturationStillForwards(t *testing.T) {
	entered, release := make(chan struct{}, 2), make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.WriteHeader(204)
	}))
	defer up.Close()
	s := startTest(t, up.URL, func(c *Config) { c.CaptureConcurrency = 1 })
	done := make(chan error, 2)
	for range 2 {
		go func() {
			r, err := http.Get("http://" + s.Addr() + "/")
			if err == nil {
				_, _ = io.Copy(io.Discard, r.Body)
				err = r.Body.Close()
			}
			done <- err
		}()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("request was blocked by capture limit")
		}
	}
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	entriesAfter(t, s, 1)
	if s.Stats().Excluded != 1 {
		t.Fatalf("stats: %+v", s.Stats())
	}
}

func TestForcedShutdownPublishesBeforeDone(t *testing.T) {
	entered := make(chan struct{})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "partial")
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
	}))
	defer up.Close()
	s := startTest(t, up.URL, nil)
	res, err := http.Get("http://" + s.Addr() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := s.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-s.Done():
	default:
		t.Fatal("done not closed")
	}
	entries := s.Snapshot(0)
	if len(entries) != 1 || entries[0].Response.Issue == "" || s.Stats().ActiveCaptures != 0 {
		t.Fatalf("incomplete shutdown: %+v %+v", entries, s.Stats())
	}
}

func TestUpstreamTLSVerification(t *testing.T) {
	up := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer up.Close()
	s := startTest(t, up.URL, nil)
	res, err := http.Get("http://" + s.Addr() + "/")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 502 {
		t.Fatalf("untrusted TLS accepted: %d", res.StatusCode)
	}
	e := entriesAfter(t, s, 1)[0]
	if e.Status != 0 || e.Response.Issue == "" {
		t.Fatal("proxy error recorded as upstream response")
	}
}
