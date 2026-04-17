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
	flags.TraceOTEndpoint = "override:4317"
	flags.TraceOTInsecure = false
	flags.TraceOTService = "cli-service"

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
}
