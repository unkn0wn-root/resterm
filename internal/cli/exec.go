package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runcheck"
	"github.com/unkn0wn-root/resterm/internal/telemetry"
	str "github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

type ExecFlags struct {
	EnvName           string
	EnvFile           string
	Workspace         string
	Timeout           time.Duration
	Insecure          bool
	Follow            bool
	ProxyURL          string
	Recursive         bool
	CompareTargetsRaw string
	CompareBaseline   string

	telemetry telemetry.Config
}

type ExecConfig struct {
	FilePath       string
	Workspace      string
	Recursive      bool
	EnvSet         vars.EnvironmentSet
	EnvName        string
	EnvFile        string
	EnvFallback    string
	HTTPOpts       httpclient.Options
	GRPCOpts       grpcclient.Options
	CompareTargets []string
	CompareBase    string
}

func NewExecFlags() ExecFlags {
	tc := telemetry.ConfigFromEnv(os.Getenv)
	return ExecFlags{
		Timeout:   30 * time.Second,
		Follow:    true,
		telemetry: tc,
	}
}

func (f *ExecFlags) Bind(fs *flag.FlagSet) {
	if fs == nil || f == nil {
		return
	}
	StringVar(fs, &f.EnvName, "env", "", "Environment name to use")
	StringVar(fs, &f.EnvFile, "env-file", "", "Path to environment file")
	StringVar(fs, &f.Workspace, "workspace", "", "Workspace directory to scan for request files")
	fs.DurationVar(&f.Timeout, "timeout", f.Timeout, "Request timeout")
	fs.BoolVar(&f.Insecure, "insecure", false, "Skip TLS certificate verification")
	fs.BoolVar(&f.Follow, "follow", f.Follow, "Follow redirects")
	StringVar(fs, &f.ProxyURL, "proxy", "", "HTTP proxy URL")
	fs.BoolVar(&f.Recursive, "recursive", false, "Recursively scan workspace for request files")
	fs.BoolVar(
		&f.Recursive,
		"recurisve",
		false,
		"(deprecated) Recursively scan workspace for request files",
	)
	f.BindTelemetryFlags(fs)
	StringVar(
		fs,
		&f.CompareTargetsRaw,
		"compare",
		"",
		"Default environments for manual compare runs (comma/space separated)",
	)
	StringVar(
		fs,
		&f.CompareBaseline,
		"compare-base",
		"",
		"Baseline environment when --compare is used (defaults to first target)",
	)
}

func (f *ExecFlags) BindTelemetryFlags(fs *flag.FlagSet) {
	if fs == nil || f == nil {
		return
	}
	StringVar(
		fs,
		&f.telemetry.Endpoint,
		"trace-otel-endpoint",
		f.telemetry.Endpoint,
		"OTLP collector endpoint used when @trace is enabled",
	)
	fs.BoolVar(
		&f.telemetry.Insecure,
		"trace-otel-insecure",
		f.telemetry.Insecure,
		"Disable TLS for OTLP trace export",
	)
	StringVar(
		fs,
		&f.telemetry.ServiceName,
		"trace-otel-service",
		f.telemetry.ServiceName,
		"Override service.name resource attribute for exported spans",
	)
}

func (f ExecFlags) ValidateEnvFlag() error {
	return runcheck.ValidateConcreteEnvironment(f.EnvName, "--env")
}

func (f ExecFlags) TelemetryConfig(version string) telemetry.Config {
	cfg := f.telemetry.Clone()
	cfg.Version = version
	return cfg
}

func (f ExecFlags) Resolve(filePath string) (ExecConfig, error) {
	if err := f.ValidateEnvFlag(); err != nil {
		return ExecConfig{}, err
	}
	filePath = CleanExecPath(filePath)
	work := resolveWorkspace(filePath, f.Workspace)

	envSet, envFile := LoadEnvironment(f.EnvFile, filePath, work)
	envName := f.EnvName
	envFallback := ""
	if envName == "" && len(envSet) > 0 {
		name := vars.DefaultEnvironment(envSet)
		if name != "" {
			envName = name
			if len(envSet) > 1 {
				envFallback = name
			}
		}
	}

	targets, err := ParseCompareTargets(f.CompareTargetsRaw)
	if err != nil {
		return ExecConfig{}, fmt.Errorf("invalid --compare value: %w", err)
	}
	base := f.CompareBaseline
	if err := runcheck.ValidateConcreteEnvironment(base, "--compare-base"); err != nil {
		return ExecConfig{}, fmt.Errorf("invalid --compare-base value: %w", err)
	}

	httpOpts := httpclient.Options{
		Timeout:            f.Timeout,
		FollowRedirects:    f.Follow,
		InsecureSkipVerify: f.Insecure,
		ProxyURL:           f.ProxyURL,
	}
	if filePath != "" {
		httpOpts.BaseDir = filepath.Dir(filePath)
	}

	return ExecConfig{
		FilePath:    filePath,
		Workspace:   work,
		Recursive:   f.Recursive,
		EnvSet:      envSet,
		EnvName:     envName,
		EnvFile:     envFile,
		EnvFallback: envFallback,
		HTTPOpts:    httpOpts,
		GRPCOpts: grpcclient.Options{
			DefaultPlaintext:    true,
			DefaultPlaintextSet: true,
		},
		CompareTargets: targets,
		CompareBase:    base,
	}, nil
}

func CleanExecPath(path string) string {
	path = str.Trim(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func resolveWorkspace(filePath, workspace string) string {
	workspace = str.Trim(workspace)
	if workspace == "" {
		if filePath != "" {
			return filepath.Dir(filePath)
		}
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
		return "."
	}
	if abs, err := filepath.Abs(workspace); err == nil {
		return abs
	}
	return workspace
}

func NewExecClient(version string, f ExecFlags) (*httpclient.Client, func() error, error) {
	client := httpclient.NewClient(nil)
	provider, err := telemetry.New(f.TelemetryConfig(version))
	if err != nil {
		return client, nil, err
	}
	client.SetTelemetry(provider)
	return client, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return provider.Shutdown(ctx)
	}, nil
}
