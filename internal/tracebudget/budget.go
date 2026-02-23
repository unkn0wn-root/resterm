package tracebudget

import (
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

var phaseMap = map[string]string{
	"dns":             string(nettrace.PhaseDNS),
	"lookup":          string(nettrace.PhaseDNS),
	"name":            string(nettrace.PhaseDNS),
	"connect":         string(nettrace.PhaseConnect),
	"dial":            string(nettrace.PhaseConnect),
	"tls":             string(nettrace.PhaseTLS),
	"handshake":       string(nettrace.PhaseTLS),
	"headers":         string(nettrace.PhaseReqHdrs),
	"request_headers": string(nettrace.PhaseReqHdrs),
	"req_headers":     string(nettrace.PhaseReqHdrs),
	"header":          string(nettrace.PhaseReqHdrs),
	"body":            string(nettrace.PhaseReqBody),
	"request_body":    string(nettrace.PhaseReqBody),
	"req_body":        string(nettrace.PhaseReqBody),
	"ttfb":            string(nettrace.PhaseTTFB),
	"first_byte":      string(nettrace.PhaseTTFB),
	"wait":            string(nettrace.PhaseTTFB),
	"transfer":        string(nettrace.PhaseTransfer),
	"download":        string(nettrace.PhaseTransfer),
	"total":           string(nettrace.PhaseTotal),
	"overall":         string(nettrace.PhaseTotal),
}

const TotalPhase = string(nettrace.PhaseTotal)

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
	t := tb.Total
	if t < 0 {
		t = 0
	}
	g := tb.Tolerance
	if g < 0 {
		g = 0
	}
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
		k := NormalizePhase(n)
		if k == "" {
			continue
		}
		ps[nettrace.PhaseKind(k)] = d
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

func NormalizePhase(n string) string {
	n = strings.ToLower(strings.TrimSpace(n))
	if n == "" {
		return ""
	}
	if c, ok := phaseMap[n]; ok {
		return c
	}
	return n
}
