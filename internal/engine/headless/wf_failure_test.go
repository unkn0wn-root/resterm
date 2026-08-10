package headless

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

// An @if that fails on purpose used to end the run regardless of on-failure, so
// this goes through the parser to prove the mode reaches the step.
func TestExecuteWorkflowContinuesPastFailedIfBranch(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := fmt.Sprintf(`### flow
# @workflow demo on-failure=continue
# @if true fail="explicit failure"
# @step Cleanup using=Cleanup

### Cleanup
# @name Cleanup
GET %s/cleanup
`, srv.URL)

	doc := parser.Parse("workflow.http", []byte(src))
	if len(doc.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(doc.Workflows))
	}

	out, err := New(engine.Config{}).ExecuteWorkflow(
		doc,
		&doc.Workflows[0],
		testSelection(""),
	)
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}
	if len(out.Steps) != 2 {
		t.Fatalf("expected both steps to report, got %d", len(out.Steps))
	}
	if got := out.Steps[0].Err; got == nil || got.Error() != "explicit failure" {
		t.Fatalf("first step error = %v, want the whole fail message", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("cleanup request ran %d times, want 1", hits.Load())
	}
	if out.Success {
		t.Fatal("a continued failure still has to mark the run unsuccessful")
	}
}
