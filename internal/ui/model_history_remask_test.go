package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
	"github.com/unkn0wn-root/resterm/internal/history"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	remaskSecret  = "later-cap-77aa-bbcc"
	historyBudget = 16 << 20
)

func remaskServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v := r.URL.Query().Get("v")
		code, reason, body := http.StatusOK, "OK", `{"saw":"`+v+`"}`
		switch r.URL.Path {
		case "/cap":
			body = `{"token":"` + v + `"}`
		case "/seenstatus":
			code, reason, body = http.StatusForbidden, "saw "+v, ""
		}
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = fmt.Fprintf(buf, "HTTP/1.1 %d %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\n"+
			"Connection: close\r\n\r\n%s", code, reason, len(body), body)
		_ = buf.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newRemaskModel(t *testing.T, src string) (*Model, *histdb.Store) {
	t.Helper()
	store := histdb.New(filepath.Join(t.TempDir(), "history.db"))
	t.Cleanup(func() { _ = store.Close() })
	m := newOrchTestModel(t, Config{
		FilePath:       "remask.http",
		InitialContent: src,
		History:        store,
		Env:            vars.Config{Selection: testSelection("dev")},
	})
	return &m, store
}

func requestNamed(t *testing.T, m *Model, name string) *restfile.Request {
	t.Helper()
	i := slices.IndexFunc(m.doc.Requests, func(r *restfile.Request) bool { return r.Metadata.Name == name })
	if i < 0 {
		t.Fatalf("no request %q", name)
	}
	return m.doc.Requests[i]
}

func sendRequest(t *testing.T, m *Model, req *restfile.Request) {
	t.Helper()
	cmd, ok := m.startRun(runSpec{doc: m.doc, req: req, opts: m.runOptions(), sel: m.ws.sel, record: true})
	msg := findRunMsg(cmd)
	if !ok || msg == nil {
		t.Fatalf("%s did not run", req.Metadata.Name)
	}
	next, _ := m.Update(msg)
	*m = next.(Model)
}

func runToEnd(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	var start func(tea.Cmd)
	start = func(c tea.Cmd) {
		if c == nil {
			return
		}
		go func() {
			if b, ok := c().(tea.BatchMsg); ok {
				for _, sub := range b {
					start(sub)
				}
			}
		}()
	}
	start(cmd)
	var msgs []tea.Msg
	for done := false; !done; {
		select {
		case msg := <-m.runMsgChan:
			msgs = append(msgs, msg)
			_, done = msg.(runWorkerDoneMsg)
		case <-time.After(10 * time.Second):
			t.Fatal("run did not finish")
		}
	}
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		*m = next.(Model)
	}
}

func storedText(e history.Entry) string {
	parts := []string{e.RequestName, e.URL, e.Status, e.BodySnippet, e.RequestText, e.Description}
	if p := e.ProfileResults; p != nil {
		parts = append(parts, p.Error)
		for _, f := range p.Failures {
			parts = append(parts, f.Reason)
		}
	}
	if c := e.Compare; c != nil {
		for _, r := range c.Results {
			parts = append(parts, r.Status, r.BodySnippet, r.RequestText, r.Error)
		}
	}
	return strings.Join(parts, "\n")
}

const remaskSrc = `### Seen
# @name Seen
GET BASE/seen?v=later-cap-77aa-bbcc

### Each
# @name Each
# @for-each ["a","b"] as item
GET BASE/seen?v=later-cap-77aa-bbcc

### Cmp
# @name Cmp
# @compare dev stage
GET BASE/seen?v=later-cap-77aa-bbcc

### Prof
# @name Prof
# @profile count=1
GET BASE/seen?v=later-cap-77aa-bbcc

### SeenStatus
# @name SeenStatus
GET BASE/seenstatus?v=later-cap-77aa-bbcc

### Login
# @name Login
# @capture file-secret tok {{response.json.token}}
GET BASE/cap?v=later-cap-77aa-bbcc

# @workflow wf
# @step One using=SeenStatus
`

func TestHistoryRemasksSessionEntries(t *testing.T) {
	srv := remaskServer(t)
	store := histdb.New(filepath.Join(t.TempDir(), "history.db"))
	t.Cleanup(func() { _ = store.Close() })
	old := history.Entry{
		ID:          "1",
		ExecutedAt:  time.Unix(100, 0),
		RequestName: "old",
		URL:         srv.URL + "/seen?v=" + remaskSecret,
		BodySnippet: `{"saw":"` + remaskSecret + `"}`,
	}
	if err := store.Append(old); err != nil {
		t.Fatal(err)
	}
	before, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	mv := newOrchTestModel(t, Config{
		FilePath:       "remask.http",
		InitialContent: strings.ReplaceAll(remaskSrc, "BASE", srv.URL),
		History:        store,
		Env:            vars.Config{Selection: testSelection("dev")},
	})
	m := &mv
	opts := m.runOptions()
	sendRequest(t, m, requestNamed(t, m, "Seen"))
	runToEnd(t, m, m.startForEachRun(m.doc, requestNamed(t, m, "Each"), opts))
	cmp := requestNamed(t, m, "Cmp")
	runToEnd(t, m, m.startCompareRun(m.doc, cmp, cmp.Metadata.Compare, opts))
	runToEnd(t, m, m.startProfileRun(m.doc, requestNamed(t, m, "Prof"), opts))
	runToEnd(t, m, m.startWorkflowRun(m.doc, m.doc.Workflows[0], opts))
	url := srv.URL + "/seen?v=" + remaskSecret
	for _, tt := range []struct {
		name string
		body string
	}{
		{name: "Fallback", body: `{"saw":"` + remaskSecret + `"}`},
		{name: "Big", body: "x" + remaskSecret + strings.Repeat("x", historyBudget)},
	} {
		req := &restfile.Request{Method: "GET", URL: url, Metadata: restfile.RequestMetadata{Name: tt.name}}
		resp := &httpx.Response{Status: "200 OK", StatusCode: http.StatusOK, Body: []byte(tt.body)}
		m.recordHTTPHistory(resp, req, "GET "+url+"\n", "dev", vars.Selection{})
	}

	es, err := store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if !strings.Contains(storedText(e), remaskSecret) {
			t.Errorf("%s does not hold the value before it is a secret:\n%s", e.RequestName, storedText(e))
		}
	}
	sendRequest(t, m, requestNamed(t, m, "Login"))

	es, err = store.Entries()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, e := range es {
		counts[e.RequestName]++
		if e.ID == old.ID {
			continue
		}
		if text := storedText(e); strings.Contains(text, remaskSecret) || !strings.Contains(text, redact.Mask) {
			t.Errorf("%s still holds the secret:\n%s", e.RequestName, text[:min(len(text), 600)])
		}
	}
	want := map[string]int{
		"old":      1,
		"Seen":     1,
		"Each":     2,
		"Cmp":      1,
		"Prof":     1,
		"wf":       1,
		"Fallback": 1,
		"Big":      1,
		"Login":    1,
	}
	if fmt.Sprint(counts) != fmt.Sprint(want) {
		t.Errorf("entries = %v, want %v", counts, want)
	}
	if !slices.ContainsFunc(es, func(e history.Entry) bool {
		return e.RequestName == "Big" && e.BodySnippet == redact.Mask && e.URL == srv.URL+"/seen?v="+redact.Mask
	}) {
		t.Error("the entry over the budget does not show the mask")
	}
	if got := es[len(es)-1]; got.ID != old.ID || fmt.Sprint(got) != fmt.Sprint(before[0]) {
		t.Errorf("entry from before the session = %+v, want %+v", got, before[0])
	}
}

func TestHistoryRequestScopeMatchesMaskedURL(t *testing.T) {
	srv := remaskServer(t)
	m, store := newRemaskModel(t, strings.ReplaceAll(`### secret
# @request-secret tok lit-s3c-44-abcdefgh
GET BASE/seen?v=lit-s3c-44-abcdefgh

### plain
GET BASE/seen?v=plain
`, "BASE", srv.URL))
	for i, req := range m.doc.Requests {
		sendRequest(t, m, req)
		m.currentRequest = req
		m.historyScope = historyScopeRequest
		m.syncHistory()
		if len(m.historyEntries) != 1 {
			t.Errorf("request %d: scope lists %d entries, want 1", i, len(m.historyEntries))
		}
	}
	es, _ := store.Entries()
	if len(es) != 2 || es[1].URL != srv.URL+"/seen?v="+redact.Mask || es[0].URL != srv.URL+"/seen?v=plain" {
		t.Fatalf("stored %+v, want the literal masked", es)
	}
}

func TestHistoryWorkflowScopeMatchesMaskedName(t *testing.T) {
	srv := remaskServer(t)
	m, store := newRemaskModel(t, strings.ReplaceAll(`### secret
# @name secret
# @request-secret tok lit-s3c-44-abcdefgh
GET BASE/seen?v=plain

# @workflow flow-lit-s3c-44-abcdefgh
# @step One using=secret

# @workflow plain-flow
# @step One using=secret
`, "BASE", srv.URL))
	sendRequest(t, m, m.doc.Requests[0])
	for _, wf := range m.doc.Workflows {
		runToEnd(t, m, m.startWorkflowRun(m.doc, wf, m.runOptions()))
		m.setHistoryWorkflow(wf.Name)
		m.syncHistory()
		if len(m.historyEntries) != 1 {
			t.Errorf("%s: scope lists %d entries, want 1", wf.Name, len(m.historyEntries))
		}
	}
	es, _ := store.Entries()
	if len(es) != 3 || es[1].RequestName != "flow-"+redact.Mask || es[0].RequestName != "plain-flow" {
		t.Fatalf("stored %+v, want the workflow name masked", es)
	}
}

func TestProfileBaselineMatchesMaskedKey(t *testing.T) {
	srv := remaskServer(t)
	m, store := newRemaskModel(t, strings.ReplaceAll(`### Prof
# @name Prof
# @profile count=1
# @request-secret tok lit-s3c-44-abcdefgh
GET BASE/seen?v=lit-s3c-44-abcdefgh

### Plain
# @name Plain
# @profile count=1
GET BASE/seen?v=plain
`, "BASE", srv.URL))
	for _, req := range m.doc.Requests {
		runToEnd(t, m, m.startProfileRun(m.doc, req, m.runOptions()))
		pl, err := core.PrepareProfile(m.doc, req, core.RunMeta{Env: m.ws.active})
		if err != nil {
			t.Fatal(err)
		}
		if m.profileBaseline(pl.Key(), time.Now()) == nil {
			t.Errorf("%s: no baseline", req.Metadata.Name)
		}
	}
	es, _ := store.Entries()
	if len(es) != 2 || es[1].URL != srv.URL+"/seen?v="+redact.Mask || es[0].URL != srv.URL+"/seen?v=plain" {
		t.Fatalf("stored %+v, want the literal masked", es)
	}
}
