package prerequest

import (
	"net/http"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestNormalize(t *testing.T) {
	body := "body"
	out := Output{
		Headers:   http.Header{},
		Query:     map[string]string{},
		Body:      &body,
		Variables: map[string]string{},
		Globals:   map[string]vars.GlobalMutation{},
	}

	Normalize(&out)
	if out.Headers != nil || out.Query != nil || out.Variables != nil || out.Globals != nil {
		t.Fatalf("expected empty collections to be nil: %#v", out)
	}
	if out.Body == nil || *out.Body != body {
		t.Fatalf("expected body to be preserved: %#v", out.Body)
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
