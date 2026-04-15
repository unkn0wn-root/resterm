package headless

import (
	"net/http"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestCloneHTTP(t *testing.T) {
	src := &httpclient.Response{
		Headers:        http.Header{"X-Test": {"one"}},
		RequestHeaders: http.Header{"Accept": {"application/json"}},
		ReqTE:          []string{"trailers"},
		Body:           []byte("body"),
		Request: &restfile.Request{
			Method:  "GET",
			URL:     "https://example.com",
			Headers: http.Header{"Authorization": {"Bearer one"}},
		},
		Timeline: &nettrace.Timeline{
			Phases: []nettrace.Phase{{
				Kind:     nettrace.PhaseDNS,
				Duration: time.Second,
			}},
			Details: &nettrace.TraceDetails{
				Connection: &nettrace.ConnDetails{
					ResolvedAddrs: []string{"1.1.1.1"},
				},
			},
		},
		TraceReport: &nettrace.Report{
			Timeline: &nettrace.Timeline{
				Phases: []nettrace.Phase{{
					Kind:     nettrace.PhaseTTFB,
					Duration: 2 * time.Second,
				}},
			},
			Budget: nettrace.Budget{
				Phases: map[nettrace.PhaseKind]time.Duration{
					nettrace.PhaseTTFB: time.Second,
				},
			},
			BudgetReport: nettrace.BudgetReport{
				Breaches: []nettrace.BudgetBreach{{
					Kind:   nettrace.PhaseTTFB,
					Limit:  time.Second,
					Actual: 2 * time.Second,
					Over:   time.Second,
				}},
			},
		},
	}

	got := cloneHTTP(src)
	if got == nil {
		t.Fatal("cloneHTTP() returned nil")
	}

	got.Headers.Set("X-Test", "two")
	got.RequestHeaders.Set("Accept", "text/plain")
	got.ReqTE[0] = "gzip"
	got.Body[0] = 'B'
	got.Request.Headers.Set("Authorization", "Bearer two")
	got.Timeline.Phases[0].Err = "dns changed"
	got.Timeline.Details.Connection.ResolvedAddrs[0] = "2.2.2.2"
	got.TraceReport.Timeline.Phases[0].Err = "trace changed"
	got.TraceReport.Budget.Phases[nettrace.PhaseTTFB] = 3 * time.Second
	got.TraceReport.BudgetReport.Breaches[0].Over = 2 * time.Second

	if src.Headers.Get("X-Test") != "one" {
		t.Fatalf("source headers changed to %q", src.Headers.Get("X-Test"))
	}
	if src.RequestHeaders.Get("Accept") != "application/json" {
		t.Fatalf("source request headers changed to %q", src.RequestHeaders.Get("Accept"))
	}
	if src.ReqTE[0] != "trailers" {
		t.Fatalf("source request TE changed to %q", src.ReqTE[0])
	}
	if string(src.Body) != "body" {
		t.Fatalf("source body changed to %q", string(src.Body))
	}
	if src.Request.Headers.Get("Authorization") != "Bearer one" {
		t.Fatalf("source request changed to %q", src.Request.Headers.Get("Authorization"))
	}
	if src.Timeline.Phases[0].Err != "" {
		t.Fatalf("source timeline changed to %q", src.Timeline.Phases[0].Err)
	}
	if src.Timeline.Details.Connection.ResolvedAddrs[0] != "1.1.1.1" {
		t.Fatalf("source timeline details changed to %q", src.Timeline.Details.Connection.ResolvedAddrs[0])
	}
	if src.TraceReport.Timeline.Phases[0].Err != "" {
		t.Fatalf("source trace report timeline changed to %q", src.TraceReport.Timeline.Phases[0].Err)
	}
	if src.TraceReport.Budget.Phases[nettrace.PhaseTTFB] != time.Second {
		t.Fatalf("source trace budget changed to %s", src.TraceReport.Budget.Phases[nettrace.PhaseTTFB])
	}
	if src.TraceReport.BudgetReport.Breaches[0].Over != time.Second {
		t.Fatalf("source trace breaches changed to %s", src.TraceReport.BudgetReport.Breaches[0].Over)
	}
}

func TestCloneGRPC(t *testing.T) {
	src := &grpcclient.Response{
		Headers:  map[string][]string{"x-id": {"1"}},
		Trailers: map[string][]string{"x-trailer": {"done"}},
		Body:     []byte("body"),
		Wire:     []byte("wire"),
	}

	got := cloneGRPC(src)
	if got == nil {
		t.Fatal("cloneGRPC() returned nil")
	}

	got.Headers["x-id"][0] = "2"
	got.Trailers["x-trailer"][0] = "later"
	got.Body[0] = 'B'
	got.Wire[0] = 'W'

	if src.Headers["x-id"][0] != "1" {
		t.Fatalf("source grpc headers changed to %q", src.Headers["x-id"][0])
	}
	if src.Trailers["x-trailer"][0] != "done" {
		t.Fatalf("source grpc trailers changed to %q", src.Trailers["x-trailer"][0])
	}
	if string(src.Body) != "body" {
		t.Fatalf("source grpc body changed to %q", string(src.Body))
	}
	if string(src.Wire) != "wire" {
		t.Fatalf("source grpc wire changed to %q", string(src.Wire))
	}
}
