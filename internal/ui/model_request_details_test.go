package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestOpenRequestDetailsCapturesFields(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com/users/1",
		LineRange: restfile.LineRange{
			Start: 7,
		},
		Metadata: restfile.RequestMetadata{
			Name:        "Get user",
			Description: "Fetch a user profile",
			Tags:        []string{"users", "beta"},
			Auth:        &restfile.AuthSpec{Type: "bearer"},
			Scripts:     []restfile.ScriptBlock{{Kind: "test"}},
			Compare:     &restfile.CompareSpec{},
			Profile:     &restfile.ProfileSpec{Count: 3},
			Trace:       &restfile.TraceSpec{Enabled: true},
			NoLog:       true,
		},
	}

	model.doc = &restfile.Document{Requests: []*restfile.Request{req}}
	model.currentRequest = req
	model.currentFile = "/tmp/demo.http"
	model.workspaceRoot = "/tmp"
	_ = model.setFocus(focusRequests)

	model.openRequestDetails()

	if !model.showRequestDetails {
		t.Fatalf("expected details modal to open")
	}
	if model.requestDetailTitle == "" {
		t.Fatalf("expected detail title to be set")
	}
	if model.requestDetailTitle != "Get user" {
		t.Fatalf("expected request name only in title, got %q", model.requestDetailTitle)
	}

	body := ansi.Strip(renderDetailFields(model.requestDetailFields, 80, theme.DefaultTheme()))
	expect := []string{
		"Get user",
		"https://example.com/users/1",
		"#users",
		"#beta",
		"demo.http:7",
		"Auth:BEARER",
	}
	for _, want := range expect {
		if !strings.Contains(body, want) {
			t.Fatalf("expected details body to include %q, got %q", want, body)
		}
	}
}

func TestOpenRequestDetailsRequiresNavigatorFocus(t *testing.T) {
	model := New(Config{})
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
	}
	model.doc = &restfile.Document{Requests: []*restfile.Request{req}}
	model.currentRequest = req
	_ = model.setFocus(focusEditor)

	model.openRequestDetails()

	if model.showRequestDetails {
		t.Fatalf("expected details modal to remain closed outside navigator focus")
	}
}
