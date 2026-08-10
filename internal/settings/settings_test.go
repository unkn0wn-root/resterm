package settings

import (
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpver"
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
	if httpOpts.HTTPVersion != httpver.V2 {
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
	if httpOpts.HTTPVersion != httpver.V11 {
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
