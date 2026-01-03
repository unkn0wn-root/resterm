package history

import (
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/traceutil"
)

type TraceSummary struct {
	Started   time.Time     `json:"started,omitempty"`
	Completed time.Time     `json:"completed,omitempty"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error,omitempty"`
	Phases    []TracePhase  `json:"phases,omitempty"`
	Details   *TraceDetails `json:"details,omitempty"`
	Budgets   *TraceBudget  `json:"budgets,omitempty"`
	Breaches  []TraceBreach `json:"breaches,omitempty"`
}

type TracePhase struct {
	Kind     string         `json:"kind"`
	Duration time.Duration  `json:"duration"`
	Error    string         `json:"error,omitempty"`
	Meta     TracePhaseMeta `json:"meta,omitempty"`
}

type TracePhaseMeta struct {
	Addr   string `json:"addr,omitempty"`
	Reused bool   `json:"reused,omitempty"`
	Cached bool   `json:"cached,omitempty"`
}

type TraceDetails struct {
	Connection *TraceConn `json:"connection,omitempty"`
	TLS        *TraceTLS  `json:"tls,omitempty"`
}

type TraceConn struct {
	Reused        bool          `json:"reused,omitempty"`
	WasIdle       bool          `json:"wasIdle,omitempty"`
	IdleTime      time.Duration `json:"idleTime,omitempty"`
	Network       string        `json:"network,omitempty"`
	DialAddr      string        `json:"dialAddr,omitempty"`
	LocalAddr     string        `json:"localAddr,omitempty"`
	RemoteAddr    string        `json:"remoteAddr,omitempty"`
	ResolvedAddrs []string      `json:"resolvedAddrs,omitempty"`
	Proxy         string        `json:"proxy,omitempty"`
	ProxyTunnel   bool          `json:"proxyTunnel,omitempty"`
	SSH           string        `json:"ssh,omitempty"`
	Protocol      string        `json:"protocol,omitempty"`
}

type TraceTLS struct {
	Version      string      `json:"version,omitempty"`
	Cipher       string      `json:"cipher,omitempty"`
	ALPN         string      `json:"alpn,omitempty"`
	ServerName   string      `json:"serverName,omitempty"`
	Resumed      bool        `json:"resumed,omitempty"`
	Verified     bool        `json:"verified,omitempty"`
	Certificates []TraceCert `json:"certificates,omitempty"`
}

type TraceCert struct {
	Subject   string    `json:"subject,omitempty"`
	Issuer    string    `json:"issuer,omitempty"`
	SANs      []string  `json:"sans,omitempty"`
	NotBefore time.Time `json:"notBefore,omitempty"`
	NotAfter  time.Time `json:"notAfter,omitempty"`
	Serial    string    `json:"serial,omitempty"`
}

type TraceBudget struct {
	Total     time.Duration            `json:"total,omitempty"`
	Tolerance time.Duration            `json:"tolerance,omitempty"`
	Phases    map[string]time.Duration `json:"phases,omitempty"`
}

type TraceBreach struct {
	Kind   string        `json:"kind"`
	Limit  time.Duration `json:"limit"`
	Actual time.Duration `json:"actual"`
	Over   time.Duration `json:"over"`
}

func NewTraceSummary(tl *nettrace.Timeline, rep *nettrace.Report) *TraceSummary {
	if tl == nil {
		return nil
	}

	summary := &TraceSummary{
		Started:   tl.Started,
		Completed: tl.Completed,
		Duration:  tl.Duration,
		Error:     tl.Err,
	}
	summary.Details = traceDetailsFromTimeline(tl.Details)

	if len(tl.Phases) == 0 {
		return summary
	}

	summary.Phases = make([]TracePhase, len(tl.Phases))
	for i, phase := range tl.Phases {
		summary.Phases[i] = TracePhase{
			Kind:     string(phase.Kind),
			Duration: phase.Duration,
			Error:    phase.Err,
			Meta: TracePhaseMeta{
				Addr:   phase.Meta.Addr,
				Reused: phase.Meta.Reused,
				Cached: phase.Meta.Cached,
			},
		}
	}

	if rep != nil {
		budget := rep.Budget.Clone()
		if traceutil.HasBudget(budget) {
			bud := &TraceBudget{Total: budget.Total, Tolerance: budget.Tolerance}
			if len(budget.Phases) > 0 {
				phases := make(map[string]time.Duration, len(budget.Phases))
				for kind, dur := range budget.Phases {
					if dur <= 0 {
						continue
					}
					phases[string(kind)] = dur
				}
				if len(phases) > 0 {
					bud.Phases = phases
				}
			}
			summary.Budgets = bud
		}

		if len(rep.BudgetReport.Breaches) > 0 {
			breaches := make([]TraceBreach, 0, len(rep.BudgetReport.Breaches))
			for _, br := range rep.BudgetReport.Breaches {
				breaches = append(breaches, TraceBreach{
					Kind:   string(br.Kind),
					Limit:  br.Limit,
					Actual: br.Actual,
					Over:   br.Over,
				})
			}
			summary.Breaches = breaches
		}
	}

	return summary
}

func (s *TraceSummary) Timeline() *nettrace.Timeline {
	if s == nil {
		return nil
	}

	tl := &nettrace.Timeline{
		Started:   s.Started,
		Completed: s.Completed,
		Duration:  s.Duration,
		Err:       s.Error,
	}
	tl.Details = traceDetailsToTimeline(s.Details)
	if len(s.Phases) == 0 {
		return tl
	}

	phases := make([]nettrace.Phase, len(s.Phases))
	anchor := s.Started
	for i, phase := range s.Phases {
		dur := phase.Duration
		start := anchor
		end := start
		if !start.IsZero() && dur > 0 {
			end = start.Add(dur)
		}
		phases[i] = nettrace.Phase{
			Kind:     nettrace.PhaseKind(phase.Kind),
			Start:    start,
			End:      end,
			Duration: dur,
			Err:      phase.Error,
			Meta: nettrace.PhaseMeta{
				Addr:   phase.Meta.Addr,
				Reused: phase.Meta.Reused,
				Cached: phase.Meta.Cached,
			},
		}
		if !anchor.IsZero() {
			anchor = end
		}
	}

	tl.Phases = phases
	if tl.Duration <= 0 {
		var sum time.Duration
		for _, phase := range phases {
			sum += phase.Duration
		}
		tl.Duration = sum
	}
	if tl.Completed.IsZero() && !tl.Started.IsZero() && tl.Duration > 0 {
		tl.Completed = tl.Started.Add(tl.Duration)
	}
	if tl.Started.IsZero() && !tl.Completed.IsZero() && tl.Duration > 0 {
		tl.Started = tl.Completed.Add(-tl.Duration)
	}
	return tl
}

func (s *TraceSummary) Report() *nettrace.Report {
	if s == nil {
		return nil
	}

	tl := s.Timeline()
	if tl == nil {
		return nil
	}

	var budget nettrace.Budget
	if s.Budgets != nil {
		budget.Total = s.Budgets.Total
		budget.Tolerance = s.Budgets.Tolerance
		if len(s.Budgets.Phases) > 0 {
			phases := make(map[nettrace.PhaseKind]time.Duration, len(s.Budgets.Phases))
			for name, dur := range s.Budgets.Phases {
				if dur <= 0 {
					continue
				}
				phases[nettrace.PhaseKind(name)] = dur
			}
			if len(phases) > 0 {
				budget.Phases = phases
			}
		}
	}

	rep := nettrace.NewReport(tl, budget)
	if rep == nil {
		return nil
	}
	if len(s.Breaches) == 0 {
		return rep
	}

	breaches := make([]nettrace.BudgetBreach, len(s.Breaches))
	for i, br := range s.Breaches {
		breaches[i] = nettrace.BudgetBreach{
			Kind:   nettrace.PhaseKind(br.Kind),
			Limit:  br.Limit,
			Actual: br.Actual,
			Over:   br.Over,
		}
	}
	rep.BudgetReport.Breaches = breaches
	return rep
}

func traceDetailsFromTimeline(details *nettrace.TraceDetails) *TraceDetails {
	if details == nil {
		return nil
	}
	out := &TraceDetails{
		Connection: connFromTimeline(details.Connection),
		TLS:        tlsFromTimeline(details.TLS),
	}
	if out.Connection == nil && out.TLS == nil {
		return nil
	}
	return out
}

func traceDetailsToTimeline(details *TraceDetails) *nettrace.TraceDetails {
	if details == nil {
		return nil
	}
	out := &nettrace.TraceDetails{
		Connection: connToTimeline(details.Connection),
		TLS:        tlsToTimeline(details.TLS),
	}
	if out.Connection == nil && out.TLS == nil {
		return nil
	}
	return out
}

func connFromTimeline(c *nettrace.ConnDetails) *TraceConn {
	if c == nil {
		return nil
	}
	return &TraceConn{
		Reused:        c.Reused,
		WasIdle:       c.WasIdle,
		IdleTime:      c.IdleTime,
		Network:       c.Network,
		DialAddr:      c.DialAddr,
		LocalAddr:     c.LocalAddr,
		RemoteAddr:    c.RemoteAddr,
		ResolvedAddrs: cloneStrings(c.ResolvedAddrs),
		Proxy:         c.Proxy,
		ProxyTunnel:   c.ProxyTunnel,
		SSH:           c.SSH,
		Protocol:      c.Protocol,
	}
}

func tlsFromTimeline(t *nettrace.TLSDetails) *TraceTLS {
	if t == nil {
		return nil
	}
	return &TraceTLS{
		Version:      t.Version,
		Cipher:       t.Cipher,
		ALPN:         t.ALPN,
		ServerName:   t.ServerName,
		Resumed:      t.Resumed,
		Verified:     t.Verified,
		Certificates: certsFromTimeline(t.Certificates),
	}
}

func certsFromTimeline(certs []nettrace.TLSCert) []TraceCert {
	if len(certs) == 0 {
		return nil
	}
	out := make([]TraceCert, len(certs))
	for i, cert := range certs {
		out[i] = TraceCert{
			Subject:   cert.Subject,
			Issuer:    cert.Issuer,
			SANs:      cloneStrings(cert.SANs),
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
			Serial:    cert.Serial,
		}
	}
	return out
}

func connToTimeline(c *TraceConn) *nettrace.ConnDetails {
	if c == nil {
		return nil
	}
	return &nettrace.ConnDetails{
		Reused:        c.Reused,
		WasIdle:       c.WasIdle,
		IdleTime:      c.IdleTime,
		Network:       c.Network,
		DialAddr:      c.DialAddr,
		LocalAddr:     c.LocalAddr,
		RemoteAddr:    c.RemoteAddr,
		ResolvedAddrs: cloneStrings(c.ResolvedAddrs),
		Proxy:         c.Proxy,
		ProxyTunnel:   c.ProxyTunnel,
		SSH:           c.SSH,
		Protocol:      c.Protocol,
	}
}

func tlsToTimeline(t *TraceTLS) *nettrace.TLSDetails {
	if t == nil {
		return nil
	}
	return &nettrace.TLSDetails{
		Version:      t.Version,
		Cipher:       t.Cipher,
		ALPN:         t.ALPN,
		ServerName:   t.ServerName,
		Resumed:      t.Resumed,
		Verified:     t.Verified,
		Certificates: certsToTimeline(t.Certificates),
	}
}

func certsToTimeline(certs []TraceCert) []nettrace.TLSCert {
	if len(certs) == 0 {
		return nil
	}
	out := make([]nettrace.TLSCert, len(certs))
	for i, cert := range certs {
		out[i] = nettrace.TLSCert{
			Subject:   cert.Subject,
			Issuer:    cert.Issuer,
			SANs:      cloneStrings(cert.SANs),
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
			Serial:    cert.Serial,
		}
	}
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
