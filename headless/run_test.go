package headless

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"

	"github.com/coder/websocket"
	"google.golang.org/grpc"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/reflection"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/runx/fail"
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
		HTTPOptions: httpx.Options{
			Timeout:         DefaultHTTPTimeout,
			FollowRedirects: true,
		},
		GRPCOptions: grpcx.Options{
			DefaultPlaintext: restfile.OptOf(true),
		},
	})
	if err != nil {
		t.Fatalf("runner RunContext: %v", err)
	}
	assertReportParity(t, reportFromRunner(want), got)
}

func TestRunUsesBaseURLFromInjectedEnvironment(t *testing.T) {
	seen := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.URL.RequestURI()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	file := filepath.Join(t.TempDir(), "relative.http")
	got, err := Run(t.Context(), Options{
		Source: Source{
			Path:    file,
			Content: []byte("# @name users\nGET users?page=2\n"),
		},
		Environment: EnvironmentOptions{
			Set: EnvironmentSet{
				"dev": {"settings.base-url": srv.URL + "/v1/"},
			},
			Name: "dev",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got.Passed != 1 || got.Failed != 0 || len(got.Results) != 1 {
		t.Fatalf("report = %+v, want one passing request", got)
	}
	if want := "users?page=2"; got.Results[0].Target != want {
		t.Fatalf("source target = %q, want %q", got.Results[0].Target, want)
	}
	select {
	case requestURI := <-seen:
		if requestURI != "/v1/users?page=2" {
			t.Fatalf("server request URI = %q, want %q", requestURI, "/v1/users?page=2")
		}
	default:
		t.Fatal("server did not receive the request")
	}
}

func TestRunMasksSecretsInEffectiveTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Setenv("RESTERM_HEADLESS_TOKEN", "os-secret-42")

	src := "# @file-secret key file-secret-42\n# @name leak\nGET " + srv.URL + "/items?key={{key}}&token={{token}}\n"
	rep, err := Run(t.Context(), Options{
		Source: Source{Path: filepath.Join(t.TempDir(), "leak.http"), Content: []byte(src)},
		Environment: EnvironmentOptions{
			Set:  EnvironmentSet{"dev": {"token": "env:RESTERM_HEADLESS_TOKEN"}},
			Name: "dev",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.Passed != 1 || len(rep.Results) != 1 {
		t.Fatalf("report = %+v, want one passing request", rep)
	}
	if want := srv.URL + "/items?key=•••&token=•••"; rep.Results[0].EffectiveTarget != want {
		t.Fatalf("effective target = %q, want %q", rep.Results[0].EffectiveTarget, want)
	}

	var out strings.Builder
	for _, f := range []Format{Text, JSON, JUnit} {
		if err := rep.Encode(&out, f); err != nil {
			t.Fatalf("Encode(%v) error = %v", f, err)
		}
	}
	for _, secret := range []string{"file-secret-42", "os-secret-42"} {
		if strings.Contains(out.String(), secret) {
			t.Fatalf("report contains %q:\n%s", secret, out.String())
		}
	}
}

func TestRunMasksSecretsInTheQuotedLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fmt.Fprint(w, `{"items":[{"id":"a"}]}`); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	src := "# @file-secret key file-secret-42\n# @name leak\n" +
		"# @assert json.get(response.json(), \"items[*]\") == \"file-secret-42\"\nGET " + srv.URL + "\n"
	rep, err := Run(t.Context(), Options{
		Source: Source{Path: filepath.Join(t.TempDir(), "leak.http"), Content: []byte(src)},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.Failed != 1 || len(rep.Results) != 1 {
		t.Fatalf("report = %+v, want one failed request", rep)
	}
	want := `3 | # @assert json.get(response.json(), "items[*]") == "•••"`
	if got := rep.Results[0].ScriptErrorDetail; !strings.Contains(got, want) {
		t.Fatalf("script error detail does not quote %q:\n%s", want, got)
	}

	var out strings.Builder
	for _, f := range []Format{Text, JSON, JUnit} {
		if err := rep.Encode(&out, f); err != nil {
			t.Fatalf("Encode(%v) error = %v", f, err)
		}
	}
	if strings.Contains(out.String(), "file-secret-42") {
		t.Fatalf("report contains the secret:\n%s", out.String())
	}
}

func TestRunSendsSchemelessTargetThroughTheParser(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "host and port", line: "%s/users?page=2", want: "/users?page=2"},
		{name: "url valued query", line: "%s/p?next=http://example.com&a=1", want: "/p?next=http://example.com&a=1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := make(chan string, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen <- r.URL.RequestURI()
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()

			target := fmt.Sprintf(tt.line, strings.TrimPrefix(srv.URL, "http://"))
			got, err := Run(t.Context(), Options{
				Source: Source{
					Path:    filepath.Join(t.TempDir(), "schemeless.http"),
					Content: []byte("# @name probe" + "\n" + "GET " + target + "\n"),
				},
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got.Passed != 1 || got.Failed != 0 {
				t.Fatalf("report = %+v, want one passing request", got)
			}
			if got.Results[0].Target != target {
				t.Fatalf("source target = %q, want it unmodified", got.Results[0].Target)
			}
			select {
			case uri := <-seen:
				if uri != tt.want {
					t.Fatalf("server request URI = %q, want %q", uri, tt.want)
				}
			default:
				t.Fatal("server did not receive the request")
			}
		})
	}
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

	cat, err := vars.LoadEnvironmentFile(envFile)
	if err != nil {
		t.Fatalf("load env file: %v", err)
	}
	sel := cat.DefaultSelection()
	want, err := runner.RunContext(context.Background(), runner.Options{
		FilePath:        path,
		WorkspaceRoot:   dir,
		Catalog:         cat,
		Selection:       sel,
		EnvironmentFile: envFile,
		Compare:         engine.CompareConfig{Targets: []string{"dev", "stage"}, Base: "stage"},
		HTTPOptions: httpx.Options{
			Timeout:         DefaultHTTPTimeout,
			FollowRedirects: true,
		},
		GRPCOptions: grpcx.Options{
			DefaultPlaintext: restfile.OptOf(true),
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
	src := "GET http://127.0.0.1:1/unreachable\n"
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
		HTTPOptions: httpx.Options{
			Timeout:         2 * time.Second,
			FollowRedirects: true,
		},
		GRPCOptions: grpcx.Options{
			DefaultPlaintext: restfile.OptOf(true),
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

// signalStop stands in for the cause of a signal.NotifyContext cancel.
type signalStop struct{}

func (signalStop) Error() string { return "interrupt signal received" }

func (signalStop) Is(target error) bool { return target == context.Canceled }

type arriveSvc struct {
	testgrpc.UnimplementedTestServiceServer
	arrive func(context.Context)
}

func (s arriveSvc) UnaryCall(ctx context.Context, _ *testgrpc.SimpleRequest) (*testgrpc.SimpleResponse, error) {
	s.arrive(ctx)
	return &testgrpc.SimpleResponse{}, nil
}

// Stages that reach the server fire the stop from the handler, so a slow send
// cannot stop the run early. A timeout gives the request a second to arrive.
func TestRunKeepsTheStopCause(t *testing.T) {
	const (
		pause    = 50 * time.Millisecond
		deadline = time.Second
	)
	cancelWith := func(cause error) func(*testing.T) (context.Context, func()) {
		return func(t *testing.T) (context.Context, func()) {
			ctx, cancel := context.WithCancelCause(t.Context())
			return ctx, func() { cancel(cause) }
		}
	}
	timeoutWith := func(cause error) func(*testing.T) (context.Context, func()) {
		return func(t *testing.T) (context.Context, func()) {
			ctx, cancel := context.WithTimeoutCause(t.Context(), deadline, cause)
			t.Cleanup(cancel)
			return ctx, func() {}
		}
	}
	ctxs := []struct {
		name string
		ctx  func(*testing.T) (context.Context, func())
		code FailureCode
		exit int
		err  string
		// The texts of an HTTP send error and of a @poll or @retry error.
		sendText   string
		repeatText string
	}{
		{
			name:       "custom cancel",
			ctx:        cancelWith(errors.New("stop")),
			code:       FailureCanceled,
			exit:       ExitCanceled,
			err:        "context canceled: stop",
			sendText:   `context canceled: perform request: Get %q: stop`,
			repeatText: "context canceled: repeat HTTP request: stop",
		},
		{
			name:       "custom timeout",
			ctx:        timeoutWith(errors.New("too slow")),
			code:       FailureTimeout,
			exit:       ExitTimeout,
			err:        "context deadline exceeded: too slow",
			sendText:   `context deadline exceeded: perform request: Get %q: too slow`,
			repeatText: "context deadline exceeded: repeat HTTP request: too slow",
		},
		{
			name:       "signal",
			ctx:        cancelWith(signalStop{}),
			code:       FailureCanceled,
			exit:       ExitCanceled,
			err:        "interrupt signal received",
			sendText:   `perform request: Get %q: interrupt signal received`,
			repeatText: "interrupt signal received",
		},
		{
			name:       "plain cancel",
			ctx:        cancelWith(nil),
			code:       FailureCanceled,
			exit:       ExitCanceled,
			err:        "context canceled",
			sendText:   `perform request: Get %q: context canceled`,
			repeatText: "context canceled",
		},
		{
			name:       "plain timeout",
			ctx:        timeoutWith(nil),
			code:       FailureTimeout,
			exit:       ExitTimeout,
			err:        "context deadline exceeded",
			sendText:   `perform request: Get %q: context deadline exceeded`,
			repeatText: "context deadline exceeded",
		},
	}

	overHTTP := func(t *testing.T, arrive func(context.Context)) (string, string) {
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			arrive(r.Context())
		}))
		t.Cleanup(srv.Close)
		return "GET " + srv.URL + "\n", srv.URL
	}
	overGRPC := func(t *testing.T, arrive func(context.Context)) (string, string) {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		srv := grpc.NewServer()
		testgrpc.RegisterTestServiceServer(srv, arriveSvc{arrive: arrive})
		reflection.Register(srv)
		go func() { _ = srv.Serve(lis) }()
		t.Cleanup(srv.Stop)
		addr := lis.Addr().String()
		return "# @grpc grpc.testing.TestService/UnaryCall\n# @grpc-plaintext true\nGRPC " + addr + "\n\n{}\n", addr
	}
	stages := []struct {
		name   string
		script string
		serve  func(*testing.T, func(context.Context)) (string, string)
		before bool
		// Nothing reaches the server, so the stop fires after a pause.
		timed      bool
		block      bool
		sent       int32
		sendText   bool
		repeatText bool
		// gRPC sends the deadline to the server, which can end the call
		// before the client context expires.
		cancelOnly bool
	}{
		{name: "before the run", serve: overHTTP, before: true},
		{name: "pre-request", script: "# @script pre-request\n> while (true) {}\n", serve: overHTTP, timed: true},
		{name: "send", serve: overHTTP, block: true, sent: 1, sendText: true},
		{name: "grpc send", serve: overGRPC, block: true, sent: 1, cancelOnly: true},
		{name: "checks", script: "# @script test\n> while (true) {}\n", serve: overHTTP, sent: 1},
		{
			name:       "poll wait",
			script:     "# @poll every=1m timeout=1m until=response.statusCode == 201\n",
			serve:      overHTTP,
			sent:       1,
			repeatText: true,
		},
		{
			name:       "retry wait",
			script:     "# @retry count=1\n# @retry-when response.statusCode == 200\n# @retry-backoff exponential(1m, 1m)\n",
			serve:      overHTTP,
			sent:       1,
			repeatText: true,
		},
	}
	for _, c := range ctxs {
		for _, st := range stages {
			if st.cancelOnly && c.code == FailureTimeout {
				continue
			}
			t.Run(c.name+"/"+st.name, func(t *testing.T) {
				t.Parallel()
				ctx, stop := c.ctx(t)
				var sent atomic.Int32
				arrive := func(ctx context.Context) {
					sent.Add(1)
					if st.block {
						stop()
						<-ctx.Done()
						return
					}
					time.AfterFunc(pause, stop)
				}

				file := filepath.Join(t.TempDir(), "stop.http")
				req, url := st.serve(t, arrive)
				src := "# @name one\n" + st.script + req
				if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
					t.Fatalf("write file: %v", err)
				}
				switch {
				case st.before:
					stop()
					<-ctx.Done()
				case st.timed:
					time.AfterFunc(pause, stop)
				}

				rep, err := Run(ctx, Options{Source: Source{Path: file}})
				if err != nil {
					t.Fatalf("Run: %v", err)
				}
				res := rep.Results[0]
				if res.Failure == nil {
					t.Fatalf("result carries no failure: %+v", res)
				}
				if res.Failure.Code != c.code {
					t.Fatalf("failure code = %q (%s), want %q", res.Failure.Code, res.Failure.Message, c.code)
				}
				if got := rep.ExitCode(ExitCodeDetailed); got != c.exit {
					t.Fatalf("exit code = %d, want %d", got, c.exit)
				}
				want := c.err
				switch {
				case st.sendText:
					want = fmt.Sprintf(c.sendText, url)
				case st.repeatText:
					want = c.repeatText
				}
				if res.Error != want {
					t.Fatalf("error = %q, want %q", res.Error, want)
				}
				if got := sent.Load(); got != st.sent {
					t.Fatalf("sent %d requests, want %d", got, st.sent)
				}
			})
		}
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
	if _, ok := got.HTTPOptions.MaxRedirects.Get(); ok {
		t.Fatalf("default MaxRedirects is explicitly set: %+v", got.HTTPOptions.MaxRedirects)
	}
	if got.HTTPOptions.MaxResponseBytes.Set() {
		t.Fatalf("default MaxResponseBytes is explicitly set: %+v", got.HTTPOptions.MaxResponseBytes)
	}
	if v, ok := got.GRPCOptions.DefaultPlaintext.Get(); !ok || !v {
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
	if got.GRPCOptions.DefaultPlaintext.Or(true) {
		t.Fatalf("expected plaintext=false, got %+v", got.GRPCOptions)
	}
}

func TestBuildOptionsRespectHTTPBounds(t *testing.T) {
	dir := t.TempDir()
	maxRedirects := 3
	maxResponseBytes := int64(64 << 20)
	got, err := buildOptions(Options{
		Source: Source{Path: filepath.Join(dir, "one.http")},
		HTTP: HTTPOptions{
			MaxRedirects:     &maxRedirects,
			MaxResponseBytes: &maxResponseBytes,
		},
	})
	if err != nil {
		t.Fatalf("buildOptions: %v", err)
	}
	if value, ok := got.HTTPOptions.MaxRedirects.Get(); !ok || value != maxRedirects {
		t.Fatalf("MaxRedirects = %d (set %t), want %d", value, ok, maxRedirects)
	}
	if value := got.HTTPOptions.MaxResponseBytes.Or(-1); value != maxResponseBytes {
		t.Fatalf("MaxResponseBytes = %d, want %d", value, maxResponseBytes)
	}
}

func TestBuildOptionsAllowDisablingHTTPBounds(t *testing.T) {
	dir := t.TempDir()
	zeroRedirects := 0
	unlimitedResponse := int64(0)
	got, err := buildOptions(Options{
		Source: Source{Path: filepath.Join(dir, "one.http")},
		HTTP: HTTPOptions{
			MaxRedirects:     &zeroRedirects,
			MaxResponseBytes: &unlimitedResponse,
		},
	})
	if err != nil {
		t.Fatalf("buildOptions: %v", err)
	}
	if value, ok := got.HTTPOptions.MaxRedirects.Get(); !ok || value != 0 {
		t.Fatalf("MaxRedirects = %d (set %t), want an explicit zero", value, ok)
	}
	if !got.HTTPOptions.MaxResponseBytes.Set() || got.HTTPOptions.MaxResponseBytes.Or(-1) != 0 {
		t.Fatalf("MaxResponseBytes = %+v, want an explicit unlimited budget",
			got.HTTPOptions.MaxResponseBytes)
	}
}

func TestBuildOptionsRejectNegativeHTTPBounds(t *testing.T) {
	negativeInt := -1
	negativeInt64 := int64(-1)
	tests := []struct {
		name string
		http HTTPOptions
		want string
	}{
		{
			name: "redirects",
			http: HTTPOptions{MaxRedirects: &negativeInt},
			want: "http.maxRedirects",
		},
		{
			name: "response bytes",
			http: HTTPOptions{MaxResponseBytes: &negativeInt64},
			want: "http.maxResponseBytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildOptions(Options{
				Source: Source{Path: filepath.Join(t.TempDir(), "one.http")},
				HTTP:   tt.http,
			})
			if !IsUsageError(err) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want a usage error containing %q", err, tt.want)
			}
		})
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
	bad := filepath.Join(dir, "bad.http")
	if err := os.WriteFile(bad, []byte("# @k8s namespace=default\nGET https://example.com\n"), 0o644); err != nil {
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
			name: "missing request file",
			opt:  Options{Source: Source{Path: filepath.Join(dir, "missing.http")}},
			check: func(t *testing.T, err error) {
				t.Helper()
				if !IsUsageError(err) || !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("expected usage error wrapping os.ErrNotExist, got %v", err)
				}
			},
		},
		{
			name: "request file with a parse error",
			opt:  Options{Source: Source{Path: bad}},
			check: func(t *testing.T, err error) {
				t.Helper()
				if !IsUsageError(err) || !strings.Contains(diag.Render(err), "bad.http:1") {
					t.Fatalf("expected usage error that keeps the parse diagnostic, got\n%s", diag.Render(err))
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
	if err := os.WriteFile(file, []byte("GET http://127.0.0.1:1/unreachable\n"), 0o644); err != nil {
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
		wg.Go(func() {
			rep, err := RunPlan(context.Background(), pl)
			if err != nil {
				errs <- err
				return
			}
			if rep.Passed != 1 || rep.Failed != 0 {
				errs <- fmt.Errorf("unexpected report: %+v", rep)
			}
		})
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

func TestRunPlanReportsDoNotShareSelection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	pl, err := Build(Options{
		Source: Source{
			Path:    filepath.Join(t.TempDir(), "api.http"),
			Content: []byte("# @name ok\nGET {{api.url}}\n"),
		},
		Environment: EnvironmentOptions{Grouped: &GroupedEnvironmentSet{
			Groups: EnvironmentGroups{"api": {
				Default:  "dev",
				Profiles: EnvironmentSet{"dev": {"api.url": srv.URL}, "prod": {"api.url": srv.URL}},
			}},
		}},
		Compare: CompareOptions{Targets: []string{"dev", "prod"}, Group: "api"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	const n = 6
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Go(func() {
			rep, err := RunPlan(context.Background(), pl)
			if err != nil {
				errs <- err
				return
			}
			sels := []EnvironmentSelection{rep.EnvironmentSelection}
			for _, res := range rep.Results {
				sels = append(sels, res.EnvironmentSelection)
				for _, step := range res.Steps {
					sels = append(sels, step.EnvironmentSelection)
				}
			}
			for _, sel := range sels {
				if sel["api"] == "edited" {
					errs <- fmt.Errorf("report shares a selection map: %v", sel)
				}
				sel["api"] = "edited"
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("RunPlan: %v", err)
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

func TestRunKeepsAWebSocketStepFailureClass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer func() {
			_ = conn.Close(websocket.StatusNormalClosure, "bye")
		}()
		for {
			typ, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			if err := conn.Write(r.Context(), typ, data); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	file := filepath.Join(dir, "ws.http")
	src := fmt.Sprintf(
		"# @name chat\n# @ws send hello\n# @ws send-file missing-payload.bin\nGET %s\n",
		strings.Replace(srv.URL, "http", "ws", 1),
	)
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	rep, err := Run(context.Background(), Options{Source: Source{Path: file}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(rep.Results))
	}

	res := rep.Results[0]
	if res.Failure == nil {
		t.Fatalf("result carries no failure: %+v", res)
	}
	if res.Failure.Code != FailureCode(runfail.CodeFilesystem) {
		t.Fatalf("failure code = %q, want %q", res.Failure.Code, runfail.CodeFilesystem)
	}
	if res.Failure.ExitCode != runfail.ExitFilesystem {
		t.Fatalf("exit code = %d, want %d", res.Failure.ExitCode, runfail.ExitFilesystem)
	}
	if res.Failure.Source != "stream" {
		t.Fatalf("failure source = %q, want %q", res.Failure.Source, "stream")
	}
	if rep.ExitCode(ExitCodeDetailed) != runfail.ExitFilesystem {
		t.Fatalf("report exit code = %d, want %d", rep.ExitCode(ExitCodeDetailed), runfail.ExitFilesystem)
	}

	if res.Stream == nil {
		t.Fatalf("result kept no stream: %+v", res)
	}
	if !strings.Contains(res.Stream.Error, "send_file") {
		t.Fatalf("stream error = %q, want the step that failed", res.Stream.Error)
	}
	if res.Stream.Summary["errorClass"] != string(diag.ClassFilesystem) {
		t.Fatalf("stream summary = %+v, want a filesystem errorClass", res.Stream.Summary)
	}
	if res.Stream.Summary["sentCount"] != 1 {
		t.Fatalf("sentCount = %v, want the frame sent before the failure",
			res.Stream.Summary["sentCount"])
	}
}
