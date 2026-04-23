package k8s

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	gruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	kruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const (
	diagCap    = 128
	diagMaxAge = 5 * time.Second
	diagSkew   = 300 * time.Millisecond
)

var (
	diagInstallOnce sync.Once
	pfErrPattern    = regexp.MustCompile(`an error occurred forwarding (\d+)\s*->\s*(\d+):\s*(.+)$`)
	nsPattern       = regexp.MustCompile(`inside namespace "[^"]+"`)
	podPattern      = regexp.MustCompile(`to pod [^,]+,\s*uid\s*:\s*`)
)

type diagRecord struct {
	at  time.Time
	raw string
	pf  *pfErr
}

type pfErr struct {
	local   int
	remote  int
	summary string
}

type diagPortKey struct {
	local  int
	remote int
}

type diagState struct {
	mu         sync.RWMutex
	collectors map[diagPortKey]map[*diagCollector]struct{}
}

type diagCollector struct {
	mu        sync.Mutex
	buf       ring[diagRecord]
	key       diagPortKey
	reg       *diagState
	closed    bool
	closeOnce sync.Once
}

type RequestDiag struct {
	mu        sync.RWMutex
	collector *diagCollector
}

type ring[T any] struct {
	vals []T
	next int
	n    int
}

type requestDiagContextKey struct{}

var rtDiag = newDiagState()

func newDiagState() *diagState {
	return &diagState{collectors: make(map[diagPortKey]map[*diagCollector]struct{})}
}

func newDiagCollector(local, remote, capacity int) *diagCollector {
	reg := rtDiag
	collector := &diagCollector{
		buf: newRing[diagRecord](capacity),
		key: diagPortKey{local: local, remote: remote},
		reg: reg,
	}
	if reg != nil {
		reg.register(collector)
	}
	return collector
}

func newRing[T any](capacity int) ring[T] {
	if capacity < 1 {
		capacity = 1
	}
	return ring[T]{vals: make([]T, capacity)}
}

func (r *ring[T]) push(v T) {
	r.vals[r.next] = v
	r.next = (r.next + 1) % len(r.vals)
	if r.n < len(r.vals) {
		r.n++
	}
}

func (r *ring[T]) eachNewest(fn func(T) bool) {
	if r.n == 0 {
		return
	}
	for i := 0; i < r.n; i++ {
		j := r.next - 1 - i
		if j < 0 {
			j += len(r.vals)
		}
		if !fn(r.vals[j]) {
			return
		}
	}
}

func (s *diagState) register(collector *diagCollector) {
	if s == nil || collector == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	slot := s.collectors[collector.key]
	if slot == nil {
		slot = make(map[*diagCollector]struct{})
		s.collectors[collector.key] = slot
	}
	slot[collector] = struct{}{}
}

func (s *diagState) unregister(collector *diagCollector) {
	if s == nil || collector == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	slot := s.collectors[collector.key]
	if len(slot) == 0 {
		return
	}
	delete(slot, collector)
	if len(slot) == 0 {
		delete(s.collectors, collector.key)
	}
}

func (s *diagState) dispatch(rec diagRecord) {
	if s == nil || rec.pf == nil {
		return
	}

	key := diagPortKey{local: rec.pf.local, remote: rec.pf.remote}
	s.mu.RLock()
	slot := s.collectors[key]
	collectors := make([]*diagCollector, 0, len(slot))
	for collector := range slot {
		collectors = append(collectors, collector)
	}
	s.mu.RUnlock()

	for _, collector := range collectors {
		collector.push(rec)
	}
}

func (c *diagCollector) push(rec diagRecord) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.buf.push(rec)
}

func (c *diagCollector) latestPFErr(now, startedAt time.Time, maxAge time.Duration) (pfErr, bool) {
	if c == nil {
		return pfErr{}, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	startCut := startedAt.Add(-diagSkew)
	var out pfErr
	var ok bool
	c.buf.eachNewest(func(rec diagRecord) bool {
		if maxAge > 0 && now.Sub(rec.at) > maxAge {
			return false
		}
		if !startedAt.IsZero() && rec.at.Before(startCut) {
			return false
		}
		if rec.pf == nil {
			return true
		}
		out = *rec.pf
		ok = true
		return false
	})
	return out, ok
}

func (c *diagCollector) close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		if c.reg != nil {
			c.reg.unregister(c)
		}
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
	})
}

func (d *RequestDiag) attach(collector *diagCollector) {
	if d == nil || collector == nil {
		return
	}
	d.mu.Lock()
	d.collector = collector
	d.mu.Unlock()
}

func (d *RequestDiag) latestPFErr(now, startedAt time.Time, maxAge time.Duration) (pfErr, bool) {
	if d == nil {
		return pfErr{}, false
	}
	d.mu.RLock()
	collector := d.collector
	d.mu.RUnlock()
	if collector == nil {
		return pfErr{}, false
	}
	return collector.latestPFErr(now, startedAt, maxAge)
}

func BindRequestContext(ctx context.Context) (context.Context, *RequestDiag) {
	if ctx == nil {
		ctx = context.Background()
	}
	diag := &RequestDiag{}
	return context.WithValue(ctx, requestDiagContextKey{}, diag), diag
}

func RequestDiagFromContext(ctx context.Context) *RequestDiag {
	if ctx == nil {
		return nil
	}
	diag, _ := ctx.Value(requestDiagContextKey{}).(*RequestDiag)
	return diag
}

func AnnotateRequestError(err error, startedAt time.Time, diag *RequestDiag) error {
	if err == nil {
		return nil
	}
	pf, ok := diag.latestPFErr(time.Now(), startedAt, diagMaxAge)
	if !ok || strings.TrimSpace(pf.summary) == "" {
		return err
	}
	if strings.Contains(err.Error(), pf.summary) {
		return err
	}
	return fmt.Errorf("%w | %s", err, pf.summary)
}

func bindRequestDiag(ctx context.Context, collector *diagCollector) {
	if collector == nil || ctx == nil {
		return
	}
	diag := RequestDiagFromContext(ctx)
	if diag == nil {
		return
	}
	diag.attach(collector)
}

func ensureRuntimeDiagInstalled() {
	diagInstallOnce.Do(func() {
		kruntime.ErrorHandlers = buildRuntimeDiagErrorHandlers(kruntime.ErrorHandlers)
	})
}

func buildRuntimeDiagErrorHandlers(prev []kruntime.ErrorHandler) []kruntime.ErrorHandler {
	hs := make([]kruntime.ErrorHandler, 0, len(prev)+1)
	hs = append(hs, captureRuntimeErr)
	for _, h := range prev {
		if isDefaultRuntimeLogHandler(h) {
			continue
		}
		hs = append(hs, h)
	}
	return hs
}

func isDefaultRuntimeLogHandler(h kruntime.ErrorHandler) bool {
	name := runtimeHandlerName(h)
	return strings.Contains(name, "k8s.io/apimachinery/pkg/util/runtime") &&
		strings.HasSuffix(name, ".logError")
}

func runtimeHandlerName(h kruntime.ErrorHandler) string {
	if h == nil {
		return ""
	}
	ptr := reflect.ValueOf(h).Pointer()
	if ptr == 0 {
		return ""
	}
	fn := gruntime.FuncForPC(ptr)
	if fn == nil {
		return ""
	}
	return fn.Name()
}

func captureRuntimeErr(_ context.Context, err error, _ string, _ ...any) {
	pushRuntimeErr(time.Now(), err)
}

func pushRuntimeErr(at time.Time, err error) {
	if err == nil {
		return
	}
	raw := strings.TrimSpace(err.Error())
	if raw == "" {
		return
	}

	rec := diagRecord{at: at, raw: raw}
	if pf, ok := parsePFErr(raw); ok {
		rec.pf = &pf
	}

	rtDiag.dispatch(rec)
}

func parsePFErr(raw string) (pfErr, bool) {
	m := pfErrPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) != 4 {
		return pfErr{}, false
	}
	lp, err := strconv.Atoi(m[1])
	if err != nil {
		return pfErr{}, false
	}
	rp, err := strconv.Atoi(m[2])
	if err != nil {
		return pfErr{}, false
	}

	detail := summarizePFDetail(m[3])
	return pfErr{
		local:   lp,
		remote:  rp,
		summary: fmt.Sprintf("k8s port-forward %d->%d: %s", lp, rp, detail),
	}, true
}

func summarizePFDetail(raw string) string {
	v := strings.TrimSpace(raw)
	v = strings.Join(strings.Fields(v), " ")
	v = podPattern.ReplaceAllString(v, "to pod ")
	v = nsPattern.ReplaceAllString(v, "inside pod network namespace")

	if _, after, ok := strings.Cut(v, "failed to connect to "); ok {
		v = "failed to connect to " + after
	}
	const max = 220
	if len(v) > max {
		v = v[:max-3] + "..."
	}
	return v
}
