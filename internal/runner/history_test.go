package runner

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/history"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func historyServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		reply := func(code int, reason, ctype, body string) {
			conn, buf, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = conn.Close() }()
			_, _ = fmt.Fprintf(
				buf,
				"HTTP/1.1 %d %s\r\nContent-Type: %s\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
				code,
				reason,
				ctype,
				len(body),
				body,
			)
			_ = buf.Flush()
		}
		v := q.Get("v")
		switch r.URL.Path {
		case "/status":
			reply(http.StatusUnauthorized, "token "+q.Get("k"), "text/plain", "")
		case "/ok":
			reply(http.StatusOK, "OK", "text/plain", "plain p1a1n body")
		case "/cap":
			reply(http.StatusOK, "OK", "application/json", `{"token":"`+v+`"}`)
		case "/seen":
			reply(http.StatusOK, "OK", "application/json", `{"saw":"`+v+`"}`)
		case "/seenstatus":
			reply(http.StatusForbidden, "saw "+v, "text/plain", "")
		case "/cut":
			off, _ := strconv.Atoi(q.Get("off"))
			reply(http.StatusOK, "OK", "text/plain", strings.Repeat("x", off)+v+strings.Repeat("y", 50))
		case "/forms":
			f := escapedForms(v)
			reply(http.StatusOK, "OK", "application/json", fmt.Sprintf(
				`{"json": "%s", "pct": "%s", "pctup": "%s", "html": "%s", "b64": "%s"}`,
				f["json"], f["pct"], f["pctup"], f["html"], f["b64"]))
		case "/formstatus":
			reply(http.StatusUnauthorized, `denied {"t": "`+strings.ReplaceAll(v, "/", `\/`)+`"}`, "text/plain", "")
		default:
			reply(http.StatusNotFound, "Not Found", "text/plain", "")
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func escapedForms(v string) map[string]string {
	return map[string]string{
		"json":  strings.ReplaceAll(v, "/", `\/`),
		"pct":   strings.ToLower(url.QueryEscape(v)),
		"pctup": url.QueryEscape(v),
		"html":  strings.ReplaceAll(html.EscapeString(v), "/", "&#47;"),
		"b64":   base64.StdEncoding.EncodeToString([]byte("bin-" + v)),
	}
}

func historyText(e history.Entry) string {
	parts := []string{e.RequestName, e.URL, e.Status, e.BodySnippet, e.RequestText, e.Description}
	parts = append(parts, e.Tags...)
	if p := e.ProfileResults; p != nil {
		parts = append(parts, p.Error)
		for _, f := range p.Failures {
			parts = append(parts, f.Reason)
		}
	}
	if tr := e.Trace; tr != nil {
		parts = append(parts, tr.Error)
		for _, ph := range tr.Phases {
			parts = append(parts, ph.Error, ph.Meta.Addr)
		}
	}
	if c := e.Compare; c != nil {
		for _, r := range c.Results {
			parts = append(parts, r.Status, r.BodySnippet, r.RequestText, r.Error)
		}
	}
	return strings.Join(parts, "\n")
}

func readHistory(t *testing.T, stateDir string) []history.Entry {
	t.Helper()
	store := histdb.New(filepath.Join(stateDir, "history.db"))
	t.Cleanup(func() { _ = store.Close() })
	es, err := store.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	slices.Reverse(es)
	return es
}

const historyFile = `### literal
# @name literal
# @request-secret tok lit-s3c-44-abcdefgh
GET BASE/status?k=lit-s3c-44-abcdefgh

### reason
# @name reason
# @request-secret tok rsn-s3c-55-abcdefgh
GET BASE/status?k={{tok}}

### forms
# @name forms
# @request-secret tok sl/x10-abcdefghijk
GET BASE/forms?v={{tok}}

### formstatus
# @name formstatus
# @request-secret tok a/b9z-abcdefghijkl
GET BASE/formstatus?v={{tok}}

### described
# @name described
# @description uses desc-s3c-12-abcdefgh here
# @tag tag-s3c-13-abcdefgh
# @request-secret d desc-s3c-12-abcdefgh
# @request-secret t tag-s3c-13-abcdefgh
GET BASE/ok

### traced
# @name traced
# @trace total<=5s
# @request-secret tok tr-s3c-14-abcdefgh
GET CLOSED/x?k={{tok}}

### skipped
# @name skipped
# @request-secret tok sk-s3c-15-abcdefgh
# @when vars.get("tok") == "sk-s3c-15-abcdefgh-no"
GET BASE/ok

### Seen body
# @name SeenBody
GET BASE/seen?v=later-cap-77aa-bbcc

### Seen status
# @name SeenStatus
GET BASE/seenstatus?v=later-cap-77aa-bbcc

### Cut
# @name Cut
GET BASE/cut?off=1995&v=later-cap-77aa-bbcc

### Script
# @name Script
# @script pre-request
> vars.global.set("gtok", "wf-sec-88dd-abcdefgh", {secret: true});
> request.setQueryParam("k", vars.global.get("gtok"));
GET BASE/ok

### Logged
# @name Logged
# @log-sensitive-headers
# @request-secret tok mix-log-88-abcdefgh
GET BASE/ok
Authorization: Bearer mix-log-88-abcdefgh
X-Api-Key: rawkey-99-abcdefgh

### NoLog
# @name NoLog
# @no-log
GET BASE/seen?v=nolog

### Plain
# @name Plain
GET BASE/ok

### SeenWF
# @name SeenWF
GET BASE/seenstatus?v=wf-later-99ee-abcdefgh

### CapWF
# @name CapWF
# @capture file-secret wfcap {{response.json.token}}
GET BASE/cap?v=wf-later-99ee-abcdefgh

### Each
# @name Each
# @request-secret tok each-s3c-12-abcdefgh
# @for-each item in ["a", "b"]
GET BASE/seen?v={{tok}}-{{= item}}

### Prof
# @name Prof
# @profile count=2
# @capture request-secret ptok {{response.json.token}}
GET BASE/cap?v=prof-cap-34-abcdefgh

### Cmp
# @name Cmp
# @compare dev stage
GET BASE/status?k={{ctok}}

### Login
# @name Login
# @capture file-secret cap {{response.json.token}}
GET BASE/cap?v=later-cap-77aa-bbcc

### WF
# @workflow wf on-failure=continue
# @step First using=SeenWF
# @step Second using=CapWF
`

func TestRunHistoryUsesRunSecrets(t *testing.T) {
	t.Setenv("RST442_CDEV", "cmp-dev-s3c-56-abcdefgh")
	t.Setenv("RST442_CSTG", "cmp-stg-s3c-78-abcdefgh")
	srv := historyServer(t)
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	cat, err := vars.NewCatalog(vars.EnvironmentSet{
		"dev":   {"ctok": "env:RST442_CDEV"},
		"stage": {"ctok": "env:RST442_CSTG"},
	})
	if err != nil {
		t.Fatal(err)
	}
	sel, err := cat.Select("dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	file := filepath.Join(dir, "history.http")
	src := strings.NewReplacer("BASE", srv.URL, "CLOSED", closed.URL).Replace(historyFile)
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, s := range []Select{{All: true}, {Workflow: "wf"}} {
		if _, err := RunContext(context.Background(), Options{
			FilePath:      file,
			WorkspaceRoot: dir,
			StateDir:      stateDir,
			History:       true,
			Catalog:       cat,
			Selection:     sel,
			Select:        s,
		}); err != nil {
			t.Fatalf("Run %+v: %v", s, err)
		}
	}

	es := readHistory(t, stateDir)
	var names []string
	for i, e := range es {
		names = append(names, e.RequestName)
		if i > 0 && (e.ID <= es[i-1].ID || !e.ExecutedAt.After(es[i-1].ExecutedAt)) {
			t.Errorf("entry %d (%s) is out of append order", i, e.RequestName)
		}
	}
	want := []string{
		"literal", "reason", "forms", "formstatus", "described", "traced", "skipped", "SeenBody", "SeenStatus",
		"Cut", "Script", "Logged", "NoLog", "Plain", "SeenWF", "CapWF", "Each", "Each", "Prof", "Cmp", "Login", "wf",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("entries = %q, want %q", names, want)
	}

	secrets := []string{
		"lit-s3c-44-abcdefgh", "rsn-s3c-55-abcdefgh", "a/b9z-abcdefghijkl", "desc-s3c-12-abcdefgh",
		"tag-s3c-13-abcdefgh", "tr-s3c-14-abcdefgh", "sk-s3c-15-abcdefgh", "later-cap-77aa-bbcc",
		"wf-sec-88dd-abcdefgh", "mix-log-88-abcdefgh", "wf-later-99ee-abcdefgh", "each-s3c-12-abcdefgh",
		"prof-cap-34-abcdefgh", "cmp-dev-s3c-56-abcdefgh", "cmp-stg-s3c-78-abcdefgh",
		`a\/b9z-abcdefghijkl`,
	}
	for _, f := range escapedForms("sl/x10-abcdefghijk") {
		secrets = append(secrets, f)
	}
	byName := map[string]history.Entry{}
	for _, e := range es {
		byName[e.RequestName] = e
		text := historyText(e)
		for _, s := range secrets {
			if strings.Contains(text, s) {
				t.Errorf("%s holds %q:\n%s", e.RequestName, s, text)
			}
		}
		if strings.Contains(text, `***`) {
			t.Errorf("%s holds the old mask:\n%s", e.RequestName, text)
		}
	}

	for name, got := range map[string]string{
		"literal URL":     byName["literal"].URL,
		"reason status":   byName["reason"].Status,
		"SeenStatus":      byName["SeenStatus"].Status,
		"wf report":       byName["wf"].BodySnippet,
		"Cmp stage row":   byName["Cmp"].Compare.Results[1].Status,
		"Prof URL":        byName["Prof"].URL,
		"Logged known":    byName["Logged"].RequestText,
		"described descr": byName["described"].Description,
	} {
		if !strings.Contains(got, redact.Mask) {
			t.Errorf("%s = %q, want %s in place of the secret", name, got, redact.Mask)
		}
	}
	cut := byName["Cut"].BodySnippet
	for k := 1; k <= len("later-cap-77aa-bbcc"); k++ {
		if strings.HasSuffix(cut, "later-cap-77aa-bbcc"[:k]) {
			t.Errorf("Cut snippet ends in %q", "later-cap-77aa-bbcc"[:k])
		}
	}
	if !strings.Contains(byName["Logged"].RequestText, "X-Api-Key: rawkey-99-abcdefgh") {
		t.Errorf("Logged request text = %q, want the logged header kept", byName["Logged"].RequestText)
	}
	if got := byName["NoLog"].BodySnippet; got != history.BodySuppressed {
		t.Errorf("NoLog snippet = %q, want %q", got, history.BodySuppressed)
	}
	plain := byName["Plain"]
	if plain.URL != srv.URL+"/ok" || plain.Status != "200 OK" || plain.BodySnippet != "plain p1a1n body" ||
		strings.Contains(plain.RequestText, redact.Mask) {
		t.Errorf("Plain = %+v, want it unchanged", plain)
	}
}

func TestRunHistoryOnEngineError(t *testing.T) {
	srv := historyServer(t)
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	file := filepath.Join(dir, "err.http")
	src := strings.Join([]string{
		"### Login",
		"# @name Login",
		"# @capture file-secret tok {{response.json.token}}",
		"GET " + srv.URL + "/cap?v=later-cap-77aa-bbcc",
		"",
		"### Socket",
		"# @name Socket",
		"# @websocket",
		"GET ws" + strings.TrimPrefix(srv.URL, "http") + "/ws",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RunContext(context.Background(), Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		StateDir:      stateDir,
		History:       true,
		Select:        Select{All: true},
	}); err == nil {
		t.Fatal("Run passed, want the engine error")
	}
	es := readHistory(t, stateDir)
	if len(es) != 1 || es[0].RequestName != "Login" {
		t.Fatalf("entries = %+v, want Login only", es)
	}
	if text := historyText(
		es[0],
	); strings.Contains(text, "later-cap-77aa-bbcc") ||
		!strings.Contains(text, redact.Mask) {
		t.Errorf("Login holds the captured secret:\n%s", text)
	}
}

func TestRunWithoutHistoryWritesNothing(t *testing.T) {
	srv := historyServer(t)
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	file := filepath.Join(dir, "plain.http")
	if err := os.WriteFile(file, []byte("# @name Plain\nGET "+srv.URL+"/ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := RunContext(context.Background(), Options{
		FilePath:       file,
		WorkspaceRoot:  dir,
		StateDir:       stateDir,
		PersistGlobals: true,
	})
	if err != nil || !rep.Success() {
		t.Fatalf("Run = %+v, %v", rep, err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "history.db")); !os.IsNotExist(err) {
		t.Fatalf("history.db exists: %v", err)
	}
}
