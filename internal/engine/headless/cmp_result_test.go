package headless

import (
	"net/http"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
)

func TestHeadersEqualOrder(t *testing.T) {
	a := http.Header{"X-Test": {"b", "a"}}
	b := http.Header{"X-Test": {"a", "b"}}

	if !headersEqual(a, b) {
		t.Fatalf("expected headersEqual() to ignore value ordering")
	}
}

func TestCompareSummaryHTTPDiffs(t *testing.T) {
	base := engine.CompareRow{
		Environment: "base",
		Response: &httpclient.Response{
			StatusCode: http.StatusOK,
			Headers:    http.Header{"X-Test": {"one"}},
			Body:       []byte("alpha"),
		},
	}
	row := engine.CompareRow{
		Environment: "other",
		Response: &httpclient.Response{
			StatusCode: http.StatusCreated,
			Headers:    http.Header{"X-Test": {"two"}},
			Body:       []byte("beta"),
		},
	}

	if got := compareSummary(base, row); got != "status, headers, body differ" {
		t.Fatalf("compareSummary() = %q", got)
	}
}
