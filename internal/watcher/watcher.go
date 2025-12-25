package watcher

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type EventKind int

const (
	EventChanged EventKind = iota
	EventMissing
)

type Fingerprint struct {
	Mod  time.Time
	Size int64
	Hash string
}

type Event struct {
	Path string
	Kind EventKind
	Prev Fingerprint
	Curr Fingerprint
}

type Options struct {
	Interval time.Duration
	Buffer   int
	// HashUnchanged forces a content hash even when modtime and size are unchanged.
	// When false (default), scans skip hashing if metadata is unchanged to avoid
	// unnecessary I/O for large files. Maybe I was overthinking here but though
	// that I'll keep that so if I would ever wire up forcing full file hashing
	// i know where and why. Behavior is still the same anyways.
	HashUnchanged bool
}

type entry struct {
	path    string
	fp      Fingerprint
	missing bool
}

type Watcher struct {
	mu       sync.RWMutex
	entries  map[string]*entry
	out      chan Event
	interval time.Duration
	hashAll  bool
	stop     chan struct{}
	wg       sync.WaitGroup
	started  bool
	closed   bool
}

const (
	defaultInterval = time.Second
	defaultBuffer   = 16
	hashPrefix      = "sha256:"
)

func New(opts Options) *Watcher {
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	buf := opts.Buffer
	if buf <= 0 {
		buf = defaultBuffer
	}
	return &Watcher{
		entries:  make(map[string]*entry),
		out:      make(chan Event, buf),
		interval: interval,
		hashAll:  opts.HashUnchanged,
	}
}

func (w *Watcher) Events() <-chan Event {
	return w.out
}

func (w *Watcher) Start() {
	w.mu.Lock()
	if w.started || w.closed {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.stop = make(chan struct{})
	w.wg.Add(1)
	w.mu.Unlock()

	go func() {
		defer w.wg.Done()
		t := time.NewTicker(w.interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				w.Scan()
			case <-w.stop:
				return
			}
		}
	}()
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	if w.started && w.stop != nil {
		close(w.stop)
	}
	w.mu.Unlock()
	if w.started {
		w.wg.Wait()
	}
	close(w.out)
}

func (w *Watcher) Track(path string, data []byte) {
	clean, ok := cleanPath(path)
	if !ok {
		return
	}
	fp := buildFingerprint(clean, data)

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.entries[clean] = &entry{path: clean, fp: fp}
}

func (w *Watcher) Forget(path string) {
	clean, ok := cleanPath(path)
	if !ok {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.entries, clean)
}

func (w *Watcher) Scan() {
	if w.isClosed() {
		return
	}

	entries := w.snapshot()
	for _, e := range entries {
		if evt, ok := w.check(e); ok {
			w.emit(evt)
		}
	}
}

func (w *Watcher) snapshot() []*entry {
	w.mu.RLock()
	defer w.mu.RUnlock()

	list := make([]*entry, 0, len(w.entries))
	for _, e := range w.entries {
		list = append(list, &entry{
			path:    e.path,
			fp:      e.fp,
			missing: e.missing,
		})
	}
	return list
}

func (w *Watcher) check(e *entry) (Event, bool) {
	info, err := os.Stat(e.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if e.missing {
				return Event{}, false
			}
			w.markMissing(e.path)
			return Event{Path: e.path, Kind: EventMissing, Prev: e.fp}, true
		}
		return Event{}, false
	}

	metaUnchanged := metaSame(info, e.fp, w.hashAll, e.missing)
	if metaUnchanged {
		return Event{}, false
	}

	data, readErr := os.ReadFile(e.path)
	if readErr != nil {
		// treat read failure as missing to avoid silence
		w.markMissing(e.path)
		return Event{Path: e.path, Kind: EventMissing, Prev: e.fp}, true
	}

	next := fingerprintFromStat(info, data)
	changed := e.missing || next.Hash != e.fp.Hash || !next.Mod.Equal(e.fp.Mod) ||
		next.Size != e.fp.Size

	prev := e.fp
	w.updateEntry(e.path, next, false)
	if !changed {
		return Event{}, false
	}

	return Event{Path: e.path, Kind: EventChanged, Prev: prev, Curr: next}, true
}

func (w *Watcher) markMissing(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e, ok := w.entries[path]; ok {
		e.missing = true
	}
}

func (w *Watcher) updateEntry(path string, fp Fingerprint, missing bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e, ok := w.entries[path]; ok {
		e.fp = fp
		e.missing = missing
	}
}

func (w *Watcher) emit(evt Event) {
	w.mu.RLock()
	if w.closed {
		w.mu.RUnlock()
		return
	}
	select {
	case w.out <- evt:
	default:
	}
	w.mu.RUnlock()
}

func (w *Watcher) isClosed() bool {
	w.mu.RLock()
	closed := w.closed
	w.mu.RUnlock()
	return closed
}

func metaSame(info fs.FileInfo, fp Fingerprint, hashAll bool, missing bool) bool {
	if missing || hashAll || info == nil {
		return false
	}
	return info.ModTime().Equal(fp.Mod) && info.Size() == fp.Size
}

func cleanPath(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	clean := filepath.Clean(path)
	if clean == "" || clean == "." {
		return "", false
	}
	return clean, true
}

func buildFingerprint(path string, data []byte) Fingerprint {
	info, err := os.Stat(path)
	if err != nil {
		return Fingerprint{Hash: hashBytes(data), Size: int64(len(data))}
	}
	return fingerprintFromStat(info, data)
}

func fingerprintFromStat(info fs.FileInfo, data []byte) Fingerprint {
	mod := time.Time{}
	if info != nil {
		mod = info.ModTime()
	}
	return Fingerprint{
		Mod:  mod,
		Size: int64(len(data)),
		Hash: hashBytes(data),
	}
}

func hashBytes(data []byte) string {
	if len(data) == 0 {
		return hashPrefix + "0"
	}
	sum := sha256Sum(data)
	return hashPrefix + sum
}

func sha256Sum(data []byte) string {
	h := sha256.New()
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
