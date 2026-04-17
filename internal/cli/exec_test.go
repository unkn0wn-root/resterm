package cli

import (
	"testing"
	"time"
)

func TestExecFlagsTelemetryConfigPreservesEnvDerivedFields(t *testing.T) {
	t.Setenv("RESTERM_TRACE_OTEL_ENDPOINT", "collector:4317")
	t.Setenv("RESTERM_TRACE_OTEL_INSECURE", "true")
	t.Setenv("RESTERM_TRACE_OTEL_SERVICE", "resterm-ci")
	t.Setenv("RESTERM_TRACE_OTEL_TIMEOUT", "15s")
	t.Setenv("RESTERM_TRACE_OTEL_HEADERS", "x-api-key=secret, x-tenant = demo")

	flags := NewExecFlags()
	fs := NewFlagSet("test")
	flags.Bind(fs)
	if err := fs.Parse([]string{
		"--trace-otel-endpoint", "override:4317",
		"--trace-otel-insecure=false",
		"--trace-otel-service", "cli-service",
	}); err != nil {
		t.Fatalf("Parse(...): %v", err)
	}

	cfg := flags.TelemetryConfig("1.2.3")
	if cfg.Endpoint != "override:4317" {
		t.Fatalf("endpoint = %q, want %q", cfg.Endpoint, "override:4317")
	}
	if cfg.Insecure {
		t.Fatalf("insecure = true, want false")
	}
	if cfg.ServiceName != "cli-service" {
		t.Fatalf("service = %q, want %q", cfg.ServiceName, "cli-service")
	}
	if cfg.Version != "1.2.3" {
		t.Fatalf("version = %q, want %q", cfg.Version, "1.2.3")
	}
	if cfg.DialTimeout != 15*time.Second {
		t.Fatalf("dial timeout = %s, want %s", cfg.DialTimeout, 15*time.Second)
	}
	if len(cfg.Headers) != 2 || cfg.Headers["x-api-key"] != "secret" ||
		cfg.Headers["x-tenant"] != "demo" {
		t.Fatalf("unexpected headers: %#v", cfg.Headers)
	}

	cfg.Headers["x-api-key"] = "changed"
	next := flags.TelemetryConfig("1.2.3")
	if next.Headers["x-api-key"] != "secret" {
		t.Fatalf("headers mutated through returned config: %#v", next.Headers)
	}
}

func TestExecFlagsBindTelemetryFlags(t *testing.T) {
	flags := NewExecFlags()
	fs := NewFlagSet("test")
	flags.BindTelemetryFlags(fs)
	if err := fs.Parse([]string{
		"--trace-otel-endpoint", "collector:4317",
		"--trace-otel-insecure=true",
		"--trace-otel-service", "cli-service",
	}); err != nil {
		t.Fatalf("Parse(...): %v", err)
	}

	cfg := flags.TelemetryConfig("test-version")
	if cfg.Endpoint != "collector:4317" {
		t.Fatalf("endpoint = %q, want %q", cfg.Endpoint, "collector:4317")
	}
	if !cfg.Insecure {
		t.Fatalf("insecure = false, want true")
	}
	if cfg.ServiceName != "cli-service" {
		t.Fatalf("service = %q, want %q", cfg.ServiceName, "cli-service")
	}
	if cfg.Version != "test-version" {
		t.Fatalf("version = %q, want %q", cfg.Version, "test-version")
	}
}
