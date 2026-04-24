package headless

import (
	"bytes"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"google.golang.org/grpc/codes"
)

func compareSuccess(row engine.CompareRow) bool {
	if row.Canceled || row.Skipped || row.Err != nil || row.ScriptErr != nil {
		return false
	}
	for _, test := range row.Tests {
		if !test.Passed {
			return false
		}
	}

	switch {
	case row.Response != nil:
		return row.Response.StatusCode < 400
	case row.GRPC != nil:
		return row.GRPC.StatusCode == codes.OK
	default:
		return false
	}
}

func compareStatus(row engine.CompareRow) (string, string) {
	switch {
	case row.Canceled:
		return "canceled", "-"
	case row.Skipped:
		return "skipped", "-"
	case row.Err != nil:
		return "error", ""
	case row.Response != nil:
		return row.Response.Status, fmt.Sprintf("%d", row.Response.StatusCode)
	case row.GRPC != nil:
		return row.GRPC.StatusCode.String(), fmt.Sprintf("%d", row.GRPC.StatusCode)
	default:
		return "pending", "-"
	}
}

func compareSummary(base, row engine.CompareRow) string {
	if row.Canceled {
		return "canceled"
	}
	if row.Skipped {
		if reason := strings.TrimSpace(row.SkipReason); reason != "" {
			return "skipped: " + reason
		}
		return "skipped"
	}
	if row.Err != nil {
		return "error: " + row.Err.Error()
	}
	if row.ScriptErr != nil {
		return "tests error: " + row.ScriptErr.Error()
	}

	failedTests := 0
	for _, test := range row.Tests {
		if !test.Passed {
			failedTests++
		}
	}
	if failedTests > 0 {
		return fmt.Sprintf("%d test(s) failed", failedTests)
	}
	if strings.EqualFold(base.Environment, row.Environment) {
		return "baseline"
	}

	switch {
	case row.Response != nil && base.Response != nil:
		return summarizeHTTP(base.Response, row.Response)
	case row.GRPC != nil && base.GRPC != nil:
		return summarizeGRPC(base.GRPC, row.GRPC)
	default:
		return "unavailable"
	}
}

func summarizeHTTP(base, row *httpclient.Response) string {
	if base == nil || row == nil {
		return "unavailable"
	}

	var diff []string
	if row.StatusCode != base.StatusCode {
		diff = append(diff, "status")
	}
	if !headersEqual(row.Headers, base.Headers) {
		diff = append(diff, "headers")
	}
	if !bytes.Equal(row.Body, base.Body) {
		diff = append(diff, "body")
	}
	if len(diff) == 0 {
		return "match"
	}
	return strings.Join(diff, ", ") + " differ"
}

func summarizeGRPC(base, row *grpcclient.Response) string {
	if base == nil || row == nil {
		return "unavailable"
	}

	var diff []string
	if row.StatusCode != base.StatusCode {
		diff = append(diff, "status")
	}
	if strings.TrimSpace(row.StatusMessage) != strings.TrimSpace(base.StatusMessage) {
		diff = append(diff, "message")
	}
	if strings.TrimSpace(row.Message) != strings.TrimSpace(base.Message) {
		diff = append(diff, "body")
	}
	if len(diff) == 0 {
		return "match"
	}
	return strings.Join(diff, ", ") + " differ"
}

func headersEqual(a, b http.Header) bool {
	if len(a) != len(b) {
		return false
	}
	for name, values := range a {
		other, ok := b[name]
		if !ok || !headerValuesEqual(values, other) {
			return false
		}
	}
	return true
}

func headerValuesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	left := slices.Clone(a)
	right := slices.Clone(b)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}
