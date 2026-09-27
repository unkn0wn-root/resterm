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
	if got, err := formatAuthParam("scope", "read write"); err != nil || got != "scope='read write'" {
		t.Fatalf("formatAuthParam() = %q, %v", got, err)
	}
	_, err := formatAuthParam("scope", `say "hi" it's`)
	if err == nil || !strings.Contains(err.Error(), "writer: @auth scope") {
		t.Fatalf("error = %v, want one naming the @auth option", err)
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
