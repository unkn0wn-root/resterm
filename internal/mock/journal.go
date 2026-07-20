package mock

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
)

type IncompleteError struct {
	Reason string
}

func (e *IncompleteError) Error() string {
	return "mock request journal is incomplete: " + e.Reason
}

type requestRecord struct {
	method        string
	path          string
	rawPath       string
	host          string
	query         url.Values
	headers       http.Header
	body          []byte
	bodyTruncated bool
	size          int64
}

func (r requestRecord) headerValues(name string) []string {
	if strings.EqualFold(name, "Host") {
		if r.host == "" {
			return nil
		}
		return []string{r.host}
	}
	return r.headers.Values(name)
}

type requestJournal struct {
	mu sync.RWMutex

	entries      []requestRecord
	entryLimit   int
	byteLimit    int64
	bodyLimit    int64
	retained     int64
	isIncomplete bool
}

type JournalStats struct {
	Entries    int
	EntryLimit int
	Retained   int64
	ByteLimit  int64
	Complete   bool
}

func newRequestJournal(opts Options) (*requestJournal, error) {
	entryLimit := opts.JournalEntries
	if entryLimit <= 0 {
		entryLimit = DefaultJournalEntries
	}
	byteLimit := opts.JournalBytes
	if byteLimit <= 0 {
		byteLimit = DefaultJournalBytes
	}
	bodyLimit := opts.JournalBodyLimit
	if bodyLimit <= 0 {
		bodyLimit = DefaultJournalBodyLimit
	}
	maxInt := int64(^uint(0) >> 1)
	switch {
	case bodyLimit >= maxInt:
		return nil, fmt.Errorf("mock journal body limit is too large")
	case bodyLimit > byteLimit:
		return nil, fmt.Errorf("mock journal body limit exceeds total byte limit")
	}
	return &requestJournal{
		entryLimit: entryLimit,
		byteLimit:  byteLimit,
		bodyLimit:  bodyLimit,
	}, nil
}

func (j *requestJournal) add(entry requestRecord) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if entry.size > j.byteLimit {
		j.isIncomplete = true
		return
	}
	for len(j.entries) >= j.entryLimit || entry.size > j.byteLimit-j.retained {
		j.dropOldest()
		j.isIncomplete = true
	}
	j.entries = append(j.entries, entry)
	j.retained += entry.size
}

// dropOldest advances the slice head instead of shifting entries down. The
// vacated backing array is reclaimed when append reallocates.
func (j *requestJournal) dropOldest() {
	if len(j.entries) == 0 {
		return
	}
	j.retained -= j.entries[0].size
	j.entries[0] = requestRecord{}
	j.entries = j.entries[1:]
}

func (j *requestJournal) clear() {
	j.mu.Lock()
	j.entries = nil
	j.retained = 0
	j.isIncomplete = false
	j.mu.Unlock()
}

func (j *requestJournal) count(ctx context.Context, pattern RequestPattern) (uint64, error) {
	compiled, err := compileRequestPattern(pattern)
	if err != nil {
		return 0, err
	}
	j.mu.RLock()
	entries := slices.Clone(j.entries)
	isIncomplete := j.isIncomplete
	j.mu.RUnlock()
	if isIncomplete {
		return 0, &IncompleteError{
			Reason: "older requests were evicted or dropped; clear the journal to reset verification",
		}
	}

	var count uint64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		matched, err := compiled.matches(entry)
		if err != nil {
			return 0, err
		}
		if matched {
			count++
		}
	}
	return count, nil
}

func (j *requestJournal) stats() JournalStats {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return JournalStats{
		Entries:    len(j.entries),
		EntryLimit: j.entryLimit,
		Retained:   j.retained,
		ByteLimit:  j.byteLimit,
		Complete:   !j.isIncomplete,
	}
}

// capture snapshots the request for the journal and replaces r.Body so
// the mock handler can still read it. When the body read fails, the replacement
// replays both the captured prefix and the same error.
func (j *requestJournal) capture(r *http.Request) (requestRecord, error) {
	entry := requestRecord{
		method:  r.Method,
		path:    r.URL.Path,
		rawPath: r.URL.RawPath,
		host:    r.Host,
		query:   r.URL.Query(),
		headers: r.Header.Clone(),
	}
	var readErr error
	if r.Body != nil && r.Body != http.NoBody {
		body := r.Body
		b, err := io.ReadAll(io.LimitReader(body, j.bodyLimit+1))
		entry.body = b
		switch {
		case err != nil:
			entry.bodyTruncated = true
			r.Body = &replayReadCloser{
				Reader: io.MultiReader(bytes.NewReader(b), terminalErrorReader{err: err}),
				Closer: body,
			}
			readErr = fmt.Errorf("read request journal body: %w", err)
		case int64(len(b)) > j.bodyLimit:
			entry.bodyTruncated = true
			r.Body = &replayReadCloser{
				Reader: io.MultiReader(bytes.NewReader(b), body),
				Closer: body,
			}
		default:
			// the limited read drained the body, so replay the prefix alone
			r.Body = &replayReadCloser{Reader: bytes.NewReader(b), Closer: body}
		}
		if int64(len(entry.body)) > j.bodyLimit {
			entry.body = entry.body[:j.bodyLimit]
		}
	}
	entry.size = entry.retainedSize()
	return entry, readErr
}

func (r requestRecord) retainedSize() int64 {
	size := len(r.method) + len(r.path) + len(r.rawPath) + len(r.host)
	for name, values := range r.query {
		size += len(name)
		for _, value := range values {
			size += len(value)
		}
	}
	for name, values := range r.headers {
		size += len(name)
		for _, value := range values {
			size += len(value)
		}
	}
	return int64(size + len(r.body))
}

type replayReadCloser struct {
	io.Reader
	io.Closer
}

type terminalErrorReader struct {
	err error
}

func (r terminalErrorReader) Read([]byte) (int, error) {
	return 0, r.err
}
