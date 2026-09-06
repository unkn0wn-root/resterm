package recorder

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const minTapBuffer = 4096

type exchangeKey struct{}

type exchange struct {
	request  *bodyTap
	response *bodyTap
	headers  http.Header
	status   int
	issue    Issue
}

func exchangeFrom(ctx context.Context) *exchange {
	x, _ := ctx.Value(exchangeKey{}).(*exchange)
	return x
}

// Save the original request before the proxy changes it.
type capture struct {
	started  time.Time
	method   string
	uri      string
	headers  http.Header
	exchange *exchange
}

// Do not hold mu during network reads. Read and Close may run concurrently.
type bodyTap struct {
	io.ReadCloser
	store *store

	mu       sync.Mutex
	buf      []byte
	n        int64
	expected int64
	eof      bool
	closed   bool
	issue    Issue
	once     sync.Once
}

func newBodyTap(s *store, body io.ReadCloser, length int64) *bodyTap {
	if body == nil {
		body = http.NoBody
	}
	return &bodyTap{ReadCloser: body, store: s, expected: length, eof: body == http.NoBody}
}

func (b *bodyTap) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return n, err
	}
	b.n += int64(n)
	switch {
	case err == io.EOF:
		b.eof = true
	case err != nil:
		b.issue = issueBodyRead
	}
	if b.issue.ok() && n > 0 && b.grow(n) {
		b.buf = append(b.buf, p[:n]...)
	}
	return n, err
}

func (b *bodyTap) grow(n int) bool {
	limit := b.store.cfg.BodyLimit
	need := len(b.buf) + n
	if int64(need) > limit {
		b.issue = issueBodyLimit
		return false
	}
	if need <= cap(b.buf) {
		return true
	}

	size := min(int(limit), max(need, max(minTapBuffer, cap(b.buf)*2)))
	if !b.store.reserve(int64(size - cap(b.buf))) {
		b.issue = issueCaptureBuffer
		return false
	}
	buf := make([]byte, len(b.buf), size)
	copy(buf, b.buf)
	b.buf = buf
	return true
}

func (b *bodyTap) Close() error {
	var err error
	b.once.Do(func() { err = b.ReadCloser.Close() })
	return err
}

func (b *bodyTap) result() ([]byte, Issue) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	if b.issue.ok() && !b.eof && (b.expected < 0 || b.n != b.expected) {
		b.issue = issueBodyIncomplete
	}
	if !b.issue.ok() {
		return nil, b.issue
	}
	return b.buf, ""
}

// Callers must copy any data they need before discard clears the buffer.
func (b *bodyTap) discard() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.store.release(int64(cap(b.buf)))
	clear(b.buf)
	b.buf = nil
}

func (s *Session) captureResponse(resp *http.Response) error {
	x := exchangeFrom(resp.Request.Context())
	if x == nil {
		return nil
	}

	x.status = resp.StatusCode
	if headerSize(resp.Header) > maxMetadataBytes {
		x.issue = issueHeaderLimit
		return nil
	}
	x.headers = resp.Header.Clone()
	if strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		x.issue = issueEventStream
		return nil
	}

	x.response = newBodyTap(s.store, resp.Body, resp.ContentLength)
	resp.Body = x.response
	return nil
}

func (s *Session) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect || r.Header.Get("Upgrade") != "" {
		s.store.refuse()
		http.Error(w, "recorder: tunnels and protocol upgrades are unsupported", http.StatusNotImplemented)
		return
	}
	if !s.store.admit(r) {
		s.proxy.ServeHTTP(w, r)
		return
	}
	defer s.store.captureDone()

	c := capture{
		started:  time.Now(),
		method:   r.Method,
		uri:      r.URL.RequestURI(),
		headers:  r.Header.Clone(),
		exchange: &exchange{request: newBodyTap(s.store, r.Body, r.ContentLength)},
	}
	r = r.WithContext(context.WithValue(r.Context(), exchangeKey{}, c.exchange))
	r.Body = c.exchange.request

	forwarded := false
	defer func() { s.publish(c, forwarded) }()
	s.proxy.ServeHTTP(w, r)
	forwarded = true
}

func (s *Session) publish(c capture, forwarded bool) {
	x := c.exchange
	_ = x.request.Close()
	request, requestIssue := x.request.result()
	defer x.request.discard()

	var response []byte
	responseIssue := x.issue
	switch {
	case x.response != nil:
		response, responseIssue = x.response.result()
		defer x.response.discard()
	case responseIssue.ok():
		responseIssue = issueNoResponse
	}
	if !forwarded {
		response, responseIssue = nil, issueInterrupted
	}

	e := Entry{
		Started:  c.started,
		Duration: time.Since(c.started),
		Method:   c.method,
		URL:      s.cfg.Upstream + c.uri,
		Status:   x.status,
		Request:  Message{Headers: c.headers, Body: request, Issue: requestIssue},
		Response: Message{Headers: x.headers, Body: response, Issue: responseIssue},
	}
	s.sanitize(&e, int64(len(request)+len(response)))
	s.store.publish(e)
}

// Reserve space before decoding, including room for gzip expansion.
func (s *Session) sanitize(e *Entry, captured int64) {
	need := captured
	if gzipEncoded(e.Request.Headers) {
		need += s.cfg.BodyLimit
	}
	if gzipEncoded(e.Response.Headers) {
		need += s.cfg.BodyLimit
	}

	if s.store.reserve(need) {
		defer s.store.release(need)
	} else {
		e.Request.Body, e.Response.Body = nil, nil
		e.Request.Issue, e.Response.Issue = issueRedactBuffer, issueRedactBuffer
	}

	s.policy.sanitize(e, s.cfg.BodyLimit)
	e.Request.Body = bytes.Clone(e.Request.Body)
	e.Response.Body = bytes.Clone(e.Response.Body)
}

func gzipEncoded(h http.Header) bool {
	return strings.EqualFold(h.Get("Content-Encoding"), "gzip")
}
