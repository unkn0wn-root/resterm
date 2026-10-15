package request

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/diag"
	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestExecuteWithReportsInsecureSSHWarning(t *testing.T) {
	tests := []struct {
		name              string
		hasSSH            bool
		strict            bool
		mode              ExecMode
		expectedWarnings  int
		expectedExplain   bool
		expectedTransport bool
		expectedEvents    []string
	}{
		{
			name:              "send with verification disabled",
			hasSSH:            true,
			strict:            false,
			expectedWarnings:  1,
			expectedExplain:   true,
			expectedTransport: true,
			expectedEvents:    []string{"warning", "transport"},
		},
		{
			name:              "preview with verification disabled",
			hasSSH:            true,
			strict:            false,
			mode:              ExecModePreview,
			expectedExplain:   true,
			expectedTransport: false,
			expectedEvents:    []string{},
		},
		{
			name:              "send with verification enabled",
			hasSSH:            true,
			strict:            true,
			expectedTransport: true,
			expectedEvents:    []string{"transport"},
		},
		{
			name:              "send without ssh",
			expectedTransport: true,
			expectedEvents:    []string{"transport"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			events := []string{}
			transportCalled := false
			client := httpx.NewClientWithOptions(
				httpx.WithHTTPFactory(func(opts httpx.Options) (*http.Client, error) {
					transportCalled = true
					events = append(events, "transport")
					activeSSH := opts.SSH != nil && opts.SSH.Active()
					if activeSSH != tt.hasSSH {
						t.Errorf("HTTP options active SSH = %v, want %v", activeSSH, tt.hasSSH)
					}
					return &http.Client{
						Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
							return &http.Response{
								Status:     "200 OK",
								StatusCode: http.StatusOK,
								Header:     make(http.Header),
								Body:       io.NopCloser(strings.NewReader("ok")),
								Request:    req,
							}, nil
						}),
					}, nil
				}),
			)
			e := New(engcfg.Config{Client: client}, nil)
			req := &restfile.Request{
				Method: http.MethodGet,
				URL:    "http://example.test",
			}
			if tt.hasSSH {
				req.SSH = &restfile.SSHSpec{
					Inline: &restfile.SSHProfile{
						Host: "jump.example.test",
						Strict: restfile.Opt[bool]{
							Val: tt.strict,
							Set: true,
						},
					},
				}
			}

			warnings := []Warning{}
			res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{
				Mode: tt.mode,
				OnWarning: func(warning Warning) {
					events = append(events, "warning")
					warnings = append(warnings, warning)
				},
			})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if res.Err != nil {
				t.Fatalf("ExecuteWith() result error = %v", res.Err)
			}
			if transportCalled != tt.expectedTransport {
				t.Fatalf(
					"HTTP transport called = %v, want %v",
					transportCalled,
					tt.expectedTransport,
				)
			}
			if !slices.Equal(events, tt.expectedEvents) {
				t.Fatalf("event order = %v, want %v", events, tt.expectedEvents)
			}

			if got := len(warnings); got != tt.expectedWarnings {
				t.Fatalf("callback warnings = %d, want %d", got, tt.expectedWarnings)
			}
			if tt.expectedWarnings > 0 {
				if warnings[0] != WarningSSHHostKeyVerificationDisabled {
					t.Fatalf("warning = %q", warnings[0])
				}
			}

			explainWarning := false
			if res.Explain != nil {
				if slices.Contains(res.Explain.Warnings, string(WarningSSHHostKeyVerificationDisabled)) {
					explainWarning = true
				}
			}
			if explainWarning != tt.expectedExplain {
				t.Fatalf(
					"Explain contains insecure SSH warning = %v, want %v",
					explainWarning,
					tt.expectedExplain,
				)
			}
		})
	}
}

func newPreviewTestEngine(t *testing.T) (*Engine, func() bool) {
	t.Helper()
	transportCalled := false
	client := httpx.NewClientWithOptions(
		httpx.WithHTTPFactory(func(opts httpx.Options) (*http.Client, error) {
			transportCalled = true
			return &http.Client{}, nil
		}),
	)
	return New(engcfg.Config{Client: client}, nil), func() bool { return transportCalled }
}

func TestPreviewWithUnresolvedVariablesDoesNotExecute(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method:  http.MethodGet,
		URL:     "http://example.test",
		Headers: http.Header{"X-Trace": []string{"{{traceID}}"}},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "{{auth.globalToken}}"},
			},
		},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if !res.Preview || res.Err != nil {
		t.Fatalf("expected clean preview, got preview=%v err=%v", res.Preview, res.Err)
	}
	if res.Explain == nil {
		t.Fatalf("expected explain report")
	}
	if transportCalled() {
		t.Fatalf("preview must not construct a transport")
	}
}

func TestPreviewResolvesScopedBaseURLWithoutMutatingSourceTarget(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	doc := &restfile.Document{
		Settings: map[string]string{"Base-Url": "https://file.example/v1/"},
	}
	req := &restfile.Request{
		Method:   http.MethodGet,
		URL:      "users",
		Settings: map[string]string{"BASE-URL": "https://request.example/v2/"},
	}
	env := testEnvValues(t, "dev", map[string]string{
		"settings.base-url": "https://global.example/v0/",
	})

	res, err := e.ExecuteWith(doc, req, env, ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if !res.Preview || res.Err != nil {
		t.Fatalf("expected clean preview, got preview=%v err=%v", res.Preview, res.Err)
	}
	if res.Explain == nil || res.Explain.Final == nil {
		t.Fatalf("expected prepared explain report, got %#v", res.Explain)
	}
	if got, want := res.Explain.Final.URL, "https://request.example/v2/users"; got != want {
		t.Fatalf("explain URL = %q, want %q", got, want)
	}
	if res.Executed == nil || res.Executed.URL != "users" {
		t.Fatalf("executed source target = %#v, want relative target", res.Executed)
	}
	if got := res.Executed.Settings["base-url"]; got != "https://request.example/v2/" {
		t.Fatalf("merged base-url = %q, want request override", got)
	}
	if transportCalled() {
		t.Fatal("preview must not construct a transport")
	}
}

func TestPreviewAbsoluteURLDoesNotResolveConfiguredBaseURL(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method:   http.MethodGet,
		URL:      "https://absolute.example/status",
		Settings: map[string]string{"base-url": "{{missing}}"},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if !res.Preview || res.Err != nil {
		t.Fatalf("expected clean preview, got preview=%v err=%v", res.Preview, res.Err)
	}
	if got := res.Explain.Final.URL; got != "https://absolute.example/status" {
		t.Fatalf("explain URL = %q, want absolute target unchanged", got)
	}
	if transportCalled() {
		t.Fatal("preview must not construct a transport")
	}
}

func TestPreviewBuildFailureStillShortCircuits(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://{{host}}/status",
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if !res.Preview {
		t.Fatalf("preview error result must keep Preview set")
	}
	if res.Err == nil {
		t.Fatalf("expected request build error")
	}
	if transportCalled() {
		t.Fatalf("failed preview must not fall through to execution")
	}
}

func TestPreviewCyclicVariablesReportsError(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method:  http.MethodGet,
		URL:     "http://example.test",
		Headers: http.Header{"X-Test": []string{"{{a}}"}},
		Variables: []restfile.Variable{
			{Name: "a", Value: "{{b}}", Authored: true},
			{Name: "b", Value: "{{a}}", Authored: true},
		},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if !res.Preview {
		t.Fatalf("preview error result must keep Preview set")
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "variable cycle") {
		t.Fatalf("expected cycle error, got %v", res.Err)
	}
	if transportCalled() {
		t.Fatalf("failed preview must not fall through to execution")
	}
}

func TestPreviewUnresolvedAuthParamSkipsAuthStage(t *testing.T) {
	cases := []struct {
		name string
		auth *restfile.AuthSpec
	}{
		{
			name: "command argv",
			auth: &restfile.AuthSpec{
				Type:   "command",
				Params: map[string]string{"argv": `["demo-auth","{{missing}}"]`},
			},
		},
		{
			name: "command typed param",
			auth: &restfile.AuthSpec{
				Type:   "command",
				Params: map[string]string{"argv": `["demo-auth"]`, "timeout": "{{missing}}"},
			},
		},
		{
			name: "oauth token url",
			auth: &restfile.AuthSpec{
				Type:   "oauth2",
				Params: map[string]string{"token_url": "{{missing}}"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, transportCalled := newPreviewTestEngine(t)
			req := &restfile.Request{
				Method:   http.MethodGet,
				URL:      "http://example.test",
				Metadata: restfile.RequestMetadata{Auth: tc.auth},
			}

			res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if !res.Preview || res.Err != nil {
				t.Fatalf("expected clean preview, got preview=%v err=%v", res.Preview, res.Err)
			}
			skipped := false
			for _, st := range res.Explain.Stages {
				if st.Name == xplain.StageAuth && st.Status == xplain.StageSkipped {
					skipped = true
				}
			}
			if !skipped {
				t.Fatalf("expected skipped auth stage, stages: %+v", res.Explain.Stages)
			}
			if transportCalled() {
				t.Fatalf("preview must not construct a transport")
			}
		})
	}
}

func TestPreviewCommandAuthStructuralErrorStaysFatal(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Variables: []restfile.Variable{
			{Name: "a", Value: "{{b}}", Authored: true},
			{Name: "b", Value: "{{a}}", Authored: true},
		},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type: "command",
				Params: map[string]string{
					"argv":      `["demo-auth"]`,
					"cache_key": "{{a}}",
				},
			},
		},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "variable cycle") {
		t.Fatalf("expected fatal cycle error, got %v", res.Err)
	}
	if !res.Preview {
		t.Fatalf("preview-mode failure result must keep Preview set")
	}
	if transportCalled() {
		t.Fatalf("failed preview must not fall through to execution")
	}
}

func TestPreviewGRPCUnresolvedVariablesStillPreviews(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method:   "GRPC",
		URL:      "{{host}}:50051",
		Settings: map[string]string{"base-url": "{{missingBase}}"},
		GRPC: &restfile.GRPCRequest{
			Target:     "{{host}}:50051",
			FullMethod: "/pkg.Service/GetUser",
			Message:    `{"id":"{{missing}}"}`,
		},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if !res.Preview || res.Err != nil {
		t.Fatalf("expected clean preview, got preview=%v err=%v", res.Preview, res.Err)
	}
	if got := res.Executed.GRPC.Target; got != "{{host}}:50051" {
		t.Fatalf("expected literal target placeholder, got %q", got)
	}
	if got := res.Executed.GRPC.Message; got != `{"id":"{{missing}}"}` {
		t.Fatalf("expected literal message placeholder, got %q", got)
	}
	if transportCalled() {
		t.Fatalf("preview must not construct a transport")
	}
}

func TestPreviewWebSocketUnresolvedStepStillPreviews(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "ws://example.test/ws",
		WebSocket: &restfile.WebSocketRequest{
			Steps: []restfile.WebSocketStep{
				{Type: restfile.WebSocketStepSendJSON, Value: `{"msg":"{{missing}}"}`},
			},
		},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if !res.Preview || res.Err != nil {
		t.Fatalf("expected clean preview, got preview=%v err=%v", res.Preview, res.Err)
	}
	if got := res.Executed.WebSocket.Steps[0].Value; got != `{"msg":"{{missing}}"}` {
		t.Fatalf("expected literal step placeholder, got %q", got)
	}
	if transportCalled() {
		t.Fatalf("preview must not construct a transport")
	}
}

func TestPreviewGRPCCyclicVariablesStaysFatal(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method: "GRPC",
		URL:    "localhost:50051",
		Variables: []restfile.Variable{
			{Name: "a", Value: "{{b}}", Authored: true},
			{Name: "b", Value: "{{a}}", Authored: true},
		},
		GRPC: &restfile.GRPCRequest{
			Target:     "localhost:50051",
			FullMethod: "/pkg.Service/GetUser",
			Message:    `{"id":"{{a}}"}`,
		},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "variable cycle") {
		t.Fatalf("expected fatal cycle error, got %v", res.Err)
	}
	if !res.Preview {
		t.Fatalf("preview-mode failure result must keep Preview set")
	}
	if transportCalled() {
		t.Fatalf("failed preview must not fall through to execution")
	}
}

func TestSendGRPCUnresolvedVariablesFailsClosed(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method: "GRPC",
		URL:    "{{host}}:50051",
		GRPC: &restfile.GRPCRequest{
			Target:     "{{host}}:50051",
			FullMethod: "/pkg.Service/GetUser",
		},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "undefined variable: host") {
		t.Fatalf("expected undefined variable error, got %v", res.Err)
	}
	if transportCalled() {
		t.Fatalf("failed build must not construct a transport")
	}
}

func TestPreviewOAuthStructuralErrorOutranksUndefined(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Variables: []restfile.Variable{
			{Name: "a", Value: "{{b}}", Authored: true},
			{Name: "b", Value: "{{a}}", Authored: true},
		},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type: "oauth2",
				Params: map[string]string{
					"token_url": "{{missing}}",
					"cache_key": "{{a}}",
				},
			},
		},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "variable cycle") {
		t.Fatalf("expected fatal cycle error, got %v", res.Err)
	}
	if !res.Preview {
		t.Fatalf("preview-mode failure result must keep Preview set")
	}
	if transportCalled() {
		t.Fatalf("failed preview must not fall through to execution")
	}
}

func TestPreviewCommandAuthStructuralErrorOutranksUndefined(t *testing.T) {
	// Command auth params come out of a map, so an early-return regression
	// would only mask the cycle on some runs. Repeat to make it reliable.
	for range 10 {
		e, transportCalled := newPreviewTestEngine(t)
		req := &restfile.Request{
			Method: http.MethodGet,
			URL:    "http://example.test",
			Variables: []restfile.Variable{
				{Name: "a", Value: "{{b}}", Authored: true},
				{Name: "b", Value: "{{a}}", Authored: true},
			},
			Metadata: restfile.RequestMetadata{
				Auth: &restfile.AuthSpec{
					Type: "command",
					Params: map[string]string{
						"argv":      `["demo-auth"]`,
						"cache_key": "{{a}}",
						"timeout":   "{{missing}}",
					},
				},
			},
		}

		res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
		if err != nil {
			t.Fatalf("ExecuteWith() error = %v", err)
		}
		if res.Err == nil || !strings.Contains(res.Err.Error(), "variable cycle") {
			t.Fatalf("expected fatal cycle error, got %v", res.Err)
		}
		if transportCalled() {
			t.Fatalf("failed preview must not fall through to execution")
		}
	}
}

func TestPreviewCommandAuthArgvCycleOutranksUndefinedParam(t *testing.T) {
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Variables: []restfile.Variable{
			{Name: "a", Value: "{{b}}", Authored: true},
			{Name: "b", Value: "{{a}}", Authored: true},
		},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type: "command",
				Params: map[string]string{
					"argv":    `["demo-auth","{{a}}"]`,
					"timeout": "{{missing}}",
				},
			},
		},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "variable cycle") {
		t.Fatalf("expected fatal cycle error, got %v", res.Err)
	}
	if transportCalled() {
		t.Fatalf("failed preview must not fall through to execution")
	}
}

func TestPreviewCommandAuthUndefinedParamOutranksDerivedParseError(t *testing.T) {
	// cache_key fails to expand and is left out of the parsed params, so
	// Parse claims ttl requires cache_key. The undefined variable is the
	// real problem and must be the one reported.
	e, transportCalled := newPreviewTestEngine(t)
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type: "command",
				Params: map[string]string{
					"argv":      `["demo-auth"]`,
					"ttl":       "30s",
					"cache_key": "{{missing}}",
				},
			},
		},
	}

	res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{Mode: ExecModePreview})
	if err != nil {
		t.Fatalf("ExecuteWith() error = %v", err)
	}
	if !res.Preview || res.Err != nil {
		t.Fatalf("expected clean preview, got preview=%v err=%v", res.Preview, res.Err)
	}
	skipped := false
	for _, st := range res.Explain.Stages {
		if st.Name == xplain.StageAuth && st.Status == xplain.StageSkipped {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("expected skipped auth stage, stages: %+v", res.Explain.Stages)
	}
	if transportCalled() {
		t.Fatalf("preview must not construct a transport")
	}
}

func TestResponseTestsRunAssertsBeforeScripts(t *testing.T) {
	doc, req := parseDoc(
		t,
		"### one\n# @assert response.statusCode == 200\n# @assert fail(\"boom\")\nGET http://example.test\n",
	)
	req.Metadata.Scripts = append(req.Metadata.Scripts, jsTest(`
vars.global.set("seen", "yes");
tests.assert(response.statusCode === 200, "js");
`))
	eng, _ := newStubEngine(t)
	env := testEnv("")
	res, err := eng.ExecuteWith(doc, req, env, ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	var names []string
	for _, tr := range res.Tests {
		if !tr.Passed {
			t.Fatalf("test %q failed: %s", tr.Name, tr.Message)
		}
		names = append(names, tr.Name)
	}
	if want := []string{"response.statusCode == 200", "js"}; !slices.Equal(names, want) {
		t.Fatalf("tests = %q, want %q", names, want)
	}
	if res.ScriptErr == nil || !strings.Contains(res.ScriptErr.Error(), "boom") {
		t.Fatalf("script error = %v, want the assert error", res.ScriptErr)
	}
	if got := eng.rt.Globals().Snapshot(env.Scope())["seen"].Value; got != "yes" {
		t.Fatalf("store seen = %q, want %q", got, "yes")
	}
}

func TestResponseTestsKeepEveryScriptError(t *testing.T) {
	doc, req := parseDoc(
		t,
		"### one\n# @assert fail(\"boom\")\n# @script test\n> throw new Error(\"bad\");\nGET http://example.test\n",
	)
	eng, _ := newStubEngine(t)
	res, err := eng.ExecuteWith(doc, req, testEnv(""), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	rep := diag.ReportOf(res.ScriptErr)
	if len(rep.Items) != 2 {
		t.Fatalf("report has %d items, want 2: %v", len(rep.Items), res.ScriptErr)
	}
	as, js := rep.Items[0], rep.Items[1]
	if as.Message != "boom" || as.Span.Start != (diag.Pos{Path: "env_ref.http", Line: 2, Col: 15}) ||
		len(as.Frames) == 0 {
		t.Fatalf("assert diagnostic = %+v, want boom at env_ref.http:2:15 with frames", as)
	}
	if js.Class != diag.ClassScript || js.Message != "Error: bad" ||
		js.Span.Start != (diag.Pos{Path: "env_ref.http", Line: 4, Col: 9}) {
		t.Fatalf("script diagnostic = %+v, want Error: bad at env_ref.http:4:9", js)
	}
	if got := diag.ClassOf(res.ScriptErr); got != diag.ClassScript {
		t.Fatalf("class = %s, want %s", got, diag.ClassScript)
	}
}

func TestSSEScriptTestsReadTheStream(t *testing.T) {
	client := httpx.NewClientWithOptions(
		httpx.WithHTTPFactory(func(httpx.Options) (*http.Client, error) {
			return &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					h := make(http.Header)
					h.Set("Content-Type", "text/event-stream")
					return &http.Response{
						Status:     "200 OK",
						StatusCode: http.StatusOK,
						Header:     h,
						Body:       io.NopCloser(strings.NewReader("data: hello\n\n")),
						Request:    r,
					}, nil
				}),
			}, nil
		}),
	)
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "https://example.test/events",
		SSE:    &restfile.SSERequest{},
		Metadata: restfile.RequestMetadata{
			Scripts: []restfile.ScriptBlock{
				jsTest(`
tests.assert(response.json().summary.eventCount === 1, "event count");
tests.assert(stream.summary().dropped === 0, "complete transcript");
`),
				{Kind: "test", Lang: "rts", Name: "rts events", Body: `let evs = stream.events()
if len(evs) != 1 or evs[0].data != "hello" {
  t.error(evs)
}`},
			},
		},
	}
	res, err := New(engcfg.Config{Client: client}, nil).ExecuteWith(nil, req, testEnv(""), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	if res.Stream == nil || res.Stream.Kind != "sse" {
		t.Fatalf("stream = %+v, want sse", res.Stream)
	}
	if res.ScriptErr != nil {
		t.Fatalf("script error = %v", res.ScriptErr)
	}
	if len(res.Tests) != 3 || res.Tests[0].Name != "rts events" {
		t.Fatalf("tests = %+v, want the rts block then 2", res.Tests)
	}
	for _, tr := range res.Tests {
		if !tr.Passed {
			t.Fatalf("test %q failed: %s", tr.Name, tr.Message)
		}
	}
}

func rtsTestDoc(t *testing.T, src string) (*restfile.Document, *restfile.Request) {
	t.Helper()
	dir := t.TempDir()
	mod := `export fn orders(list) {
  let out = []
  for let i, o range list {
    if o.total < 0 {
      out = rts.list.append(out, "Order " + str(i) + " has negative total")
    }
  }
  return out
}
`
	if err := os.WriteFile(filepath.Join(dir, "checks.rts"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	doc := parser.Parse(filepath.Join(dir, "api.http"), []byte(src))
	if len(doc.Errors) != 0 || len(doc.Requests) != 1 {
		t.Fatalf("parse errors = %v", doc.Errors)
	}
	return doc, doc.Requests[0]
}

func TestRTSTestBlocksRunBetweenAssertsAndScripts(t *testing.T) {
	doc, req := rtsTestDoc(t, `# @use ./checks.rts as checks
### orders
# @assert response.statusCode == 200
# @rts test "orders are valid"
> let body = try response.json()
> if not body.ok {
>   t.fatal("body is not JSON:", body.error)
> }
> for let _, msg range checks.orders(body.value.orders) {
>   t.error(msg)
> }
# @rts test stock
> if response.json().stock < 1 {
>   t.error("no stock")
> }
# @script test
> tests.assert(response.statusCode === 200, "js");
GET http://example.test/orders
`)
	eng, st := newStubEngine(t)
	st.body = `{"orders": [{"total": 5}, {"total": -1}, {"total": -2}], "stock": 3}`
	res, err := eng.ExecuteWith(doc, req, testEnv(""), ExecOptions{})
	if err != nil || res.Err != nil || res.ScriptErr != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v, script error = %v", err, res.Err, res.ScriptErr)
	}
	type result struct {
		name   string
		passed bool
		msg    string
	}
	var got []result
	for _, tr := range res.Tests {
		got = append(got, result{tr.Name, tr.Passed, tr.Message})
	}
	want := []result{
		{"response.statusCode == 200", true, ""},
		{"orders are valid", false, "api.http:10: Order 1 has negative total\napi.http:10: Order 2 has negative total"},
		{"stock", true, ""},
		{"js", true, ""},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tests = %+v, want %+v", got, want)
	}
}

func TestRTSTestBlockErrorKeepsEveryScriptError(t *testing.T) {
	doc, req := rtsTestDoc(t, `### one
# @rts test "broken block"
> t.error("before")
> let x = missing.value
# @rts test after
> t.error("ran")
# @script test
> throw new Error("bad");
GET http://example.test
`)
	eng, _ := newStubEngine(t)
	res, err := eng.ExecuteWith(doc, req, testEnv(""), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	if len(res.Tests) != 2 || res.Tests[0].Name != "broken block" || res.Tests[0].Message != "api.http:3: before" ||
		res.Tests[1].Name != "after" || res.Tests[1].Message != "api.http:6: ran" {
		t.Fatalf("tests = %+v, want broken then after", res.Tests)
	}
	rep := diag.ReportOf(res.ScriptErr)
	if len(rep.Items) != 2 {
		t.Fatalf("report has %d items, want 2: %v", len(rep.Items), res.ScriptErr)
	}
	block, js := rep.Items[0], rep.Items[1]
	if block.Message != `undefined name "missing"` || block.Span.Start != (diag.Pos{Path: doc.Path, Line: 4, Col: 11}) {
		t.Fatalf("block diagnostic = %+v, want the missing name at line 4", block)
	}
	if js.Message != "Error: bad" {
		t.Fatalf("script diagnostic = %+v, want Error: bad", js)
	}
	out := diag.Render(res.ScriptErr)
	if !strings.Contains(out, "   4 | > let x = missing.value") ||
		!strings.Contains(out, `:3:1 in @rts test "broken block"`) {
		t.Fatalf("rendered error has no source line or block frame:\n%s", out)
	}
}

func TestRTSOneLineBlocksRun(t *testing.T) {
	doc, req := rtsTestDoc(t, `### one
# @rts pre-request
> {% request.setHeader("X-One", "yes") %}
# @rts test ran
> {% t.error("ran") %}
# @rts test broken
> {% let x = missing.value %}
GET http://example.test
`)
	eng, st := newStubEngine(t)
	res, err := eng.ExecuteWith(doc, req, testEnv(""), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	if got := st.wire.Header.Get("X-One"); got != "yes" {
		t.Fatalf("X-One = %q, want the pre-request block to set it", got)
	}
	if len(res.Tests) != 2 || res.Tests[0].Name != "ran" || res.Tests[0].Message != "api.http:5: ran" ||
		res.Tests[1].Name != "broken" || res.Tests[1].Passed {
		t.Fatalf("tests = %+v, want ran then broken", res.Tests)
	}
	rep := diag.ReportOf(res.ScriptErr)
	if len(rep.Items) != 1 || rep.Items[0].Span.Start != (diag.Pos{Path: doc.Path, Line: 7, Col: 14}) {
		t.Fatalf("script error = %+v, want the missing name at 7:14", rep.Items)
	}
}

func TestRTSTestBlocksSkipWhenTheResponseHasNoRTSView(t *testing.T) {
	client := httpx.NewClientWithOptions(
		httpx.WithHTTPFactory(func(httpx.Options) (*http.Client, error) {
			return &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{
						Status:     "200 OK",
						StatusCode: http.StatusOK,
						Header:     http.Header{"X Bad": {"x"}},
						Body:       io.NopCloser(strings.NewReader("ok")),
						Request:    r,
					}, nil
				}),
			}, nil
		}),
	)
	doc, req := parseDoc(t, "### one\n# @rts test\n> t.error(\"ran\")\n# @script test\n"+
		"> tests.assert(true, \"js\");\nGET http://example.test\n")
	res, err := New(engcfg.Config{Client: client}, nil).ExecuteWith(doc, req, testEnv(""), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	if len(res.Tests) != 1 || res.Tests[0].Name != "js" {
		t.Fatalf("tests = %+v, want only the JavaScript test", res.Tests)
	}
	if rep := diag.ReportOf(res.ScriptErr); len(rep.Items) != 1 ||
		!strings.Contains(rep.Items[0].Message, "not an HTTP header name") {
		t.Fatalf("script error = %v, want the header error once", res.ScriptErr)
	}
}
