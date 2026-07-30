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
	"github.com/unkn0wn-root/resterm/internal/rts"
)

// @for-each binds its item as a typed value in ExecOptions.Values. Every other
// expression in the run reads it, and @apply used to be handed nil instead, so a
// patch dereferencing the loop item failed with an undefined name.
func TestApplySeesTypedForEachValues(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "member of a dict item",
			expr: `{headers: {"X-Item": item.id}}`,
			want: "42",
		},
		{
			name: "element of a list item",
			expr: `{headers: {"X-Item": list[1]}}`,
			want: "second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent *http.Request
			client := httpclient.NewClientWithOptions(
				httpclient.WithHTTPFactory(func(httpclient.Options) (*http.Client, error) {
					return &http.Client{
						Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
							sent = r
							return &http.Response{
								Status:     "200 OK",
								StatusCode: http.StatusOK,
								Header:     make(http.Header),
								Body:       io.NopCloser(strings.NewReader("ok")),
								Request:    r,
							}, nil
						}),
					}, nil
				}),
			)

			e := New(engcfg.Config{Client: client}, nil)
			req := &restfile.Request{
				Method: http.MethodGet,
				URL:    "http://example.test",
				Metadata: restfile.RequestMetadata{
					Applies: []restfile.ApplySpec{{Expression: tt.expr}},
				},
			}

			res, err := e.ExecuteWith(nil, req, testEnv(""), ExecOptions{
				Values: map[string]rts.Value{
					"item": rts.Dict(map[string]rts.Value{"id": rts.Str("42")}),
					"list": rts.List([]rts.Value{rts.Str("first"), rts.Str("second")}),
				},
			})
			if err != nil {
				t.Fatalf("ExecuteWith: %v", err)
			}
			if res.Err != nil {
				t.Fatalf("ExecuteWith result error: %v", res.Err)
			}
			if sent == nil {
				t.Fatal("request was never sent")
			}
			if got := sent.Header.Get("X-Item"); got != tt.want {
				t.Fatalf("X-Item = %q, want %q", got, tt.want)
			}
		})
	}
}

// The status bar only holds a warning while the run is in flight, so the report
// has to carry parse warnings for anything that looks at a finished run.
func TestExplainReportCarriesParseWarnings(t *testing.T) {
	client := httpclient.NewClientWithOptions(
		httpclient.WithHTTPFactory(func(httpclient.Options) (*http.Client, error) {
			return &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{
						Status:     "200 OK",
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader("ok")),
						Request:    r,
					}, nil
				}),
			}, nil
		}),
	)

	doc := &restfile.Document{
		Path: "warn.http",
		Warnings: []restfile.ParseError{
			{Line: 3, Message: `unknown @sse option "max-event"`},
		},
	}
	req := &restfile.Request{Method: http.MethodGet, URL: "http://example.test"}

	res, err := New(engcfg.Config{Client: client}, nil).ExecuteWith(
		doc,
		req,
		testEnv(""),
		ExecOptions{},
	)
	if err != nil {
		t.Fatalf("ExecuteWith: %v", err)
	}
	if res.Explain == nil {
		t.Fatal("expected an explain report")
	}

	want := `warn.http:3: unknown @sse option "max-event"`
	if slices.Contains(res.Explain.Warnings, want) {
		return
	}
	t.Fatalf("explain warnings = %v, want one to be %q", res.Explain.Warnings, want)
}

// The ssh/k8s conflict returns before the request runs and builds its own
// report, so it needs the parse warnings too.
func TestExplainReportCarriesParseWarningsOnEarlyFailure(t *testing.T) {
	doc := &restfile.Document{
		Path: "warn.http",
		Warnings: []restfile.ParseError{
			{Line: 3, Message: `unknown @sse option "max-event"`},
		},
	}
	req := &restfile.Request{
		Method: http.MethodGet,
		URL:    "http://example.test",
		SSH:    &restfile.SSHSpec{Inline: &restfile.SSHProfile{Host: "jump.example.test"}},
		K8s:    &restfile.K8sSpec{Use: "prof"},
	}

	res, err := New(engcfg.Config{}, nil).ExecuteWith(doc, req, testEnv(""), ExecOptions{})
	if err != nil {
		t.Fatalf("ExecuteWith: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected the ssh/k8s conflict to fail the request")
	}
	if res.Explain == nil {
		t.Fatal("expected an explain report")
	}

	want := `warn.http:3: unknown @sse option "max-event"`
	if slices.Contains(res.Explain.Warnings, want) {
		return
	}
	t.Fatalf("explain warnings = %v, want one to be %q", res.Explain.Warnings, want)
}
