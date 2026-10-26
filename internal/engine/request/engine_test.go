package request

import (
	"fmt"
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
			Target:      "{{host}}:50051",
			FullMethod:  "/pkg.Service/GetUser",
			Message:     `{"id":"{{missing}}"}`,
			ProtoFiles:  []string{"{{protoDir}}/svc.proto"},
			ImportPaths: []string{"{{protoDir}}"},
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
	if got := res.Executed.GRPC.ProtoFiles; !slices.Equal(got, []string{"{{protoDir}}/svc.proto"}) {
		t.Fatalf("expected literal proto file placeholder, got %q", got)
	}
	if got := res.Executed.GRPC.ImportPaths; !slices.Equal(got, []string{"{{protoDir}}"}) {
		t.Fatalf("expected literal import path placeholder, got %q", got)
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

func TestRTSTestBlocksReadRequestState(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
		err  string
	}{
		{
			name: "captures",
			src: `### one
# @capture request token = response.json().token
# @rts test
> t.error("token", vars.get("token"))
GET http://example.test
`,
			want: []string{"@rts test api.http:4: token abc"},
		},
		{
			name: "after an assert error",
			src: `### one
# @assert fail("boom")
# @rts test ran
> t.error("ran")
GET http://example.test
`,
			want: []string{"ran api.http:4: ran"},
			err:  "boom",
		},
		{
			name: "after an assert error with a module",
			src: `# @use ./checks.rts as checks
### one
# @assert fail("boom")
# @rts test ran
> t.error("ran", len(checks.orders([])))
GET http://example.test
`,
			want: []string{"ran api.http:5: ran 0"},
			err:  "boom",
		},
		{
			name: "import named t",
			src: `# @use ./checks.rts as t
### one
# @rts test
> t.error("shadowed")
GET http://example.test
`,
			want: []string{"@rts test api.http:4: shadowed"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, req := rtsTestDoc(t, tc.src)
			eng, st := newStubEngine(t)
			st.body = `{"token": "abc"}`
			res, err := eng.ExecuteWith(doc, req, testEnv(""), ExecOptions{})
			if err != nil || res.Err != nil {
				t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
			}
			var got []string
			for _, tr := range res.Tests {
				if tr.Passed {
					t.Fatalf("test %q passed, want each block to report", tr.Name)
				}
				got = append(got, tr.Name+" "+tr.Message)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("tests = %q, want %q", got, tc.want)
			}
			var msg string
			if res.ScriptErr != nil {
				msg = res.ScriptErr.Error()
			}
			if !strings.Contains(msg, tc.err) || (tc.err == "") != (res.ScriptErr == nil) {
				t.Fatalf("script error = %v, want %q", res.ScriptErr, tc.err)
			}
		})
	}
}

func TestRTSModuleThatFailsToLoadIsReportedOnce(t *testing.T) {
	const src = `### one
# @use ./bad.rts
# @assert response.statusCode == 200
# @assert response.statusCode == 201
# @rts test one
> t.error("ran")
# @rts test two
> let x = missing.value
# @rts test three
> vars.set("a", "b")
# @script test
> tests.assert(true, "js");
GET http://example.test
`
	tests := []struct {
		name string
		mod  string
		msg  string
		line string
	}{
		{
			"syntax error",
			"module bad\n\nexport fn ok(x) {\n  return x ==\n}\n",
			"unexpected }",
			"   5 | }\n     | ^",
		},
		{
			"function named rts",
			"module bad\n\nexport fn rts(x) {\n  return x\n}\n",
			`name already defined: "rts"`,
			"   3 | export fn rts(x) {\n     |        ^",
		},
		{"module named rts", "module rts\n", "alias already defined: rts", "   2 | # @use ./bad.rts\n     | ^"},
		{"missing file", "", "bad.rts not found", "   2 | # @use ./bad.rts\n     | ^"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.mod != "" {
				if err := os.WriteFile(filepath.Join(dir, "bad.rts"), []byte(tt.mod), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			doc := parser.Parse(filepath.Join(dir, "api.http"), []byte(src))
			st := &stubTransport{}
			eng := New(engcfg.Config{Client: st.client()}, nil)
			res, err := eng.ExecuteWith(doc, doc.Requests[0], testEnv(""), ExecOptions{})
			if err != nil || res.Err != nil {
				t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
			}
			want := []row{{"one", false}, {"two", false}, {"three", false}, {"js", true}}
			if got := rows(res); !slices.Equal(got, want) {
				t.Fatalf("tests = %+v, want %+v", got, want)
			}
			for _, tr := range res.Tests[:3] {
				if tr.Message != "" {
					t.Fatalf("test %q message = %q, want none", tr.Name, tr.Message)
				}
			}
			rep := diag.ReportOf(res.ScriptErr)
			if len(rep.Items) != 1 || strings.Count(res.ScriptErr.Error(), tt.msg) != 1 {
				t.Fatalf("script error = %v, want %q once", res.ScriptErr, tt.msg)
			}
			if out := diag.Render(res.ScriptErr); strings.Count(out, tt.line) != 1 {
				t.Fatalf("rendered error does not show %q once:\n%s", tt.line, out)
			}
		})
	}
}

func TestMissingModuleIsReportedAtItsUseLine(t *testing.T) {
	dir := t.TempDir()
	src := "### one\n# @use ./bad.rts\n# @rts test one\n> t.error(\"ran\")\nGET http://example.test\n"
	doc := parser.Parse(filepath.Join(dir, "api.http"), []byte(src))
	st := &stubTransport{}
	eng := New(engcfg.Config{Client: st.client()}, nil)
	res, err := eng.ExecuteWith(doc, doc.Requests[0], testEnv(""), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	out := diag.Render(res.ScriptErr)
	for _, want := range []string{
		"error[script]: module " + filepath.Join(dir, "bad.rts") + " not found",
		"   2 | # @use ./bad.rts\n     | ^",
	} {
		if strings.Count(out, want) != 1 {
			t.Fatalf("rendered error does not show %q once:\n%s", want, out)
		}
	}
	if strings.Contains(out, "stat ") {
		t.Fatalf("rendered error names the stat call:\n%s", out)
	}
}

func TestRTSModuleErrorsShowTheModuleLikeTheRequestFile(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir("sub", 0o755); err != nil {
		t.Fatal(err)
	}
	mod := "module bad\n\nexport fn boom() {\n  return null.a\n}\n"
	if err := os.WriteFile(filepath.Join("sub", "bad.rts"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	const src = "# @use ./bad.rts\n# @rts test\n> bad.boom()\nGET http://example.test\n"
	abs := filepath.Join(root, "sub")
	tests := []struct {
		name string
		path string
		mod  string
	}{
		{"relative", "sub/api.http", "sub/bad.rts"},
		{"absolute", filepath.Join(abs, "api.http"), filepath.Join(abs, "bad.rts")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := parser.Parse(tt.path, []byte(src))
			eng, _ := newStubEngine(t)
			res, err := eng.ExecuteWith(doc, doc.Requests[0], testEnv(""), ExecOptions{})
			if err != nil || res.Err != nil {
				t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
			}
			out := diag.Render(res.ScriptErr)
			for _, want := range []string{"--> " + tt.mod + ":4:", "at " + tt.path + ":3:"} {
				if !strings.Contains(out, want) {
					t.Fatalf("rendered error does not contain %q:\n%s", want, out)
				}
			}
		})
	}
}

func TestRTSScriptsShadowStandardLibraryNames(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []row
	}{
		{
			"use alias",
			`# @use ./m.rts as json
# @assert json.v == 42
# @assert text() == "ok"
GET http://example.test
`,
			[]row{{"json.v == 42", true}, {`text() == "ok"`, true}},
		},
		{
			"let",
			`# @rts pre-request
> let json = 1
> vars.set("n", rts.json.stringify(json))
# @assert vars.get("n") == "1"
# @assert text() == "ok"
# @rts test block
> let json = response.text()
> if json != "ok" { t.error(json) }
GET http://example.test
`,
			[]row{{`vars.get("n") == "1"`, true}, {`text() == "ok"`, true}, {"block", true}},
		},
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m.rts"), []byte("export let v = 42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := parser.Parse(filepath.Join(dir, "api.http"), []byte(tt.src))
			st := &stubTransport{}
			eng := New(engcfg.Config{Client: st.client()}, nil)
			res, err := eng.ExecuteWith(doc, doc.Requests[0], testEnv(""), ExecOptions{})
			if err != nil || res.Err != nil || res.ScriptErr != nil {
				t.Fatalf("ExecuteWith() error = %v, result error = %v, script error = %v", err, res.Err, res.ScriptErr)
			}
			if got := rows(res); !slices.Equal(got, tt.want) {
				t.Fatalf("tests = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRTSModuleStateLastsAcrossChecksAndRequests(t *testing.T) {
	dir := t.TempDir()
	mod := `module cnt
let n = 0
let inits = 0
inits = inits + 1
export fn bump() {
  n = n + 1
  return n
}
export fn seen() {
  return inits
}
`
	if err := os.WriteFile(filepath.Join(dir, "cnt.rts"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	doc := parser.Parse(filepath.Join(dir, "api.http"), []byte(`# @use ./cnt.rts
### a
# @rts pre-request
> request.setHeader("X-N", str(cnt.bump()))
# @assert cnt.bump() == 2
# @rts test one
> t.error(cnt.bump(), cnt.seen())
# @rts test two
> t.error(cnt.bump(), cnt.seen())
GET http://example.test

### b
# @rts test three
> t.error(cnt.bump(), cnt.seen())
GET http://example.test
`))
	eng, st := newStubEngine(t)
	var got []string
	for _, req := range doc.Requests {
		res, err := eng.ExecuteWith(doc, req, testEnv(""), ExecOptions{})
		if err != nil || res.Err != nil || res.ScriptErr != nil {
			t.Fatalf("ExecuteWith() error = %v, result error = %v, script error = %v", err, res.Err, res.ScriptErr)
		}
		if req == doc.Requests[0] && st.wire.Header.Get("X-N") != "1" {
			t.Fatalf("X-N = %q, want 1 from the pre-request block", st.wire.Header.Get("X-N"))
		}
		for _, tr := range res.Tests {
			got = append(got, fmt.Sprintf("%s %t %s", tr.Name, tr.Passed, tr.Message))
		}
	}
	want := []string{
		"cnt.bump() == 2 true ",
		"one false api.http:7: 3 1",
		"two false api.http:9: 4 1",
		"three false api.http:14: 5 1",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tests = %q, want %q", got, want)
	}
}

func TestRTSRequestsMayImportUnderTheSameAlias(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b"} {
		mod := "module " + name + "\n\nexport fn name() {\n  return \"" + name + "\"\n}\n"
		if err := os.WriteFile(filepath.Join(dir, name+".rts"), []byte(mod), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	doc := parser.Parse(filepath.Join(dir, "api.http"), []byte(`### a
# @use ./a.rts as v
# @assert v.name() == "a"
GET http://example.test/a

### b
# @use ./b.rts as v
# @assert v.name() == "b"
GET http://example.test/b
`))
	eng, _ := newStubEngine(t)
	for _, req := range doc.Requests {
		res, err := eng.ExecuteWith(doc, req, testEnv(""), ExecOptions{})
		if err != nil || res.Err != nil || res.ScriptErr != nil {
			t.Fatalf("%s: error = %v, result error = %v, script error = %v", req.URL, err, res.Err, res.ScriptErr)
		}
		if got := rows(res); len(got) != 1 || !got[0].passed {
			t.Fatalf("%s: tests = %+v, want one passing assert", req.URL, got)
		}
	}
}

func TestRTSModuleImportStaysInItsRequest(t *testing.T) {
	tests := []struct {
		name string
		src  string
		errs []string
	}{
		{
			"leak",
			"### one\n# @assert h.two() == 2\nGET http://example.test/one\n\n" +
				"### two\n# @use ./h.rts\n# @assert h.two() == 2\nGET http://example.test/two\n",
			[]string{`undefined name "h"`, ""},
		},
		{
			"load error",
			"### one\n# @use ./missing.rts\n# @assert true\nGET http://example.test/one\n\n" +
				"### two\n# @assert true\nGET http://example.test/two\n",
			[]string{"missing.rts", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			mod := "module h\n\nexport fn two() {\n  return 2\n}\n"
			if err := os.WriteFile(filepath.Join(dir, "h.rts"), []byte(mod), 0o600); err != nil {
				t.Fatal(err)
			}
			doc := parser.Parse(filepath.Join(dir, "api.http"), []byte(tt.src))
			if len(doc.Requests) != len(tt.errs) {
				t.Fatalf("got %d requests, want %d", len(doc.Requests), len(tt.errs))
			}
			eng, _ := newStubEngine(t)
			for i, req := range doc.Requests {
				res, err := eng.ExecuteWith(doc, req, testEnv(""), ExecOptions{})
				if err != nil || res.Err != nil {
					t.Fatalf("%s: error = %v, result error = %v", req.URL, err, res.Err)
				}
				var msg string
				if res.ScriptErr != nil {
					msg = res.ScriptErr.Error()
				}
				if want := tt.errs[i]; !strings.Contains(msg, want) || (want == "") != (res.ScriptErr == nil) {
					t.Fatalf("%s: script error = %v, want %q", req.URL, res.ScriptErr, want)
				}
			}
		})
	}
}

func TestRTSModuleLoadTimeoutIsNamedOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "slow.rts"), []byte("module slow\nfor {\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc := parser.Parse(filepath.Join(dir, "api.http"), []byte(`### one
# @timeout 50ms
# @use ./slow.rts
# @rts test first
> t.error("ran")
# @script test
> tests.assert(true, "js");
GET http://example.test
`))
	eng, _ := newStubEngine(t)
	eng.re.Core().Lim.MaxSteps = 0
	res, err := eng.ExecuteWith(doc, doc.Requests[0], testEnv(""), ExecOptions{})
	if err != nil || res.Err != nil {
		t.Fatalf("ExecuteWith() error = %v, result error = %v", err, res.Err)
	}
	if got, want := rows(res), []row{{"js", true}}; !slices.Equal(got, want) {
		t.Fatalf("tests = %+v, want %+v", got, want)
	}
	const cause = "response checks timed out after 50ms: context deadline exceeded"
	if res.ScriptErr == nil || strings.Count(res.ScriptErr.Error(), cause) != 1 {
		t.Fatalf("script error = %v, want %q once", res.ScriptErr, cause)
	}
	if got := diag.ClassOf(res.ScriptErr); got != diag.ClassTimeout {
		t.Fatalf("class = %s, want %s", got, diag.ClassTimeout)
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
