package request

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
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
			client := httpclient.NewClientWithOptions(
				httpclient.WithHTTPFactory(func(opts httpclient.Options) (*http.Client, error) {
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
				for _, warning := range res.Explain.Warnings {
					if warning == string(WarningSSHHostKeyVerificationDisabled) {
						explainWarning = true
						break
					}
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
