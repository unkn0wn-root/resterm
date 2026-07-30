package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestExecFlagsShortAliases(t *testing.T) {
	flags := NewExecFlags()
	fs := NewFlagSet("test")
	flags.Bind(fs)

	if err := fs.Parse([]string{
		"-e", "dev",
		"-E", "env.json",
		"-w", "/tmp/workspace",
		"-t", "5s",
		"-k",
		"-L=false",
		"-x", "http://proxy.example",
		"-R",
		"-C", "dev,prod",
		"-B", "prod",
		"-toe", "collector:4317",
		"-toi=true",
		"-tos", "resterm-test",
	}); err != nil {
		t.Fatalf("Parse(...): %v", err)
	}

	if flags.EnvName != "dev" {
		t.Fatalf("env = %q, want dev", flags.EnvName)
	}
	if flags.EnvFile != "env.json" {
		t.Fatalf("env file = %q, want env.json", flags.EnvFile)
	}
	if flags.Workspace != "/tmp/workspace" {
		t.Fatalf("workspace = %q, want /tmp/workspace", flags.Workspace)
	}
	if flags.Timeout != 5*time.Second {
		t.Fatalf("timeout = %s, want 5s", flags.Timeout)
	}
	if !flags.Insecure {
		t.Fatalf("insecure = false, want true")
	}
	if flags.Follow {
		t.Fatalf("follow = true, want false")
	}
	if flags.ProxyURL != "http://proxy.example" {
		t.Fatalf("proxy = %q, want http://proxy.example", flags.ProxyURL)
	}
	if !flags.Recursive {
		t.Fatalf("recursive = false, want true")
	}
	if flags.CompareTargetsRaw != "dev,prod" {
		t.Fatalf("compare = %q, want dev,prod", flags.CompareTargetsRaw)
	}
	if flags.CompareBaseline != "prod" {
		t.Fatalf("compare base = %q, want prod", flags.CompareBaseline)
	}

	cfg := flags.TelemetryConfig("test-version")
	if cfg.Endpoint != "collector:4317" {
		t.Fatalf("endpoint = %q, want collector:4317", cfg.Endpoint)
	}
	if !cfg.Insecure {
		t.Fatalf("telemetry insecure = false, want true")
	}
	if cfg.ServiceName != "resterm-test" {
		t.Fatalf("telemetry service = %q, want resterm-test", cfg.ServiceName)
	}
}

func TestExecFlagsParseGroupedSelections(t *testing.T) {
	flags := NewExecFlags()
	fs := NewFlagSet("test")
	flags.Bind(fs)
	err := fs.Parse([]string{
		"--env-group", "app=dev app 1",
		"--env-group", "api=dev",
		"--compare", "dev app 1,dev app 2",
		"--compare-group", "app",
	})
	if err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	if got, want := map[string]string(flags.EnvGroups), map[string]string{
		"api": "dev",
		"app": "dev app 1",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("group flags = %#v, want %#v", got, want)
	}
	targets, err := ParseCompareTargets(flags.CompareTargetsRaw)
	if err != nil {
		t.Fatalf("parse compare: %v", err)
	}
	if want := []string{"dev app 1", "dev app 2"}; !reflect.DeepEqual(targets, want) {
		t.Fatalf("compare targets = %#v, want %#v", targets, want)
	}
}

func TestExecFlagsRejectDuplicateGroupedSelection(t *testing.T) {
	flags := NewExecFlags()
	fs := NewFlagSet("test")
	flags.Bind(fs)
	err := fs.Parse([]string{
		"--env-group", "api=dev",
		"--env-group", "API=prod",
	})
	if err == nil || !strings.Contains(err.Error(), "selected more than once") {
		t.Fatalf("error = %v, want duplicate group error", err)
	}
}

func TestExecFlagsResolveGroupedDefaultsAndCompare(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "resterm.env.json")
	data := `{
		"$groups": {
			"api": {
				"$default": "dev",
				"dev": {"api.url": "dev"},
				"prod": {"api.url": "prod"}
			},
			"app": {
				"$default": "dev app 1",
				"dev app 1": {"app.url": "one"},
				"dev app 2": {"app.url": "two"}
			}
		}
	}`
	if err := os.WriteFile(envFile, []byte(data), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	flags := NewExecFlags()
	flags.EnvFile = envFile
	flags.EnvGroups = GroupFlags{"app": "dev app 2"}
	flags.CompareTargetsRaw = "dev,prod"
	flags.CompareBaseline = "prod"
	flags.CompareGroup = "api"
	cfg, err := flags.Resolve(filepath.Join(dir, "api.http"))
	if err != nil {
		t.Fatalf("resolve flags: %v", err)
	}
	env, err := cfg.Catalog.Resolve(cfg.Selection)
	if err != nil {
		t.Fatalf("resolve environment: %v", err)
	}
	if got, want := env.Label(), "api=dev, app=dev app 2"; got != want {
		t.Fatalf("environment = %q, want %q", got, want)
	}
	if cfg.Compare.Group != "api" || cfg.Compare.Base != "prod" {
		t.Fatalf("compare config = group %q base %q", cfg.Compare.Group, cfg.Compare.Base)
	}
}

func TestExecFlagsRejectEnvironmentModeConflicts(t *testing.T) {
	dir := t.TempDir()
	flat := filepath.Join(dir, "flat.json")
	grouped := filepath.Join(dir, "grouped.json")
	if err := os.WriteFile(flat, []byte(`{"dev": {}, "prod": {}}`), 0o600); err != nil {
		t.Fatalf("write flat env: %v", err)
	}
	if err := os.WriteFile(grouped, []byte(`{
		"$groups": {
			"api": {
				"$default": "dev",
				"dev": {},
				"prod": {}
			}
		}
	}`), 0o600); err != nil {
		t.Fatalf("write grouped env: %v", err)
	}

	tests := []struct {
		name  string
		flags ExecFlags
		want  string
	}{
		{
			name: "name and group flags",
			flags: ExecFlags{
				EnvName:   "dev",
				EnvGroups: GroupFlags{"api": "dev"},
			},
			want: "--env cannot be combined",
		},
		{
			name: "group selection with flat catalog",
			flags: ExecFlags{
				EnvFile:   flat,
				EnvGroups: GroupFlags{"api": "dev"},
			},
			want: "requires grouped environments",
		},
		{
			name: "name with grouped catalog",
			flags: ExecFlags{
				EnvFile: grouped,
				EnvName: "dev",
			},
			want: "cannot be combined with grouped environments",
		},
		{
			name: "grouped compare without group",
			flags: ExecFlags{
				EnvFile:           grouped,
				CompareTargetsRaw: "dev,prod",
			},
			want: "grouped compare requires a group",
		},
		{
			name: "compare group with flat catalog",
			flags: ExecFlags{
				EnvFile:           flat,
				CompareTargetsRaw: "dev,prod",
				CompareGroup:      "api",
			},
			want: "requires grouped environments",
		},
		{
			name: "compare group without targets",
			flags: ExecFlags{
				EnvFile:      grouped,
				CompareGroup: "api",
			},
			want: "--compare-group requires --compare",
		},
		{
			name: "compare base not in targets",
			flags: ExecFlags{
				EnvFile:           flat,
				CompareTargetsRaw: "dev,prod",
				CompareBaseline:   "stage",
			},
			want: "must match a compare target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.flags.Resolve(filepath.Join(dir, "api.http"))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
