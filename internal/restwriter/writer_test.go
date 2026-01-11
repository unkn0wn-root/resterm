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

	out := Render(doc, Options{})
	if !strings.Contains(out, "# @setting timeout 2s") {
		t.Fatalf("expected file setting in output: %q", out)
	}
	if !strings.Contains(out, "# @setting proxy http://proxy") {
		t.Fatalf("expected request setting in output: %q", out)
	}
}
