package cli

import (
	"context"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runx/check"
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
	MaxResponseSize   string
	MaxRedirects      int
	Recursive         bool
	CompareTargetsRaw string
	CompareBaseline   string
	CompareGroup      string
	EnvGroups         GroupFlags

	telemetry telemetry.Config
}

type ExecConfig struct {
	FilePath  string
	Workspace string
	Recursive bool
	Env       vars.Config
	HTTPOpts  httpx.Options
	GRPCOpts  grpcx.Options
	Compare   engine.CompareConfig
}

func NewExecFlags() ExecFlags {
	tc := telemetry.ConfigFromEnv(os.Getenv)
	return ExecFlags{
		Timeout:      30 * time.Second,
		Follow:       true,
		MaxRedirects: httpx.DefaultMaxRedirects,
		telemetry:    tc,
	}
}

func (f *ExecFlags) Bind(fs *flag.FlagSet) {
	if fs == nil || f == nil {
		return
	}
	StringVarAliases(fs, &f.EnvName, "", "Environment name to use", "env", "e")
	fs.Var(&f.EnvGroups, "env-group", "Select a grouped environment as group=profile (repeatable)")
	StringVarAliases(fs, &f.EnvFile, "", "Path to environment file", "env-file", "E")
	StringVarAliases(
		fs,
		&f.Workspace,
		"",
		"Workspace directory to scan for request files",
		"workspace",
		"w",
	)
	DurationVarAliases(fs, &f.Timeout, f.Timeout, "Request timeout", "timeout", "t")
	BoolVarAliases(fs, &f.Insecure, false, "Skip TLS certificate verification", "insecure", "k")
	BoolVarAliases(fs, &f.Follow, f.Follow, "Follow redirects", "follow", "L")
	IntVarAliases(
		fs,
		&f.MaxRedirects,
		f.MaxRedirects,
		"Redirects to follow before giving up (0 follows none)",
		"max-redirects",
	)
	StringVarAliases(fs, &f.ProxyURL, "", "HTTP proxy URL", "proxy", "x")
	StringVarAliases(
		fs,
		&f.MaxResponseSize,
		"",
		"Response body limit such as 100mb, or none to read without a limit",
		"max-response-size",
	)
	BoolVarAliases(
		fs,
		&f.Recursive,
		false,
		"Recursively scan workspace for request files",
		"recursive",
		"R",
	)
	f.BindTelemetryFlags(fs)
	StringVarAliases(
		fs,
		&f.CompareTargetsRaw,
		"",
		"Default environments or profiles for manual compare runs (comma separated)",
		"compare",
		"C",
	)
	StringVarAliases(
		fs,
		&f.CompareBaseline,
		"",
		"Baseline environment when --compare is used (defaults to first target)",
		"compare-base",
		"B",
	)
	StringVarAliases(
		fs,
		&f.CompareGroup,
		"",
		"Environment group varied by --compare",
		"compare-group",
	)
}

func (f *ExecFlags) BindTelemetryFlags(fs *flag.FlagSet) {
	if fs == nil || f == nil {
		return
	}
	StringVarAliases(
		fs,
		&f.telemetry.Endpoint,
		f.telemetry.Endpoint,
		"OTLP collector endpoint used when @trace is enabled",
		"trace-otel-endpoint",
		"toe",
	)
	BoolVarAliases(
		fs,
		&f.telemetry.Insecure,
		f.telemetry.Insecure,
		"Disable TLS for OTLP trace export",
		"trace-otel-insecure",
		"toi",
	)
	StringVarAliases(
		fs,
		&f.telemetry.ServiceName,
		f.telemetry.ServiceName,
		"Override service.name resource attribute for exported spans",
		"trace-otel-service",
		"tos",
	)
}

func (f ExecFlags) ValidateEnvFlag() error {
	if f.EnvName != "" && len(f.EnvGroups) > 0 {
		return fmt.Errorf("--env cannot be combined with --env-group")
	}
	return runcheck.ValidateConcreteEnvironment(f.EnvName, "--env")
}

func (f ExecFlags) TelemetryConfig(version string) telemetry.Config {
	cfg := f.telemetry.Clone()
	cfg.Version = version
	return cfg
}

// Resolve builds a run configuration and requires a valid environment file.
func (f ExecFlags) Resolve(filePath string) (ExecConfig, error) {
	cfg, err := f.ResolveSession(filePath)
	if err != nil {
		return ExecConfig{}, err
	}
	if cfg.Env.FileErr != nil {
		return ExecConfig{}, cfg.Env.FileErr
	}
	return cfg, nil
}

// ResolveSession builds an editor configuration. Parse errors are returned in
// Env.FileErr so the editor can open the file, while other load errors remain
// fatal. Requests stay blocked until the file parses successfully.
func (f ExecFlags) ResolveSession(filePath string) (ExecConfig, error) {
	if err := f.ValidateEnvFlag(); err != nil {
		return ExecConfig{}, err
	}
	filePath = CleanExecPath(filePath)
	work := resolveWorkspace(filePath, f.Workspace)

	cat, envFile, fileErr := f.loadEnvironment(filePath, work)
	if fileErr != nil && diag.ClassOf(fileErr) != diag.ClassParse {
		return ExecConfig{}, fileErr
	}

	compare, err := f.compareConfig()
	if err != nil {
		return ExecConfig{}, err
	}

	env := vars.Config{
		Catalog:      cat,
		File:         envFile,
		FileExplicit: str.Trim(f.EnvFile) != "",
		Intent:       vars.Intent{Name: f.EnvName, Groups: maps.Clone(f.EnvGroups)},
		FileErr:      fileErr,
	}
	// Keep selection and compare settings until a catalog is available. They
	// cannot be checked against the empty catalog left by a parse error.
	if fileErr == nil {
		if env.Selection, err = cat.Select(f.EnvName, f.EnvGroups); err != nil {
			return ExecConfig{}, err
		}
		if f.EnvName == "" && len(f.EnvGroups) == 0 && !cat.Empty() {
			active, err := cat.Resolve(env.Selection)
			if err != nil {
				return ExecConfig{}, err
			}
			if cat.Len() > 1 {
				env.Fallback = active.Label()
			}
		}
		if len(compare.Targets) > 0 {
			_, err := cat.CompareTargets(env.Selection, compare.Group, compare.Base, compare.Targets)
			if err != nil {
				return ExecConfig{}, fmt.Errorf("invalid compare selection: %w", err)
			}
		}
	}

	httpOpts := httpx.Options{
		Timeout:            f.Timeout,
		FollowRedirects:    f.Follow,
		InsecureSkipVerify: f.Insecure,
		ProxyURL:           f.ProxyURL,
	}
	if f.MaxRedirects < 0 {
		return ExecConfig{}, fmt.Errorf(
			"invalid --max-redirects value: %d must be non-negative",
			f.MaxRedirects,
		)
	}
	httpOpts.MaxRedirects = restfile.OptOf(f.MaxRedirects)
	if f.MaxResponseSize != "" {
		limit, err := bytesize.ParseBudget(f.MaxResponseSize)
		if err != nil {
			return ExecConfig{}, fmt.Errorf("invalid --max-response-size value: %w", err)
		}
		httpOpts.MaxResponseBytes = limit
	}
	if filePath != "" {
		httpOpts.BaseDir = filepath.Dir(filePath)
	}

	return ExecConfig{
		FilePath:  filePath,
		Workspace: work,
		Recursive: f.Recursive,
		Env:       env,
		HTTPOpts:  httpOpts,
		GRPCOpts:  grpcx.Options{DefaultPlaintext: restfile.OptOf(true)},
		Compare:   compare,
	}, nil
}

func (f ExecFlags) compareConfig() (engine.CompareConfig, error) {
	targets, err := ParseCompareTargets(f.CompareTargetsRaw)
	if err != nil {
		return engine.CompareConfig{}, fmt.Errorf("invalid --compare value: %w", err)
	}
	base := f.CompareBaseline
	if err := runcheck.ValidateConcreteEnvironment(base, "--compare-base"); err != nil {
		return engine.CompareConfig{}, fmt.Errorf("invalid --compare-base value: %w", err)
	}
	group := str.Trim(f.CompareGroup)
	if group != "" && len(targets) == 0 {
		return engine.CompareConfig{}, fmt.Errorf("--compare-group requires --compare")
	}
	return engine.CompareConfig{Targets: targets, Base: base, Group: group}, nil
}

// loadEnvironment loads the file named with --env-file, or discovers one near
// the launch context. It returns the path with load errors so the editor can
// open a file that failed to parse.
func (f ExecFlags) loadEnvironment(filePath, work string) (vars.Catalog, string, error) {
	if explicit := str.Trim(f.EnvFile); explicit != "" {
		cat, err := vars.LoadEnvironmentFile(explicit)
		return cat, explicit, err
	}

	var fileDir string
	if filePath != "" {
		fileDir = filepath.Dir(filePath)
	}
	cwd, _ := os.Getwd()
	return vars.Discover(fileDir, work, cwd)
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

func NewExecClient(version string, f ExecFlags) (*httpx.Client, func() error, error) {
	provider, err := telemetry.New(f.TelemetryConfig(version))
	if err != nil {
		return httpx.NewClientWithOptions(), nil, err
	}
	client := httpx.NewClientWithOptions(httpx.WithTelemetry(provider))
	return client, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return provider.Shutdown(ctx)
	}, nil
}
