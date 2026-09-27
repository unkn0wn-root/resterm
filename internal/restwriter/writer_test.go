package restwriter

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRenderSettings(t *testing.T) {
	doc := &restfile.Document{
		Settings: map[string]string{
			"timeout": "2s",
		},
		Requests: []*restfile.Request{{
			Method: "GET",
			URL:    "https://example.com",
			Settings: map[string]string{
				"proxy": "http://proxy",
			},
		}},
	}

	out := mustRender(t, doc)
	if !strings.Contains(out, "# @setting timeout 2s") {
		t.Fatalf("expected file setting in output: %q", out)
	}
	if !strings.Contains(out, "# @setting proxy http://proxy") {
		t.Fatalf("expected request setting in output: %q", out)
	}
}

func TestRenderCommandAuth(t *testing.T) {
	doc := &restfile.Document{
		Requests: []*restfile.Request{{
			Method: "GET",
			URL:    "https://example.com",
			Metadata: restfile.RequestMetadata{
				Auth: &restfile.AuthSpec{Type: "command", Params: map[string]string{
					"argv":      `["gh","auth","token"]`,
					"cache_key": "github",
					"timeout":   "5s",
				}},
			},
		}},
	}

	out := mustRender(t, doc)
	if !strings.Contains(
		out,
		`# @auth command argv=["gh","auth","token"] cache_key=github timeout=5s`,
	) {
		t.Fatalf("expected command auth in output: %q", out)
	}
}

func TestRenderBodyOptions(t *testing.T) {
	doc := &restfile.Document{
		Requests: []*restfile.Request{{
			Method: "POST",
			URL:    "https://example.com",
			Body: restfile.BodySource{
				Text: "< this is just a string",
				Options: restfile.BodyOptions{
					ExpandTemplates: true,
					ForceInline:     true,
				},
			},
		}},
	}

	out := mustRender(t, doc)
	if !strings.Contains(out, "# @body expand") {
		t.Fatalf("expected body expand directive in output: %q", out)
	}
	if !strings.Contains(out, "# @body inline") {
		t.Fatalf("expected body inline directive in output: %q", out)
	}
}

func TestRenderAmbiguousInlineBodyAddsDirective(t *testing.T) {
	doc := &restfile.Document{
		Requests: []*restfile.Request{{
			Method: "POST",
			URL:    "https://example.com",
			Body: restfile.BodySource{
				Text: "< ./not-a-file-reference",
			},
		}},
	}

	out := mustRender(t, doc)
	if !strings.Contains(out, "# @body inline") {
		t.Fatalf("expected body inline directive in output: %q", out)
	}
}

func TestFormatAuthParam(t *testing.T) {
	tests := []struct {
		key, val, want string
	}{
		{"argv", `["tool","arg with space"]`, `argv='["tool","arg with space"]'`},
		{"cmd", `mycli --name 'a b'`, `cmd="mycli --name 'a b'"`},
		{"scheme", `"quoted"`, `scheme='"quoted"'`},
		{"header", "X-Token", "header=X-Token"},
	}
	for _, tt := range tests {
		got, err := formatAuthParam(tt.key, tt.val)
		if err != nil || got != tt.want {
			t.Fatalf("formatAuthParam(%q, %q) = %q, %v, want %q", tt.key, tt.val, got, err, tt.want)
		}
	}
}

func mustRender(t *testing.T, doc *restfile.Document) string {
	t.Helper()
	out, err := Render(doc, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
