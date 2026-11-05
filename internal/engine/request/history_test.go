package request

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type memHistory struct{ entries []history.Entry }

func (s *memHistory) Load() error                                { return nil }
func (s *memHistory) Append(e history.Entry) error               { s.entries = append(s.entries, e); return nil }
func (s *memHistory) Entries() ([]history.Entry, error)          { return s.entries, nil }
func (s *memHistory) ByRequest(string) ([]history.Entry, error)  { return nil, nil }
func (s *memHistory) ByWorkflow(string) ([]history.Entry, error) { return nil, nil }
func (s *memHistory) ByFile(string) ([]history.Entry, error)     { return nil, nil }
func (s *memHistory) Delete(string) (bool, error)                { return false, nil }
func (s *memHistory) Close() error                               { return nil }

const historySecret = "later-cap-77aa-bbcc"

func recordAll(req *restfile.Request, text string, results ...runResult) []history.Entry {
	store := &memHistory{}
	e := New(engine.Config{}, rtrun.New(rtrun.Config{History: store}))
	for _, res := range results {
		res.RequestText, res.RuntimeSecrets = text, []string{historySecret}
		e.record(nil, req, res)
	}
	return store.entries
}

func TestRecordLeavesMaskingToTheStore(t *testing.T) {
	body := strings.Repeat("x", 3000) + historySecret
	url := "https://api.test/x?k=" + historySecret
	text := "GET " + url + "\nAuthorization: Bearer " + historySecret + "\nX-Other: " + historySecret + "\n"
	masked := "GET " + url + "\nAuthorization: " + redact.Mask + "\nX-Other: " + historySecret + "\n"
	reason := `vars.get("tok") == "` + historySecret + `"`
	results := []runResult{
		{Response: &httpx.Response{
			Status:     "401 token " + historySecret,
			StatusCode: http.StatusUnauthorized,
			Headers:    http.Header{"Content-Type": {"text/plain"}},
			Body:       []byte(body),
		}},
		{Skipped: true, SkipReason: reason},
		{GRPC: &grpcx.Response{Message: body}},
		{Response: &httpx.Response{Status: "200 OK", Body: []byte{0, 1, 2, 0xff}}},
	}
	req := &restfile.Request{Method: "GET", URL: url, Metadata: restfile.RequestMetadata{Name: "r"}}
	es := recordAll(req, text, results...)
	if len(es) != len(results) {
		t.Fatalf("recorded %d entries, want %d", len(es), len(results))
	}
	for i, want := range []string{body, reason, body, "<binary body 4 B>"} {
		e := es[i]
		if e.BodySnippet != want || e.RequestText != masked || e.URL != url {
			t.Errorf("entry %d: snippet of %d bytes, request text %q, URL %q, want the raw values and the header rule",
				i, len(e.BodySnippet), e.RequestText, e.URL)
		}
	}
	if es[0].Status != "401 token "+historySecret {
		t.Errorf("status = %q, want it raw", es[0].Status)
	}

	req.Metadata.NoLog = true
	for i, e := range recordAll(req, text, results[0], results[2]) {
		if e.BodySnippet != history.BodySuppressed {
			t.Errorf("@no-log entry %d: snippet = %q, want %q", i, e.BodySnippet, history.BodySuppressed)
		}
	}
	if es := recordAll(req, text, runResult{Skipped: true}); es[0].BodySnippet != history.BodySkipped {
		t.Errorf("skipped without a reason: snippet = %q, want %q", es[0].BodySnippet, history.BodySkipped)
	}
}

func TestRecordKeepsHeadersOnlyWhenLogged(t *testing.T) {
	text := "GET https://api.test/x\nAuthorization: Bearer " + historySecret + "\n"
	res := runResult{Response: &httpx.Response{Status: "200 OK", Body: []byte("ok")}}
	for _, tt := range []struct {
		logged bool
		want   string
	}{
		{logged: false, want: "GET https://api.test/x\nAuthorization: " + redact.Mask + "\n"},
		{logged: true, want: text},
	} {
		req := &restfile.Request{
			Method:   "GET",
			URL:      "https://api.test/x",
			Metadata: restfile.RequestMetadata{AllowSensitiveHeaders: tt.logged},
		}
		if got := recordAll(req, text, res)[0].RequestText; got != tt.want {
			t.Errorf("logged %v: request text = %q, want %q", tt.logged, got, tt.want)
		}
	}
}

type learnHistory struct {
	memHistory
	log []string
}

func (s *learnHistory) Learn(secrets ...string) {
	s.log = append(s.log, "learn "+strings.Join(secrets, " "))
}

func (s *learnHistory) Append(e history.Entry) error {
	s.log = append(s.log, "append "+e.Status)
	return s.memHistory.Append(e)
}

func TestRecordLearnsBeforeAppend(t *testing.T) {
	store := &learnHistory{}
	e := New(engine.Config{}, rtrun.New(rtrun.Config{History: store}))
	req := &restfile.Request{Method: "GET", URL: "https://api.test/x"}
	for _, res := range []runResult{
		{Response: &httpx.Response{Status: "200 OK", Body: []byte("ok")}},
		{Skipped: true},
		{GRPC: &grpcx.Response{Message: "ok"}},
	} {
		res.RuntimeSecrets = []string{historySecret}
		e.record(nil, req, res)
	}
	want := []string{
		"learn " + historySecret, "append 200 OK",
		"learn " + historySecret, "append SKIPPED",
		"learn " + historySecret, "append OK",
	}
	if !slices.Equal(store.log, want) {
		t.Fatalf("store saw %q, want each entry's secrets before the entry", store.log)
	}
}
