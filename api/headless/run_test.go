package headless

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestRunRequestParity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "ok.http")
	src := fmt.Sprintf("# @name ok\nGET %s\n", srv.URL)
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	path, err := filepath.Abs(file)
	if err != nil {
		t.Fatalf("abs file: %v", err)
	}

	got, err := Run(context.Background(), Opt{FilePath: file})
	if err != nil {
		t.Fatalf("public Run: %v", err)
	}
	want, err := runner.RunContext(context.Background(), runner.Options{
		FilePath:      path,
		WorkspaceRoot: dir,
		HTTPOptions: httpclient.Options{
			Timeout:         defTimeout,
			FollowRedirects: true,
		},
		GRPCOptions: grpcclient.Options{
			DefaultPlaintext:    true,
			DefaultPlaintextSet: true,
		},
	})
	if err != nil {
		t.Fatalf("runner RunContext: %v", err)
	}
	assertReportParity(t, reportFromRunner(want), got)
}

func TestRunCompareParityWithEnvResolve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dev":
			fmt.Fprint(w, `{"env":"dev"}`)
		case "/stage":
			fmt.Fprint(w, `{"env":"stage"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "cmp.http")
	envFile := filepath.Join(dir, "rest-client.env.json")
	src := fmt.Sprintf("# @name cmp\nGET %s/{{path}}\n", srv.URL)
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	envRaw := `{
  "dev": {"path": "dev"},
  "stage": {"path": "stage"}
}`
	if err := os.WriteFile(envFile, []byte(envRaw), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	path, err := filepath.Abs(file)
	if err != nil {
		t.Fatalf("abs file: %v", err)
	}

	got, err := Run(context.Background(), Opt{
		FilePath:       file,
		CompareTargets: []string{"dev", "stage"},
		CompareBase:    "stage",
	})
	if err != nil {
		t.Fatalf("public Run: %v", err)
	}

	envs, err := vars.LoadEnvironmentFile(envFile)
	if err != nil {
		t.Fatalf("load env file: %v", err)
	}
	envName, _ := cli.SelectDefaultEnvironment(envs)
	want, err := runner.RunContext(context.Background(), runner.Options{
		FilePath:        path,
		WorkspaceRoot:   dir,
		EnvSet:          envs,
		EnvName:         envName,
		EnvironmentFile: envFile,
		CompareTargets:  []string{"dev", "stage"},
		CompareBase:     "stage",
		HTTPOptions: httpclient.Options{
			Timeout:         defTimeout,
			FollowRedirects: true,
		},
		GRPCOptions: grpcclient.Options{
			DefaultPlaintext:    true,
			DefaultPlaintextSet: true,
		},
	})
	if err != nil {
		t.Fatalf("runner RunContext: %v", err)
	}
	assertReportParity(t, reportFromRunner(want), got)
}

func TestRunUsageError(t *testing.T) {
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

	_, err := Run(context.Background(), Opt{FilePath: file})
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !IsUsageError(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestRunUsesContext(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "ctx.http")
	src := "GET http://127.0.0.1:1/nope\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	path, err := filepath.Abs(file)
	if err != nil {
		t.Fatalf("abs file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := Run(ctx, Opt{FilePath: file, HTTP: HTTPOpt{Timeout: 2 * time.Second}})
	if err != nil {
		t.Fatalf("public Run: %v", err)
	}
	want, err := runner.RunContext(ctx, runner.Options{
		FilePath:      path,
		WorkspaceRoot: dir,
		HTTPOptions: httpclient.Options{
			Timeout:         2 * time.Second,
			FollowRedirects: true,
		},
		GRPCOptions: grpcclient.Options{
			DefaultPlaintext:    true,
			DefaultPlaintextSet: true,
		},
	})
	if err != nil {
		t.Fatalf("runner RunContext: %v", err)
	}
	assertReportParity(t, reportFromRunner(want), got)
	if got.Failed != 1 || len(got.Results) != 1 {
		t.Fatalf("unexpected canceled report: %+v", got)
	}
	if msg := strings.ToLower(got.Results[0].Error); !strings.Contains(msg, "canceled") {
		t.Fatalf("expected canceled error, got %+v", got.Results[0])
	}
}

func TestRunnerOptUsesHeadlessDefaults(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	got, err := runnerOpt(Opt{FilePath: file})
	if err != nil {
		t.Fatalf("runnerOpt: %v", err)
	}
	if got.HTTPOptions.Timeout != defTimeout || !got.HTTPOptions.FollowRedirects {
		t.Fatalf("unexpected http defaults: %+v", got.HTTPOptions)
	}
	if !got.GRPCOptions.DefaultPlaintext || !got.GRPCOptions.DefaultPlaintextSet {
		t.Fatalf("unexpected grpc defaults: %+v", got.GRPCOptions)
	}
}

func TestRunnerOptRespectsExplicitBoolOptions(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	follow := false
	plain := false
	got, err := runnerOpt(Opt{
		FilePath: file,
		HTTP:     HTTPOpt{Follow: &follow},
		GRPC:     GRPCOpt{Plaintext: &plain},
	})
	if err != nil {
		t.Fatalf("runnerOpt: %v", err)
	}
	if got.HTTPOptions.FollowRedirects {
		t.Fatalf("expected follow=false, got %+v", got.HTTPOptions)
	}
	if got.GRPCOptions.DefaultPlaintext {
		t.Fatalf("expected plaintext=false, got %+v", got.GRPCOptions)
	}
}

func assertReportParity(t *testing.T, want, got *Report) {
	t.Helper()
	want = stableReport(want)
	got = stableReport(got)
	var wantJSON strings.Builder
	if err := want.WriteJSON(&wantJSON); err != nil {
		t.Fatalf("want WriteJSON: %v", err)
	}
	var gotJSON strings.Builder
	if err := got.WriteJSON(&gotJSON); err != nil {
		t.Fatalf("got WriteJSON: %v", err)
	}
	if gotJSON.String() != wantJSON.String() {
		t.Fatalf("report mismatch\nwant:\n%s\ngot:\n%s", wantJSON.String(), gotJSON.String())
	}
}

func stableReport(src *Report) *Report {
	if src == nil {
		return nil
	}
	out := *src
	out.StartedAt = time.Time{}
	out.EndedAt = time.Time{}
	out.Duration = 0
	if len(src.Results) == 0 {
		out.Results = nil
		return &out
	}
	out.Results = make([]Result, 0, len(src.Results))
	for _, item := range src.Results {
		out.Results = append(out.Results, stableResult(item))
	}
	return &out
}

func stableResult(src Result) Result {
	out := src
	out.Duration = 0
	if len(src.Steps) == 0 {
		out.Steps = nil
		return out
	}
	out.Steps = make([]Step, 0, len(src.Steps))
	for _, step := range src.Steps {
		step.Duration = 0
		out.Steps = append(out.Steps, step)
	}
	return out
}
