package headless

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/history"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type memHistory struct{ entries []history.Entry }

func (s *memHistory) Load() error                                { return nil }
func (s *memHistory) Append(e history.Entry) error               { s.entries = append(s.entries, e); return nil }
func (s *memHistory) Entries() ([]history.Entry, error)          { return s.entries, nil }
func (s *memHistory) ByRequest(string) ([]history.Entry, error)  { return s.entries, nil }
func (s *memHistory) ByWorkflow(string) ([]history.Entry, error) { return s.entries, nil }
func (s *memHistory) ByFile(string) ([]history.Entry, error)     { return s.entries, nil }
func (s *memHistory) Delete(string) (bool, error)                { return false, nil }
func (s *memHistory) Close() error                               { return nil }

func TestCompareHistoryMasksRuntimeSecrets(t *testing.T) {
	const secret = "compare-runtime-secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprintf(w, `{"echo":%q}`, secret); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := fmt.Sprintf(`### probe
# @name probe
# @script pre-request
> vars.global.set("token", "%s", {secret: true});
> request.setHeader("X-Token", vars.global.get("token"));
GET %s/probe
`, secret, srv.URL)

	doc := parser.Parse("compare.http", []byte(src))
	if len(doc.Requests) != 1 {
		t.Fatalf("parsed %d requests, want 1", len(doc.Requests))
	}

	cl := newHTTPClientWithFactory(func(httpx.Options) (*http.Client, error) {
		return srv.Client(), nil
	})
	store := &memHistory{}
	rt := rtrun.New(rtrun.Config{Client: cl, History: store})
	t.Cleanup(func() { _ = rt.Close() })

	cfg := engine.Config{Client: cl, History: store}
	eng := newWithDeps(request.New(cfg, rt), rt, cfg)

	out, err := eng.ExecuteCompare(doc, doc.Requests[0], &restfile.CompareSpec{
		Environments: []string{"one", "two"},
		Baseline:     "one",
	}, testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteCompare: %v", err)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(out.Rows))
	}
	for _, row := range out.Rows {
		if len(row.RuntimeSecrets) == 0 {
			t.Fatalf("row %q carried no runtime secrets", row.Environment)
		}
	}

	if len(store.entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(store.entries))
	}
	ent := store.entries[0]
	if ent.Compare == nil {
		t.Fatal("history entry has no compare results")
	}
	texts := map[string]string{"entry request text": ent.RequestText}
	for i, res := range ent.Compare.Results {
		texts[fmt.Sprintf("row %d request text", i)] = res.RequestText
		texts[fmt.Sprintf("row %d body snippet", i)] = res.BodySnippet
	}
	for field, text := range texts {
		if strings.Contains(text, secret) {
			t.Fatalf("%s contains the plaintext secret: %s", field, text)
		}
	}
}
