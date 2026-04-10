package ui

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scripts"
	"google.golang.org/grpc/codes"
)

type compareResult struct {
	Environment string
	Response    *httpclient.Response
	GRPC        *grpcclient.Response
	Stream      *scripts.StreamInfo
	Transcript  []byte
	Err         error
	Tests       []scripts.TestResult
	ScriptErr   error
	Request     *restfile.Request
	RequestText string
	Canceled    bool
	Skipped     bool
	SkipReason  string
}

func (m *Model) resetCompareState() {
	if m.compareSnapshots != nil {
		for k := range m.compareSnapshots {
			delete(m.compareSnapshots, k)
		}
	}
	m.compareRowIndex = 0
	m.compareSelectedEnv = ""
	m.compareFocusedEnv = ""
}

func (m *Model) setCompareSnapshot(env string, snap *responseSnapshot) {
	name := strings.TrimSpace(env)
	if name == "" || snap == nil {
		return
	}
	if m.compareSnapshots == nil {
		m.compareSnapshots = make(map[string]*responseSnapshot)
	}
	key := strings.ToLower(name)
	m.compareSnapshots[key] = snap
	if strings.TrimSpace(snap.environment) == "" {
		snap.environment = name
	}
}

func (m *Model) compareSnapshot(env string) *responseSnapshot {
	if m.compareSnapshots == nil {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(env))
	if key == "" {
		return nil
	}
	return m.compareSnapshots[key]
}

type compareBundle struct {
	Baseline string
	Rows     []compareRow
}

type compareRow struct {
	Result   *compareResult
	Status   string
	Code     string
	Duration time.Duration
	Summary  string
}

func buildCompareBundle(results []compareResult, baseline string) *compareBundle {
	if len(results) == 0 {
		return nil
	}

	baseIdx := findBaselineIndex(results, baseline)
	if baseIdx < 0 {
		baseIdx = 0
	}

	base := &results[baseIdx]
	out := &compareBundle{
		Baseline: base.Environment,
		Rows:     make([]compareRow, 0, len(results)),
	}
	for i := range results {
		res := &results[i]
		status, code := compareRowStatus(res)
		out.Rows = append(out.Rows, compareRow{
			Result:   res,
			Status:   status,
			Code:     code,
			Duration: compareRowDuration(res),
			Summary:  summarizeCompareDelta(base, res),
		})
	}
	return out
}

func findBaselineIndex(results []compareResult, baseline string) int {
	if strings.TrimSpace(baseline) == "" {
		return -1
	}
	for i := range results {
		if strings.EqualFold(results[i].Environment, baseline) {
			return i
		}
	}
	return -1
}

func compareRowStatus(result *compareResult) (string, string) {
	switch {
	case result == nil:
		return "n/a", "-"
	case result.Canceled:
		return "canceled", "-"
	case result.Skipped:
		return "skipped", "-"
	case result.Err != nil:
		return "error", ""
	case result.Response != nil:
		return result.Response.Status, fmt.Sprintf("%d", result.Response.StatusCode)
	case result.GRPC != nil:
		return result.GRPC.StatusCode.String(), fmt.Sprintf("%d", result.GRPC.StatusCode)
	default:
		return "pending", "-"
	}
}

func compareRowDuration(result *compareResult) time.Duration {
	switch {
	case result == nil:
		return 0
	case result.Response != nil:
		return result.Response.Duration
	case result.GRPC != nil:
		return result.GRPC.Duration
	default:
		return 0
	}
}

func summarizeCompareDelta(base, target *compareResult) string {
	if target == nil {
		return "unavailable"
	}
	if target.Canceled {
		return "canceled"
	}
	if base != nil && strings.EqualFold(base.Environment, target.Environment) {
		return "baseline"
	}
	if target.Skipped {
		reason := strings.TrimSpace(target.SkipReason)
		if reason == "" {
			return "skipped"
		}
		return fmt.Sprintf("skipped: %s", reason)
	}
	if base != nil && base.Skipped {
		return "baseline skipped"
	}
	if target.Err != nil {
		return fmt.Sprintf("error: %s", errdef.Message(target.Err))
	}
	if target.ScriptErr != nil {
		return fmt.Sprintf("tests error: %v", target.ScriptErr)
	}
	if fails := countTestFailures(target.Tests); fails > 0 {
		return fmt.Sprintf("%d test(s) failed", fails)
	}

	switch {
	case target.Response != nil && base != nil && base.Response != nil:
		return summarizeHTTPDelta(base.Response, target.Response)
	case target.GRPC != nil && base != nil && base.GRPC != nil:
		return summarizeGRPCDelta(base.GRPC, target.GRPC)
	default:
		return "unavailable"
	}
}

func countTestFailures(tests []scripts.TestResult) int {
	n := 0
	for _, t := range tests {
		if !t.Passed {
			n++
		}
	}
	return n
}

func summarizeHTTPDelta(base, target *httpclient.Response) string {
	if base == nil || target == nil {
		return "unavailable"
	}
	var ds []string
	if target.StatusCode != base.StatusCode {
		ds = append(ds, "status")
	}
	if !bytes.Equal(target.Body, base.Body) {
		ds = append(ds, "body")
	}
	if !headersEqual(target.Headers, base.Headers) {
		ds = append(ds, "headers")
	}
	if len(ds) == 0 {
		return "match"
	}
	return strings.Join(ds, ", ") + " differ"
}

func summarizeGRPCDelta(base, target *grpcclient.Response) string {
	if base == nil || target == nil {
		return "unavailable"
	}
	var ds []string
	if target.StatusCode != base.StatusCode {
		ds = append(ds, "status")
	}
	if strings.TrimSpace(target.StatusMessage) != strings.TrimSpace(base.StatusMessage) {
		ds = append(ds, "message")
	}
	if strings.TrimSpace(target.Message) != strings.TrimSpace(base.Message) {
		ds = append(ds, "body")
	}
	if len(ds) == 0 {
		return "match"
	}
	return strings.Join(ds, ", ") + " differ"
}

func headersEqual(a, b http.Header) bool {
	if len(a) != len(b) {
		return false
	}
	for key, vals := range a {
		other, ok := b[key]
		if !ok {
			return false
		}
		x := append([]string(nil), vals...)
		y := append([]string(nil), other...)
		sort.Strings(x)
		sort.Strings(y)
		if len(x) != len(y) {
			return false
		}
		for i := range x {
			if x[i] != y[i] {
				return false
			}
		}
	}
	return true
}

func compareResultSuccess(result *compareResult) bool {
	if result == nil || result.Canceled || result.Skipped || result.Err != nil || result.ScriptErr != nil {
		return false
	}
	if countTestFailures(result.Tests) > 0 {
		return false
	}
	switch {
	case result.Response != nil:
		return result.Response.StatusCode < 400
	case result.GRPC != nil:
		return result.GRPC.StatusCode == codes.OK
	default:
		return false
	}
}
