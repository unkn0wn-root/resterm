package settings

import (
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
)

func TestApplyAllDispatchesHandlers(t *testing.T) {
	httpOpts := httpclient.Options{}
	grpcOpts := grpcclient.Options{}

	applier := New(
		HTTPHandler(&httpOpts, nil),
		GRPCHandler(&grpcOpts, nil),
	)

	left, err := applier.ApplyAll(map[string]string{
		"timeout":        "3s",
		"http-insecure":  "true",
		"grpc-insecure":  "true",
		"feature.flag":   "on",
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
	httpOpts := httpclient.Options{}
	applier := New(HTTPHandler(&httpOpts, nil))
	settings := map[string]string{
		"timeout":          "2s",
		"proxy":            "http://proxy",
		"followredirects":  "false",
		"insecure":         "true",
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
