package request

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	xplain "github.com/unkn0wn-root/resterm/internal/explain"
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
			{Name: "a", Value: "{{b}}"},
			{Name: "b", Value: "{{a}}"},
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
			{Name: "a", Value: "{{b}}"},
			{Name: "b", Value: "{{a}}"},
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
			{Name: "a", Value: "{{b}}"},
			{Name: "b", Value: "{{a}}"},
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
			{Name: "a", Value: "{{b}}"},
			{Name: "b", Value: "{{a}}"},
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
				{Name: "a", Value: "{{b}}"},
				{Name: "b", Value: "{{a}}"},
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
			{Name: "a", Value: "{{b}}"},
			{Name: "b", Value: "{{a}}"},
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
