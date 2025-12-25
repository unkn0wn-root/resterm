package telemetry

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestInstrumenterRecordsTimeline(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	inst, err := New(
		Config{ServiceName: "resterm-test", Version: "test"},
		WithSpanProcessor(recorder),
	)
	if err != nil {
		t.Fatalf("New instrumenter: %v", err)
	}
	t.Cleanup(func() {
		_ = inst.Shutdown(context.Background())
	})

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/api/health",
		Metadata: restfile.RequestMetadata{
			Name:        "health",
			Description: "Health check",
			Tags:        []string{"smoke", "status"},
		},
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), req.Method, req.URL, nil)
	if err != nil {
		t.Fatalf("build http request: %v", err)
	}

	budget := nettrace.Budget{Total: 200 * time.Millisecond}
	ctx, span := inst.Start(
		context.Background(),
		RequestStart{Request: req, HTTPRequest: httpReq, Budget: &budget},
	)
	if ctx == nil || span == nil {
		t.Fatalf("expected span to be created")
	}

	timeline := &nettrace.Timeline{
		Started:   time.Now().Add(-250 * time.Millisecond),
		Completed: time.Now(),
		Duration:  180 * time.Millisecond,
		Phases: []nettrace.Phase{
			{
				Kind:     nettrace.PhaseDNS,
				Start:    time.Now().Add(-250 * time.Millisecond),
				End:      time.Now().Add(-220 * time.Millisecond),
				Duration: 30 * time.Millisecond,
			},
			{
				Kind:     nettrace.PhaseConnect,
				Start:    time.Now().Add(-220 * time.Millisecond),
				End:      time.Now().Add(-120 * time.Millisecond),
				Duration: 100 * time.Millisecond,
				Meta: nettrace.PhaseMeta{
					Addr:   "93.184.216.34:443",
					Reused: true,
				},
			},
			{
				Kind:     nettrace.PhaseTTFB,
				Start:    time.Now().Add(-120 * time.Millisecond),
				End:      time.Now().Add(-60 * time.Millisecond),
				Duration: 60 * time.Millisecond,
			},
		},
	}

	report := nettrace.NewReport(timeline, budget)

	span.RecordTrace(timeline, report)
	span.End(RequestResult{StatusCode: 200, Report: report})

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	ro := spans[0]
	if got := ro.Name(); got != "health" {
		t.Fatalf("unexpected span name %q", got)
	}
	assertAttribute(t, ro, "resterm.trace.duration_ms", int64(180))
	assertAttribute(t, ro, "resterm.trace.enabled", true)
	assertAttribute(t, ro, "http.method", "GET")
	assertAttribute(t, ro, "resterm.request.name", "health")
	if ro.Status().Code != codes.Ok && ro.Status().Code != codes.Unset {
		t.Fatalf("expected span status OK or unset, got %v", ro.Status().Code)
	}

	events := ro.Events()
	var phaseEvents int
	for _, ev := range events {
		if ev.Name == "resterm.trace.phase" {
			phaseEvents++
		}
	}
	if phaseEvents == 0 {
		t.Fatalf("expected at least one phase event, got 0")
	}
}

func assertAttribute(t *testing.T, span sdktrace.ReadOnlySpan, key string, want interface{}) {
	t.Helper()
	attrs := span.Attributes()
	for _, attr := range attrs {
		if string(attr.Key) != key {
			continue
		}
		switch v := want.(type) {
		case string:
			if attr.Value.AsString() == v {
				return
			}
		case bool:
			if attr.Value.AsBool() == v {
				return
			}
		case int64:
			if attr.Value.AsInt64() == v {
				return
			}
		}
		t.Fatalf("attribute %s mismatch: got %v, want %v", key, attr.Value, want)
	}
	t.Fatalf("attribute %s not found", key)
}
