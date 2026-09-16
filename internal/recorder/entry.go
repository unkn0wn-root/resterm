package recorder

import (
	"bytes"
	"net/http"
	"time"
)

// An empty Issue means success, including for a valid empty body.
type Issue string

func (i Issue) ok() bool { return i == "" }

const (
	issueBodyRead       Issue = "body read failed"
	issueBodyLimit      Issue = "body exceeds capture limit"
	issueBodyIncomplete Issue = "body was not completely read"
	issueCaptureBuffer  Issue = "capture buffer limit reached"
	issueRedactBuffer   Issue = "redaction buffer limit reached"
	issueHeaderLimit    Issue = "response headers exceed capture limit"
	issueEventStream    Issue = "event streams cannot be exported"
	issueNoResponse     Issue = "no upstream response was captured"
	issueInterrupted    Issue = "response forwarding was interrupted"
)

const (
	issueEncoding      Issue = "unsupported content encoding"
	issueGzip          Issue = "invalid gzip body"
	issueDecodedBody   Issue = "invalid or oversized decoded body"
	issueBinary        Issue = "binary body cannot be exported"
	issueContentType   Issue = "invalid content type"
	issueJSON          Issue = "invalid JSON, excessive nesting, or duplicate object keys"
	issueForm          Issue = "form cannot be parsed"
	issueMediaType     Issue = "unsupported body media type"
	issueRedactedLimit Issue = "redacted body exceeds capture limit"
	issueURL           Issue = "URL cannot be safely rewritten"
)

const (
	issueRequestMethodCase Issue = "method casing cannot be preserved by the request parser"
	issueRequestTemplate   Issue = "request URL contains template syntax"
	issueRequestMethod     Issue = "request method is unsupported by the request parser"
	issueHeaderTemplate    Issue = "request header contains template syntax"
	issueRequestURL        Issue = "request URL cannot be represented"
	issueRequestRender     Issue = "request cannot be written and read back unchanged"
	issueMockMethodCase    Issue = "method casing cannot be preserved by the mock writer"
	issueMockStatus        Issue = "upstream status cannot be mocked"
	issueMockURL           Issue = "mock URL cannot be represented"
	issueMockPath          Issue = "path cannot be represented by the mock router"
	issueMockQuery         Issue = "query cannot be represented"
	issueMockMatchLimit    Issue = "request body is too large to use as a mock matcher"
	issueMockEngine        Issue = "route or match cannot be represented by the mock engine"
	issueMockRender        Issue = "mock cannot be written and read back unchanged"
)

type Message struct {
	Headers    http.Header
	Body       []byte
	Issue      Issue
	Redactions int
	// MatchJSON excludes redacted fields and arrays containing them.
	MatchJSON []byte
}

type Entry struct {
	ID       uint64
	Started  time.Time
	Duration time.Duration
	Method   string
	URL      string
	Status   int
	Request  Message
	Response Message
}

func (e Entry) clone() Entry {
	e.Request = e.Request.clone()
	e.Response = e.Response.clone()
	return e
}

func (e Entry) summary() Entry {
	e.Request = Message{Issue: e.Request.Issue, Redactions: e.Request.Redactions}
	e.Response = Message{Issue: e.Response.Issue, Redactions: e.Response.Redactions}
	return e
}

func (m Message) clone() Message {
	m.Headers = m.Headers.Clone()
	m.Body = bytes.Clone(m.Body)
	m.MatchJSON = bytes.Clone(m.MatchJSON)
	return m
}

const (
	entryOverhead       = 512
	headerValueOverhead = 32
)

func (e Entry) size() int64 {
	return int64(len(e.Method) + len(e.URL) +
		len(e.Request.Body) + len(e.Response.Body) + len(e.Request.MatchJSON) +
		headerSize(e.Request.Headers) + headerSize(e.Response.Headers) + entryOverhead)
}

func headerSize(h http.Header) int {
	n := 0
	for name, values := range h {
		n += len(name)
		for _, value := range values {
			n += len(value) + headerValueOverhead
		}
	}
	return n
}

// Limit records the first storage limit reached. Forwarding continues.
type Limit string

const (
	LimitEntries Limit = "entry limit reached"
	LimitBytes   Limit = "retained byte limit reached"
)

type Stats struct {
	Received       uint64
	Entries        int
	Excluded       uint64
	RetainedBytes  int64
	ActiveCaptures int
	Limit          Limit
	Running        bool
}
