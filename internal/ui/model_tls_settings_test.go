package ui

import (
	"testing"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/settings"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestApplyHTTPSettingsParsesTLS(t *testing.T) {
	req := &restfile.Request{
		Settings: map[string]string{
			"http-root-cas":    "a.pem, b.pem",
			"http-client-cert": "cert-{{env.val}}",
			"http-client-key":  "key-{{env.val}}",
			"http-insecure":    "true",
		},
	}
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"val": "one"}))
	opts := httpclient.Options{}

	if err := settings.ApplyHTTPSettings(&opts, req.Settings, resolver); err != nil {
		t.Fatalf("applyHTTPSettings returned error: %v", err)
	}
	if !opts.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify to be set")
	}
	if opts.ClientCert != "cert-one" || opts.ClientKey != "key-one" {
		t.Fatalf(
			"expected client cert/key to expand templates, got %q / %q",
			opts.ClientCert,
			opts.ClientKey,
		)
	}
	if len(opts.RootCAs) != 2 || opts.RootCAs[0] != "a.pem" || opts.RootCAs[1] != "b.pem" {
		t.Fatalf("unexpected root CAs: %#v", opts.RootCAs)
	}
}

func TestApplyGRPCSettingsParsesTLS(t *testing.T) {
	req := &restfile.Request{
		Settings: map[string]string{
			"grpc-root-ca":     "ca-one",
			"grpc-client-cert": "cert-{{x}}",
			"grpc-client-key":  "key-{{x}}",
			"grpc-insecure":    "false",
		},
	}
	resolver := vars.NewResolver(vars.NewMapProvider("x", map[string]string{"x": "two"}))
	opts := grpcclient.Options{Insecure: true}

	if err := settings.ApplyGRPCSettings(&opts, req.Settings, resolver); err != nil {
		t.Fatalf("applyGRPCSettings returned error: %v", err)
	}
	if opts.Insecure {
		t.Fatalf("expected insecure to be overridden to false")
	}
	if opts.ClientCert != "cert-two" || opts.ClientKey != "key-two" {
		t.Fatalf(
			"expected grpc client cert/key to expand templates, got %q / %q",
			opts.ClientCert,
			opts.ClientKey,
		)
	}
	if len(opts.RootCAs) != 1 || opts.RootCAs[0] != "ca-one" {
		t.Fatalf("unexpected grpc root CAs: %#v", opts.RootCAs)
	}
}

func TestSettingsFromEnvAndMerge(t *testing.T) {
	envs := vars.EnvironmentSet{
		"prod": {
			"settings.http-insecure": "false",
			"settings.http-root-cas": "root.pem",
			"other":                  "ignore",
		},
	}
	global := settings.FromEnv(envs, "prod")
	file := map[string]string{"http-root-cas": "file.pem"}
	req := map[string]string{"http-insecure": "true"}

	merged := settings.Merge(global, file, req)
	if merged["http-root-cas"] != "file.pem" {
		t.Fatalf("expected file to override global for root cas, got %q", merged["http-root-cas"])
	}
	if merged["http-insecure"] != "true" {
		t.Fatalf("expected request to override global insecure, got %q", merged["http-insecure"])
	}
}
