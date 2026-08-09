package mock

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsJSONMediaType(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "json", value: "application/json", want: true},
		{name: "json parameters", value: "application/json; charset=utf-8", want: true},
		{name: "json suffix", value: "application/problem+json", want: true},
		{name: "non-json", value: "text/plain"},
		{name: "invalid", value: "not a media type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isJSONMediaType(tt.value); got != tt.want {
				t.Fatalf("isJSONMediaType(%q) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}

func TestMatchJSONNumberHugeExponentDoesNotBlowUp(t *testing.T) {
	handler := compileSource(t, `# @mock method=POST path=/n
# @match json={"n":1}
HTTP/1.1 200 OK

matched

### rules
# @mock method=POST path=/r
# @match json-rules={"n":{"gt":1}}
HTTP/1.1 200 OK

matched

### query
# @mock method=GET path=/q
# @match query={"n":{"gte":1}}
HTTP/1.1 200 OK

matched`)
	huge := "1e" + strings.Repeat("9", 1<<20)

	t.Run("a literal 1 is not that number", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/n", strings.NewReader(`{"n":`+huge+`}`))
		req.Header.Set("Content-Type", "application/json")
		assertResponse(t, handler, req, http.StatusNotFound, "no mock scenario matched")
	})
	t.Run("json rules read it as huge", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/r", strings.NewReader(`{"n":`+huge+`}`))
		req.Header.Set("Content-Type", "application/json")
		assertResponse(t, handler, req, http.StatusOK, "matched")
	})
	t.Run("query rules read it as huge", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/q?n="+huge, nil)
		assertResponse(t, handler, req, http.StatusOK, "matched")
	})
}

func TestMatchReadsTheBodyOncePerRequest(t *testing.T) {
	tests := []struct {
		name        string
		option      string
		contentType string
		body        string
		status      int
		contains    string
	}{
		{
			name: "literal subset matches", option: `json={"n":1}`,
			contentType: "application/json", body: `{"n":1,"extra":true}`,
			status: http.StatusOK, contains: "matched",
		},
		{
			name: "rules match a +json media type", option: `json-rules={"n":{"gt":0}}`,
			contentType: "application/vnd.acme+json", body: `{"n":1}`,
			status: http.StatusOK, contains: "matched",
		},
		{
			name: "a non-JSON media type never matches", option: `json-rules={"n":{"gt":0}}`,
			contentType: "text/plain", body: `{"n":1}`,
			status: http.StatusNotFound, contains: "no mock scenario matched",
		},
		{
			name: "a malformed body is a bad request", option: `json-rules={"n":{"gt":0}}`,
			contentType: "application/json", body: `{"n":`,
			status: http.StatusBadRequest, contains: "invalid JSON request body",
		},
		{
			name: "trailing JSON is a bad request", option: `json={"n":1}`,
			contentType: "application/json", body: `{"n":1}{"n":2}`,
			status: http.StatusBadRequest, contains: "invalid JSON request body",
		},
		{
			name: "an oversized body is rejected before decoding", option: `json-rules={"n":{"gt":0}}`,
			contentType: "application/json",
			body:        `{"n":1,"pad":"` + strings.Repeat("x", maxMockRequestBody) + `"}`,
			status:      http.StatusRequestEntityTooLarge, contains: "4 MiB limit",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := compileSource(t, `# @mock method=POST path=/n
# @match `+test.option+`
HTTP/1.1 200 OK

matched`)
			req := httptest.NewRequest(http.MethodPost, "/n", strings.NewReader(test.body))
			req.Header.Set("Content-Type", test.contentType)
			assertResponse(t, handler, req, test.status, test.contains)
		})
	}
}
