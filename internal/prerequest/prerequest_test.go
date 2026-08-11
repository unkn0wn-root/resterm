package prerequest

import (
	"net/http"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestNormalize(t *testing.T) {
	body := "body"
	out := Output{
		Headers: http.Header{},
		Query:   map[string]string{},
		Body:    &body,
	}

	Normalize(&out)
	if out.Headers != nil || out.Query != nil {
		t.Fatalf("expected empty collections to be nil: %#v", out)
	}
	if out.Body == nil || *out.Body != body {
		t.Fatalf("expected body to be preserved: %#v", out.Body)
	}
}

func TestApplyRemovesDeclaredHeaders(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://example.com",
		Headers: http.Header{
			"X-Declared": {"declared"},
			"X-Replaced": {"declared"},
		},
	}

	var out Output
	out.DelHeader("X-Declared")
	out.SetHeader("X-Replaced", "script")
	out.DelHeader("X-Replaced")
	out.SetHeader("X-Replaced", "final")

	if err := Apply(req, out); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, ok := req.Headers["X-Declared"]; ok {
		t.Fatalf("expected the removed header to be gone, got %#v", got)
	}
	if got := req.Headers.Get("X-Replaced"); got != "final" {
		t.Fatalf("X-Replaced = %q, want %q", got, "final")
	}
}

func TestApplyPreservesTemplatedURL(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "{{base_url}}/anything",
	}

	err := Apply(req, Output{
		Query: map[string]string{
			"mode": "debug",
			"pre":  "1",
		},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !strings.Contains(req.URL, "{{base_url}}") {
		t.Fatalf("expected templated base_url preserved, got %q", req.URL)
	}
	if strings.Contains(req.URL, "%7B%7B") || strings.Contains(req.URL, "%7D%7D") {
		t.Fatalf("expected template braces to remain unescaped, got %q", req.URL)
	}
	if !strings.Contains(req.URL, "mode=debug") || !strings.Contains(req.URL, "pre=1") {
		t.Fatalf("expected merged query params, got %q", req.URL)
	}
}
