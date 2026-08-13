package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestStatusPulseBaseUsesWarnText(t *testing.T) {
	m := Model{}
	m.sending = true
	m.statusPulseOn = true
	m.statusPulseBase = "Sending"

	msg := statusMsg{text: "Request skipped", level: statusWarn}
	m.setStatusMessage(msg)

	if m.statusPulseBase != "Request skipped" {
		t.Fatalf("expected pulse base to track warn text, got %q", m.statusPulseBase)
	}
}

func TestSetActiveRequestDoesNotReplaceStatusMessage(t *testing.T) {
	m := New(Config{})
	m.statusMessage = statusMsg{text: "Existing status", level: statusWarn}

	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Metadata: restfile.RequestMetadata{
			Tags: []string{"alpha"},
		},
	}
	m.setActiveRequest(req)

	if m.statusMessage.text != "Existing status" || m.statusMessage.level != statusWarn {
		t.Fatalf("expected active request not to replace status message, got %+v", m.statusMessage)
	}
	if m.activeRequestKey == "" {
		t.Fatal("expected active request state to update")
	}
}

func TestStatusRunLabelNamesTheEndpoint(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{url: "https://api.example.com/v1/customers/42/orders", want: "orders"},
		{url: "https://api.example.com/orders?limit=10", want: "orders"},
		{url: "https://api.example.com/", want: "api.example.com"},
		{url: "https://api.example.com", want: "api.example.com"},
		{url: "grpc://localhost:50051/pkg.Svc/Method", want: "Method"},
		{url: "{{baseUrl}}/v1/orders", want: "orders"},
		{name: "Login", url: "https://api.example.com/v1/auth/token", want: "Login"},
	}

	for _, tt := range tests {
		req := &restfile.Request{
			Method:   "GET",
			URL:      tt.url,
			Metadata: restfile.RequestMetadata{Name: tt.name},
		}
		if got := statusRunLabel(nil, req); got != tt.want {
			t.Fatalf("statusRunLabel(%q, name %q) = %q, want %q", tt.url, tt.name, got, tt.want)
		}
	}
}

func TestStatusRunLabelCapsLongNames(t *testing.T) {
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Name: "Fetch every customer order for the reporting dashboard",
		},
	}

	got := statusRunLabel(nil, req)
	if len([]rune(got)) > statusRunLabelMax {
		t.Fatalf("statusRunLabel = %q, wider than %d", got, statusRunLabelMax)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected the capped label to mark the cut, got %q", got)
	}
}

func TestStatusRunTitlesKeepsBothForms(t *testing.T) {
	content := "### Orders\nGET https://api.example.com/v1/customers/42/orders\n"
	doc := parser.Parse("orders.http", []byte(content))
	m := New(Config{})

	title, short := m.statusRunTitles(doc, doc.Requests[0])
	if want := "GET https://api.example.com/v1/customers/42/orders"; title != want {
		t.Fatalf("title = %q, want %q", title, want)
	}
	if short != "orders" {
		t.Fatalf("short = %q, want %q", short, "orders")
	}
}

func TestCompareStatusLineUsesTheShortLabel(t *testing.T) {
	state := &compareState{
		label:       "Compare GET https://api.example.com/v1/customers/42/orders",
		statusLabel: "Compare orders",
	}

	if got := state.statusLine(); got != "Compare orders" {
		t.Fatalf("statusLine = %q, want the short label", got)
	}
	if !strings.Contains(state.label, "https://api.example.com") {
		t.Fatalf("expected history to keep the full label, got %q", state.label)
	}
}

func TestStatusRequestLabelPrefersName(t *testing.T) {
	content := "# @name Login\nGET https://api.example.com/login\n\n###\nGET https://api.example.com/health\n"
	doc := parser.Parse("sample.http", []byte(content))
	m := New(Config{})

	if len(doc.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(doc.Requests))
	}
	if got := m.statusRequestLabel(doc, doc.Requests[0]); got != "Login" {
		t.Fatalf("named request: want %q, got %q", "Login", got)
	}
	if got := m.statusRequestLabel(doc, doc.Requests[1]); got != "https://api.example.com/health" {
		t.Fatalf("unnamed request: want URL fallback, got %q", got)
	}
}
