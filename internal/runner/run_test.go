package runner

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestRunSingleRequestDefaultSelection(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	if err := os.WriteFile(file, []byte("GET https://example.com/status\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					Status:     "204 No Content",
					StatusCode: http.StatusNoContent,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 1 || rep.Passed != 1 || rep.Failed != 0 || rep.Skipped != 0 {
		t.Fatalf("unexpected report counts: %+v", rep)
	}
	var out strings.Builder
	if err := rep.WriteText(&out); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "PASS GET https://example.com/status") {
		t.Fatalf("expected pass line, got %q", text)
	}
	if !strings.Contains(text, "Summary: total=1 passed=1 failed=0 skipped=0") {
		t.Fatalf("expected summary, got %q", text)
	}
}

func TestRequestRunResultUsesExplainMissingVarsAndEffectiveURL(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "{{services.api.base}}/reports",
	}

	item := requestRunResult(req, engine.RequestResult{
		Response: &httpclient.Response{
			Status:       "200 OK",
			StatusCode:   200,
			EffectiveURL: "https://httpbin.org/anything/api/reports",
		},
		Explain: &xplain.Report{
			Vars: []xplain.Var{
				{Name: "services.api.base", Missing: false},
				{Name: "reporting.apiKey", Missing: false},
				{Name: "reporting.token", Missing: true},
			},
		},
	}, "dev")

	if item.Target != "https://httpbin.org/anything/api/reports" {
		t.Fatalf("expected effective target, got %q", item.Target)
	}
	got, ok := item.UnresolvedTemplateVars()
	if !ok {
		t.Fatalf("expected unresolved variable metadata")
	}
	if len(got) != 1 || got[0] != "reporting.token" {
		t.Fatalf("expected only reporting.token to remain unresolved, got %v", got)
	}
}

func TestRunSelectRequestByName(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "many.http")
	src := strings.Join([]string{
		"### One",
		"# @name one",
		"GET https://example.com/one",
		"",
		"### Two",
		"# @name two",
		"GET https://example.com/two",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var seen string
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				seen = req.URL.String()
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("{}")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		Select:        Select{Request: "two"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 1 {
		t.Fatalf("expected one result, got %+v", rep)
	}
	if seen != "https://example.com/two" {
		t.Fatalf("expected second request, got %q", seen)
	}
}

func TestRunSelectRequestByLine(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "many.http")
	src := strings.Join([]string{
		"### One",
		"# @name one",
		"GET https://example.com/one",
		"X-Test: 1",
		"",
		"### Two",
		"# @name two",
		"GET https://example.com/two",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var seen string
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				seen = req.URL.String()
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("{}")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		Select:        Select{Line: 4},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 1 {
		t.Fatalf("expected one result, got %+v", rep)
	}
	if seen != "https://example.com/one" {
		t.Fatalf("expected first request, got %q", seen)
	}
}

func TestRunPersistsCookiesAcrossRequests(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cookies.http")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/"})
		case "/echo":
			if cookie, err := r.Cookie("session"); err == nil {
				_, _ = io.WriteString(w, cookie.String())
			}
		}
	}))
	defer srv.Close()

	src := strings.Join([]string{
		"### Set",
		"GET " + srv.URL + "/set",
		"",
		"### Echo",
		"GET " + srv.URL + "/echo",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Select:        Select{All: true},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Results) != 2 || rep.Results[1].Response == nil {
		t.Fatalf("expected two HTTP results, got %+v", rep.Results)
	}
	if got := strings.TrimSpace(string(rep.Results[1].Response.Body)); got != "session=abc123" {
		t.Fatalf("expected persisted cookie on second request, got %q", got)
	}
}

func TestRunNoCookiesSettingSkipsJarForRequest(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cookies.http")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/"})
		case "/echo":
			if cookie, err := r.Cookie("session"); err == nil {
				_, _ = io.WriteString(w, cookie.String())
			}
		}
	}))
	defer srv.Close()

	src := strings.Join([]string{
		"### Set",
		"GET " + srv.URL + "/set",
		"",
		"### Echo",
		"# @setting no-cookies true",
		"GET " + srv.URL + "/echo",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Select:        Select{All: true},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Results) != 2 || rep.Results[1].Response == nil {
		t.Fatalf("expected two HTTP results, got %+v", rep.Results)
	}
	if got := strings.TrimSpace(string(rep.Results[1].Response.Body)); got != "" {
		t.Fatalf("expected no cookie on second request, got %q", got)
	}
}

func TestRunUsesWorkspaceGlobalPatchProfile(t *testing.T) {
	dir := t.TempDir()
	defs := filepath.Join(dir, "defs.http")
	use := filepath.Join(dir, "use.http")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Test"); got != "1" {
			http.Error(w, "missing header", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	if err := os.WriteFile(
		defs,
		[]byte(`# @patch global addHdr {headers: {"X-Test": "1"}}`),
		0o644,
	); err != nil {
		t.Fatalf("write defs: %v", err)
	}
	if err := os.WriteFile(
		use,
		[]byte(strings.Join([]string{
			"# @name use",
			"# @apply use=addHdr",
			"GET " + srv.URL,
			"",
		}, "\n")),
		0o644,
	); err != nil {
		t.Fatalf("write use: %v", err)
	}

	rep, err := Run(Options{
		FilePath:      use,
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Success() {
		t.Fatalf("expected report to pass, got %+v", rep)
	}
	if len(rep.Results) != 1 || rep.Results[0].Response == nil {
		t.Fatalf("expected one HTTP result, got %+v", rep.Results)
	}
	if got := rep.Results[0].Response.StatusCode; got != http.StatusOK {
		t.Fatalf("unexpected status code %d", got)
	}
}

func TestRunUsesWorkspaceGlobalInheritedAuth(t *testing.T) {
	dir := t.TempDir()
	defs := filepath.Join(dir, "defs.http")
	use := filepath.Join(dir, "use.http")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer global-token" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	if err := os.WriteFile(defs, []byte(`# @auth global bearer global-token`), 0o644); err != nil {
		t.Fatalf("write defs: %v", err)
	}
	if err := os.WriteFile(
		use,
		[]byte(strings.Join([]string{
			"# @name use",
			"GET " + srv.URL,
			"",
		}, "\n")),
		0o644,
	); err != nil {
		t.Fatalf("write use: %v", err)
	}

	rep, err := Run(Options{
		FilePath:      use,
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Success() {
		t.Fatalf("expected report to pass, got %+v", rep)
	}
	if len(rep.Results) != 1 || rep.Results[0].Response == nil {
		t.Fatalf("expected one HTTP result, got %+v", rep.Results)
	}
	if got := rep.Results[0].Response.StatusCode; got != http.StatusOK {
		t.Fatalf("unexpected status code %d", got)
	}
}

func TestRunRejectsMultipleRequestsWithoutSelector(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "many.http")
	src := strings.Join([]string{
		"### One",
		"GET https://example.com/one",
		"",
		"### Two",
		"GET https://example.com/two",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := Run(Options{FilePath: file, WorkspaceRoot: dir})
	if err == nil {
		t.Fatalf("expected selector error")
	}
	if !strings.Contains(err.Error(), "use --request, --tag, --line, or --all") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsLineSelectorConflict(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "many.http")
	src := strings.Join([]string{
		"### One",
		"GET https://example.com/one",
		"",
		"### Two",
		"GET https://example.com/two",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Select:        Select{Request: "two", Line: 5},
	})
	if err == nil {
		t.Fatalf("expected selector conflict")
	}
	if !strings.Contains(err.Error(), "--line cannot be combined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCountsFailedAsserts(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "assert.http")
	src := strings.Join([]string{
		"### Assert",
		"# @name bad",
		"# @assert response.statusCode == 201",
		"GET https://example.com/assert",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("{}")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 1 || rep.Failed != 1 {
		t.Fatalf("expected failed result, got %+v", rep)
	}
	if rep.Success() {
		t.Fatalf("expected report to fail")
	}
}

func TestRunWorkflowByName(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "workflow.http")
	src := strings.Join([]string{
		"# @workflow demo",
		"# @step Login using=login expect.statuscode=200",
		"# @step Use using=use expect.statuscode=200",
		"",
		"### Login",
		"# @name login",
		"# @capture global auth.token {{response.json.token}}",
		"GET https://example.com/login",
		"",
		"### Use",
		"# @name use",
		"GET https://example.com/use",
		"Authorization: Bearer {{auth.token}}",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var auth string
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				hdr := make(http.Header)
				body := "{}"
				if req.URL.Path == "/login" {
					hdr.Set("Content-Type", "application/json")
					body = `{"token":"wf-123"}`
				}
				if req.URL.Path == "/use" {
					auth = req.Header.Get("Authorization")
				}
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     hdr,
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		Select:        Select{Workflow: "demo"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 1 || rep.Passed != 1 {
		t.Fatalf("unexpected report counts: %+v", rep)
	}
	if auth != "Bearer wf-123" {
		t.Fatalf("expected workflow capture to feed next step, got %q", auth)
	}
	if len(rep.Results) != 1 || rep.Results[0].Kind != ResultKindWorkflow {
		t.Fatalf("expected workflow result, got %+v", rep.Results)
	}
	if len(rep.Results[0].Steps) != 2 {
		t.Fatalf("expected two workflow steps, got %+v", rep.Results[0].Steps)
	}

	var out strings.Builder
	if err := rep.WriteText(&out); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "PASS WORKFLOW demo") {
		t.Fatalf("expected workflow pass line, got %q", text)
	}
	if !strings.Contains(text, "1. PASS Login") {
		t.Fatalf("expected workflow step output, got %q", text)
	}

}

func TestRunWorkflowByLine(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "workflow.http")
	src := strings.Join([]string{
		"# @workflow demo",
		"# @step Login using=login expect.statuscode=200",
		"# @step Use using=use expect.statuscode=200",
		"",
		"### Login",
		"# @name login",
		"# @capture global auth.token {{response.json.token}}",
		"GET https://example.com/login",
		"",
		"### Use",
		"# @name use",
		"GET https://example.com/use",
		"Authorization: Bearer {{auth.token}}",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var auth string
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				hdr := make(http.Header)
				body := "{}"
				if req.URL.Path == "/login" {
					hdr.Set("Content-Type", "application/json")
					body = `{"token":"wf-123"}`
				}
				if req.URL.Path == "/use" {
					auth = req.Header.Get("Authorization")
				}
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     hdr,
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		Select:        Select{Line: 2},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 1 || rep.Passed != 1 {
		t.Fatalf("unexpected report counts: %+v", rep)
	}
	if auth != "Bearer wf-123" {
		t.Fatalf("expected workflow capture to feed next step, got %q", auth)
	}
	if len(rep.Results) != 1 || rep.Results[0].Kind != ResultKindWorkflow {
		t.Fatalf("expected workflow result, got %+v", rep.Results)
	}
}

func TestRunRequestForEach(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "each.http")
	src := strings.Join([]string{
		"@items = [\"a\",\"b\",\"c\"]",
		"",
		"### Each",
		"# @name each",
		"# @for-each item in json.parse(vars.require(\"items\"))",
		"GET https://example.com/items/{{vars.request.item}}",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var seen []string
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				seen = append(seen, req.URL.Path)
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("{}")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		Select:        Select{Request: "each"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 1 || rep.Passed != 1 {
		t.Fatalf("unexpected report counts: %+v", rep)
	}
	if want := []string{
		"/items/a",
		"/items/b",
		"/items/c",
	}; strings.Join(
		seen,
		",",
	) != strings.Join(
		want,
		",",
	) {
		t.Fatalf("unexpected iteration targets: got %v want %v", seen, want)
	}
	if len(rep.Results) != 1 || rep.Results[0].Kind != ResultKindForEach {
		t.Fatalf("expected for-each result, got %+v", rep.Results)
	}
	if len(rep.Results[0].Steps) != 3 {
		t.Fatalf("expected three for-each iterations, got %+v", rep.Results[0].Steps)
	}
}

func TestRunAllCarriesJSPreRequestGlobals(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "js.http")
	src := strings.Join([]string{
		"### Seed",
		"# @name seed",
		"# @script pre-request",
		"> vars.global.set(\"auth.token\", \"js-123\", {secret: false});",
		"GET https://example.com/seed",
		"",
		"### Use",
		"# @name use",
		"GET https://example.com/use",
		"Authorization: Bearer {{auth.token}}",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var auth string
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/use" {
					auth = req.Header.Get("Authorization")
				}
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("{}")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		Select:        Select{All: true},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 2 || rep.Passed != 2 {
		t.Fatalf("unexpected report counts: %+v", rep)
	}
	if auth != "Bearer js-123" {
		t.Fatalf("expected runtime global in second request, got %q", auth)
	}
}

func TestRunAllCarriesCapturesAcrossRequests(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "capture.http")
	src := strings.Join([]string{
		"### Login",
		"# @name login",
		"# @capture global auth.token {{response.json.token}}",
		"GET https://example.com/login",
		"",
		"### Use",
		"# @name use",
		"GET https://example.com/use",
		"Authorization: Bearer {{auth.token}}",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var auth string
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				body := "{}"
				hdr := make(http.Header)
				if req.URL.Path == "/login" {
					hdr.Set("Content-Type", "application/json")
					body = `{"token":"cap-123"}`
				}
				if req.URL.Path == "/use" {
					auth = req.Header.Get("Authorization")
				}
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     hdr,
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		Select:        Select{All: true},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 2 || rep.Passed != 2 {
		t.Fatalf("unexpected report counts: %+v", rep)
	}
	if auth != "Bearer cap-123" {
		t.Fatalf("expected captured global in second request, got %q", auth)
	}
}

func TestRunAllCarriesRTSPreRequestGlobals(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "rts.http")
	src := strings.Join([]string{
		"### Seed",
		"# @name seed",
		"# @script pre-request lang=rts",
		"> vars.global.set(\"mode\", \"rts\", false)",
		"GET https://example.com/seed",
		"",
		"### Use",
		"# @name use",
		"GET https://example.com/use",
		"X-Mode: {{mode}}",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var mode string
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/use" {
					mode = req.Header.Get("X-Mode")
				}
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("{}")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		Select:        Select{All: true},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 2 || rep.Passed != 2 {
		t.Fatalf("unexpected report counts: %+v", rep)
	}
	if mode != "rts" {
		t.Fatalf("expected RTS runtime global in second request, got %q", mode)
	}
}

func TestReportWriteJSON(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "json.http")
	src := strings.Join([]string{
		"### JSON",
		"# @name json",
		"# @script pre-request lang=rts",
		"> request.setMethod(\"POST\")",
		"> request.setURL(\"https://example.com/json?mode=1\")",
		"# @assert response.statusCode == 200",
		"GET https://example.com/original",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("{}")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		Version:       "test",
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var out strings.Builder
	if err := rep.WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got struct {
		Version string `json:"version"`
		Summary struct {
			Total  int `json:"total"`
			Passed int `json:"passed"`
		} `json:"summary"`
		Results []struct {
			Name   string `json:"name"`
			Method string `json:"method"`
			Target string `json:"target"`
			Status string `json:"status"`
			HTTP   struct {
				StatusCode int `json:"statusCode"`
			} `json:"http"`
			Tests []struct {
				Name   string `json:"name"`
				Passed bool   `json:"passed"`
			} `json:"tests"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if got.Version != "test" {
		t.Fatalf("expected version test, got %q", got.Version)
	}
	if got.Summary.Total != 1 || got.Summary.Passed != 1 {
		t.Fatalf("unexpected summary: %+v", got.Summary)
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected one result, got %+v", got.Results)
	}
	item := got.Results[0]
	if item.Name != "json" || item.Method != "POST" ||
		item.Target != "https://example.com/json?mode=1" {
		t.Fatalf("unexpected result identity: %+v", item)
	}
	if item.Status != "pass" || item.HTTP.StatusCode != http.StatusOK {
		t.Fatalf("unexpected result status: %+v", item)
	}
	if len(item.Tests) != 1 || item.Tests[0].Name != "response.statusCode == 200" ||
		!item.Tests[0].Passed {
		t.Fatalf("unexpected tests payload: %+v", item.Tests)
	}
}

func TestWorkflowWriteJSONIncludesSteps(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "workflow-json.http")
	src := strings.Join([]string{
		"# @workflow demo",
		"# @step Login using=login expect.statuscode=200",
		"# @step Use using=use expect.statuscode=200",
		"",
		"### Login",
		"# @name login",
		"# @capture global auth.token {{response.json.token}}",
		"GET https://example.com/login",
		"",
		"### Use",
		"# @name use",
		"GET https://example.com/use",
		"Authorization: Bearer {{auth.token}}",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				hdr := make(http.Header)
				body := "{}"
				if req.URL.Path == "/login" {
					hdr.Set("Content-Type", "application/json")
					body = `{"token":"wf-123"}`
				}
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     hdr,
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		Version:       "test",
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		Select:        Select{Workflow: "demo"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	var out strings.Builder
	if err := rep.WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	var got struct {
		Results []struct {
			Kind   string `json:"kind"`
			Status string `json:"status"`
			Steps  []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"steps"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected one workflow result, got %+v", got.Results)
	}
	item := got.Results[0]
	if item.Kind != "workflow" || item.Status != "pass" {
		t.Fatalf("unexpected workflow json: %+v", item)
	}
	if len(item.Steps) != 2 || item.Steps[0].Name != "Login" || item.Steps[0].Status != "pass" {
		t.Fatalf("unexpected workflow steps: %+v", item.Steps)
	}
}

func TestRunAllCarriesStreamCapturesAndArtifacts(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "stream.http")
	artifacts := filepath.Join(dir, "artifacts")
	src := strings.Join([]string{
		"### Events",
		"# @name events",
		"# @sse max-events=1",
		"# @assert stream.summary().eventCount == 1",
		"# @capture global saved.count {{stream.summary.eventCount}}",
		"GET https://example.com/events",
		"",
		"### Use",
		"# @name use",
		"# @assert response.statusCode == 200",
		"GET https://example.com/use",
		"X-Stream-Count: {{saved.count}}",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var gotHdr string
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				hdr := make(http.Header)
				body := "{}"
				if req.URL.Path == "/events" {
					hdr.Set("Content-Type", "text/event-stream")
					body = "data: hello\n\n"
				}
				if req.URL.Path == "/use" {
					gotHdr = req.Header.Get("X-Stream-Count")
				}
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     hdr,
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		Version:       "test",
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		ArtifactDir:   artifacts,
		Select:        Select{All: true},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotHdr != "1" {
		t.Fatalf("expected stream capture to feed next request, got %q", gotHdr)
	}
	if len(rep.Results) != 2 {
		t.Fatalf("expected two results, got %+v", rep.Results)
	}
	stream := rep.Results[0].Stream
	if stream == nil || stream.Kind != "sse" || stream.EventCount != 1 {
		t.Fatalf("unexpected stream summary: %+v", stream)
	}
	if stream.TranscriptPath == "" {
		t.Fatalf("expected transcript path on stream result")
	}
	data, err := os.ReadFile(stream.TranscriptPath)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	var raw struct {
		Summary struct {
			EventCount int `json:"eventCount"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal transcript: %v", err)
	}
	if raw.Summary.EventCount != 1 {
		t.Fatalf("unexpected transcript summary: %+v", raw.Summary)
	}

	var out strings.Builder
	if err := rep.WriteJSON(&out); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got struct {
		Results []struct {
			Name   string `json:"name"`
			Stream struct {
				Kind           string `json:"kind"`
				EventCount     int    `json:"eventCount"`
				TranscriptPath string `json:"transcriptPath"`
			} `json:"stream"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("unmarshal report json: %v", err)
	}
	if len(got.Results) != 2 {
		t.Fatalf("expected two json results, got %+v", got.Results)
	}
	if got.Results[0].Name != "events" || got.Results[0].Stream.Kind != "sse" ||
		got.Results[0].Stream.EventCount != 1 {
		t.Fatalf("unexpected first json result: %+v", got.Results[0])
	}
	if got.Results[0].Stream.TranscriptPath == "" {
		t.Fatalf("expected transcript path in json result")
	}
}

func TestRunCompareFromCLIFlags(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "compare.http")
	src := strings.Join([]string{
		"### Compare",
		"# @name cmp",
		"GET https://{{host}}/status",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var seen []string
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				seen = append(seen, req.URL.Host)
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(req.URL.Host)),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		Version:       "test",
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
		EnvSet: vars.EnvironmentSet{
			"dev":   {"host": "dev.example.com"},
			"stage": {"host": "stage.example.com"},
		},
		CompareTargets: []string{"dev", "stage"},
		CompareBase:    "stage",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := []string{
		"dev.example.com",
		"stage.example.com",
	}; strings.Join(
		seen,
		",",
	) != strings.Join(
		want,
		",",
	) {
		t.Fatalf("unexpected compare targets: got %v want %v", seen, want)
	}
	if rep.Total != 1 || rep.Passed != 1 {
		t.Fatalf("unexpected report counts: %+v", rep)
	}
	if len(rep.Results) != 1 {
		t.Fatalf("expected one result, got %+v", rep.Results)
	}
	item := rep.Results[0]
	if item.Kind != ResultKindCompare {
		t.Fatalf("expected compare result, got %+v", item)
	}
	if item.Compare == nil || item.Compare.Baseline != "stage" {
		t.Fatalf("unexpected compare info: %+v", item.Compare)
	}
	if len(item.Steps) != 2 || item.Steps[0].Environment != "dev" ||
		item.Steps[1].Environment != "stage" {
		t.Fatalf("unexpected compare steps: %+v", item.Steps)
	}

	var text strings.Builder
	if err := rep.WriteText(&text); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	if !strings.Contains(text.String(), "PASS COMPARE cmp") {
		t.Fatalf("expected compare text output, got %q", text.String())
	}

	var raw strings.Builder
	if err := rep.WriteJSON(&raw); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got struct {
		Results []struct {
			Kind    string `json:"kind"`
			Compare struct {
				Baseline string `json:"baseline"`
			} `json:"compare"`
			Steps []struct {
				Environment string `json:"environment"`
				Status      string `json:"status"`
			} `json:"steps"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Kind != "compare" {
		t.Fatalf("unexpected compare json: %+v", got.Results)
	}
	if got.Results[0].Compare.Baseline != "stage" {
		t.Fatalf("unexpected compare baseline: %+v", got.Results[0].Compare)
	}
	if len(got.Results[0].Steps) != 2 || got.Results[0].Steps[0].Environment != "dev" {
		t.Fatalf("unexpected compare steps json: %+v", got.Results[0].Steps)
	}
}

func TestRunProfileMetadata(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "profile.http")
	src := strings.Join([]string{
		"### Profile",
		"# @name prof",
		"# @profile count=3 warmup=1",
		"GET https://example.com/profile",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	count := 0
	client := httpclient.NewClient(nil)
	client.SetHTTPFactory(func(httpclient.Options) (*http.Client, error) {
		return &http.Client{
			Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				count++
				return &http.Response{
					Status:     "200 OK",
					StatusCode: http.StatusOK,
					Proto:      "HTTP/1.1",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("{}")),
					Request:    req,
				}, nil
			}),
		}, nil
	})

	rep, err := Run(Options{
		Version:       "test",
		FilePath:      file,
		WorkspaceRoot: dir,
		Client:        client,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4 profile requests, got %d", count)
	}
	if rep.Total != 1 || rep.Passed != 1 {
		t.Fatalf("unexpected report counts: %+v", rep)
	}
	if len(rep.Results) != 1 {
		t.Fatalf("expected one result, got %+v", rep.Results)
	}
	item := rep.Results[0]
	if item.Kind != ResultKindProfile {
		t.Fatalf("expected profile result, got %+v", item)
	}
	if item.Profile == nil || item.Profile.Results == nil {
		t.Fatalf("expected profile summary, got %+v", item.Profile)
	}
	if item.Profile.Results.TotalRuns != 4 || item.Profile.Results.WarmupRuns != 1 {
		t.Fatalf("unexpected profile totals: %+v", item.Profile.Results)
	}
	if item.Profile.Results.SuccessfulRuns != 3 || item.Profile.Results.FailedRuns != 0 {
		t.Fatalf("unexpected profile outcomes: %+v", item.Profile.Results)
	}

	var text strings.Builder
	if err := rep.WriteText(&text); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	if !strings.Contains(text.String(), "PASS PROFILE prof") {
		t.Fatalf("expected profile text output, got %q", text.String())
	}

	var raw strings.Builder
	if err := rep.WriteJSON(&raw); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got struct {
		Results []struct {
			Kind    string `json:"kind"`
			Profile struct {
				TotalRuns      int `json:"totalRuns"`
				WarmupRuns     int `json:"warmupRuns"`
				SuccessfulRuns int `json:"successfulRuns"`
				FailedRuns     int `json:"failedRuns"`
			} `json:"profile"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Kind != "profile" {
		t.Fatalf("unexpected profile json: %+v", got.Results)
	}
	if got.Results[0].Profile.TotalRuns != 4 || got.Results[0].Profile.SuccessfulRuns != 3 {
		t.Fatalf("unexpected profile json payload: %+v", got.Results[0].Profile)
	}
}

func TestRunPersistGlobalsAcrossInvocations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"token":"persist-123"}`)
		case "/use":
			if got := r.Header.Get("Authorization"); got != "Bearer persist-123" {
				http.Error(w, "missing auth", http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	seedFile := filepath.Join(dir, "seed.http")
	useFile := filepath.Join(dir, "use.http")
	seedSrc := strings.Join([]string{
		"# @name seed",
		"# @capture global auth.token {{response.json.token}}",
		"GET " + srv.URL + "/login",
		"",
	}, "\n")
	useSrc := strings.Join([]string{
		"# @name use",
		"GET " + srv.URL + "/use",
		"Authorization: Bearer {{auth.token}}",
		"",
	}, "\n")
	if err := os.WriteFile(seedFile, []byte(seedSrc), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	if err := os.WriteFile(useFile, []byte(useSrc), 0o644); err != nil {
		t.Fatalf("write use file: %v", err)
	}

	first, err := Run(Options{
		FilePath:       seedFile,
		WorkspaceRoot:  dir,
		StateDir:       stateDir,
		PersistGlobals: true,
	})
	if err != nil {
		t.Fatalf("Run(seed): %v", err)
	}
	if !first.Success() {
		t.Fatalf("expected seed run to pass, got %+v", first)
	}

	second, err := Run(Options{
		FilePath:       useFile,
		WorkspaceRoot:  dir,
		StateDir:       stateDir,
		PersistGlobals: true,
	})
	if err != nil {
		t.Fatalf("Run(use): %v", err)
	}
	if !second.Success() {
		t.Fatalf("expected persisted globals run to pass, got %+v", second)
	}
}

func TestRunPersistOAuthAcrossInvocations(t *testing.T) {
	var tokenCalls int
	var authHdr string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			tokenCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(
				w,
				`{"access_token":"oauth-123","token_type":"Bearer","expires_in":3600}`,
			)
		case "/seed":
			_, _ = io.WriteString(w, `{"ok":true}`)
		case "/use":
			authHdr = r.Header.Get("Authorization")
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	seedFile := filepath.Join(dir, "seed.http")
	useFile := filepath.Join(dir, "use.http")
	seedSrc := strings.Join([]string{
		"# @name seed",
		"# @auth request oauth2 token_url=\"" + srv.URL + "/token\" client_id=ci cache_key=ci-api",
		"GET " + srv.URL + "/seed",
		"",
	}, "\n")
	useSrc := strings.Join([]string{
		"# @name use",
		"# @auth request oauth2 cache_key=ci-api",
		"GET " + srv.URL + "/use",
		"",
	}, "\n")
	if err := os.WriteFile(seedFile, []byte(seedSrc), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	if err := os.WriteFile(useFile, []byte(useSrc), 0o644); err != nil {
		t.Fatalf("write use file: %v", err)
	}

	first, err := Run(Options{
		FilePath:      seedFile,
		WorkspaceRoot: dir,
		StateDir:      stateDir,
		PersistAuth:   true,
	})
	if err != nil {
		t.Fatalf("Run(seed): %v", err)
	}
	if !first.Success() {
		t.Fatalf("expected oauth seed run to pass, got %+v", first)
	}

	second, err := Run(Options{
		FilePath:      useFile,
		WorkspaceRoot: dir,
		StateDir:      stateDir,
		PersistAuth:   true,
	})
	if err != nil {
		t.Fatalf("Run(use): %v", err)
	}
	if !second.Success() {
		t.Fatalf("expected oauth cached run to pass, got %+v", second)
	}
	if tokenCalls != 1 {
		t.Fatalf("expected one oauth token fetch across runs, got %d", tokenCalls)
	}
	if authHdr != "Bearer oauth-123" {
		t.Fatalf("expected cached oauth header, got %q", authHdr)
	}
}

func TestRunFailsOnTraceBudgetBreachAndWritesTraceArtifacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(25 * time.Millisecond)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "trace.http")
	artifacts := filepath.Join(dir, "artifacts")
	src := strings.Join([]string{
		"# @name slow",
		"# @trace total<=1ms",
		"GET " + srv.URL,
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		ArtifactDir:   artifacts,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Success() || rep.Failed != 1 {
		t.Fatalf("expected trace failure report, got %+v", rep)
	}
	if len(rep.Results) != 1 || rep.Results[0].Trace == nil || rep.Results[0].Trace.Summary == nil {
		t.Fatalf("expected trace summary on result, got %+v", rep.Results)
	}
	if len(rep.Results[0].Trace.Summary.Breaches) == 0 {
		t.Fatalf("expected trace budget breaches, got %+v", rep.Results[0].Trace.Summary)
	}
	if rep.Results[0].Trace.ArtifactPath == "" {
		t.Fatalf("expected trace artifact path")
	}
	if _, err := os.Stat(rep.Results[0].Trace.ArtifactPath); err != nil {
		t.Fatalf("stat trace artifact: %v", err)
	}

	var text strings.Builder
	if err := rep.WriteText(&text); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	if !strings.Contains(text.String(), "trace budget breach") {
		t.Fatalf("expected trace failure text, got %q", text.String())
	}

	var raw strings.Builder
	if err := rep.WriteJSON(&raw); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got struct {
		Results []struct {
			Status string `json:"status"`
			Trace  struct {
				ArtifactPath string `json:"artifactPath"`
				Breaches     []struct {
					Kind string `json:"kind"`
				} `json:"breaches"`
			} `json:"trace"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw.String()), &got); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if len(got.Results) != 1 || got.Results[0].Status != "fail" {
		t.Fatalf("unexpected trace json result: %+v", got.Results)
	}
	if got.Results[0].Trace.ArtifactPath == "" || len(got.Results[0].Trace.Breaches) == 0 {
		t.Fatalf("expected trace json payload, got %+v", got.Results[0].Trace)
	}
}

func TestRunPersistsHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	file := filepath.Join(dir, "history.http")
	src := strings.Join([]string{
		"# @name hist",
		"GET " + srv.URL,
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
		StateDir:      stateDir,
		History:       true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Success() {
		t.Fatalf("expected history run to pass, got %+v", rep)
	}

	store := histdb.New(filepath.Join(stateDir, "history.db"))
	t.Cleanup(func() { _ = store.Close() })
	entries, err := store.Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one history entry, got %d", len(entries))
	}
	if entries[0].RequestName != "hist" {
		t.Fatalf("unexpected history entry: %+v", entries[0])
	}
}

func TestRunRejectsUnseededHeadlessOAuthAuthorizationCode(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "authcode.http")
	src := strings.Join([]string{
		"# @name authcode",
		"# @auth request oauth2 token_url=\"https://auth.local/token\" auth_url=\"https://auth.local/auth\" client_id=ci grant=authorization_code cache_key=ci-api",
		"GET " + srv.URL,
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rep, err := Run(Options{
		FilePath:      file,
		WorkspaceRoot: dir,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Success() || rep.Failed != 1 {
		t.Fatalf("expected auth-code failure, got %+v", rep)
	}
	if len(rep.Results) != 1 || rep.Results[0].Err == nil {
		t.Fatalf("expected request error result, got %+v", rep.Results)
	}
	if !strings.Contains(
		rep.Results[0].Err.Error(),
		"headless oauth authorization_code requires a cached or refreshable token",
	) {
		t.Fatalf("unexpected oauth error: %v", rep.Results[0].Err)
	}
	if calls != 0 {
		t.Fatalf("expected request to stop before network call, got %d calls", calls)
	}
}
