package k8s

import (
	"context"
	"fmt"
	"regexp"
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

type diagState struct {
	mu  sync.Mutex
	buf ring[diagRecord]
}

type ring[T any] struct {
	vals []T
	next int
	n    int
}

var rtDiag = newDiagState(diagCap)

func init() {
	installRuntimeDiagCapture()
}

func newDiagState(capacity int) *diagState {
	return &diagState{buf: newRing[diagRecord](capacity)}
}

func newRing[T any](capacity int) ring[T] {
	if capacity < 1 {
		capacity = 1
	}
	return ring[T]{vals: make([]T, capacity)}
}

func (r *ring[T]) push(v T) {
	if len(r.vals) == 0 {
		return
	}
	r.vals[r.next] = v
	r.next = (r.next + 1) % len(r.vals)
	if r.n < len(r.vals) {
		r.n++
	}
}

func (r *ring[T]) eachNewest(fn func(T) bool) {
	if len(r.vals) == 0 || r.n == 0 {
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

func installRuntimeDiagCapture() {
	diagInstallOnce.Do(func() {
		prev := append([]kruntime.ErrorHandler(nil), kruntime.ErrorHandlers...)
		hs := make([]kruntime.ErrorHandler, 0, len(prev))
		hs = append(hs, captureRuntimeErr)
		if len(prev) > 1 {
			// Keep any non-logging follow-up handlers (like throttling) and drop
			// the default stderr logger to protect the TUI frame.
			hs = append(hs, prev[1:]...)
		}
		kruntime.ErrorHandlers = hs
	})
}

func captureRuntimeErr(_ context.Context, err error, _ string, _ ...interface{}) {
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

	rtDiag.mu.Lock()
	rtDiag.buf.push(rec)
	rtDiag.mu.Unlock()
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

	if i := strings.Index(v, "failed to connect to "); i >= 0 {
		v = v[i:]
	}
	const max = 220
	if len(v) > max {
		v = v[:max-3] + "..."
	}
	return v
}

func latestPFErr(now, startedAt time.Time, maxAge time.Duration) (pfErr, bool) {
	rtDiag.mu.Lock()
	defer rtDiag.mu.Unlock()

	startCut := startedAt.Add(-diagSkew)
	var out pfErr
	var ok bool
	rtDiag.buf.eachNewest(func(rec diagRecord) bool {
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

// AnnotateRequestError appends recent k8s port-forward diagnostics to a
// transport error so the UI can surface the reason in the response pane.
func AnnotateRequestError(err error, startedAt time.Time) error {
	if err == nil {
		return nil
	}
	pf, ok := latestPFErr(time.Now(), startedAt, diagMaxAge)
	if !ok || strings.TrimSpace(pf.summary) == "" {
		return err
	}
	if strings.Contains(err.Error(), pf.summary) {
		return err
	}
	return fmt.Errorf("%w; %s", err, pf.summary)
}
