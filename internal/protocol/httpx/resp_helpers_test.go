package httpx

import (
	"maps"
	"net/http"
	"slices"
	"testing"
)

func TestSentHeadersAddWhatRequestFieldsCarry(t *testing.T) {
	resp := &Response{
		RequestHeaders: http.Header{"Accept": {"*/*"}},
		ReqHost:        "example.com",
		ReqLen:         42,
		ReqTE:          []string{"chunked"},
	}
	want := http.Header{
		"Accept":            {"*/*"},
		"Host":              {"example.com"},
		"Content-Length":    {"42"},
		"Transfer-Encoding": {"chunked"},
	}
	if got := resp.SentHeaders(); !maps.EqualFunc(got, want, slices.Equal) {
		t.Fatalf("SentHeaders() = %v, want %v", got, want)
	}
	if len(resp.RequestHeaders) != 1 {
		t.Fatalf("SentHeaders changed RequestHeaders to %v", resp.RequestHeaders)
	}
}
