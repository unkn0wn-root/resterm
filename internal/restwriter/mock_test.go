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
				Query: map[string]restfile.StringList{"mode": {"test"}},
				Headers: map[string]restfile.MockHeaderRule{
					"X-Tenant": {Op: restfile.MockHeaderOpExact, Values: []string{"acme"}},
				},
				JSON: []byte(`{"amount":100}`),
			},
			Responses: []restfile.MockResponse{{
				Status:  http.StatusAccepted,
				Headers: http.Header{"Content-Type": {"application/json"}},
				Body:    restfile.BodySource{Text: "{\"id\":\"pay_123\"}\n"},
			}},
		},
		{
			Title:  "Fixture",
			Method: http.MethodGet,
			Path:   "/fixture",
			Responses: []restfile.MockResponse{{
				Status: http.StatusOK,
				Body:   restfile.BodySource{FilePath: "./fixtures/data.json"},
			}},
		},
	}}

	rendered := mustRender(t, doc)
	parsed := parser.Parse("generated.http", []byte(rendered))
	if len(parsed.Errors) != 0 || len(parsed.Mocks) != 2 {
		t.Fatalf("round-trip errors=%+v mocks=%d\n%s", parsed.Errors, len(parsed.Mocks), rendered)
	}
	first := parsed.Mocks[0]
	if first.Name != "accepted" || first.Responses[0].Status != http.StatusAccepted ||
		string(first.Match.JSON) != `{"amount":100}` || first.Responses[0].Body.Text != "{\"id\":\"pay_123\"}\n" {
		t.Fatalf("round-trip mock = %+v", first)
	}
	if got := parsed.Mocks[1].Responses[0].Body.FilePath; got != "./fixtures/data.json" {
		t.Fatalf("fixture path = %q", got)
	}
}

func TestRenderRejectsUnsafeMockBodies(t *testing.T) {
	mock := &restfile.Mock{
		Responses: []restfile.MockResponse{{
			Status: http.StatusOK,
			Body:   restfile.BodySource{Text: "a\n### b"},
		}},
	}
	_, err := Render(&restfile.Document{Mocks: []*restfile.Mock{mock}}, Options{})
	if err == nil || strings.TrimSpace(err.Error()) == "" {
		t.Fatalf("Render() error = %v", err)
	}
}

func TestRenderMockSequenceRoundTrip(t *testing.T) {
	doc := &restfile.Document{Mocks: []*restfile.Mock{{
		Title:                "Polling",
		Sequence:             "polling",
		SequenceKey:          restfile.MockSequenceKey{Source: restfile.MockSequenceKeySourcePath, Name: "id"},
		Method:               http.MethodGet,
		Path:                 "/payments/{id}",
		DisableInterpolation: true,
		Expectation:          &restfile.MockExpectation{Calls: 2},
		Match: restfile.MockMatch{Headers: map[string]restfile.MockHeaderRule{
			"Authorization": {
				Op: restfile.MockHeaderOpPrefix, Values: []string{"Bearer "},
			},
		}},
		Responses: []restfile.MockResponse{
			{
				Status:  http.StatusServiceUnavailable,
				Headers: http.Header{"Retry-After": {"1"}},
				Body:    restfile.BodySource{Text: "pending"},
			},
			{
				Status: http.StatusOK,
				Body:   restfile.BodySource{Text: `{"status":"{{literal}}"}`},
			},
		},
	}}}

	rendered := mustRender(t, doc)
	if !strings.Contains(rendered, "sequence=polling sequence-key=path.id interpolate=false") ||
		!strings.Contains(rendered, "# @expect calls=2") ||
		!strings.Contains(rendered, `headers={"Authorization":{"prefix":"Bearer "}}`) ||
		!strings.Contains(rendered, "\n---\nHTTP/1.1 200 OK") {
		t.Fatalf("rendered sequence:\n%s", rendered)
	}
	parsed := parser.Parse("generated.http", []byte(rendered))
	if len(parsed.Errors) != 0 || len(parsed.Mocks) != 1 {
		t.Fatalf("round-trip errors=%+v mocks=%d\n%s", parsed.Errors, len(parsed.Mocks), rendered)
	}
	m := parsed.Mocks[0]
	if m.Sequence != "polling" || m.SequenceKey.String() != "path.id" ||
		m.Expectation == nil || m.Expectation.Calls != 2 ||
		m.Match.Headers["Authorization"].Op != restfile.MockHeaderOpPrefix ||
		!m.DisableInterpolation || len(m.Responses) != 2 ||
		m.Responses[1].Body.Text != `{"status":"{{literal}}"}` {
		t.Fatalf("round-trip mock = %+v", m)
	}
}

func TestRenderRejectsMockSequenceDelimiterInBody(t *testing.T) {
	mock := &restfile.Mock{
		Sequence: "polling",
		Responses: []restfile.MockResponse{
			{Status: http.StatusOK, Body: restfile.BodySource{Text: "before\n---\nafter"}},
			{Status: http.StatusOK},
		},
	}
	_, err := Render(&restfile.Document{Mocks: []*restfile.Mock{mock}}, Options{})
	if err == nil || !strings.Contains(err.Error(), "response delimiter") {
		t.Fatalf("Render() error = %v", err)
	}
}
