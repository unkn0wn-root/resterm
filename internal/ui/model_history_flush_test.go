package ui

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/engine/core"
	rqeng "github.com/unkn0wn-root/resterm/internal/engine/request"
	"github.com/unkn0wn-root/resterm/internal/history"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	waitLimit = 5 * time.Second
	quietWait = 200 * time.Millisecond
)

type gateStore struct {
	history.Store
	entered chan struct{}
	release chan struct{}
	open    func()
}

func newGateStore(t *testing.T) *gateStore {
	t.Helper()
	s := histdb.New(filepath.Join(t.TempDir(), "history.db"))
	t.Cleanup(func() { _ = s.Close() })
	g := &gateStore{Store: s, entered: make(chan struct{}, 1), release: make(chan struct{})}
	g.open = sync.OnceFunc(func() { close(g.release) })
	t.Cleanup(g.open)
	return g
}

func (g *gateStore) Append(e history.Entry) error {
	select {
	case g.entered <- struct{}{}:
	default:
	}
	<-g.release
	return g.Store.Append(e)
}

type failStore struct{ history.Store }

func (failStore) Append(history.Entry) error { return errors.New("disk I/O error") }

func (failStore) Delete(string) (bool, error) { return false, errors.New("disk I/O error") }

func within[T any](t *testing.T, what string, f func() T) T {
	t.Helper()
	done := make(chan T, 1)
	go func() { done <- f() }()
	select {
	case v := <-done:
		return v
	case <-time.After(waitLimit):
		t.Fatalf("%s did not finish within %v", what, waitLimit)
		var zero T
		return zero
	}
}

func findMsg[T tea.Msg](cmd tea.Cmd) (T, bool) {
	var zero T
	if cmd == nil {
		return zero, false
	}
	switch msg := cmd().(type) {
	case T:
		return msg, true
	case tea.BatchMsg:
		for _, sub := range msg {
			if got, ok := findMsg[T](sub); ok {
				return got, true
			}
		}
	}
	return zero, false
}

func awaitHistoryFlush(t *testing.T, m *Model) {
	t.Helper()
	for {
		msg := within(t, "the history flush", func() tea.Msg { return <-m.runMsgChan })
		next, _ := m.Update(msg)
		*m = next.(Model)
		if _, ok := msg.(historyFlushedMsg); ok {
			return
		}
	}
}

func selectOldest(t *testing.T, m *Model, s history.Store) {
	t.Helper()
	for i := range 2 {
		e := history.Entry{
			ID:          history.NewID(),
			ExecutedAt:  time.Unix(int64(100+i), 0),
			RequestName: "old",
			URL:         "https://api.test/old",
		}
		if err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	m.syncHistory()
	m.historyList.Select(1)
	m.historySelectedID = m.historyEntries[1].ID
}

func newFlushModel(t *testing.T, src string, hs history.Store) *Model {
	t.Helper()
	m := newOrchTestModel(t, Config{
		FilePath:       "flush.http",
		InitialContent: src,
		History:        hs,
		Env:            vars.Config{Selection: testSelection("dev")},
	})
	m.historyScope = historyScopeGlobal
	return &m
}

func TestRunResultDoesNotWaitForHistory(t *testing.T) {
	srv := remaskServer(t)
	g := newGateStore(t)
	m := newFlushModel(t, strings.ReplaceAll(remaskSrc, "BASE", srv.URL), g)
	selectOldest(t, m, g.Store)
	cmd, ok := m.startRun(runSpec{
		doc:    m.doc,
		req:    requestNamed(t, m, "Login"),
		opts:   m.runOptions(),
		sel:    m.ws.sel,
		record: true,
	})
	if !ok {
		t.Fatal("Login did not start")
	}
	msg := within(t, "the run with its history write blocked", func() tea.Msg { return findRunMsg(cmd) })
	if _, ok := msg.(runReqMsg); !ok {
		t.Fatalf("run returned %T, want runReqMsg", msg)
	}
	next, _ := m.Update(msg)
	*m = next.(Model)
	g.open()
	awaitHistoryFlush(t, m)
	es := m.historyEntries
	if len(es) != 3 || es[0].RequestName != "Login" || m.historySelectedID != es[0].ID {
		t.Fatalf("history lists %d entries, selected %q, want Login selected", len(es), m.historySelectedID)
	}
	if text := storedText(es[0]); strings.Contains(text, remaskSecret) {
		t.Fatalf("Login entry holds its captured secret:\n%s", text)
	}
}

func TestHistoryLookupsDuringFlush(t *testing.T) {
	g := newGateStore(t)
	m := newFlushModel(t, "GET https://api.test/x?k="+remaskSecret+"\n", g)
	if err := m.hist.Append(history.Entry{ID: "a", URL: "https://api.test/x?k=" + remaskSecret}); err != nil {
		t.Fatal(err)
	}
	flushed := make(chan error, 1)
	go func() { flushed <- m.hist.Flush(remaskSecret) }()
	t.Cleanup(func() {
		g.open()
		if err := <-flushed; err != nil {
			t.Error(err)
		}
	})
	within(t, "the flush", func() struct{} { return <-g.entered })
	m.currentRequest, m.historyScope = m.doc.Requests[0], historyScopeRequest
	within(t, "syncHistory during a flush", func() bool {
		m.syncHistory()
		return true
	})
	if got := within(t, "Secrets during a flush", m.hist.Secrets).Text(remaskSecret); got != redact.Mask {
		t.Fatalf("Secrets during a flush masks the secret as %q", got)
	}
}

func TestHistoryWritersDoNotWaitForTheStore(t *testing.T) {
	const tok = "writer-secret-0001-abcd"
	req := &restfile.Request{
		Method:   "GET",
		URL:      "https://api.test/x?k=" + tok,
		Metadata: restfile.RequestMetadata{Name: "W"},
	}
	resp := &httpx.Response{Status: "200 OK", StatusCode: http.StatusOK, Body: []byte(`{"t":"` + tok + `"}`)}
	for name, write := range map[string]func(*Model) tea.Cmd{
		"http": func(m *Model) tea.Cmd {
			return m.recordHTTPHistory(resp, req, "GET "+req.URL+"\n", "dev", vars.Selection{}, tok)
		},
		"skipped": func(m *Model) tea.Cmd {
			return m.recordSkippedHistory(req, "", "dev", "saw "+tok, vars.Selection{}, tok)
		},
		"grpc": func(m *Model) tea.Cmd {
			return m.recordGRPCHistory(&grpcx.Response{Message: tok}, req, "", "dev", vars.Selection{}, tok)
		},
		"compare": func(m *Model) tea.Cmd {
			m.hist.Learn(tok)
			rows := []compareResult{{Environment: "dev", Request: req, Response: resp}}
			return m.recordCompareHistory(&compareState{base: req, results: rows})
		},
		"workflow": func(m *Model) tea.Cmd {
			m.hist.Learn(tok)
			st := &workflowState{workflow: restfile.Workflow{Name: "wf-" + tok}, start: time.Now()}
			return m.recordWorkflowHistory(st, "done", "saw "+tok)
		},
		"profile": func(m *Model) tea.Cmd {
			m.hist.Learn(tok)
			return m.recordProfileHistory(&profileState{req: req, env: m.ws.active}, core.ProfileSnapshot{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			g := newGateStore(t)
			m := newFlushModel(t, "", g)
			selectOldest(t, m, g.Store)
			cmd := within(t, "the writer", func() tea.Cmd { return write(m) })
			if cmd == nil {
				t.Fatal("the writer returned no Cmd to write the entry")
			}
			g.open()
			cmd()
			awaitHistoryFlush(t, m)
			es := m.historyEntries
			if len(es) == 0 || es[0].RequestName == "old" || m.historySelectedID != es[0].ID {
				t.Fatalf(
					"history lists %d entries, selected %q, want the new entry selected",
					len(es),
					m.historySelectedID,
				)
			}
			if text := storedText(es[0]); strings.Contains(text, tok) {
				t.Fatalf("entry holds the secret:\n%s", text)
			}
		})
	}
}

func TestHistoryFlushErrorShowsWarning(t *testing.T) {
	srv := remaskServer(t)
	store := histdb.New(filepath.Join(t.TempDir(), "history.db"))
	t.Cleanup(func() { _ = store.Close() })
	m := newFlushModel(t, strings.ReplaceAll(remaskSrc, "BASE", srv.URL), failStore{store})
	cmd, ok := m.startRun(
		runSpec{doc: m.doc, req: requestNamed(t, m, "Seen"), opts: m.runOptions(), sel: m.ws.sel, record: true},
	)
	if !ok {
		t.Fatal("Seen did not start")
	}
	next, _ := m.Update(findRunMsg(cmd))
	*m = next.(Model)
	awaitHistoryFlush(t, m)
	if got := m.statusMessage; got.text != "history error: disk I/O error" || got.level != statusWarn {
		t.Fatalf("status = %q at level %v, want the history error as a warning", got.text, got.level)
	}
}

func runCmds(cmd tea.Cmd, got chan<- tea.Msg) {
	go func() {
		msg := cmd()
		if b, ok := msg.(tea.BatchMsg); ok {
			for _, sub := range b {
				if sub != nil {
					runCmds(sub, got)
				}
			}
			return
		}
		got <- msg
	}()
}

func TestRunEndsReturnTheHistoryWrite(t *testing.T) {
	req := &restfile.Request{Method: "GET", URL: "https://api.test/x", Metadata: restfile.RequestMetadata{Name: "End"}}
	resp := &httpx.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": {"application/json"}},
		Body:       []byte(`{"ok":true}`),
	}
	for name, end := range map[string]func(*testing.T, *Model) tea.Cmd{
		"response": func(_ *testing.T, m *Model) tea.Cmd {
			return m.handleResponseMessage(responseMsg{response: resp, executed: req, environment: "dev"})
		},
		"compare": func(_ *testing.T, m *Model) tea.Cmd {
			rows := []compareResult{{Environment: "dev", Request: req, Response: resp}}
			return m.finalizeCompareRun(&compareState{base: req, results: rows})
		},
		"workflow": func(_ *testing.T, m *Model) tea.Cmd {
			return m.finalizeWorkflowRun(&workflowState{workflow: restfile.Workflow{Name: "End"}, start: time.Now()})
		},
		"profile": func(t *testing.T, m *Model) tea.Cmd {
			startTestProfile(t, m, restfile.ProfileSpec{Count: 1}, restfile.RequestMetadata{})
			return m.finalizeProfileRun(m.profileRun)
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := histdb.New(filepath.Join(t.TempDir(), "history.db"))
			t.Cleanup(func() { _ = store.Close() })
			m := newFlushModel(t, "", store)
			if cmd := end(t, m); cmd != nil {
				runCmds(cmd, make(chan tea.Msg, 16))
			}
			awaitHistoryFlush(t, m)
			if es, err := store.Entries(); err != nil || len(es) != 1 {
				t.Fatalf("store holds %d entries (err %v) after the run's history write, want 1", len(es), err)
			}
		})
	}
}

func TestHistoryFlushedKeepsReadingRunMessages(t *testing.T) {
	m := newFlushModel(t, "", nil)
	_, cmd := m.Update(historyFlushedMsg{})
	if cmd == nil {
		t.Fatal("Update returned no Cmd to read the next run message")
	}
	m.runMsgChan <- runWarningMsg{text: "after the flush"}
	got := make(chan tea.Msg, 16)
	runCmds(cmd, got)
	for {
		msg := within(t, "the next run message", func() tea.Msg { return <-got })
		if w, ok := msg.(runWarningMsg); ok && w.text == "after the flush" {
			return
		}
	}
}

func TestCloseWritesPendingHistory(t *testing.T) {
	g := newGateStore(t)
	m := newFlushModel(t, "", g)
	add := func(id string) error {
		m.hist.Learn(remaskSecret)
		return m.hist.Append(
			history.Entry{ID: id, ExecutedAt: time.Now(), URL: "https://api.test/" + id + "?k=" + remaskSecret},
		)
	}
	if err := add("running"); err != nil {
		t.Fatal(err)
	}
	flushed := make(chan error, 1)
	go func() { flushed <- m.hist.Flush() }()
	within(t, "the flush", func() struct{} { return <-g.entered })
	if err := within(t, "an append during a flush", func() error { return add("pending") }); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- m.Close() }()
	g.open()
	for _, ch := range []chan error{flushed, closed} {
		if err := within(t, "quit", func() error { return <-ch }); err != nil {
			t.Fatal(err)
		}
	}
	es, err := g.Entries()
	if err != nil || len(es) != 2 {
		t.Fatalf("store holds %d entries (err %v) after quit, want the running and the pending one", len(es), err)
	}
	for _, e := range es {
		if e.URL != "https://api.test/"+e.ID+"?k="+redact.Mask {
			t.Errorf("entry %s = %q, want it masked", e.ID, e.URL)
		}
	}
}

func TestHistoryLookupsMatchEarlierSessions(t *testing.T) {
	srv := remaskServer(t)
	src := strings.ReplaceAll(`###
# @request-secret tok lit-s3c-44-abcdefgh
GET BASE/seen?v=lit-s3c-44-abcdefgh

###
# @name Prof
# @profile count=1
# @request-secret tok lit-s3c-44-abcdefgh
GET BASE/seen?p=1&v=lit-s3c-44-abcdefgh
`, "BASE", srv.URL)
	path := filepath.Join(t.TempDir(), "history.db")
	first := histdb.New(path)
	m := newFlushModel(t, src, first)
	sendRequest(t, m, m.doc.Requests[0])
	runToEnd(t, m, m.startProfileRun(m.doc, requestNamed(t, m, "Prof"), m.runOptions()))
	if err := errors.Join(m.Close(), first.Close()); err != nil {
		t.Fatal(err)
	}

	second := histdb.New(path)
	t.Cleanup(func() { _ = second.Close() })
	m = newFlushModel(t, src, second)
	m.currentRequest, m.historyScope = m.doc.Requests[0], historyScopeRequest
	m.syncHistory()
	if len(m.historyEntries) != 1 {
		t.Errorf("request scope lists %d entries from the earlier session, want 1", len(m.historyEntries))
	}
	prof := requestNamed(t, m, "Prof")
	m.currentRequest = prof
	pl, err := core.PrepareProfile(m.doc, prof, core.RunMeta{Env: m.ws.active})
	if err != nil {
		t.Fatal(err)
	}
	if m.profileBaseline(pl.Key(), time.Now()) == nil {
		t.Error("no profile baseline from the earlier session")
	}
}

func TestHistoryDeleteDoesNotWaitForFlush(t *testing.T) {
	for name, tt := range map[string]struct {
		marks  int
		status string
	}{
		"single":    {marks: 0, status: "History entry deleted"},
		"selection": {marks: 2, status: "Deleted 2 history entries"},
	} {
		t.Run(name, func(t *testing.T) {
			g := newGateStore(t)
			m := newFlushModel(t, "", g)
			selectOldest(t, m, g.Store)
			m.focus = focusResponse
			m.responsePanes[0].activeTab = responseTabHistory
			key := func(k tea.KeyMsg) tea.Cmd {
				next, cmd := m.Update(k)
				*m = next.(Model)
				return cmd
			}
			ids := []string{m.historySelectedID}
			if tt.marks > 0 {
				ids = nil
				for i := range tt.marks {
					m.historyList.Select(i)
					key(tea.KeyMsg{Type: tea.KeySpace})
					ids = append(ids, m.historyEntries[i].ID)
				}
			}
			err := m.hist.Append(history.Entry{ID: "held", ExecutedAt: time.Now(), URL: "https://api.test/held"})
			if err != nil {
				t.Fatal(err)
			}
			flushed := make(chan error, 1)
			go func() { flushed <- m.hist.Flush() }()
			t.Cleanup(func() {
				g.open()
				if err := <-flushed; err != nil {
					t.Error(err)
				}
			})
			within(t, "the flush", func() struct{} { return <-g.entered })
			cmd := within(t, "pressing d during a flush", func() tea.Cmd {
				return key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
			})
			g.open()
			msg := within(t, "the delete", func() historyDeletedMsg {
				msg, _ := findMsg[historyDeletedMsg](cmd)
				return msg
			})
			next, _ := m.Update(msg)
			*m = next.(Model)
			es, err := g.Entries()
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range es {
				if slices.Contains(ids, e.ID) {
					t.Errorf("entry %s is still stored after the delete", e.ID)
				}
			}
			if m.statusMessage.text != tt.status {
				t.Errorf("status = %q, want %q", m.statusMessage.text, tt.status)
			}
		})
	}
}

func TestHistoryDeleteReportsStoreErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		marks   int
		gone    bool
		fail    bool
		preview bool
		status  string
		level   statusLevel
		left    int
	}{
		"single": {fail: true, status: "history delete error: disk I/O error", level: statusError, left: 2},
		"selection": {
			marks:  2,
			fail:   true,
			status: "Deleted 0 history entries, failed 2: disk I/O error",
			level:  statusError,
			left:   2,
		},
		"selection with a row gone": {marks: 2, gone: true, status: "Deleted 1 history entry", level: statusInfo},
		"preview open": {
			fail:    true,
			preview: true,
			status:  "history delete error: disk I/O error",
			level:   statusError,
			left:    2,
		},
	} {
		t.Run(name, func(t *testing.T) {
			store := histdb.New(filepath.Join(t.TempDir(), "history.db"))
			t.Cleanup(func() { _ = store.Close() })
			var hs history.Store = store
			if tt.fail {
				hs = failStore{store}
			}
			m := newFlushModel(t, "", hs)
			selectOldest(t, m, store)
			m.focus = focusResponse
			m.responsePanes[0].activeTab = responseTabHistory
			for i := range tt.marks {
				m.historyList.Select(i)
				next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
				*m = next.(Model)
			}
			if tt.gone {
				if _, err := store.Delete(m.historyEntries[0].ID); err != nil {
					t.Fatal(err)
				}
			}
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
			*m = next.(Model)
			if tt.preview {
				m.openHistoryPreview(m.historyEntries[0])
			}
			msg, _ := findMsg[historyDeletedMsg](cmd)
			next, _ = m.Update(msg)
			*m = next.(Model)
			if got := m.statusMessage; got.text != tt.status || got.level != tt.level {
				t.Errorf("status = %q at level %v, want %q at level %v", got.text, got.level, tt.status, tt.level)
			}
			if len(m.historyEntries) != tt.left || len(m.historySelected) != 0 {
				t.Errorf("history lists %d entries with %d marked, want %d and none marked",
					len(m.historyEntries), len(m.historySelected), tt.left)
			}
			if m.showHistoryPreview != tt.preview {
				t.Errorf("preview open = %v after the delete, want %v", m.showHistoryPreview, tt.preview)
			}
		})
	}
}

func TestRecordingCallWithNilCtx(t *testing.T) {
	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }),
	)
	t.Cleanup(srv.Close)
	store := histdb.New(filepath.Join(t.TempDir(), "history.db"))
	t.Cleanup(func() { _ = store.Close() })
	m := newFlushModel(t, "### A\n# @name A\nGET "+srv.URL+"/a\n", store)
	env, err := m.environment(m.ws.sel)
	if err != nil {
		t.Fatal(err)
	}
	rq := m.newUIRequestEngine(m.requestSvc(m.runOptions()), "")
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("a recording call with a nil Ctx panics: %v", r)
			}
		}()
		if _, err := rq.ExecuteWith(m.doc, m.doc.Requests[0], env, rqeng.ExecOptions{Record: true}); err != nil {
			t.Fatal(err)
		}
	}()
	awaitHistoryFlush(t, m)
	if es, err := store.Entries(); err != nil || len(es) != 1 {
		t.Fatalf("store holds %d entries (err %v), want the recorded one", len(es), err)
	}
}

func TestForEachFlushesOneItemAtATime(t *testing.T) {
	const tok = "foreach-secret-0001-abcd"
	for name, tt := range map[string]struct {
		cancel bool
		rows   int
	}{
		"released": {rows: 4},
		"canceled": {cancel: true, rows: 2},
	} {
		t.Run(name, func(t *testing.T) {
			var hits atomic.Int32
			seen := make(chan struct{}, 8)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				seen <- struct{}{}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"saw":%q}`, r.URL.Query().Get("v"))
			}))
			t.Cleanup(srv.Close)
			g := newGateStore(t)
			src := "### Each\n# @name Each\n# @request-secret tok " + tok + "\n# @for-each [\"a\",\"b\",\"c\",\"d\"] as item\n" +
				"GET " + srv.URL + "/seen?v=" + tok + "\n"
			m := newFlushModel(t, src, g)
			runCmds(m.startForEachRun(m.doc, requestNamed(t, m, "Each"), m.runOptions()), make(chan tea.Msg, 16))
			within(t, "the first item's flush", func() struct{} { return <-g.entered })
			for range 2 {
				within(t, "a request", func() struct{} { return <-seen })
			}
			select {
			case <-seen:
				t.Fatalf("server saw %d requests while the first flush was blocked, want 2", hits.Load())
			case <-time.After(quietWait):
			}
			if tt.cancel {
				m.sendCancel()
				within(t, "the canceled run", func() struct{} {
					for {
						if _, ok := (<-m.runMsgChan).(runWorkerDoneMsg); ok {
							return struct{}{}
						}
					}
				})
				g.open()
				for range 2 {
					awaitHistoryFlush(t, m)
				}
			} else {
				g.open()
				runToEnd(t, m, nil)
			}
			es, err := g.Entries()
			if err != nil || len(es) != tt.rows {
				t.Fatalf("store holds %d entries (err %v), want %d", len(es), err, tt.rows)
			}
			for _, e := range es {
				if text := storedText(e); strings.Contains(text, tok) {
					t.Errorf("entry %s holds the secret:\n%s", e.ID, text)
				}
			}
		})
	}
}

func TestHistorySeedsOnlyMaskedLookups(t *testing.T) {
	const lit = "lit-s3c-44-abcdefgh"
	store := histdb.New(filepath.Join(t.TempDir(), "history.db"))
	t.Cleanup(func() { _ = store.Close() })
	mv := newOrchTestModel(t, Config{
		InitialContent: "# @request-secret tok " + lit + "\nGET https://api.test/x?k=" + lit + "\n",
		History:        store,
	})
	m := &mv
	m.currentRequest = m.doc.Requests[0]
	for _, scope := range []historyScope{historyScopeGlobal, historyScopeFile, historyScopeRequest} {
		m.historyScope = scope
		m.syncHistory()
		learned := m.hist.Secrets().Text(lit) == redact.Mask
		if want := scope == historyScopeRequest; learned != want {
			t.Errorf("after a sync in scope %v the lookup secrets hold the literal: %v, want %v", scope, learned, want)
		}
	}
}
