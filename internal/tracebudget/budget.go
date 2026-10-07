package tracebudget

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// Completion inserted the hyphenated request names, so files carry them.
var phaseMap = map[string]nettrace.PhaseKind{
	"dns":             nettrace.PhaseDNS,
	"lookup":          nettrace.PhaseDNS,
	"name":            nettrace.PhaseDNS,
	"connect":         nettrace.PhaseConnect,
	"dial":            nettrace.PhaseConnect,
	"tls":             nettrace.PhaseTLS,
	"handshake":       nettrace.PhaseTLS,
	"headers":         nettrace.PhaseReqHdrs,
	"request_headers": nettrace.PhaseReqHdrs,
	"request-headers": nettrace.PhaseReqHdrs,
	"req_headers":     nettrace.PhaseReqHdrs,
	"header":          nettrace.PhaseReqHdrs,
	"body":            nettrace.PhaseReqBody,
	"request_body":    nettrace.PhaseReqBody,
	"request-body":    nettrace.PhaseReqBody,
	"req_body":        nettrace.PhaseReqBody,
	"ttfb":            nettrace.PhaseTTFB,
	"first_byte":      nettrace.PhaseTTFB,
	"wait":            nettrace.PhaseTTFB,
	"transfer":        nettrace.PhaseTransfer,
	"download":        nettrace.PhaseTransfer,
	"total":           nettrace.PhaseTotal,
	"overall":         nettrace.PhaseTotal,
}

func FromSpec(spec *restfile.TraceSpec) (nettrace.Budget, bool) {
	if spec == nil || !spec.Enabled {
		return nettrace.Budget{}, false
	}

	b := FromTrace(spec.Budgets)
	if HasBudget(b) {
		return b, true
	}
	return nettrace.Budget{}, false
}

func FromTrace(tb restfile.TraceBudget) nettrace.Budget {
	t := max(tb.Total, 0)
	g := max(tb.Tolerance, 0)
	b := nettrace.Budget{
		Total:     t,
		Tolerance: g,
	}

	if len(tb.Phases) == 0 {
		return b
	}

	ps := make(map[nettrace.PhaseKind]time.Duration, len(tb.Phases))
	for n, d := range tb.Phases {
		if d <= 0 {
			continue
		}
		k, ok := NormalizePhase(n)
		if !ok {
			continue
		}
		ps[k] = d
	}
	if len(ps) > 0 {
		b.Phases = ps
	}
	return b
}

func HasBudget(b nettrace.Budget) bool {
	if b.Total > 0 || b.Tolerance > 0 {
		return true
	}
	return len(b.Phases) > 0
}

// NormalizePhase reports false for a name that is not a phase or an alias of one.
func NormalizePhase(n string) (nettrace.PhaseKind, bool) {
	k, ok := phaseMap[strings.ToLower(strings.TrimSpace(n))]
	return k, ok
}
