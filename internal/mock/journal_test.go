package mock

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRequestJournalCountsMockPatternsAndExpectations(t *testing.T) {
	handler := compileSource(t, `### Payment webhook
# @mock method=POST path=/webhooks/{id}
# @match query={"kind":"payment"} headers={"Authorization":{"prefix":"Bearer "},"X-Trace":{"present":true},"X-Debug":{"absent":true}} json={"status":"completed"}
# @expect calls=1
HTTP/1.1 204 No Content`)
	server, err := Start("127.0.0.1:0", handler, Options{EnableControl: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	client := &http.Client{Timeout: 2 * time.Second}
	send := func(method, target, body string, headers map[string]string) {
		t.Helper()
		request, err := http.NewRequest(method, "http://"+server.Addr()+target, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
	}
	send(http.MethodPost, "/webhooks/pay_1?kind=payment", `{"status":"completed","id":"pay_1"}`, map[string]string{
		"Authorization": "Bearer token",
		"Content-Type":  "application/json",
		"X-Trace":       "trace-1",
	})
	send(http.MethodPost, "/webhooks/pay_2?kind=other", `{"status":"pending"}`, map[string]string{
		"Content-Type": "application/json",
	})
	send(http.MethodGet, "/not-found", "", nil)

	count, err := server.Count(context.Background(), RequestPattern{})
	if err != nil || count != 3 {
		t.Fatalf("Count(all) = %d, %v, want 3", count, err)
	}
	results := Verify(context.Background(), server, handler.Expectations())
	if len(results) != 1 || !results[0].Passed || results[0].Actual != 1 {
		t.Fatalf("Verify() = %+v", results)
	}

	control, err := NewClient("http://"+server.Addr(), ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	count, err = control.Count(context.Background(), RequestPattern{Method: http.MethodGet})
	if err != nil || count != 1 {
		t.Fatalf("control Count(GET) = %d, %v, want 1", count, err)
	}
	if got := server.JournalStats().Entries; got != 3 {
		t.Fatalf("control request was journaled: entries = %d", got)
	}
	if got := server.Stats().Calls; got != 3 {
		t.Fatalf("control request was logged as a call: calls = %d", got)
	}
	if err := control.Clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	count, err = server.Count(context.Background(), RequestPattern{})
	if err != nil || count != 0 {
		t.Fatalf("Count() after clear = %d, %v", count, err)
	}
}

func TestRequestJournalFailsClosedAfterEvictionAndForTruncatedJSON(t *testing.T) {
	handler := compileSource(t, `# @mock method=POST path=/events default=true
HTTP/1.1 204 No Content`)
	server, err := Start("127.0.0.1:0", handler, Options{
		JournalEntries:   1,
		JournalBytes:     1 << 20,
		JournalBodyLimit: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	client := &http.Client{Timeout: 2 * time.Second}
	post := func(body string) {
		t.Helper()
		request, err := http.NewRequest(
			http.MethodPost,
			"http://"+server.Addr()+"/events",
			strings.NewReader(body),
		)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
	}
	post(`{"id":1}`)
	post(`{"id":2}`)
	_, err = server.Count(context.Background(), RequestPattern{Method: http.MethodPost})
	var incomplete *IncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("Count() error = %v, want IncompleteError", err)
	}

	server.ClearRequests()
	post(`{"status":"completed"}`)
	count, err := server.Count(context.Background(), RequestPattern{Method: http.MethodPost})
	if err != nil || count != 1 {
		t.Fatalf("metadata Count() = %d, %v, want 1", count, err)
	}
	_, err = server.Count(context.Background(), RequestPattern{JSON: []byte(`{"status":"completed"}`)})
	if !errors.As(err, &incomplete) {
		t.Fatalf("JSON Count() error = %v, want IncompleteError", err)
	}
}

func TestRequestPatternRejectsAmbiguityAndHandlesTruncationAfterMetadata(t *testing.T) {
	journal, err := newRequestJournal(Options{})
	if err != nil {
		t.Fatal(err)
	}
	journal.add(requestRecord{
		method:        http.MethodPost,
		path:          "/events",
		query:         map[string][]string{"present": {""}},
		headers:       http.Header{"Content-Type": {"text/plain"}},
		bodyTruncated: true,
		size:          32,
	})
	count, err := journal.count(context.Background(), RequestPattern{
		Query: map[string]restfile.StringList{"missing": {}},
	})
	if err != nil || count != 0 {
		t.Fatalf("missing empty query count = %d, %v, want 0", count, err)
	}
	count, err = journal.count(context.Background(), RequestPattern{JSON: []byte(`{}`)})
	if err != nil || count != 0 {
		t.Fatalf("truncated non-JSON count = %d, %v, want 0", count, err)
	}

	exact := restfile.MockHeaderRule{Op: restfile.MockHeaderOpExact, Values: []string{"one"}}
	_, err = compileRequestPattern(RequestPattern{Headers: map[string]restfile.MockHeaderRule{
		"X-Test": exact,
		"x-test": exact,
	}})
	if err == nil || !strings.Contains(err.Error(), "different casing") {
		t.Fatalf("case-duplicate header error = %v", err)
	}
	_, err = compileRequestPattern(RequestPattern{Query: map[string]restfile.StringList{"job": nil}})
	if err == nil || !strings.Contains(err.Error(), "cannot be null") {
		t.Fatalf("null query error = %v", err)
	}
}

func TestRequestPatternCatchAllRejectsUnslashedPathLikeLiveRouting(t *testing.T) {
	compiled, err := compileRequestPattern(RequestPattern{Path: "/files/{rest...}"})
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{"/files": false, "/files/a": true, "/files/a/b": true} {
		matched, err := compiled.matches(requestRecord{method: http.MethodGet, path: path})
		if err != nil || matched != want {
			t.Fatalf("matches(%q) = %t, %v, want %t", path, matched, err, want)
		}
	}
}

func TestExpectationsRetainHeaderRules(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/secure
# @match headers={"Authorization":{"present":true}}
# @expect calls=1
HTTP/1.1 204 No Content`)
	expectations := handler.Expectations()
	if len(expectations) != 1 {
		t.Fatalf("expectations = %+v", expectations)
	}
	rule, ok := expectations[0].Pattern.Headers["Authorization"]
	if !ok || rule.Op != restfile.MockHeaderOpPresent {
		t.Fatalf("Authorization rule = %+v, %t", rule, ok)
	}
}

func TestJournalSkipsCORSPreflight(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/data
HTTP/1.1 204 No Content`)
	server, err := Start("127.0.0.1:0", handler, Options{CORS: WildcardCORS()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })

	client := &http.Client{Timeout: 2 * time.Second}
	preflight, err := http.NewRequest(http.MethodOptions, "http://"+server.Addr()+"/data", nil)
	if err != nil {
		t.Fatal(err)
	}
	preflight.Header.Set("Origin", "https://app.example")
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	response, err := client.Do(preflight)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	get, err := client.Get("http://" + server.Addr() + "/data")
	if err != nil {
		t.Fatal(err)
	}
	_ = get.Body.Close()

	count, err := server.Count(context.Background(), RequestPattern{})
	if err != nil || count != 1 {
		t.Fatalf("Count(all) = %d, %v, want the GET only", count, err)
	}
}

func TestServerContinuesAfterJournalBodyReadError(t *testing.T) {
	h := compileSource(t, `# @mock method=POST path=/data
HTTP/1.1 200 OK

ok`)
	s := newJournalTestServer(t, h, Options{})
	req := httptest.NewRequest(http.MethodPost, "/data", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = unreadableBody(`{"kind":"ok"}`)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("response = %d %q, want 200 %q", rec.Code, rec.Body.String(), "ok")
	}
	n, err := s.Count(context.Background(), RequestPattern{Method: http.MethodPost})
	if err != nil || n != 1 {
		t.Fatalf("metadata Count() = %d, %v, want 1", n, err)
	}
	_, err = s.Count(context.Background(), RequestPattern{JSON: []byte(`{"kind":"ok"}`)})
	var incomplete *IncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("JSON Count() error = %v, want IncompleteError", err)
	}
}

func TestServerReplaysJournalBodyReadErrorToMatcher(t *testing.T) {
	h := compileSource(t, `# @mock method=POST path=/data
# @match json={"kind":"ok"}
HTTP/1.1 200 OK`)
	s := newJournalTestServer(t, h, Options{})
	req := httptest.NewRequest(http.MethodPost, "/data", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Body = unreadableBody(`{"kind":"ok"}`)
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), "read JSON request body: broken request body") {
		t.Fatalf("response = %d %q, want body read error", rec.Code, rec.Body.String())
	}
}

func TestJournalSkipsUnreadableCORSPreflight(t *testing.T) {
	h := compileSource(t, `# @mock method=GET path=/data
HTTP/1.1 204 No Content`)
	s := newJournalTestServer(t, h, Options{CORS: WildcardCORS()})
	req := httptest.NewRequest(http.MethodOptions, "/data", nil)
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Body = unreadableBody("partial")
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := s.JournalStats().Entries; got != 0 {
		t.Fatalf("preflight journal entries = %d, want 0", got)
	}
}

func TestJournalCountsHEADAsActualMethod(t *testing.T) {
	h := compileSource(t, `# @mock method=GET path=/data
HTTP/1.1 204 No Content`)
	s := newJournalTestServer(t, h, Options{})
	rec := httptest.NewRecorder()

	s.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, "/data", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("HEAD status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	for method, want := range map[string]uint64{http.MethodHead: 1, http.MethodGet: 0} {
		n, err := s.Count(context.Background(), RequestPattern{Method: method})
		if err != nil || n != want {
			t.Fatalf("Count(%s) = %d, %v, want %d", method, n, err, want)
		}
	}
}

func newJournalTestServer(t *testing.T, h *Handler, opts Options) *Server {
	t.Helper()
	j, err := newRequestJournal(opts)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		opts:    opts,
		logs:    ring{events: make([]Event, 0, DefaultLogs), limit: DefaultLogs},
		journal: j,
	}
	h.setSequenceKeyLimit(opts.SequenceKeyLimit)
	s.handler.Store(h)
	return s
}

func unreadableBody(prefix string) io.ReadCloser {
	return io.NopCloser(io.MultiReader(
		strings.NewReader(prefix),
		iotest.ErrReader(errors.New("broken request body")),
	))
}
