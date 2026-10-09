package history

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
)

func TestNewTraceSummary(t *testing.T) {
	tl := &nettrace.Timeline{
		Started:   time.Unix(0, 0),
		Completed: time.Unix(0, int64(15*time.Millisecond)),
		Duration:  15 * time.Millisecond,
		Phases: []nettrace.Phase{
			{
				Kind:     nettrace.PhaseDNS,
				Duration: 5 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "example.com", Cached: true},
			},
			{
				Kind:     nettrace.PhaseConnect,
				Duration: 10 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "93.184.216.34:443"},
			},
		},
		Details: &nettrace.TraceDetails{
			Connection: &nettrace.ConnDetails{
				Reused:        true,
				IdleTime:      5 * time.Millisecond,
				DialAddr:      "93.184.216.34:443",
				ResolvedAddrs: []string{"93.184.216.34"},
				K8s:           "kind-dev default/api:8080",
				Protocol:      "HTTP/2.0",
			},
			TLS: &nettrace.TLSDetails{
				Version:    "TLS 1.3",
				Cipher:     "TLS_AES_128_GCM_SHA256",
				ALPN:       "h2",
				ServerName: "example.com",
				Resumed:    true,
				Verified:   true,
				Certificates: []nettrace.TLSCert{
					{
						Subject:   "example.com",
						Issuer:    "Example CA",
						SANs:      []string{"example.com"},
						NotBefore: time.Unix(0, 0),
						NotAfter:  time.Unix(0, int64(24*time.Hour)),
						Serial:    "01",
					},
				},
			},
		},
	}

	report := nettrace.NewReport(tl, nettrace.Budget{Total: 10 * time.Millisecond})
	summary := NewTraceSummary(tl, report)
	if summary == nil {
		t.Fatalf("expected summary")
	}
	if summary.Duration != 15*time.Millisecond {
		t.Fatalf("unexpected duration: %v", summary.Duration)
	}
	if len(summary.Phases) != 2 {
		t.Fatalf("expected 2 phases, got %d", len(summary.Phases))
	}
	if summary.Phases[0].Kind != string(nettrace.PhaseDNS) {
		t.Fatalf("unexpected first phase kind: %s", summary.Phases[0].Kind)
	}
	if !summary.Phases[0].Meta.Cached {
		t.Fatalf("expected cached flag to propagate")
	}
	if summary.Phases[1].Meta.Addr != "93.184.216.34:443" {
		t.Fatalf("unexpected address metadata: %s", summary.Phases[1].Meta.Addr)
	}
	if summary.Budgets == nil || summary.Budgets.Total != 10*time.Millisecond {
		t.Fatalf("expected budgets to be captured")
	}
	if len(summary.Breaches) == 0 {
		t.Fatalf("expected breach to be present")
	}
	if summary.Details == nil || summary.Details.Connection == nil || summary.Details.TLS == nil {
		t.Fatalf("expected trace details to be captured")
	}
	if summary.Details.Connection.Protocol != "HTTP/2.0" {
		t.Fatalf("unexpected protocol: %s", summary.Details.Connection.Protocol)
	}
	if summary.Details.Connection.K8s != "kind-dev default/api:8080" {
		t.Fatalf("unexpected k8s detail: %s", summary.Details.Connection.K8s)
	}
	if got := summary.Details.TLS.Certificates[0].Subject; got != "example.com" {
		t.Fatalf("unexpected cert subject: %s", got)
	}
}

func TestNewTraceSummaryNil(t *testing.T) {
	if summary := NewTraceSummary(nil, nil); summary != nil {
		t.Fatalf("expected nil summary for nil timeline")
	}
}

func TestTraceSummaryRoundTrip(t *testing.T) {
	tl := &nettrace.Timeline{
		Started:   time.Unix(0, 0),
		Completed: time.Unix(0, int64(120*time.Millisecond)),
		Duration:  120 * time.Millisecond,
		Phases: []nettrace.Phase{
			{
				Kind:     nettrace.PhaseDNS,
				Duration: 40 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "example.com", Cached: true},
			},
			{
				Kind:     nettrace.PhaseConnect,
				Duration: 50 * time.Millisecond,
				Meta:     nettrace.PhaseMeta{Addr: "93.184.216.34:443"},
			},
			{Kind: nettrace.PhaseTransfer, Duration: 30 * time.Millisecond},
		},
		Details: &nettrace.TraceDetails{
			Connection: &nettrace.ConnDetails{
				LocalAddr:  "127.0.0.1:5353",
				RemoteAddr: "93.184.216.34:443",
				K8s:        "default/api:8080",
				Protocol:   "HTTP/1.1",
			},
		},
	}
	budget := nettrace.Budget{
		Total: 90 * time.Millisecond,
		Phases: map[nettrace.PhaseKind]time.Duration{
			nettrace.PhaseDNS:     20 * time.Millisecond,
			nettrace.PhaseConnect: 40 * time.Millisecond,
		},
	}
	report := nettrace.NewReport(tl, budget)
	summary := NewTraceSummary(tl, report)
	if summary == nil {
		t.Fatalf("expected summary from report")
	}
	rebuilt := summary.Timeline()
	if rebuilt == nil {
		t.Fatalf("expected timeline reconstruction")
	}
	if len(rebuilt.Phases) != len(tl.Phases) {
		t.Fatalf("expected %d phases, got %d", len(tl.Phases), len(rebuilt.Phases))
	}
	for i, phase := range rebuilt.Phases {
		if phase.Duration != tl.Phases[i].Duration {
			t.Fatalf(
				"phase %d duration mismatch: %s vs %s",
				i,
				phase.Duration,
				tl.Phases[i].Duration,
			)
		}
		if phase.Meta.Addr != tl.Phases[i].Meta.Addr {
			t.Fatalf("phase %d addr mismatch", i)
		}
	}
	if rebuilt.Details == nil || rebuilt.Details.Connection == nil {
		t.Fatalf("expected trace details to round trip")
	}
	if rebuilt.Details.Connection.Protocol != "HTTP/1.1" {
		t.Fatalf("unexpected protocol after round trip: %s", rebuilt.Details.Connection.Protocol)
	}
	if rebuilt.Details.Connection.K8s != "default/api:8080" {
		t.Fatalf("unexpected k8s after round trip: %s", rebuilt.Details.Connection.K8s)
	}
	rebuiltReport := summary.Report()
	if rebuiltReport == nil {
		t.Fatalf("expected report reconstruction")
	}
	if rebuiltReport.Budget.Total != budget.Total {
		t.Fatalf("expected budget total %s, got %s", budget.Total, rebuiltReport.Budget.Total)
	}
	if len(rebuiltReport.BudgetReport.Breaches) != len(report.BudgetReport.Breaches) {
		t.Fatalf(
			"expected %d breaches, got %d",
			len(report.BudgetReport.Breaches),
			len(rebuiltReport.BudgetReport.Breaches),
		)
	}
}

func TestTraceSummaryKeepsEveryField(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	ms := time.Millisecond
	tl := &nettrace.Timeline{
		Started:   at,
		Completed: at.Add(60 * ms),
		Duration:  60 * ms,
		Err:       "read: connection reset",
		Phases: []nettrace.Phase{
			{
				Kind:     nettrace.PhaseDNS,
				Start:    at,
				End:      at.Add(10 * ms),
				Duration: 10 * ms,
				Meta:     nettrace.PhaseMeta{Addr: "api.test", Cached: true},
			},
			{
				Kind:     nettrace.PhaseConnect,
				Start:    at.Add(10 * ms),
				End:      at.Add(30 * ms),
				Duration: 20 * ms,
				Err:      "slow dial",
				Meta:     nettrace.PhaseMeta{Addr: "10.0.0.1:443", Reused: true},
			},
			{Kind: nettrace.PhaseTransfer, Start: at.Add(30 * ms), End: at.Add(60 * ms), Duration: 30 * ms},
		},
		Details: &nettrace.TraceDetails{
			Connection: &nettrace.ConnDetails{
				Reused:        true,
				WasIdle:       true,
				IdleTime:      time.Second,
				Network:       "tcp",
				DialAddr:      "api.test:443",
				LocalAddr:     "10.0.0.2:50000",
				RemoteAddr:    "10.0.0.1:443",
				ResolvedAddrs: []string{"10.0.0.1", "10.0.0.3"},
				Proxy:         "http://proxy:8080",
				ProxyTunnel:   true,
				SSH:           "bastion",
				K8s:           "default/api:8080",
				Protocol:      "HTTP/2.0",
			},
			TLS: &nettrace.TLSDetails{
				Version:    "TLS 1.3",
				Cipher:     "TLS_AES_128_GCM_SHA256",
				ALPN:       "h2",
				ServerName: "api.test",
				Resumed:    true,
				Verified:   true,
				Certificates: []nettrace.TLSCert{
					{
						Subject:   "CN=api.test",
						Issuer:    "CN=Test CA",
						SANs:      []string{"api.test", "*.api.test"},
						NotBefore: at.Add(-time.Hour),
						NotAfter:  at.Add(time.Hour),
						Serial:    "01",
					},
					{Subject: "CN=Test CA", Issuer: "CN=Root", NotAfter: at.Add(24 * time.Hour), Serial: "02"},
				},
			},
		},
	}
	budget := nettrace.Budget{
		Total:     50 * ms,
		Tolerance: ms,
		Phases:    map[nettrace.PhaseKind]time.Duration{nettrace.PhaseConnect: 5 * ms},
	}
	rep := nettrace.NewReport(tl, budget)
	if len(rep.BudgetReport.Breaches) == 0 {
		t.Fatal("the fixture should breach its budget")
	}

	raw, err := json.Marshal(NewTraceSummary(tl, rep))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var sum TraceSummary
	if err := json.Unmarshal(raw, &sum); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := sum.Timeline(); !reflect.DeepEqual(got, tl) {
		t.Fatalf("timeline = %+v\nwant %+v", got, tl)
	}
	got := sum.Report()
	if !reflect.DeepEqual(got.Budget, rep.Budget) || !reflect.DeepEqual(got.BudgetReport, rep.BudgetReport) {
		t.Fatalf("report budget = %+v %+v\nwant %+v %+v", got.Budget, got.BudgetReport, rep.Budget, rep.BudgetReport)
	}
}

func TestTraceSummaryDoesNotShareSlices(t *testing.T) {
	tl := &nettrace.Timeline{
		Details: &nettrace.TraceDetails{
			Connection: &nettrace.ConnDetails{ResolvedAddrs: []string{"10.0.0.1"}},
			TLS: &nettrace.TLSDetails{
				Certificates: []nettrace.TLSCert{{SANs: []string{"api.test"}}},
			},
		},
	}
	sum := NewTraceSummary(tl, nil)
	back := sum.Timeline()
	tl.Details.Connection.ResolvedAddrs[0] = "changed"
	tl.Details.TLS.Certificates[0].SANs[0] = "changed"
	if sum.Details.Connection.ResolvedAddrs[0] != "10.0.0.1" || sum.Details.TLS.Certificates[0].SANs[0] != "api.test" {
		t.Fatal("the summary shares slices with the timeline it came from")
	}
	sum.Details.Connection.ResolvedAddrs[0] = "changed"
	sum.Details.TLS.Certificates[0].SANs[0] = "changed"
	if back.Details.Connection.ResolvedAddrs[0] != "10.0.0.1" ||
		back.Details.TLS.Certificates[0].SANs[0] != "api.test" {
		t.Fatal("the rebuilt timeline shares slices with the summary")
	}
}
