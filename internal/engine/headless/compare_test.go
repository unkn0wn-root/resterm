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
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestExecuteCompareKeepsRequestedMissingBaselineAndFallsBackToFirstRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	cl := httpclient.NewClient(nil)
	cl.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return srv.Client(), nil
	})

	rt := rtrun.New(rtrun.Config{Client: cl})
	defer func() { _ = rt.Close() }()

	cfg := engine.Config{Client: cl}
	rq := request.New(cfg, rt)
	eng := newWithDeps(rq, rt, cfg)

	req := &restfile.Request{
		Method: "GET",
		URL:    srv.URL + "/ok",
		Metadata: restfile.RequestMetadata{
			Name: "ok",
		},
	}
	doc := &restfile.Document{
		Path:     "test.http",
		Requests: []*restfile.Request{req},
	}

	out, err := eng.ExecuteCompare(doc, req, &restfile.CompareSpec{
		Environments: []string{"one", "two"},
		Baseline:     "missing",
	}, "")
	if err != nil {
		t.Fatalf("ExecuteCompare: %v", err)
	}
	if out == nil {
		t.Fatalf("expected compare result")
	}
	if out.Baseline != "missing" {
		t.Fatalf("expected requested baseline to stay missing, got %q", out.Baseline)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("expected 2 compare rows, got %+v", out.Rows)
	}
	if out.Rows[0].Summary != "baseline" {
		t.Fatalf("expected first row to stay effective baseline, got %q", out.Rows[0].Summary)
	}
	if out.Rows[1].Summary != "match" {
		t.Fatalf("expected second row to compare against first row, got %q", out.Rows[1].Summary)
	}
	if !strings.Contains(out.Report, "Baseline: missing") {
		t.Fatalf("expected report to keep missing baseline label, got %q", out.Report)
	}
}
