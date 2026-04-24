package headless

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runfail"
)

func TestExecuteProfilePreservesWarmupStatsAndFailures(t *testing.T) {
	var (
		mu  sync.Mutex
		hit int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hit++
		cur := hit
		mu.Unlock()
		if cur == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := fmt.Fprint(w, `{"ok":false}`); err != nil {
				t.Fatalf("write response: %v", err)
			}
			return
		}
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
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
		URL:    srv.URL + "/profile",
		Metadata: restfile.RequestMetadata{
			Name: "profile",
			Profile: &restfile.ProfileSpec{
				Count:  2,
				Warmup: 1,
			},
		},
	}
	doc := &restfile.Document{
		Path:     "test.http",
		Requests: []*restfile.Request{req},
	}

	out, err := eng.ExecuteProfile(doc, req, "")
	if err != nil {
		t.Fatalf("ExecuteProfile: %v", err)
	}
	if out == nil {
		t.Fatal("expected profile result")
	}
	if out.Count != 2 || out.Warmup != 1 {
		t.Fatalf("unexpected profile counts: %+v", out)
	}
	if out.Success {
		t.Fatalf("expected failed profile run, got %+v", out)
	}
	if out.Results == nil {
		t.Fatal("expected profile stats")
	}
	if out.Results.TotalRuns != 3 || out.Results.WarmupRuns != 1 {
		t.Fatalf("unexpected profile run totals: %+v", out.Results)
	}
	if out.Results.SuccessfulRuns != 1 || out.Results.FailedRuns != 1 {
		t.Fatalf("unexpected profile success/failure counts: %+v", out.Results)
	}
	if len(out.Failures) != 1 {
		t.Fatalf("expected one profile failure, got %+v", out.Failures)
	}
	if out.Failures[0].Iteration != 2 || out.Failures[0].Warmup {
		t.Fatalf("unexpected profile failure entry: %+v", out.Failures[0])
	}
	if failure := out.Failures[0].Failure; failure.Code != runfail.CodeAssertion ||
		failure.Source != "profile" || failure.ExitCode != runfail.ExitFailure {
		t.Fatalf("unexpected profile failure classification: %+v", failure)
	}
}
