package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/core"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	rowEnv          = "row-env-s3c-77-abcdefgh"
	rowProd         = "row-prd-s3c-77-abcdefgh"
	rowDocEnv       = "row-doc-env-77-abcdefgh"
	rowReqEnv       = "row-req-env-77-abcdefgh"
	rowDocSecret    = "row-doc-sec-77-abcdefgh"
	rowFileSecret   = "row-file-cap-77-abcdefg"
	rowGlobalSecret = "row-glob-cap-77-abcdefg"
	rowPlain        = "row-plain-val-77-abcdef"
)

const rowSrc = `# @file dk env:ROW_DOC
# @file-secret ds DOC_SECRET

###
GET BASE/seen?v=ENV_SECRET

###
GET BASE/seen?t={{key}}

###
GET BASE/seen?d=DOC_ENV

###
# @request rk env:ROW_REQ
GET BASE/seen?r=REQ_ENV

###
GET BASE/seen?s=DOC_SECRET

###
# @capture file-secret ftok {{response.json.token}}
GET BASE/cap?v=FILE_SECRET

###
GET BASE/seen?f=FILE_SECRET

###
# @capture global-secret gtok {{response.json.token}}
GET BASE/cap?v=GLOBAL_SECRET

###
GET BASE/seen?g=GLOBAL_SECRET

###
GET BASE/seen?p=PLAIN_VALUE

###
# @profile count=1
GET BASE/seen?prof=1&v=ENV_SECRET

###
# @name step
GET BASE/seen?step=1

# @workflow flow-ENV_SECRET
# @step One using=step
`

const (
	reqEnvValue = iota
	reqTemplate
	reqDocRef
	reqOwnRef
	reqDocSecret
	reqFileCapture
	reqFileValue
	reqGlobalCapture
	reqGlobalValue
	reqPlainValue
	reqProfile
)

type rowSession struct {
	db, env, src string
	state        engine.RuntimeState
}

func newRowSession(t *testing.T) rowSession {
	t.Helper()
	srv := remaskServer(t)
	t.Setenv("ROW_ENV", rowEnv)
	t.Setenv("ROW_PROD", rowProd)
	t.Setenv("ROW_DOC", rowDocEnv)
	t.Setenv("ROW_REQ", rowReqEnv)
	root := t.TempDir()
	s := rowSession{
		db:  filepath.Join(root, "history.db"),
		env: filepath.Join(root, "resterm.env.json"),
		src: strings.NewReplacer(
			"BASE", srv.URL,
			"ENV_SECRET", rowEnv,
			"DOC_ENV", rowDocEnv,
			"REQ_ENV", rowReqEnv,
			"DOC_SECRET", rowDocSecret,
			"FILE_SECRET", rowFileSecret,
			"GLOBAL_SECRET", rowGlobalSecret,
			"PLAIN_VALUE", rowPlain,
		).Replace(rowSrc),
	}
	body := `{"dev":{"key":"env:ROW_ENV","plainkey":"` + rowPlain + `"},"prod":{"key":"env:ROW_PROD"}}`
	if err := os.WriteFile(s.env, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	hs := histdb.New(s.db)
	m := s.open(t, hs, s.config(t, "dev"))
	for i, req := range m.doc.Requests {
		if i == reqProfile {
			runToEnd(t, m, m.startProfileRun(m.doc, req, m.runOptions()))
			continue
		}
		sendRequest(t, m, req)
	}
	runToEnd(t, m, m.startWorkflowRun(m.doc, m.doc.Workflows[0], m.runOptions()))
	s.state = m.runtimeSvc().RuntimeState()
	es, err := hs.Entries()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		text := storedText(e)
		for _, v := range []string{rowEnv, rowDocEnv, rowReqEnv, rowDocSecret, rowFileSecret, rowGlobalSecret} {
			if strings.Contains(text, v) {
				t.Fatalf("entry %q holds %s", e.URL, v)
			}
		}
	}
	if err := errors.Join(m.Close(), hs.Close()); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s rowSession) config(t *testing.T, name string) vars.Config {
	t.Helper()
	cat, err := vars.LoadEnvironmentFile(s.env)
	if err != nil {
		t.Fatal(err)
	}
	sel, err := cat.Select(name, nil)
	if err != nil {
		t.Fatal(err)
	}
	return vars.Config{Catalog: cat, File: s.env, Selection: sel}
}

func (s rowSession) open(t *testing.T, hs *histdb.Store, env vars.Config) *Model {
	t.Helper()
	m := newOrchTestModel(t, Config{FilePath: "rows.http", InitialContent: s.src, History: hs, Env: env})
	return &m
}

func (s rowSession) reopen(t *testing.T, env vars.Config) *Model {
	t.Helper()
	hs := histdb.New(s.db)
	t.Cleanup(func() { _ = hs.Close() })
	return s.open(t, hs, env)
}

func listed(m *Model, req *restfile.Request) int {
	m.currentRequest, m.historyScope = req, historyScopeRequest
	m.syncHistory()
	return len(m.historyEntries)
}

func TestHistoryRequestScopeAfterRestart(t *testing.T) {
	s := newRowSession(t)
	for name, tt := range map[string]struct {
		req     int
		restore bool
	}{
		"env file value":        {req: reqEnvValue},
		"template":              {req: reqTemplate},
		"doc env reference":     {req: reqDocRef},
		"request env reference": {req: reqOwnRef},
		"doc secret":            {req: reqDocSecret},
		"file store restored":   {req: reqFileValue, restore: true},
		"global store restored": {req: reqGlobalValue, restore: true},
	} {
		t.Run(name, func(t *testing.T) {
			m := s.reopen(t, s.config(t, "dev"))
			if tt.restore {
				m.runtimeSvc().LoadRuntimeState(s.state)
			}
			if n := listed(m, m.doc.Requests[tt.req]); n != 1 {
				t.Errorf("request scope lists %d entries from the earlier session, want 1", n)
			}
		})
	}
}

func TestHistoryLookupKeepsPlainValues(t *testing.T) {
	s := newRowSession(t)
	m := s.reopen(t, s.config(t, "dev"))
	req := m.doc.Requests[reqPlainValue]
	if n := listed(m, req); n != 1 {
		t.Errorf("request scope lists %d entries from the earlier session, want 1", n)
	}
	if es, err := m.historyStore().ByRequest(req.URL); err != nil || len(es) != 1 {
		t.Errorf("store holds %d entries (err %v) under the raw URL, want 1", len(es), err)
	}
	if got := m.hist.Secrets().Text(req.URL); got != req.URL {
		t.Errorf("key = %q, want %q unmasked", got, req.URL)
	}
	if m.hist.Secrets().Text(rowEnv) != redact.Mask {
		t.Error("the lookup did not learn the env secret")
	}
}

func TestHistoryWorkflowScopeAfterRestart(t *testing.T) {
	s := newRowSession(t)
	m := s.reopen(t, s.config(t, "dev"))
	m.setHistoryWorkflow(m.doc.Workflows[0].Name)
	m.syncHistory()
	if len(m.historyEntries) != 1 {
		t.Errorf("workflow scope lists %d entries from the earlier session, want 1", len(m.historyEntries))
	}
}

func TestProfileBaselineAfterRestart(t *testing.T) {
	s := newRowSession(t)
	m := s.reopen(t, s.config(t, "dev"))
	req := m.doc.Requests[reqProfile]
	m.currentRequest = req
	pl, err := core.PrepareProfile(m.doc, req, core.RunMeta{Env: m.ws.active})
	if err != nil {
		t.Fatal(err)
	}
	if m.profileBaseline(pl.Key(), time.Now()) == nil {
		t.Error("no profile baseline from the earlier session")
	}
}

func TestHistoryLookupWithBrokenEnvFile(t *testing.T) {
	s := newRowSession(t)
	path := filepath.Join(t.TempDir(), "resterm.env.json")
	if err := os.WriteFile(path, []byte(brokenEnv), 0o600); err != nil {
		t.Fatal(err)
	}
	cat, err := vars.LoadEnvironmentFile(path)
	if err == nil {
		t.Fatal("the environment file loaded, so there is nothing to recover from")
	}
	for name, req := range map[string]int{
		"doc secret":        reqDocSecret,
		"doc env reference": reqDocRef,
	} {
		t.Run(name, func(t *testing.T) {
			m := s.reopen(t, vars.Config{Catalog: cat, File: path, FileErr: err})
			if n := listed(m, m.doc.Requests[req]); n != 1 {
				t.Errorf("request scope lists %d entries from the earlier session, want 1", n)
			}
		})
	}
}

func TestHistoryFindsEntriesAfterSwitchingBack(t *testing.T) {
	s := newRowSession(t)
	m := s.reopen(t, s.config(t, "prod"))
	if err := m.selectEnvironment("dev", nil); err != nil {
		t.Fatal(err)
	}
	if n := listed(m, m.doc.Requests[reqEnvValue]); n != 1 {
		t.Errorf("request scope lists %d entries after switching to dev, want 1", n)
	}
}

func TestHistoryStartupListsEarlierSession(t *testing.T) {
	s := newRowSession(t)
	m := s.reopen(t, s.config(t, "dev"))
	if m.historyScope != historyScopeRequest || m.currentRequest != m.doc.Requests[reqEnvValue] {
		t.Fatal("startup did not open the request scope on the first request")
	}
	if len(m.historyEntries) != 1 {
		t.Errorf("startup lists %d entries from the earlier session, want 1", len(m.historyEntries))
	}
}
