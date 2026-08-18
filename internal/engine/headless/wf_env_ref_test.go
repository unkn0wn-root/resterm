package headless

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestWorkflowBranchResolvesEnvRefs(t *testing.T) {
	t.Setenv("RESTERM_WF_REF_TARGET", "wf-value")

	for _, tt := range []struct {
		name  string
		alias string
	}{
		{name: "direct name", alias: "direct"},
		{name: "templated name", alias: "alias"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer srv.Close()

			src := fmt.Sprintf(`# @file picked RESTERM_WF_REF_TARGET

### flow
# @workflow probe
# @step First using=First
# @if env.get(%q) == "wf-value" run=Match
# @else run=Miss

### First
# @name First
GET %s/first

### Match
# @name Match
GET %s/match

### Miss
# @name Miss
GET %s/miss
`, tt.alias, srv.URL, srv.URL, srv.URL)

			doc := parser.Parse("wf.http", []byte(src))
			if len(doc.Errors) != 0 {
				t.Fatalf("parse errors: %#v", doc.Errors)
			}
			if len(doc.Workflows) != 1 {
				t.Fatalf("workflows = %d, want 1", len(doc.Workflows))
			}

			cat, err := vars.NewCatalog(vars.EnvironmentSet{"dev": {
				"direct": "env:RESTERM_WF_REF_TARGET",
				"alias":  "env:{{picked}}",
			}})
			if err != nil {
				t.Fatalf("NewCatalog() error = %v", err)
			}
			sel, err := cat.Select("dev", nil)
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}

			out, err := New(engine.Config{Catalog: cat}).ExecuteWorkflow(doc, &doc.Workflows[0], sel)
			if err != nil {
				t.Fatalf("ExecuteWorkflow() error = %v", err)
			}
			if len(out.Steps) != 2 {
				t.Fatalf("steps = %d, want 2", len(out.Steps))
			}
			if got := out.Steps[1].Name; got != "@if -> Match" {
				t.Fatalf("branch = %q, want the reference to resolve in the condition", got)
			}
		})
	}
}
