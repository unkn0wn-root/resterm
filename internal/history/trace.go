package history

import (
	"slices"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/tracebudget"
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
	K8s           string        `json:"k8s,omitempty"`
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
		Details:   traceDetailsFromTimeline(tl.Details),
	}
	if len(tl.Phases) == 0 {
		return summary
	}

	summary.Phases = make([]TracePhase, len(tl.Phases))
	for i, phase := range tl.Phases {
		summary.Phases[i] = TracePhase{
			Kind:     string(phase.Kind),
			Duration: phase.Duration,
			Error:    phase.Err,
			Meta:     TracePhaseMeta(phase.Meta),
		}
	}

	if rep == nil {
		return summary
	}
	if tracebudget.HasBudget(rep.Budget) {
		summary.Budgets = &TraceBudget{
			Total:     rep.Budget.Total,
			Tolerance: rep.Budget.Tolerance,
			Phases:    phaseLimits[string](rep.Budget.Phases),
		}
	}
	if len(rep.BudgetReport.Breaches) > 0 {
		summary.Breaches = make([]TraceBreach, len(rep.BudgetReport.Breaches))
		for i, br := range rep.BudgetReport.Breaches {
			summary.Breaches[i] = TraceBreach{
				Kind:   string(br.Kind),
				Limit:  br.Limit,
				Actual: br.Actual,
				Over:   br.Over,
			}
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
		Details:   traceDetailsToTimeline(s.Details),
	}
	if len(s.Phases) == 0 {
		return tl
	}

	// Only durations are stored, so phases are laid end to end from Started.
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
			Meta:     nettrace.PhaseMeta(phase.Meta),
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

	var budget nettrace.Budget
	if s.Budgets != nil {
		budget = nettrace.Budget{
			Total:     s.Budgets.Total,
			Tolerance: s.Budgets.Tolerance,
			Phases:    phaseLimits[nettrace.PhaseKind](s.Budgets.Phases),
		}
	}
	rep := nettrace.NewReport(s.Timeline(), budget)
	if len(s.Breaches) == 0 {
		return rep
	}

	rep.BudgetReport.Breaches = make([]nettrace.BudgetBreach, len(s.Breaches))
	for i, br := range s.Breaches {
		rep.BudgetReport.Breaches[i] = nettrace.BudgetBreach{
			Kind:   nettrace.PhaseKind(br.Kind),
			Limit:  br.Limit,
			Actual: br.Actual,
			Over:   br.Over,
		}
	}
	return rep
}

// A phase without a positive limit has no budget, and no limits at all stays nil.
func phaseLimits[To, From ~string](in map[From]time.Duration) map[To]time.Duration {
	out := make(map[To]time.Duration, len(in))
	for kind, limit := range in {
		if limit > 0 {
			out[To(kind)] = limit
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

// TraceConn, TraceCert and TracePhaseMeta have the same fields as their nettrace types.
// Converting directly means a field added there won't compile here until history keeps it too.
func connFromTimeline(c *nettrace.ConnDetails) *TraceConn {
	if c == nil {
		return nil
	}
	out := TraceConn(*c)
	out.ResolvedAddrs = slices.Clone(c.ResolvedAddrs)
	return &out
}

func connToTimeline(c *TraceConn) *nettrace.ConnDetails {
	if c == nil {
		return nil
	}
	out := nettrace.ConnDetails(*c)
	out.ResolvedAddrs = slices.Clone(c.ResolvedAddrs)
	return &out
}

func tlsFromTimeline(t *nettrace.TLSDetails) *TraceTLS {
	if t == nil {
		return nil
	}
	out := &TraceTLS{
		Version:    t.Version,
		Cipher:     t.Cipher,
		ALPN:       t.ALPN,
		ServerName: t.ServerName,
		Resumed:    t.Resumed,
		Verified:   t.Verified,
	}
	if len(t.Certificates) > 0 {
		out.Certificates = make([]TraceCert, len(t.Certificates))
		for i, cert := range t.Certificates {
			out.Certificates[i] = TraceCert(cert)
			out.Certificates[i].SANs = slices.Clone(cert.SANs)
		}
	}
	return out
}

func tlsToTimeline(t *TraceTLS) *nettrace.TLSDetails {
	if t == nil {
		return nil
	}
	out := &nettrace.TLSDetails{
		Version:    t.Version,
		Cipher:     t.Cipher,
		ALPN:       t.ALPN,
		ServerName: t.ServerName,
		Resumed:    t.Resumed,
		Verified:   t.Verified,
	}
	if len(t.Certificates) > 0 {
		out.Certificates = make([]nettrace.TLSCert, len(t.Certificates))
		for i, cert := range t.Certificates {
			out.Certificates[i] = nettrace.TLSCert(cert)
			out.Certificates[i].SANs = slices.Clone(cert.SANs)
		}
	}
	return out
}
