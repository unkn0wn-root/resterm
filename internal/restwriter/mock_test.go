package restwriter

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRenderMocksRoundTrip(t *testing.T) {
	doc := &restfile.Document{Mocks: []*restfile.Mock{
		{
			Title:   "Payment accepted",
			Name:    "accepted",
			Method:  http.MethodPost,
			Path:    "/payments",
			Default: true,
			Latency: 250 * time.Millisecond,
			Match: restfile.MockMatch{
				Query:   map[string][]string{"mode": {"test"}},
				Headers: map[string][]string{"X-Tenant": {"acme"}},
				JSON:    []byte(`{"amount":100}`),
			},
			Response: restfile.MockResponse{
				Status:  http.StatusAccepted,
				Headers: http.Header{"Content-Type": {"application/json"}},
				Body:    restfile.BodySource{Text: "{\"id\":\"pay_123\"}\n"},
			},
		},
		{
			Title:  "Fixture",
			Method: http.MethodGet,
			Path:   "/fixture",
			Response: restfile.MockResponse{
				Status: http.StatusOK,
				Body:   restfile.BodySource{FilePath: "./fixtures/data.json"},
			},
		},
	}}

	rendered := mustRender(t, doc)
	parsed := parser.Parse("generated.http", []byte(rendered))
	if len(parsed.Errors) != 0 || len(parsed.Mocks) != 2 {
		t.Fatalf("round-trip errors=%+v mocks=%d\n%s", parsed.Errors, len(parsed.Mocks), rendered)
	}
	first := parsed.Mocks[0]
	if first.Name != "accepted" || first.Response.Status != http.StatusAccepted ||
		string(first.Match.JSON) != `{"amount":100}` || first.Response.Body.Text != "{\"id\":\"pay_123\"}\n" {
		t.Fatalf("round-trip mock = %+v", first)
	}
	if got := parsed.Mocks[1].Response.Body.FilePath; got != "./fixtures/data.json" {
		t.Fatalf("fixture path = %q", got)
	}
}

func TestRenderRejectsUnsafeMockBodies(t *testing.T) {
	mock := &restfile.Mock{
		Response: restfile.MockResponse{
			Status: http.StatusOK,
			Body:   restfile.BodySource{Text: "a\n### b"},
		},
	}
	_, err := Render(&restfile.Document{Mocks: []*restfile.Mock{mock}}, Options{})
	if err == nil || strings.TrimSpace(err.Error()) == "" {
		t.Fatalf("Render() error = %v", err)
	}
}
