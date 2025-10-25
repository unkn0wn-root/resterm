package traceutil

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func BudgetFromSpec(spec *restfile.TraceSpec) (nettrace.Budget, bool) {
	if spec == nil || !spec.Enabled {
		return nettrace.Budget{}, false
	}

	budget := BudgetFromTraceBudget(spec.Budgets)
	if HasBudget(budget) {
		return budget, true
	}
	return nettrace.Budget{}, false
}

func BudgetFromTraceBudget(tb restfile.TraceBudget) nettrace.Budget {
	total := tb.Total
	if total < 0 {
		total = 0
	}
	tolerance := tb.Tolerance
	if tolerance < 0 {
		tolerance = 0
	}
	budget := nettrace.Budget{
		Total:     total,
		Tolerance: tolerance,
	}

	if len(tb.Phases) == 0 {
		return budget
	}

	phases := make(map[nettrace.PhaseKind]time.Duration, len(tb.Phases))
	for name, dur := range tb.Phases {
		if dur <= 0 {
			continue
		}
		kind := normalizePhaseName(name)
		phases[kind] = dur
	}
	if len(phases) > 0 {
		budget.Phases = phases
	}
	return budget
}

func HasBudget(b nettrace.Budget) bool {
	if b.Total > 0 || b.Tolerance > 0 {
		return true
	}
	return len(b.Phases) > 0
}

func normalizePhaseName(name string) nettrace.PhaseKind {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dns", "lookup", "name":
		return nettrace.PhaseDNS
	case "connect", "dial":
		return nettrace.PhaseConnect
	case "tls", "handshake":
		return nettrace.PhaseTLS
	case "headers", "request_headers", "req_headers", "header":
		return nettrace.PhaseReqHdrs
	case "body", "request_body", "req_body":
		return nettrace.PhaseReqBody
	case "ttfb", "first_byte", "wait":
		return nettrace.PhaseTTFB
	case "transfer", "download":
		return nettrace.PhaseTransfer
	case "total", "overall":
		return nettrace.PhaseTotal
	default:
		return nettrace.PhaseKind(strings.ToLower(strings.TrimSpace(name)))
	}
}
