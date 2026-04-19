package headless

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestRunRequestParity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
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

	got, err := Run(context.Background(), Options{
		Source: Source{Path: file},
	})
	if err != nil {
		t.Fatalf("public Run: %v", err)
	}
	want, err := runner.RunContext(context.Background(), runner.Options{
		FilePath:      path,
		WorkspaceRoot: dir,
		HTTPOptions: httpclient.Options{
			Timeout:         DefaultHTTPTimeout,
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
			if _, err := fmt.Fprint(w, `{"env":"dev"}`); err != nil {
				t.Fatalf("write response: %v", err)
			}
		case "/stage":
			if _, err := fmt.Fprint(w, `{"env":"stage"}`); err != nil {
				t.Fatalf("write response: %v", err)
			}
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

	got, err := Run(context.Background(), Options{
		Source: Source{Path: file},
		Compare: CompareOptions{
			Targets: []string{"dev", "stage"},
			Base:    "stage",
		},
	})
	if err != nil {
		t.Fatalf("public Run: %v", err)
	}

	envs, err := vars.LoadEnvironmentFile(envFile)
	if err != nil {
		t.Fatalf("load env file: %v", err)
	}
	envName := vars.DefaultEnvironment(envs)
	want, err := runner.RunContext(context.Background(), runner.Options{
		FilePath:        path,
		WorkspaceRoot:   dir,
		EnvSet:          envs,
		EnvName:         envName,
		EnvironmentFile: envFile,
		CompareTargets:  []string{"dev", "stage"},
		CompareBase:     "stage",
		HTTPOptions: httpclient.Options{
			Timeout:         DefaultHTTPTimeout,
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

	_, err := Run(context.Background(), Options{
		Source: Source{Path: file},
	})
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

	got, err := Run(ctx, Options{
		Source: Source{Path: file},
		HTTP: HTTPOptions{
			Timeout: 2 * time.Second,
		},
	})
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

func TestBuildOptionsUseDefaults(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	got, err := buildOptions(Options{
		Source: Source{Path: file},
	})
	if err != nil {
		t.Fatalf("buildOptions: %v", err)
	}
	if got.HTTPOptions.Timeout != DefaultHTTPTimeout || !got.HTTPOptions.FollowRedirects {
		t.Fatalf("unexpected http defaults: %+v", got.HTTPOptions)
	}
	if !got.GRPCOptions.DefaultPlaintext || !got.GRPCOptions.DefaultPlaintextSet {
		t.Fatalf("unexpected grpc defaults: %+v", got.GRPCOptions)
	}
}

func TestBuildOptionsProfileEnabled(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	got, err := buildOptions(Options{
		Source:  Source{Path: file},
		Profile: ProfileOptions{Enabled: true},
	})
	if err != nil {
		t.Fatalf("buildOptions: %v", err)
	}
	if !got.Profile {
		t.Fatalf("expected profile=true, got %+v", got)
	}
}

func TestBuildOptionsRespectExplicitBoolOptions(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	follow := false
	plain := false
	got, err := buildOptions(Options{
		Source: Source{Path: file},
		HTTP: HTTPOptions{
			FollowRedirects: &follow,
		},
		GRPC: GRPCOptions{
			Plaintext: &plain,
		},
	})
	if err != nil {
		t.Fatalf("buildOptions: %v", err)
	}
	if got.HTTPOptions.FollowRedirects {
		t.Fatalf("expected follow=false, got %+v", got.HTTPOptions)
	}
	if got.GRPCOptions.DefaultPlaintext {
		t.Fatalf("expected plaintext=false, got %+v", got.GRPCOptions)
	}
}

func TestRunUsesExplicitEmptySourceContent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	if err := os.WriteFile(file, []byte("GET https://example.com/status\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := Run(context.Background(), Options{
		Source: Source{
			Path:    file,
			Content: []byte{},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsUsageError(err) {
		t.Fatalf("expected usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "no requests found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildReturnsValidationErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	if err := os.WriteFile(file, []byte("GET https://example.com/status\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	tests := []struct {
		name  string
		opt   Options
		check func(*testing.T, error)
	}{
		{
			name: "missing source path",
			opt:  Options{},
			check: func(t *testing.T, err error) {
				t.Helper()
				if !IsUsageError(err) || !errors.Is(err, ErrNoSourcePath) {
					t.Fatalf("expected usage error wrapping ErrNoSourcePath, got %v", err)
				}
			},
		},
		{
			name: "too few compare targets",
			opt: Options{
				Source:  Source{Path: file},
				Compare: CompareOptions{Targets: []string{"dev"}},
			},
			check: func(t *testing.T, err error) {
				t.Helper()
				if !IsUsageError(err) || !errors.Is(err, ErrTooFewTargets) {
					t.Fatalf("expected usage error wrapping ErrTooFewTargets, got %v", err)
				}
			},
		},
		{
			name: "profile compare conflict",
			opt: Options{
				Source:  Source{Path: file},
				Profile: ProfileOptions{Enabled: true},
				Compare: CompareOptions{Targets: []string{"dev", "stage"}},
			},
			check: func(t *testing.T, err error) {
				t.Helper()
				if !IsUsageError(err) {
					t.Fatalf("expected usage error, got %v", err)
				}
				if !strings.Contains(
					err.Error(),
					"profile.enabled cannot be combined with compare.targets",
				) {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "workflow selection conflict",
			opt: Options{
				Source: Source{Path: file},
				Selection: Selection{
					Workflow: "deploy",
					Request:  "ping",
				},
			},
			check: func(t *testing.T, err error) {
				t.Helper()
				if !IsUsageError(err) {
					t.Fatalf("expected usage error, got %v", err)
				}
				if !strings.Contains(
					err.Error(),
					"selection.workflow cannot be combined with selection.request, selection.tag, or selection.all",
				) {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "valid options",
			opt:  Options{Source: Source{Path: file}},
			check: func(t *testing.T, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Build(tc.opt)
			tc.check(t, err)
		})
	}
}

func TestRunPlanUsesBuiltFileSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "api.http")
	src := fmt.Sprintf("# @name ok\nGET %s\n", srv.URL)
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	pl, err := Build(Options{
		Source: Source{Path: file},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := os.WriteFile(file, []byte("GET http://127.0.0.1:1/nope\n"), 0o644); err != nil {
		t.Fatalf("overwrite file: %v", err)
	}

	rep, err := RunPlan(context.Background(), pl)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if rep.Passed != 1 || rep.Failed != 0 {
		t.Fatalf("unexpected report: %+v", rep)
	}
}

func TestRunPlanUsesBuiltEnvironmentSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ok" {
			http.NotFound(w, r)
			return
		}
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "api.http")
	envFile := filepath.Join(dir, "rest-client.env.json")
	src := "GET {{base}}/ok\n"
	env := fmt.Sprintf(`{"dev":{"base":"%s"}}`, srv.URL)
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(envFile, []byte(env), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	pl, err := Build(Options{
		Source: Source{Path: file},
		Environment: EnvironmentOptions{
			Name:     "dev",
			FilePath: envFile,
		},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := os.WriteFile(
		envFile,
		[]byte(`{"dev":{"base":"http://127.0.0.1:1"}}`),
		0o644,
	); err != nil {
		t.Fatalf("overwrite env file: %v", err)
	}

	rep, err := RunPlan(context.Background(), pl)
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if rep.Passed != 1 || rep.Failed != 0 {
		t.Fatalf("unexpected report: %+v", rep)
	}
}

func TestRunPlanAllowsConcurrentReuse(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if _, err := fmt.Fprint(w, `{"ok":true}`); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "api.http")
	src := fmt.Sprintf("# @name ok\nGET %s\n", srv.URL)
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	pl, err := Build(Options{
		Source: Source{Path: file},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	const n = 6
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rep, err := RunPlan(context.Background(), pl)
			if err != nil {
				errs <- err
				return
			}
			if rep.Passed != 1 || rep.Failed != 0 {
				errs <- fmt.Errorf("unexpected report: %+v", rep)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("RunPlan: %v", err)
	}
	if got := hits.Load(); got != n {
		t.Fatalf("request count = %d, want %d", got, n)
	}
}

func TestRunPlanRejectsInvalidPlan(t *testing.T) {
	_, err := RunPlan(context.Background(), Plan{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsUsageError(err) || !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("expected usage error wrapping ErrInvalidPlan, got %v", err)
	}
}

func TestRunPlanRejectsNilContext(t *testing.T) {
	var ctx context.Context
	_, err := RunPlan(ctx, Plan{})
	if !errors.Is(err, ErrNilContext) {
		t.Fatalf("RunPlan(nil, Plan{}): got %v want %v", err, ErrNilContext)
	}
}

func TestRunReturnsValidationErrors(t *testing.T) {
	_, err := Run(context.Background(), Options{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsUsageError(err) || !errors.Is(err, ErrNoSourcePath) {
		t.Fatalf("expected usage error wrapping ErrNoSourcePath, got %v", err)
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
	for _, res := range src.Results {
		out.Results = append(out.Results, stableResult(res))
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
