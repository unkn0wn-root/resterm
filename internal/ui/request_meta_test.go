package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRequestMetaSummaryOmitsDescription(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Metadata: restfile.RequestMetadata{
			Description: "Do not show this in status bar",
			Tags:        []string{"alpha", "beta"},
		},
	}

	summary := requestMetaSummary(req)
	if strings.Contains(summary, "Do not show") {
		t.Fatalf("expected description to be omitted, got %q", summary)
	}
	if summary != "#alpha #beta" {
		t.Fatalf("expected tags only, got %q", summary)
	}
}

func TestRequestMetaSummaryEmptyWhenNoTags(t *testing.T) {
	req := &restfile.Request{
		Method: "POST",
		URL:    "https://example.com",
		Metadata: restfile.RequestMetadata{
			Description: "Only description present",
		},
	}

	if summary := requestMetaSummary(req); summary != "" {
		t.Fatalf("expected empty summary without tags, got %q", summary)
	}
}
