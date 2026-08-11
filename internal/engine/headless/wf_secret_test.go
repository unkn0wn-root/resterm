package headless

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

func TestExecuteWorkflowMasksSecretInFailureText(t *testing.T) {
	const secret = "workflow-error-secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := fmt.Sprintf(`### flow
# @workflow smoke
# @step Probe using=Probe

### Probe
# @name Probe
GET %s/probe
> {%%
vars.global.set("token", "%s", true);
throw new Error(vars.global.get("token"));
%%}
`, srv.URL, secret)

	doc := parser.Parse("workflow.http", []byte(src))
	if len(doc.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(doc.Workflows))
	}

	out, err := New(engine.Config{}).ExecuteWorkflow(doc, &doc.Workflows[0], testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}
	if len(out.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(out.Steps))
	}

	step := out.Steps[0]
	texts := map[string]string{
		"summary":      out.Summary,
		"report":       out.Report,
		"step summary": step.Summary,
		"step error":   errText(step.Err),
		"script error": errText(step.ScriptErr),
	}
	for _, tc := range step.Tests {
		texts["test "+tc.Name] = tc.Name + " " + tc.Message
	}
	for field, text := range texts {
		if strings.Contains(text, secret) {
			t.Fatalf("workflow %s contains the plaintext secret: %s", field, text)
		}
	}
	if !strings.Contains(out.Report, "•••") {
		t.Fatalf("expected the failure to still be reported, got:\n%s", out.Report)
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
