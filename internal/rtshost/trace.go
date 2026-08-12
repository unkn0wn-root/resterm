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
	tl      *nettrace.Timeline
	bud     nettrace.Budget
	br      []nettrace.BudgetBreach
	seg     []trSeg
	ag      map[string]*trAgg
	ord     []string
	members map[string]rts.Value
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
	o := &traceObj{ag: make(map[string]*trAgg)}
	if t != nil && t.Report != nil && t.Report.Timeline != nil {
		rep := t.Report
		if c := rep.Clone(); c != nil {
			rep = c
		}
		o.tl = rep.Timeline
		o.bud = rep.Budget
		o.br = rep.BudgetReport.Breaches
		for _, ph := range o.tl.Phases {
			o.addPhase(ph)
		}
	}
	o.members = map[string]rts.Value{
		"enabled":         hostFn0("trace", "enabled", o.enabled),
		"durationMs":      hostFn0("trace", "durationMs", o.durationMs),
		"durationSeconds": hostFn0("trace", "durationSeconds", o.durationSeconds),
		"durationString":  hostFn0("trace", "durationString", o.durationString),
		"error":           hostFn0("trace", "error", o.errText),
		"started":         hostFn0("trace", "started", o.started),
		"completed":       hostFn0("trace", "completed", o.completed),
		"phases":          hostFn0("trace", "phases", o.phases),
		"phaseNames":      hostFn0("trace", "phaseNames", o.phaseNames),
		"budgets":         hostFn0("trace", "budgets", o.budgets),
		"connection":      hostFn0("trace", "connection", o.connection),
		"tls":             hostFn0("trace", "tls", o.tls),
		"breaches":        hostFn0("trace", "breaches", o.breaches),
		"withinBudget":    hostFn0("trace", "withinBudget", o.withinBudget),
		"hasBudgets":      hostFn0("trace", "hasBudgets", o.hasBudgets),
		"getPhase": native.Fn1(
			"trace.getPhase", "trace.getPhase(name)", native.String, o.getPhase,
		).Value(),
	}
	return o
}

func (o *traceObj) addPhase(ph nettrace.Phase) {
	s := trSeg{
		name:  string(ph.Kind),
		start: ph.Start,
		end:   ph.End,
		dur:   ph.Duration,
		err:   ph.Err,
		meta:  ph.Meta,
	}
	o.seg = append(o.seg, s)
	if strings.TrimSpace(s.name) == "" {
		return
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

func (o *traceObj) TypeName() string { return "trace" }

func (o *traceObj) Member(_ *rts.Ctx, _ rts.Pos, name string) (rts.Value, bool, error) {
	v, ok := o.members[name]
	if !ok {
		return rts.Null(), false, nil
	}
	return v, true, nil
}

func (*traceObj) Index(*rts.Ctx, rts.Pos, rts.Value) (rts.Value, error) {
	return rts.Null(), nil
}

func (o *traceObj) enabled(native.Call) (rts.Value, error) {
	return rts.Bool(o.tl != nil), nil
}

func (o *traceObj) durationMs(native.Call) (rts.Value, error) {
	if o.tl == nil {
		return rts.Num(0), nil
	}
	return rts.Num(durMs(o.tl.Duration)), nil
}

func (o *traceObj) durationSeconds(native.Call) (rts.Value, error) {
	if o.tl == nil {
		return rts.Num(0), nil
	}
	return rts.Num(o.tl.Duration.Seconds()), nil
}

func (o *traceObj) durationString(call native.Call) (rts.Value, error) {
	if o.tl == nil {
		return rts.Str(""), nil
	}
	return native.StringValue(call.Ctx, call.Pos, o.tl.Duration.String())
}

func (o *traceObj) errText(call native.Call) (rts.Value, error) {
	if o.tl == nil {
		return rts.Str(""), nil
	}
	return native.StringValue(call.Ctx, call.Pos, o.tl.Err)
}

func (o *traceObj) started(call native.Call) (rts.Value, error) {
	if o.tl == nil || o.tl.Started.IsZero() {
		return rts.Str(""), nil
	}
	return native.StringValue(call.Ctx, call.Pos, o.tl.Started.Format(time.RFC3339Nano))
}

func (o *traceObj) completed(call native.Call) (rts.Value, error) {
	if o.tl == nil || o.tl.Completed.IsZero() {
		return rts.Str(""), nil
	}
	return native.StringValue(call.Ctx, call.Pos, o.tl.Completed.Format(time.RFC3339Nano))
}

func (o *traceObj) phases(call native.Call) (rts.Value, error) {
	if len(o.seg) == 0 {
		return rts.List(nil), nil
	}
	out := make([]any, 0, len(o.seg))
	for _, s := range o.seg {
		out = append(out, o.segMap(s))
	}
	return rts.FromIface(call.Ctx, call.Pos, out)
}

func (o *traceObj) getPhase(call native.Call, name string) (rts.Value, error) {
	if strings.TrimSpace(name) == "" {
		return rts.Null(), nil
	}
	a, ok := o.ag[phaseKey(name)]
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
	return rts.FromIface(call.Ctx, call.Pos, res)
}

func (o *traceObj) phaseNames(call native.Call) (rts.Value, error) {
	if len(o.ord) == 0 {
		return rts.List(nil), nil
	}
	out := make([]any, 0, len(o.ord))
	for _, name := range o.ord {
		out = append(out, name)
	}
	return rts.FromIface(call.Ctx, call.Pos, out)
}

func (o *traceObj) connection(call native.Call) (rts.Value, error) {
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
	return rts.FromIface(call.Ctx, call.Pos, res)
}

func (o *traceObj) tls(call native.Call) (rts.Value, error) {
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
	return rts.FromIface(call.Ctx, call.Pos, res)
}

func (o *traceObj) budgets(call native.Call) (rts.Value, error) {
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
	return rts.FromIface(call.Ctx, call.Pos, res)
}

func (o *traceObj) breaches(call native.Call) (rts.Value, error) {
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
	return rts.FromIface(call.Ctx, call.Pos, out)
}

func (o *traceObj) withinBudget(native.Call) (rts.Value, error) {
	return rts.Bool(len(o.br) == 0), nil
}

func (o *traceObj) hasBudgets(native.Call) (rts.Value, error) {
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

func traceUnavailable() rts.Value {
	return rts.Dict(map[string]rts.Value{"available": rts.Bool(false)})
}

func durMs(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
