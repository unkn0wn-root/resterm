package settings

import (
	"net/http/cookiejar"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/http/version"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
)

func TestApplyAllDispatchesHandlers(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	httpOpts := httpx.Options{CookieJar: jar}
	grpcOpts := grpcx.Options{}

	applier := New(
		HTTPHandler(&httpOpts, nil),
		GRPCHandler(&grpcOpts, nil),
	)

	left, err := applier.ApplyAll(map[string]string{
		"timeout":        "3s",
		"http-insecure":  "true",
		"http-version":   "2",
		"grpc-insecure":  "true",
		"feature.flag":   "on",
		"no-cookies":     "true",
		"proxy":          "http://proxy",
		"grpc-root-mode": "append",
	})
	if err != nil {
		t.Fatalf("ApplyAll returned error: %v", err)
	}

	if httpOpts.Timeout != 3*time.Second {
		t.Fatalf("expected timeout applied to http opts, got %v", httpOpts.Timeout)
	}
	if !httpOpts.InsecureSkipVerify {
		t.Fatalf("expected http insecure to be set")
	}
	if httpOpts.ProxyURL != "http://proxy" {
		t.Fatalf("expected proxy to be set, got %q", httpOpts.ProxyURL)
	}
	if httpOpts.HTTPVersion != version.V2 {
		t.Fatalf("expected http version 2, got %v", httpOpts.HTTPVersion)
	}
	if httpOpts.CookieJar != nil {
		t.Fatalf("expected cookie jar to be cleared")
	}
	if !grpcOpts.Insecure {
		t.Fatalf("expected grpc insecure to be set")
	}
	if grpcOpts.RootMode != tlsconfig.RootModeAppend {
		t.Fatalf("expected grpc root mode append, got %q", grpcOpts.RootMode)
	}
	if left["feature.flag"] != "on" || len(left) != 1 {
		t.Fatalf("expected leftovers to carry unknowns, got %+v", left)
	}
}

func TestApplyAllHTTPAggregated(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	httpOpts := httpx.Options{CookieJar: jar}
	applier := New(HTTPHandler(&httpOpts, nil))
	settings := map[string]string{
		"timeout":          "2s",
		"proxy":            "http://proxy",
		"followredirects":  "false",
		"insecure":         "true",
		"no-cookies":       "true",
		"http-version":     "1.1",
		"http-root-mode":   "append",
		"http-root-cas":    "a.pem,b.pem",
		"http-client-cert": "cert.pem",
		"http-client-key":  "key.pem",
	}
	left, err := applier.ApplyAll(settings)
	if err != nil {
		t.Fatalf("ApplyAll returned error: %v", err)
	}
	if len(left) != 0 {
		t.Fatalf("expected no leftovers, got %+v", left)
	}
	if httpOpts.Timeout != 2*time.Second {
		t.Fatalf("expected timeout 2s, got %v", httpOpts.Timeout)
	}
	if httpOpts.ProxyURL != "http://proxy" {
		t.Fatalf("expected proxy set, got %q", httpOpts.ProxyURL)
	}
	if httpOpts.FollowRedirects {
		t.Fatalf("expected follow redirects false")
	}
	if !httpOpts.InsecureSkipVerify {
		t.Fatalf("expected insecure skip verify true")
	}
	if httpOpts.CookieJar != nil {
		t.Fatalf("expected cookie jar to be cleared")
	}
	if httpOpts.HTTPVersion != version.V11 {
		t.Fatalf("expected http version 1.1, got %v", httpOpts.HTTPVersion)
	}
	if httpOpts.RootMode != tlsconfig.RootModeAppend {
		t.Fatalf("expected root mode append, got %q", httpOpts.RootMode)
	}
	if len(httpOpts.RootCAs) != 2 || httpOpts.RootCAs[0] != "a.pem" ||
		httpOpts.RootCAs[1] != "b.pem" {
		t.Fatalf("unexpected root CAs: %+v", httpOpts.RootCAs)
	}
	if httpOpts.ClientCert != "cert.pem" || httpOpts.ClientKey != "key.pem" {
		t.Fatalf("unexpected client cert/key: %q / %q", httpOpts.ClientCert, httpOpts.ClientKey)
	}
}

func TestApplyAllTLSNamesFollowScope(t *testing.T) {
	for _, tt := range []struct {
		name   string
		scopes []map[string]string
		want   []string
	}{
		{
			name:   "root cas over lower root ca",
			scopes: []map[string]string{{"grpc-root-ca": "low.pem"}, {"grpc-root-cas": "high.pem"}},
			want:   []string{"high.pem"},
		},
		{
			name:   "root ca over lower root cas",
			scopes: []map[string]string{{"grpc-root-cas": "low.pem"}, {"grpc-root-ca": "high.pem"}},
			want:   []string{"high.pem"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var h httpx.Options
			var g grpcx.Options
			if _, err := New(HTTPHandler(&h, nil), GRPCHandler(&g, nil)).ApplyAll(Merge(tt.scopes...)); err != nil {
				t.Fatalf("ApplyAll() error = %v", err)
			}
			if !slices.Equal(g.RootCAs, tt.want) {
				t.Fatalf("RootCAs = %v, want %v", g.RootCAs, tt.want)
			}
		})
	}
}

func TestApplyAllRejectsBothNamesInOneScope(t *testing.T) {
	for _, tt := range []struct {
		name string
		grpc bool
		set  map[string]string
		want string
		comp diag.Component
	}{
		{
			name: "insecure",
			set:  map[string]string{"insecure": "true", "http-insecure": "true"},
			want: "http-insecure and insecure are the same setting. Use one of them.",
			comp: diag.ComponentHTTP,
		},
		{
			name: "insecure with an invalid value",
			set:  map[string]string{"insecure": "true", "http-insecure": "maybe"},
			want: "http-insecure and insecure are the same setting. Use one of them.",
			comp: diag.ComponentHTTP,
		},
		{
			name: "http root ca",
			set:  map[string]string{"http-root-ca": "a.pem", "http-root-cas": "a.pem"},
			want: "http-root-ca and http-root-cas are the same setting. Use one of them.",
			comp: diag.ComponentHTTP,
		},
		{
			name: "grpc root ca",
			grpc: true,
			set:  map[string]string{"grpc-root-ca": "a.pem", "grpc-root-cas": "b.pem"},
			want: "grpc-root-ca and grpc-root-cas are the same setting. Use one of them.",
			comp: diag.ComponentGRPC,
		},
		{
			name: "grpc root ca on an HTTP request",
			set:  map[string]string{"grpc-root-ca": "a.pem", "grpc-root-cas": "b.pem"},
		},
		{
			name: "insecure on a gRPC request",
			grpc: true,
			set:  map[string]string{"insecure": "true", "http-insecure": "true"},
			want: "http-insecure and insecure are the same setting. Use one of them.",
			comp: diag.ComponentHTTP,
		},
		{
			name: "both root names empty",
			set:  map[string]string{"http-root-ca": "", "http-root-cas": ""},
			want: "http-root-ca and http-root-cas are the same setting. Use one of them.",
			comp: diag.ComponentHTTP,
		},
		{
			name: "first pair in sorted order",
			grpc: true,
			set: map[string]string{
				"timeout":       "abc",
				"insecure":      "true",
				"http-insecure": "true",
				"grpc-root-ca":  "a.pem",
				"grpc-root-cas": "b.pem",
			},
			want: "grpc-root-ca and grpc-root-cas are the same setting. Use one of them.",
			comp: diag.ComponentGRPC,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var h httpx.Options
			var g grpcx.Options
			hs := []Handler{HTTPHandler(&h, nil)}
			if tt.grpc {
				hs = append(hs, GRPCHandler(&g, nil))
			}
			_, err := New(hs...).ApplyAll(tt.set)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("ApplyAll() error = %v, want none", err)
				}
				return
			}
			if err == nil || err.Error() != tt.want {
				t.Fatalf("ApplyAll() error = %v, want %q", err, tt.want)
			}
			if rep := diag.ReportOf(err); len(rep.Items) == 0 || rep.Items[0].Component != tt.comp {
				t.Fatalf("report = %+v, want component %q", rep, tt.comp)
			}
		})
	}
}

func TestApplyAllReportsInvalidKeysInOrder(t *testing.T) {
	set := map[string]string{
		"timeout":         "abc",
		"no-cookies":      "maybe",
		"http-version":    "9",
		"followredirects": "maybe",
	}
	// Map order is random, so one pass could report the right key by luck.
	for range 10 {
		var opts httpx.Options
		_, err := New(HTTPHandler(&opts, nil)).ApplyAll(set)
		if err == nil || !strings.Contains(err.Error(), `invalid followredirects "maybe"`) {
			t.Fatalf("ApplyAll() error = %v, want the followredirects error", err)
		}
	}
}

func TestApplyAllHTTPInvalidVersionReturnsError(t *testing.T) {
	httpOpts := httpx.Options{}
	applier := New(HTTPHandler(&httpOpts, nil))

	_, err := applier.ApplyAll(map[string]string{"http-version": "unsupported"})
	if err == nil {
		t.Fatal("expected invalid http-version error")
	}
	if !strings.Contains(err.Error(), "invalid http-version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyHTTPSettingsRejectsInvalidInsecure(t *testing.T) {
	opts := httpx.Options{InsecureSkipVerify: true}

	err := ApplyHTTPSettings(&opts, map[string]string{"http-insecure": "maybe"}, nil)
	if err == nil {
		t.Fatal("expected invalid http-insecure error")
	}
	if !strings.Contains(err.Error(), `invalid http-insecure "maybe" (use true or false)`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.InsecureSkipVerify {
		t.Fatal("expected the rejected value to leave the option untouched")
	}
}

func TestApplyGRPCSettingsRejectsInvalidInsecure(t *testing.T) {
	opts := grpcx.Options{}

	err := ApplyGRPCSettings(&opts, map[string]string{"grpc-insecure": "maybe"}, nil)
	if err == nil {
		t.Fatal("expected invalid grpc-insecure error")
	}
	if !strings.Contains(err.Error(), `invalid grpc-insecure "maybe"`) {
		t.Fatalf("error must name the key the file used, got: %v", err)
	}
	if opts.Insecure {
		t.Fatal("expected the rejected value to leave the option untouched")
	}
}

// A bare key parses to an empty value, which the generic HTTP settings already
// reject. The prefixed spellings have to agree, and the wording has to say the
// value is missing rather than invalid.
func TestApplyHTTPSettingsRejectsEmptyInsecure(t *testing.T) {
	opts := httpx.Options{}

	err := ApplyHTTPSettings(&opts, map[string]string{"http-insecure": ""}, nil)
	if err == nil {
		t.Fatal("expected missing http-insecure error")
	}
	if !strings.Contains(err.Error(), "missing http-insecure value (use true or false)") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyHTTPSettingsAcceptsBooleanSpellings(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{raw: "yes", want: true},
		{raw: "on", want: true},
		{raw: "off", want: false},
		{raw: "0", want: false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			opts := httpx.Options{InsecureSkipVerify: !tc.want}
			err := ApplyHTTPSettings(&opts, map[string]string{"http-insecure": tc.raw}, nil)
			if err != nil {
				t.Fatalf("ApplyHTTPSettings returned error: %v", err)
			}
			if opts.InsecureSkipVerify != tc.want {
				t.Fatalf("insecure = %v, want %v", opts.InsecureSkipVerify, tc.want)
			}
		})
	}
}

func TestApplyHTTPSettingsRejectsInvalidRootMode(t *testing.T) {
	opts := httpx.Options{}

	err := ApplyHTTPSettings(&opts, map[string]string{"http-root-mode": "merge"}, nil)
	if err == nil {
		t.Fatal("expected invalid http-root-mode error")
	}
	if !strings.Contains(err.Error(), `invalid http-root-mode "merge" (use append or replace)`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A written-empty root mode used to be skipped in silence while every other
// setting reports a missing value.
func TestApplyHTTPSettingsRejectsEmptyRootMode(t *testing.T) {
	opts := httpx.Options{}

	err := ApplyHTTPSettings(&opts, map[string]string{"http-root-mode": ""}, nil)
	if err == nil {
		t.Fatal("expected missing http-root-mode error")
	}
	if !strings.Contains(err.Error(), "missing http-root-mode value (use append or replace)") {
		t.Fatalf("unexpected error: %v", err)
	}
}
