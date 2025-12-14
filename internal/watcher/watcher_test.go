package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChangeDetected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.http")
	write(t, path, []byte("one"))

	w := New(Options{Interval: time.Hour})
	defer w.Stop()
	w.Track(path, []byte("one"))

	write(t, path, []byte("two"))
	w.Scan()

	evt := recv(t, w.Events())
	if evt.Path != path {
		t.Fatalf("expected path %q, got %q", path, evt.Path)
	}
	if evt.Kind != EventChanged {
		t.Fatalf("expected EventChanged, got %v", evt.Kind)
	}
	if evt.Prev.Hash == evt.Curr.Hash {
		t.Fatalf("expected differing hashes")
	}
}

func TestTrackAfterSaveSuppressesEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.http")
	write(t, path, []byte("draft"))

	w := New(Options{Interval: time.Hour})
	defer w.Stop()
	w.Track(path, []byte("draft"))

	write(t, path, []byte("draft updated"))
	w.Track(path, []byte("draft updated"))
	w.Scan()

	select {
	case evt := <-w.Events():
		t.Fatalf("expected no event, got %+v", evt)
	default:
	}
}

func TestMissingEmitsOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.http")
	write(t, path, []byte("keep"))

	w := New(Options{Interval: time.Hour})
	defer w.Stop()
	w.Track(path, []byte("keep"))

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	w.Scan()

	evt := recv(t, w.Events())
	if evt.Kind != EventMissing {
		t.Fatalf("expected EventMissing, got %v", evt.Kind)
	}

	w.Scan()
	select {
	case evt := <-w.Events():
		t.Fatalf("expected no second missing event, got %+v", evt)
	default:
	}
}

func TestMissingThenReappearsSameContentEmitsChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reappear.http")
	content := []byte("keep")
	write(t, path, content)
	origMod := time.Now().Add(-time.Hour)
	setModTime(t, path, origMod)

	w := New(Options{Interval: time.Hour})
	defer w.Stop()
	w.Track(path, content)

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	w.Scan()
	missing := recv(t, w.Events())
	if missing.Kind != EventMissing {
		t.Fatalf("expected missing event, got %v", missing.Kind)
	}

	write(t, path, content)
	setModTime(t, path, origMod)
	w.Scan()

	evt := recv(t, w.Events())
	if evt.Kind != EventChanged {
		t.Fatalf("expected EventChanged on reappear, got %v", evt.Kind)
	}
	if evt.Prev.Hash != evt.Curr.Hash {
		t.Fatalf("expected same content hash after reappear")
	}
}

func TestDetectsChangeWhenModTimeAndSizeUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.http")
	first := []byte("one")
	second := []byte("two") // same length
	write(t, path, first)
	fixedMod := time.Now().Add(-2 * time.Hour)
	setModTime(t, path, fixedMod)

	w := New(Options{Interval: time.Hour, HashUnchanged: true})
	defer w.Stop()
	w.Track(path, first)

	write(t, path, second)
	setModTime(t, path, fixedMod)
	w.Scan()

	evt := recv(t, w.Events())
	if evt.Kind != EventChanged {
		t.Fatalf("expected EventChanged, got %v", evt.Kind)
	}
	if evt.Prev.Hash == evt.Curr.Hash {
		t.Fatalf("expected differing hashes for changed content")
	}
}

func TestSkipsHashWhenMetadataUnchangedByDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.http")
	first := []byte("one")
	second := []byte("two") // same length
	write(t, path, first)
	fixedMod := time.Now().Add(-2 * time.Hour)
	setModTime(t, path, fixedMod)

	w := New(Options{Interval: time.Hour})
	defer w.Stop()
	w.Track(path, first)

	write(t, path, second)
	setModTime(t, path, fixedMod)
	w.Scan()

	select {
	case evt := <-w.Events():
		t.Fatalf("expected no event when metadata unchanged by default, got %+v", evt)
	default:
	}
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func setModTime(t *testing.T, path string, ts time.Time) {
	t.Helper()
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func recv(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case evt := <-ch:
		return evt
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for event")
		return Event{}
	}
}
