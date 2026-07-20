package mock

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const pollingSequence = `# @mock method=GET path=/payments/{id} sequence=polling
HTTP/1.1 503 Service Unavailable

first failure
---
HTTP/1.1 503 Service Unavailable

second failure
---
HTTP/1.1 200 OK

completed`

func TestResponseSequenceAdvancesAndRepeatsFinalResponse(t *testing.T) {
	handler := compileSource(t, pollingSequence)
	wantStatuses := []int{503, 503, 200, 200}
	wantBodies := []string{"first failure", "second failure", "completed", "completed"}
	for i := range wantStatuses {
		response := httptest.NewRecorder()
		handler.ServeHTTP(
			response,
			httptest.NewRequest(http.MethodGet, "/payments/pay_123", nil),
		)
		if response.Code != wantStatuses[i] || response.Body.String() != wantBodies[i] {
			t.Fatalf(
				"call %d = %d %q, want %d %q",
				i+1,
				response.Code,
				response.Body.String(),
				wantStatuses[i],
				wantBodies[i],
			)
		}
	}
}

func TestResponseSequenceAdvancesIndependentlyByKey(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		path   string
		first  func(*http.Request)
		second func(*http.Request)
	}{
		{
			name:   "path",
			key:    "path.id",
			path:   "/payments/{id}",
			first:  func(r *http.Request) {},
			second: func(r *http.Request) {},
		},
		{
			name: "query",
			key:  "query.job",
			path: "/payments",
			first: func(r *http.Request) {
				r.URL.RawQuery = "job=one"
			},
			second: func(r *http.Request) {
				r.URL.RawQuery = "job=two"
			},
		},
		{
			name: "header",
			key:  "header.X-Correlation-ID",
			path: "/payments",
			first: func(r *http.Request) {
				r.Header.Set("X-Correlation-ID", "one")
			},
			second: func(r *http.Request) {
				r.Header.Set("X-Correlation-ID", "two")
			},
		},
		{
			name: "cookie",
			key:  "cookie.session",
			path: "/payments",
			first: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: "session", Value: "one"})
			},
			second: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: "session", Value: "two"})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := fmt.Sprintf(`# @mock method=GET path=%s sequence=polling sequence-key=%s
HTTP/1.1 503 Service Unavailable

pending
---
HTTP/1.1 200 OK

done`, test.path, test.key)
			handler := compileSource(t, source)
			newRequest := func(isSecond bool) *http.Request {
				target := "/payments"
				if test.name == "path" {
					target = "/payments/one"
					if isSecond {
						target = "/payments/two"
					}
				}
				req := httptest.NewRequest(http.MethodGet, target, nil)
				if isSecond {
					test.second(req)
				} else {
					test.first(req)
				}
				return req
			}

			assertResponse(t, handler, newRequest(false), http.StatusServiceUnavailable, "pending")
			assertResponse(t, handler, newRequest(false), http.StatusOK, "done")
			assertResponse(t, handler, newRequest(true), http.StatusServiceUnavailable, "pending")
		})
	}
}

func TestResponseSequenceKeyMissingAndLimitDoNotAdvance(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/payments sequence=polling sequence-key=header.X-Job
HTTP/1.1 503 Service Unavailable

pending
---
HTTP/1.1 200 OK

done`)
	handler.setSequenceKeyLimit(1)

	missing := httptest.NewRequest(http.MethodGet, "/payments", nil)
	assertResponse(t, handler, missing, http.StatusBadRequest, "missing or empty")

	oversized := httptest.NewRequest(http.MethodGet, "/payments", nil)
	oversized.Header.Set("X-Job", strings.Repeat("x", maxSequenceKeyBytes+1))
	assertResponse(t, handler, oversized, http.StatusBadRequest, "4 KiB limit")

	first := httptest.NewRequest(http.MethodGet, "/payments", nil)
	first.Header.Set("X-Job", "one")
	assertResponse(t, handler, first, http.StatusServiceUnavailable, "pending")

	full := httptest.NewRequest(http.MethodGet, "/payments", nil)
	full.Header.Set("X-Job", "two")
	assertResponse(t, handler, full, http.StatusTooManyRequests, "sequence-key limit")

	if reset := handler.ResetSequences("polling"); reset != 1 {
		t.Fatalf("ResetSequences() = %d, want 1", reset)
	}
	assertResponse(t, handler, full, http.StatusServiceUnavailable, "pending")
}

func TestResponseSequenceResetByNameAndAll(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/one sequence=polling
HTTP/1.1 503 Service Unavailable

one pending
---
HTTP/1.1 200 OK

one done
### Two
# @mock method=GET path=/two sequence=polling
HTTP/1.1 503 Service Unavailable

two pending
---
HTTP/1.1 200 OK

two done
### Other
# @mock method=GET path=/other sequence=other
HTTP/1.1 503 Service Unavailable

other pending
---
HTTP/1.1 200 OK

other done`)

	assertResponse(t, handler, httptest.NewRequest(http.MethodGet, "/one", nil), 503, "one pending")
	assertResponse(t, handler, httptest.NewRequest(http.MethodGet, "/two", nil), 503, "two pending")
	assertResponse(t, handler, httptest.NewRequest(http.MethodGet, "/other", nil), 503, "other pending")
	if reset := handler.ResetSequences("polling"); reset != 2 {
		t.Fatalf("named reset = %d, want 2", reset)
	}
	assertResponse(t, handler, httptest.NewRequest(http.MethodGet, "/one", nil), 503, "one pending")
	assertResponse(t, handler, httptest.NewRequest(http.MethodGet, "/two", nil), 503, "two pending")
	assertResponse(t, handler, httptest.NewRequest(http.MethodGet, "/other", nil), 200, "other done")
	if reset := handler.ResetSequences(""); reset != 3 {
		t.Fatalf("all reset = %d, want 3", reset)
	}
	assertResponse(t, handler, httptest.NewRequest(http.MethodGet, "/other", nil), 503, "other pending")
}

func TestResponseSequenceStatusSelectorPinsWithoutAdvancing(t *testing.T) {
	handler := compileSource(t, pollingSequence)

	pinnedEvent := new(Event)
	pinned := httptest.NewRequest(http.MethodGet, "/payments/pay_123", nil)
	pinned.Header.Set(selectorNameHeader, "polling")
	pinned.Header.Set(selectorStatusHeader, "200")
	pinned = pinned.WithContext(context.WithValue(pinned.Context(), requestEventKey{}, pinnedEvent))
	assertResponse(t, handler, pinned, http.StatusOK, "completed")
	if pinnedEvent.SequenceStep != 3 || pinnedEvent.SequenceTotal != 3 {
		t.Fatalf("pinned event = %+v", pinnedEvent)
	}

	firstFailure := httptest.NewRequest(http.MethodGet, "/payments/pay_123", nil)
	firstFailure.Header.Set(selectorStatusHeader, "503")
	assertResponse(t, handler, firstFailure, http.StatusServiceUnavailable, "first failure")

	normalEvent := new(Event)
	normal := httptest.NewRequest(http.MethodGet, "/payments/pay_123", nil)
	normal = normal.WithContext(context.WithValue(normal.Context(), requestEventKey{}, normalEvent))
	assertResponse(t, handler, normal, http.StatusServiceUnavailable, "first failure")
	if normalEvent.SequenceStep != 1 || normalEvent.SequenceTotal != 3 ||
		normalEvent.ScenarioLabel() != "polling 1/3" {
		t.Fatalf("normal event = %+v, label=%q", normalEvent, normalEvent.ScenarioLabel())
	}
}

func TestResponseSequenceUsesScenarioMatching(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/payments sequence=matched
# @match headers={"X-Mode":"poll"}
HTTP/1.1 202 Accepted

pending
---
HTTP/1.1 200 OK

completed
### Default
# @mock method=GET path=/payments name=fallback default=true
HTTP/1.1 204 No Content`)

	matched := httptest.NewRequest(http.MethodGet, "/payments", nil)
	matched.Header.Set("X-Mode", "poll")
	assertResponse(t, handler, matched, http.StatusAccepted, "pending")
	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/payments", nil),
		http.StatusNoContent,
		"",
	)
}

func TestResponseSequenceLookupDoesNotAdvance(t *testing.T) {
	handler := compileSource(t, pollingSequence)

	methodMismatch := httptest.NewRecorder()
	handler.ServeHTTP(
		methodMismatch,
		httptest.NewRequest(http.MethodPost, "/payments/pay_123", nil),
	)
	if methodMismatch.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method mismatch status = %d", methodMismatch.Code)
	}

	preflightRequest := httptest.NewRequest(http.MethodOptions, "/payments/pay_123", nil)
	preflightRequest.Header.Set("Origin", "https://app.example")
	preflightRequest.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflightResponse := httptest.NewRecorder()
	server := &Server{opts: Options{CORS: WildcardCORS()}}
	if handled := server.handleCORS(preflightResponse, preflightRequest, handler); !handled {
		t.Fatal("preflight was not handled")
	}
	if preflightResponse.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d", preflightResponse.Code)
	}

	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/payments/pay_123", nil),
		http.StatusServiceUnavailable,
		"first failure",
	)
}

func TestResponseSequenceHeadConsumesAResponse(t *testing.T) {
	handler := compileSource(t, pollingSequence)
	head := httptest.NewRecorder()
	handler.ServeHTTP(
		head,
		httptest.NewRequest(http.MethodHead, "/payments/pay_123", nil),
	)
	if head.Code != http.StatusServiceUnavailable || head.Body.Len() != 0 {
		t.Fatalf("HEAD response = %d %q", head.Code, head.Body.String())
	}
	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/payments/pay_123", nil),
		http.StatusServiceUnavailable,
		"second failure",
	)
}

func TestResponseSequenceConsumesStepWhenRequestIsCanceled(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/x sequence=slow latency=10ms
HTTP/1.1 503 Service Unavailable

first
---
HTTP/1.1 200 OK

second`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceledEvent := new(Event)
	req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx)
	req = req.WithContext(context.WithValue(req.Context(), requestEventKey{}, canceledEvent))
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if canceledEvent.SequenceStep != 1 || canceledEvent.Error != "request canceled during mock latency" {
		t.Fatalf("canceled event = %+v", canceledEvent)
	}

	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/x", nil),
		http.StatusOK,
		"second",
	)
}

func TestResponseSequenceConsumesStepWhenRenderingFails(t *testing.T) {
	handler := compileSource(t, `# @mock method=GET path=/x sequence=render
HTTP/1.1 503 Service Unavailable

{{query.required}}
---
HTTP/1.1 200 OK

second`)

	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/x", nil),
		http.StatusBadRequest,
		"missing query value",
	)
	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/x?required=value", nil),
		http.StatusOK,
		"second",
	)
}

func TestResponseSequenceConcurrentCallsReserveEachTransientStepOnce(t *testing.T) {
	const (
		steps = 8
		calls = 32
	)
	var source strings.Builder
	source.WriteString("# @mock method=GET path=/x sequence=concurrent\n")
	for i := range steps {
		if i > 0 {
			source.WriteString("---\n")
		}
		fmt.Fprintf(&source, "HTTP/1.1 200 OK\n\n%d\n", i)
	}
	handler := compileSource(t, strings.TrimSuffix(source.String(), "\n"))

	results := make(chan string, calls)
	start := make(chan struct{})
	var group sync.WaitGroup
	for range calls {
		group.Go(func() {
			<-start
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/x", nil))
			results <- response.Body.String()
		})
	}
	close(start)
	group.Wait()
	close(results)

	counts := make([]int, steps)
	for body := range results {
		step, err := strconv.Atoi(body)
		if err != nil || step < 0 || step >= steps {
			t.Fatalf("response body = %q: %v", body, err)
		}
		counts[step]++
	}
	for step := 0; step < steps-1; step++ {
		if counts[step] != 1 {
			t.Fatalf("step %d count = %d, want 1; all=%v", step, counts[step], counts)
		}
	}
	if want := calls - (steps - 1); counts[steps-1] != want {
		t.Fatalf("final step count = %d, want %d; all=%v", counts[steps-1], want, counts)
	}
}

func TestResponseSequenceReloadState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "mocks.http")
	initial := pollingSequence
	writeFile(t, path, initial)
	reloader := NewReloader(path, false)

	handler, err := reloader.Reload(path, []byte(initial))
	if err != nil || handler == nil {
		t.Fatalf("initial reload = %v, %v", handler, err)
	}
	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/payments/1", nil),
		http.StatusServiceUnavailable,
		"first failure",
	)
	if unchanged, err := reloader.Reload(path, []byte(initial)); err != nil || unchanged != nil {
		t.Fatalf("unchanged reload = %v, %v", unchanged, err)
	}
	assertResponse(
		t,
		handler,
		httptest.NewRequest(http.MethodGet, "/payments/2", nil),
		http.StatusServiceUnavailable,
		"second failure",
	)

	updated := strings.Replace(initial, "completed", "settled", 1)
	reloaded, err := reloader.Reload(path, []byte(updated))
	if err != nil || reloaded == nil {
		t.Fatalf("content reload = %v, %v", reloaded, err)
	}
	assertResponse(
		t,
		reloaded,
		httptest.NewRequest(http.MethodGet, "/payments/3", nil),
		http.StatusServiceUnavailable,
		"first failure",
	)
}

func TestResponseSequenceTracksEveryFixtureForReload(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "mocks.http")
	firstFixture := filepath.Join(root, "first.txt")
	finalFixture := filepath.Join(root, "final.txt")
	writeFile(t, firstFixture, "pending")
	writeFile(t, finalFixture, "completed")
	writeFile(t, path, `# @mock method=GET path=/x sequence=fixtures
HTTP/1.1 503 Service Unavailable

< ./first.txt
---
HTTP/1.1 200 OK

< ./final.txt`)

	reloader := NewReloader(root, false)
	handler, err := reloader.Reload("", nil)
	if err != nil || handler == nil {
		t.Fatalf("initial reload = %v, %v", handler, err)
	}
	writeFile(t, finalFixture, "settled and complete")
	reloaded, err := reloader.Reload("", nil)
	if err != nil || reloaded == nil {
		t.Fatalf("fixture reload = %v, %v", reloaded, err)
	}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(selectorStatusHeader, "200")
	assertResponse(t, reloaded, req, http.StatusOK, "settled and complete")
}
