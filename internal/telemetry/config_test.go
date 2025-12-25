package telemetry

import (
	"testing"
	"time"
)

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{
		envEndpoint:    "localhost:4317",
		envInsecure:    "true",
		envService:     "resterm-ci",
		envDialTimeout: "10s",
		envHeaders:     "x-api-key=secret, x-tenant = demo",
	}
	cfg := ConfigFromEnv(func(key string) string { return env[key] })
	if !cfg.Enabled() {
		t.Fatalf("expected telemetry to be enabled")
	}
	if cfg.Endpoint != "localhost:4317" {
		t.Fatalf("unexpected endpoint %q", cfg.Endpoint)
	}
	if !cfg.Insecure {
		t.Fatalf("expected insecure to be true")
	}
	if cfg.ServiceName != "resterm-ci" {
		t.Fatalf("unexpected service name %q", cfg.ServiceName)
	}
	if cfg.DialTimeout != 10*time.Second {
		t.Fatalf("unexpected dial timeout %s", cfg.DialTimeout)
	}
	if len(cfg.Headers) != 2 || cfg.Headers["x-api-key"] != "secret" ||
		cfg.Headers["x-tenant"] != "demo" {
		t.Fatalf("unexpected headers: %#v", cfg.Headers)
	}
}

func TestParseHeaders(t *testing.T) {
	headers, err := ParseHeaders("a=1, b=2,empty=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headers["a"] != "1" || headers["b"] != "2" || headers["empty"] != "" {
		t.Fatalf("unexpected headers: %#v", headers)
	}

	headers, err = ParseHeaders("   ")
	if err != nil || headers != nil {
		t.Fatalf("expected nil headers, got %#v (%v)", headers, err)
	}
}
