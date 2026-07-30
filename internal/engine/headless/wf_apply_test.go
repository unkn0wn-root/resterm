package headless

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

// The loop item reaches a step as a typed RTS value. @apply used to be the one
// expression in the run that never received it, so a patch reading item.id
// failed instead of seeing the object.
func TestWorkflowForEachPassesTypedItemToApply(t *testing.T) {
	var (
		mu   sync.Mutex
		seen []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Get("X-Item"))
		mu.Unlock()
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := fmt.Sprintf(`### flow
# @workflow demo
# @for-each [{"id": "first"}, {"id": "second"}] as item
# @step Ping using=Ping

### Ping
# @name Ping
# @apply {headers: {"X-Item": item.id}}
GET %s/ping
`, srv.URL)

	doc := parser.Parse("workflow.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %v", doc.Errors)
	}
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
	for _, step := range out.Steps {
		if step.Err != nil {
			t.Fatalf("step %q failed: %v", step.Name, step.Err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"first", "second"}
	if len(seen) != len(want) {
		t.Fatalf("received %d requests %v, want %d", len(seen), seen, len(want))
	}
	for i, got := range seen {
		if got != want[i] {
			t.Fatalf("request %d X-Item = %q, want %q", i, got, want[i])
		}
	}
}
