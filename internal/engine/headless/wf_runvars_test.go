package headless

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type pathLog struct {
	mu    sync.Mutex
	paths []string
}

func (l *pathLog) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		l.paths = append(l.paths, r.URL.Path)
		l.mu.Unlock()
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (l *pathLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.paths
	l.paths = nil
	return out
}

func parseRunVarDoc(t *testing.T, src string) *restfile.Document {
	t.Helper()
	doc := parser.Parse("run.http", []byte(src))
	if len(doc.Errors) != 0 {
		t.Fatalf("parse errors: %v", doc.Errors)
	}
	return doc
}

func TestWorkflowRunVarsHoldForOneRun(t *testing.T) {
	var log pathLog
	srv := log.server(t)
	doc := parseRunVarDoc(t, fmt.Sprintf(`# @workflow order
# @run var suffix = {{$fake.word}}-{{$randomString(12)}}
# @step Create using=Create
# @step Fetch using=Fetch
# @step CreateAgain using=Create

### Create
# @name Create
# @run var id = {{$uuid}}
POST %[1]s/create/{{suffix}}/{{id}}

### Fetch
# @name Fetch
GET %[1]s/fetch/{{suffix}}
`, srv.URL))
	eng := New(engine.Config{})

	run := func() []string {
		out, err := eng.ExecuteWorkflow(doc, &doc.Workflows[0], testSelection(""))
		if err != nil {
			t.Fatalf("ExecuteWorkflow: %v", err)
		}
		if !out.Success {
			t.Fatalf("workflow failed: %s", out.Summary)
		}
		return log.take()
	}

	first := run()
	if len(first) != 3 {
		t.Fatalf("sent %v, want 3 requests", first)
	}
	create := strings.Split(first[0], "/")
	suffix, id := create[2], create[3]
	if first[1] != "/fetch/"+suffix {
		t.Fatalf("Fetch sent %q, want the suffix %q from Create", first[1], suffix)
	}
	if first[2] != first[0] {
		t.Fatalf("second Create sent %q, want %q again", first[2], first[0])
	}

	second := run()
	next := strings.Split(second[0], "/")
	if next[2] == suffix || next[3] == id {
		t.Fatalf("second run sent %q, want fresh values after %q", second[0], first[0])
	}
}

func TestForEachRunVarsHoldAcrossIterations(t *testing.T) {
	var log pathLog
	srv := log.server(t)
	doc := parseRunVarDoc(t, fmt.Sprintf(`### Each
# @name Each
# @for-each ["a", "b", "c"] as item
# @run var batch = {{$uuid}}
GET %s/{{batch}}/{{vars.request.item}}
`, srv.URL))

	res, err := New(engine.Config{}).ExecuteRequest(doc, doc.Requests[0], testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if res.Workflow == nil || !res.Workflow.Success {
		t.Fatalf("for-each did not succeed: %+v", res.Workflow)
	}
	paths := log.take()
	if len(paths) != 3 {
		t.Fatalf("sent %v, want 3 requests", paths)
	}
	batch := strings.Split(paths[0], "/")[1]
	for i, item := range []string{"a", "b", "c"} {
		if want := "/" + batch + "/" + item; paths[i] != want {
			t.Fatalf("iteration %d sent %q, want %q", i, paths[i], want)
		}
	}
}

func TestWorkflowRunVarFailureFailsTheRun(t *testing.T) {
	var log pathLog
	srv := log.server(t)
	doc := parseRunVarDoc(t, fmt.Sprintf(`# @workflow broken
# @run var suffix = {{$randomInt(x)}}
# @step Create using=Create
# @step Fetch using=Fetch

### Create
# @name Create
POST %[1]s/create/{{suffix}}

### Fetch
# @name Fetch
GET %[1]s/fetch
`, srv.URL))

	out, err := New(engine.Config{}).ExecuteWorkflow(doc, &doc.Workflows[0], testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}
	if sent := log.take(); len(sent) != 0 {
		t.Fatalf("sent %v, want nothing", sent)
	}
	if out.Success {
		t.Fatalf("workflow succeeded with a broken @run var: %s", out.Summary)
	}
	if len(out.Steps) != 1 || out.Steps[0].Err == nil {
		t.Fatalf("steps = %+v, want the first step failed and the run stopped", out.Steps)
	}
	if !strings.Contains(out.Summary, "@run var suffix: ") {
		t.Fatalf("summary = %q, want the @run var error", out.Summary)
	}
}

func TestRequestRunVarDerivesFromWorkflowValue(t *testing.T) {
	var log pathLog
	srv := log.server(t)
	doc := parseRunVarDoc(t, fmt.Sprintf(`# @workflow order
# @run var suffix = base
# @step Create using=Create
# @step Fetch using=Fetch

### Create
# @name Create
# @run var suffix = {{suffix}}-x
POST %[1]s/create/{{suffix}}

### Fetch
# @name Fetch
GET %[1]s/fetch/{{suffix}}
`, srv.URL))

	out, err := New(engine.Config{}).ExecuteWorkflow(doc, &doc.Workflows[0], testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}
	if !out.Success {
		t.Fatalf("workflow failed: %s", out.Summary)
	}
	sent := log.take()
	if want := []string{"/create/base-x", "/fetch/base"}; strings.Join(sent, " ") != strings.Join(want, " ") {
		t.Fatalf("sent %v, want %v", sent, want)
	}
}
