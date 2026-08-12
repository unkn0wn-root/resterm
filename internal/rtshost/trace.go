package rtshost

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/rts"
	"github.com/unkn0wn-root/resterm/internal/rts/native"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
)

type Trace struct {
	Report *nettrace.Report
}

type traceObj struct {
	tl  *nettrace.Timeline
	bud nettrace.Budget
	br  []nettrace.BudgetBreach
	seg []trSeg
	ag  map[string]*trAgg
	ord []string
}

type trAgg struct {
	name string
	dur  time.Duration
	cnt  int
	seg  []trSeg
}

type trSeg struct {
	name  string
	start time.Time
	end   time.Time
	dur   time.Duration
	err   string
	meta  nettrace.PhaseMeta
}

func phaseKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func newTraceObj(t *Trace) *traceObj {
	if t == nil || t.Report == nil || t.Report.Timeline == nil {
		return &traceObj{}
	}
	rep := t.Report
	if c := rep.Clone(); c != nil {
		rep = c
	}
	o := &traceObj{
		tl:  rep.Timeline,
		bud: rep.Budget,
		br:  rep.BudgetReport.Breaches,
		ag:  make(map[string]*trAgg),
	}
	for _, ph := range o.tl.Phases {
		s := trSeg{
			name:  string(ph.Kind),
			start: ph.Start,
			end:   ph.End,
			dur:   ph.Duration,
			err:   ph.Err,
			meta:  ph.Meta,
		}
		o.seg = append(o.seg, s)
		// A phase with no name cannot be looked up, so it stays out of the
		// aggregate rather than sitting under a key no argument can reach.
		if strings.TrimSpace(s.name) == "" {
			continue
		}
		k := phaseKey(s.name)
		a, ok := o.ag[k]
		if !ok {
			a = &trAgg{name: s.name}
			o.ag[k] = a
			o.ord = append(o.ord, s.name)
		}
		a.dur += s.dur
		a.cnt++
		a.seg = append(a.seg, s)
	}
	return o
}

func (o *traceObj) TypeName() string { return "trace" }

func (o *traceObj) Member(_ *rts.Ctx, _ rts.Pos, name string) (rts.Value, bool, error) {
	switch name {
	case "enabled":
		return rts.NativeNamed("trace.enabled", o.enabledFn), true, nil
	case "durationMs":
		return rts.NativeNamed("trace.durationMs", o.durMsFn), true, nil
	case "durationSeconds":
		return rts.NativeNamed("trace.durationSeconds", o.durSecFn), true, nil
	case "durationString":
		return rts.NativeNamed("trace.durationString", o.durStrFn), true, nil
	case "error":
		return rts.NativeNamed("trace.error", o.errFn), true, nil
	case "started":
		return rts.NativeNamed("trace.started", o.startedFn), true, nil
	case "completed":
		return rts.NativeNamed("trace.completed", o.completedFn), true, nil
	case "phases":
		return rts.NativeNamed("trace.phases", o.phasesFn), true, nil
	case "getPhase":
		return rts.NativeNamed("trace.getPhase", o.getPhaseFn), true, nil
	case "phaseNames":
		return rts.NativeNamed("trace.phaseNames", o.phaseNamesFn), true, nil
	case "budgets":
		return rts.NativeNamed("trace.budgets", o.budgetsFn), true, nil
	case "connection":
		return rts.NativeNamed("trace.connection", o.connFn), true, nil
	case "tls":
		return rts.NativeNamed("trace.tls", o.tlsFn), true, nil
	case "breaches":
		return rts.NativeNamed("trace.breaches", o.breachesFn), true, nil
	case "withinBudget":
		return rts.NativeNamed("trace.withinBudget", o.withinFn), true, nil
	case "hasBudgets":
		return rts.NativeNamed("trace.hasBudgets", o.hasBudFn), true, nil
	}
	return rts.Null(), false, nil
}

func (o *traceObj) Index(_ *rts.Ctx, _ rts.Pos, key rts.Value) (rts.Value, error) {
	return rts.Null(), nil
}

const (
	sigTraceEnabled         = "trace.enabled()"
	sigTraceDurationMs      = "trace.durationMs()"
	sigTraceDurationSeconds = "trace.durationSeconds()"
	sigTraceDurationString  = "trace.durationString()"
	sigTraceError           = "trace.error()"
	sigTraceStarted         = "trace.started()"
	sigTraceCompleted       = "trace.completed()"
	sigTracePhases          = "trace.phases()"
	sigTraceGetPhase        = "trace.getPhase(name)"
	sigTracePhaseNames      = "trace.phaseNames()"
	sigTraceBudgets         = "trace.budgets()"
	sigTraceBreaches        = "trace.breaches()"
	sigTraceWithinBudget    = "trace.withinBudget()"
	sigTraceHasBudgets      = "trace.hasBudgets()"
)

func (o *traceObj) enabledFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceEnabled)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	return rts.Bool(o.tl != nil), nil
}

func (o *traceObj) durMsFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceDurationMs)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	if o.tl == nil {
		return rts.Num(0), nil
	}
	return rts.Num(durMs(o.tl.Duration)), nil
}

func (o *traceObj) durSecFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceDurationSeconds)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	if o.tl == nil {
		return rts.Num(0), nil
	}
	return rts.Num(o.tl.Duration.Seconds()), nil
}

func (o *traceObj) durStrFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceDurationString)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	if o.tl == nil {
		return rts.Str(""), nil
	}
	return native.StringValue(ctx, pos, o.tl.Duration.String())
}

func (o *traceObj) errFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceError)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	if o.tl == nil {
		return rts.Str(""), nil
	}
	return native.StringValue(ctx, pos, o.tl.Err)
}

func (o *traceObj) startedFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceStarted)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	if o.tl == nil || o.tl.Started.IsZero() {
		return rts.Str(""), nil
	}
	return native.StringValue(ctx, pos, o.tl.Started.Format(time.RFC3339Nano))
}

func (o *traceObj) completedFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceCompleted)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	if o.tl == nil || o.tl.Completed.IsZero() {
		return rts.Str(""), nil
	}
	return native.StringValue(ctx, pos, o.tl.Completed.Format(time.RFC3339Nano))
}

func (o *traceObj) phasesFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTracePhases)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	if len(o.seg) == 0 {
		return rts.List(nil), nil
	}
	out := make([]any, 0, len(o.seg))
	for _, s := range o.seg {
		out = append(out, o.segMap(s))
	}
	return rts.FromIface(ctx, pos, out)
}

func (o *traceObj) getPhaseFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	sig := sigTraceGetPhase
	if len(args) != 1 {
		return rts.Null(), rts.Errf(ctx, pos, "%s expects 1 arg", sig)
	}
	k, err := rts.Key(pos, args[0])
	if err != nil {
		return rts.Null(), rts.WrapErr(ctx, err)
	}
	if strings.TrimSpace(k) == "" {
		return rts.Null(), nil
	}
	a, ok := o.ag[phaseKey(k)]
	if !ok {
		return rts.Null(), nil
	}
	segs := make([]any, 0, len(a.seg))
	for _, s := range a.seg {
		segs = append(segs, o.segMap(s))
	}
	res := map[string]any{
		"name":            a.name,
		"count":           float64(a.cnt),
		"durationMs":      durMs(a.dur),
		"durationSeconds": a.dur.Seconds(),
		"durationString":  a.dur.String(),
		"segments":        segs,
	}
	return rts.FromIface(ctx, pos, res)
}

func (o *traceObj) phaseNamesFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTracePhaseNames)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	if len(o.ord) == 0 {
		return rts.List(nil), nil
	}
	out := make([]any, 0, len(o.ord))
	for _, name := range o.ord {
		out = append(out, name)
	}
	return rts.FromIface(ctx, pos, out)
}

func (o *traceObj) connFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	if err := ensureNoArgs(ctx, pos, args, "trace.connection"); err != nil {
		return rts.Null(), err
	}
	if o.tl == nil || o.tl.Details == nil || o.tl.Details.Connection == nil {
		return traceUnavailable(), nil
	}
	conn := o.tl.Details.Connection
	res := map[string]any{
		"available":     true,
		"reused":        conn.Reused,
		"wasIdle":       conn.WasIdle,
		"idleMs":        durMs(conn.IdleTime),
		"idleSeconds":   conn.IdleTime.Seconds(),
		"idleString":    conn.IdleTime.String(),
		"network":       conn.Network,
		"dialAddr":      conn.DialAddr,
		"localAddr":     conn.LocalAddr,
		"remoteAddr":    conn.RemoteAddr,
		"resolvedAddrs": toIfaceList(conn.ResolvedAddrs),
		"proxy":         conn.Proxy,
		"proxyTunnel":   conn.ProxyTunnel,
		"ssh":           conn.SSH,
		"k8s":           conn.K8s,
		"protocol":      conn.Protocol,
	}
	return rts.FromIface(ctx, pos, res)
}

func (o *traceObj) tlsFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	if err := ensureNoArgs(ctx, pos, args, "trace.tls"); err != nil {
		return rts.Null(), err
	}
	if o.tl == nil || o.tl.Details == nil || o.tl.Details.TLS == nil {
		return traceUnavailable(), nil
	}
	det := o.tl.Details.TLS
	certs := make([]any, 0, len(det.Certificates))
	for _, cert := range det.Certificates {
		certs = append(certs, map[string]any{
			"subject":   cert.Subject,
			"issuer":    cert.Issuer,
			"sans":      toIfaceList(cert.SANs),
			"notBefore": cert.NotBefore.Format(time.RFC3339Nano),
			"notAfter":  cert.NotAfter.Format(time.RFC3339Nano),
			"serial":    cert.Serial,
		})
	}
	res := map[string]any{
		"available":  true,
		"version":    det.Version,
		"cipher":     det.Cipher,
		"alpn":       det.ALPN,
		"serverName": det.ServerName,
		"resumed":    det.Resumed,
		"verified":   det.Verified,
		"certs":      certs,
	}
	return rts.FromIface(ctx, pos, res)
}

func (o *traceObj) budgetsFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceBudgets)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	if !o.hasBud() {
		return rts.Dict(map[string]rts.Value{"enabled": rts.Bool(false)}), nil
	}
	ph := make(map[string]any)
	for k, d := range o.bud.Phases {
		ph[string(k)] = durMs(d)
	}
	res := map[string]any{
		"enabled":          true,
		"totalMs":          durMs(o.bud.Total),
		"totalSeconds":     o.bud.Total.Seconds(),
		"toleranceMs":      durMs(o.bud.Tolerance),
		"toleranceSeconds": o.bud.Tolerance.Seconds(),
		"phases":           ph,
	}
	return rts.FromIface(ctx, pos, res)
}

func (o *traceObj) breachesFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceBreaches)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	if len(o.br) == 0 {
		return rts.List(nil), nil
	}
	out := make([]any, 0, len(o.br))
	for _, b := range o.br {
		out = append(out, map[string]any{
			"name":          string(b.Kind),
			"limitMs":       durMs(b.Limit),
			"limitSeconds":  b.Limit.Seconds(),
			"actualMs":      durMs(b.Actual),
			"actualSeconds": b.Actual.Seconds(),
			"overMs":        durMs(b.Over),
			"overSeconds":   b.Over.Seconds(),
		})
	}
	return rts.FromIface(ctx, pos, out)
}

func (o *traceObj) withinFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceWithinBudget)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	return rts.Bool(len(o.br) == 0), nil
}

func toIfaceList(items []string) []any {
	if len(items) == 0 {
		return nil
	}
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

func ensureNoArgs(ctx *rts.Ctx, pos rts.Pos, args []rts.Value, name string) error {
	if len(args) != 0 {
		return rts.Errf(ctx, pos, "%s() expects 0 args", name)
	}
	return nil
}

func traceUnavailable() rts.Value {
	return rts.Dict(map[string]rts.Value{"available": rts.Bool(false)})
}

func (o *traceObj) hasBudFn(ctx *rts.Ctx, pos rts.Pos, args []rts.Value) (rts.Value, error) {
	na := rts.NewArgs(ctx, pos, args, sigTraceHasBudgets)
	if err := na.None(); err != nil {
		return rts.Null(), err
	}
	return rts.Bool(o.hasBud()), nil
}

func (o *traceObj) hasBud() bool {
	if o.bud.Total > 0 || o.bud.Tolerance > 0 {
		return true
	}
	return len(o.bud.Phases) > 0
}

func (o *traceObj) segMap(s trSeg) map[string]any {
	meta := map[string]any{
		"addr":   s.meta.Addr,
		"reused": s.meta.Reused,
		"cached": s.meta.Cached,
	}
	res := map[string]any{
		"name":            s.name,
		"durationMs":      durMs(s.dur),
		"durationSeconds": s.dur.Seconds(),
		"durationString":  s.dur.String(),
		"error":           s.err,
		"meta":            meta,
	}
	if !s.start.IsZero() {
		res["start"] = s.start.Format(time.RFC3339Nano)
	}
	if !s.end.IsZero() {
		res["end"] = s.end.Format(time.RFC3339Nano)
	}
	return res
}

func durMs(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
