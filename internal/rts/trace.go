package rts

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
)

type Trace struct {
	Rep *nettrace.Report
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

func newTraceObj(t *Trace) *traceObj {
	if t == nil || t.Rep == nil || t.Rep.Timeline == nil {
		return &traceObj{}
	}
	rep := t.Rep
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
		k := strings.ToLower(s.name)
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

func (o *traceObj) GetMember(name string) (Value, bool) {
	switch name {
	case "enabled":
		return NativeNamed("trace.enabled", o.enabledFn), true
	case "durationMs":
		return NativeNamed("trace.durationMs", o.durMsFn), true
	case "durationSeconds":
		return NativeNamed("trace.durationSeconds", o.durSecFn), true
	case "durationString":
		return NativeNamed("trace.durationString", o.durStrFn), true
	case "error":
		return NativeNamed("trace.error", o.errFn), true
	case "started":
		return NativeNamed("trace.started", o.startedFn), true
	case "completed":
		return NativeNamed("trace.completed", o.completedFn), true
	case "phases":
		return NativeNamed("trace.phases", o.phasesFn), true
	case "getPhase":
		return NativeNamed("trace.getPhase", o.getPhaseFn), true
	case "phaseNames":
		return NativeNamed("trace.phaseNames", o.phaseNamesFn), true
	case "budgets":
		return NativeNamed("trace.budgets", o.budgetsFn), true
	case "connection":
		return NativeNamed("trace.connection", o.connFn), true
	case "tls":
		return NativeNamed("trace.tls", o.tlsFn), true
	case "breaches":
		return NativeNamed("trace.breaches", o.breachesFn), true
	case "withinBudget":
		return NativeNamed("trace.withinBudget", o.withinFn), true
	case "hasBudgets":
		return NativeNamed("trace.hasBudgets", o.hasBudFn), true
	}
	return Null(), false
}

func (o *traceObj) Index(key Value) (Value, error) {
	return Null(), nil
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

func (o *traceObj) enabledFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceEnabled)
	if err := na.none(); err != nil {
		return Null(), err
	}
	return Bool(o.tl != nil), nil
}

func (o *traceObj) durMsFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceDurationMs)
	if err := na.none(); err != nil {
		return Null(), err
	}
	if o.tl == nil {
		return Num(0), nil
	}
	return Num(durMs(o.tl.Duration)), nil
}

func (o *traceObj) durSecFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceDurationSeconds)
	if err := na.none(); err != nil {
		return Null(), err
	}
	if o.tl == nil {
		return Num(0), nil
	}
	return Num(o.tl.Duration.Seconds()), nil
}

func (o *traceObj) durStrFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceDurationString)
	if err := na.none(); err != nil {
		return Null(), err
	}
	if o.tl == nil {
		return Str(""), nil
	}
	return Str(o.tl.Duration.String()), nil
}

func (o *traceObj) errFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceError)
	if err := na.none(); err != nil {
		return Null(), err
	}
	if o.tl == nil {
		return Str(""), nil
	}
	return Str(o.tl.Err), nil
}

func (o *traceObj) startedFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceStarted)
	if err := na.none(); err != nil {
		return Null(), err
	}
	if o.tl == nil || o.tl.Started.IsZero() {
		return Str(""), nil
	}
	return Str(o.tl.Started.Format(time.RFC3339Nano)), nil
}

func (o *traceObj) completedFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceCompleted)
	if err := na.none(); err != nil {
		return Null(), err
	}
	if o.tl == nil || o.tl.Completed.IsZero() {
		return Str(""), nil
	}
	return Str(o.tl.Completed.Format(time.RFC3339Nano)), nil
}

func (o *traceObj) phasesFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTracePhases)
	if err := na.none(); err != nil {
		return Null(), err
	}
	if len(o.seg) == 0 {
		return List(nil), nil
	}
	out := make([]any, 0, len(o.seg))
	for _, s := range o.seg {
		out = append(out, o.segMap(s))
	}
	return fromIface(ctx, pos, out)
}

func (o *traceObj) getPhaseFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	sig := sigTraceGetPhase
	if len(args) != 1 {
		return Null(), rtErr(ctx, pos, "%s expects 1 arg", sig)
	}
	k, err := toKey(pos, args[0])
	if err != nil {
		return Null(), wrapErr(ctx, err)
	}
	k = strings.ToLower(strings.TrimSpace(k))
	if k == "" {
		return Null(), nil
	}
	a, ok := o.ag[k]
	if !ok {
		return Null(), nil
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
	return fromIface(ctx, pos, res)
}

func (o *traceObj) phaseNamesFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTracePhaseNames)
	if err := na.none(); err != nil {
		return Null(), err
	}
	if len(o.ord) == 0 {
		return List(nil), nil
	}
	out := make([]any, 0, len(o.ord))
	for _, name := range o.ord {
		out = append(out, name)
	}
	return fromIface(ctx, pos, out)
}

func (o *traceObj) connFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := ensureNoArgs(ctx, pos, args, "trace.connection"); err != nil {
		return Null(), err
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
	return fromIface(ctx, pos, res)
}

func (o *traceObj) tlsFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	if err := ensureNoArgs(ctx, pos, args, "trace.tls"); err != nil {
		return Null(), err
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
	return fromIface(ctx, pos, res)
}

func (o *traceObj) budgetsFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceBudgets)
	if err := na.none(); err != nil {
		return Null(), err
	}
	if !o.hasBud() {
		return Dict(map[string]Value{"enabled": Bool(false)}), nil
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
	return fromIface(ctx, pos, res)
}

func (o *traceObj) breachesFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceBreaches)
	if err := na.none(); err != nil {
		return Null(), err
	}
	if len(o.br) == 0 {
		return List(nil), nil
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
	return fromIface(ctx, pos, out)
}

func (o *traceObj) withinFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceWithinBudget)
	if err := na.none(); err != nil {
		return Null(), err
	}
	return Bool(len(o.br) == 0), nil
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

func ensureNoArgs(ctx *Ctx, pos Pos, args []Value, name string) error {
	if len(args) != 0 {
		return rtErr(ctx, pos, "%s() expects 0 args", name)
	}
	return nil
}

func traceUnavailable() Value {
	return Dict(map[string]Value{"available": Bool(false)})
}

func (o *traceObj) hasBudFn(ctx *Ctx, pos Pos, args []Value) (Value, error) {
	na := newNativeArgs(ctx, pos, args, sigTraceHasBudgets)
	if err := na.none(); err != nil {
		return Null(), err
	}
	return Bool(o.hasBud()), nil
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
