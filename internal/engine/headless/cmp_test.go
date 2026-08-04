package headless

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/engine/request"
	rtrun "github.com/unkn0wn-root/resterm/internal/engine/runtime"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func newHTTPClientWithFactory(factory httpx.HTTPClientFactory) *httpx.Client {
	return httpx.NewClientWithOptions(httpx.WithHTTPFactory(factory))
}

type cmpFixture struct {
	eng   *Engine
	doc   *restfile.Document
	req   *restfile.Request
	calls *int
}

func newCmpFixture(t *testing.T) cmpFixture {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	cl := newHTTPClientWithFactory(func(httpx.Options) (*http.Client, error) {
		return srv.Client(), nil
	})

	rt := rtrun.New(rtrun.Config{Client: cl})
	t.Cleanup(func() { _ = rt.Close() })

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
	return cmpFixture{eng: eng, doc: doc, req: req, calls: &calls}
}

func TestExecuteCompareBuildsRowsWithValidBaseline(t *testing.T) {
	fx := newCmpFixture(t)
	out, err := fx.eng.ExecuteCompare(fx.doc, fx.req, &restfile.CompareSpec{
		Environments: []string{"one", "two"},
		Baseline:     "one",
	}, testSelection(""))
	if err != nil {
		t.Fatalf("ExecuteCompare: %v", err)
	}
	if out == nil {
		t.Fatal("expected compare result")
	}
	if out.Baseline != "one" {
		t.Fatalf("baseline = %q, want one", out.Baseline)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("rows = %+v, want 2", out.Rows)
	}
	if out.Rows[0].Summary != "baseline" {
		t.Fatalf("first summary = %q, want baseline", out.Rows[0].Summary)
	}
	if out.Rows[1].Summary != "match" {
		t.Fatalf("second summary = %q, want match", out.Rows[1].Summary)
	}
	if !strings.Contains(out.Report, "Baseline: one") {
		t.Fatalf("report missing baseline: %q", out.Report)
	}
	if *fx.calls != 2 {
		t.Fatalf("requests = %d, want 2", *fx.calls)
	}
}

func TestExecuteCompareRejectsMissingBaselineBeforeRequest(t *testing.T) {
	fx := newCmpFixture(t)
	_, err := fx.eng.ExecuteCompare(fx.doc, fx.req, &restfile.CompareSpec{
		Environments: []string{"one", "two"},
		Baseline:     "missing",
	}, testSelection(""))
	if err == nil {
		t.Fatal("expected invalid baseline error")
	}
	if *fx.calls != 0 {
		t.Fatalf("compare made %d requests before validation", *fx.calls)
	}
}

func TestExecuteGroupedCompareHoldsOtherGroupsFixed(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path+" "+r.Header.Get("X-Token"))
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	cat, err := vars.NewGroupedCatalog(nil, []vars.Group{
		{
			Name:    "api",
			Default: "dev",
			Profiles: vars.EnvironmentSet{
				"dev":  {"api.path": "dev"},
				"prod": {"api.path": "prod"},
			},
		},
		{
			Name:    "auth",
			Default: "personal",
			Profiles: vars.EnvironmentSet{
				"personal": {"token": "personal"},
				"ci":       {"token": "ci"},
			},
		},
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	sel, err := cat.Select("", map[string]string{"auth": "ci"})
	if err != nil {
		t.Fatalf("selection: %v", err)
	}
	eng := New(engine.Config{Catalog: cat, Selection: sel})
	defer func() { _ = eng.Close() }()
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    srv.URL + "/{{api.path}}",
		Headers: http.Header{
			"X-Token": {"{{token}}"},
		},
	}
	doc := &restfile.Document{Path: "compare.http", Requests: []*restfile.Request{req}}

	out, err := eng.ExecuteCompare(doc, req, &restfile.CompareSpec{
		Environments: []string{"dev", "prod"},
		Baseline:     "prod",
		Group:        "api",
	}, sel)
	if err != nil {
		t.Fatalf("execute compare: %v", err)
	}
	if got, want := calls, []string{"/dev ci", "/prod ci"}; !slices.Equal(got, want) {
		t.Fatalf("requests = %#v, want %#v", got, want)
	}
	if out.Baseline != "prod" || out.Group != "api" || len(out.Rows) != 2 {
		t.Fatalf("compare result = %+v", out)
	}
	for _, row := range out.Rows {
		if row.Selection.Groups()["auth"] != "ci" {
			t.Fatalf("auth profile changed in row %+v", row)
		}
	}
	if out.Rows[1].Summary != "baseline" {
		t.Fatalf("prod summary = %q, want baseline", out.Rows[1].Summary)
	}
	if !strings.Contains(out.Summary, "api=prod, auth=ci*") {
		t.Fatalf("summary missing grouped baseline marker: %q", out.Summary)
	}
}

func TestExecuteGroupedCompareValidatesAllProfilesBeforeRequest(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer srv.Close()

	cat, err := vars.NewGroupedCatalog(nil, []vars.Group{{
		Name:    "api",
		Default: "dev",
		Profiles: vars.EnvironmentSet{
			"dev":  {},
			"prod": {},
		},
	}})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	sel := cat.DefaultSelection()
	eng := New(engine.Config{Catalog: cat, Selection: sel})
	defer func() { _ = eng.Close() }()
	req := &restfile.Request{Method: http.MethodGet, URL: srv.URL}
	doc := &restfile.Document{Path: "compare.http", Requests: []*restfile.Request{req}}

	_, err = eng.ExecuteCompare(doc, req, &restfile.CompareSpec{
		Environments: []string{"dev", "missing"},
		Group:        "api",
	}, sel)
	if err == nil || !strings.Contains(err.Error(), "unknown profile") {
		t.Fatalf("error = %v, want unknown profile", err)
	}
	if calls != 0 {
		t.Fatalf("compare made %d requests before validation", calls)
	}
}
