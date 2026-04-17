package headless

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// DefaultHTTPTimeout is the timeout applied when Options.HTTP.Timeout is zero.
const DefaultHTTPTimeout = 30 * time.Second

// Validate reports whether o contains a valid headless configuration.
func (o Options) Validate() error {
	_, err := runnerOptions(o)
	return err
}

// Run executes a request or workflow file and returns a stable public report.
func Run(ctx context.Context, opt Options) (*Report, error) {
	ro, err := runnerOptions(opt)
	if err != nil {
		return nil, err
	}
	rep, err := runner.RunContext(ctx, ro)
	if err != nil {
		if runner.IsUsageError(err) {
			return nil, UsageError{err: err}
		}
		return nil, err
	}
	return reportFromRunner(rep), nil
}

func runnerOptions(opt Options) (runner.Options, error) {
	path, err := absPath(opt.FilePath)
	if err != nil {
		return runner.Options{}, UsageError{err: fmt.Errorf("resolve filePath: %w", err)}
	}
	if path == "" {
		return runner.Options{}, UsageError{err: ErrNoFilePath}
	}
	work, err := workspacePath(path, opt.WorkspaceRoot)
	if err != nil {
		return runner.Options{}, UsageError{err: fmt.Errorf("resolve workspaceRoot: %w", err)}
	}
	sel, err := selectionOptions(opt.Selection)
	if err != nil {
		return runner.Options{}, err
	}
	envs, envFile, envName, err := environmentOptions(opt, path, work)
	if err != nil {
		return runner.Options{}, err
	}
	targets, err := compareTargets(opt.Compare.Targets)
	if err != nil {
		return runner.Options{}, err
	}
	base := strings.TrimSpace(opt.Compare.Base)
	if err := cli.ValidateReservedEnvironment(base, "compare.base"); err != nil {
		return runner.Options{}, UsageError{err: err}
	}
	if opt.Profile.Enabled && len(targets) > 0 {
		return runner.Options{}, UsageError{
			err: fmt.Errorf("profile.enabled cannot be combined with compare.targets"),
		}
	}
	if sel.Workflow != "" && (opt.Profile.Enabled || len(targets) > 0) {
		return runner.Options{}, UsageError{
			err: fmt.Errorf(
				"selection.workflow cannot be combined with compare.targets or profile.enabled",
			),
		}
	}
	return runner.Options{
		Version:         strings.TrimSpace(opt.Version),
		FilePath:        path,
		FileContent:     bytes.Clone(opt.FileData),
		WorkspaceRoot:   work,
		Recursive:       opt.Recursive,
		ArtifactDir:     strings.TrimSpace(opt.State.ArtifactDir),
		StateDir:        strings.TrimSpace(opt.State.StateDir),
		PersistGlobals:  opt.State.PersistGlobals,
		PersistAuth:     opt.State.PersistAuth,
		History:         opt.State.History,
		EnvSet:          envs,
		EnvName:         envName,
		EnvironmentFile: envFile,
		CompareTargets:  targets,
		CompareBase:     base,
		Profile:         opt.Profile.Enabled,
		HTTPOptions:     httpOptions(opt.HTTP),
		GRPCOptions:     grpcOptions(opt.GRPC),
		Select:          sel,
	}, nil
}

func selectionOptions(sel Selection) (runner.Select, error) {
	out := runner.Select{
		Request:  strings.TrimSpace(sel.Request),
		Workflow: strings.TrimSpace(sel.Workflow),
		Tag:      strings.TrimSpace(sel.Tag),
		All:      sel.All,
	}
	switch {
	case out.Workflow != "" && (out.All || out.Request != "" || out.Tag != ""):
		return runner.Select{}, UsageError{
			err: fmt.Errorf(
				"selection.workflow cannot be combined with selection.request, selection.tag, or selection.all",
			),
		}
	case out.All && (out.Request != "" || out.Tag != ""):
		return runner.Select{}, UsageError{
			err: fmt.Errorf(
				"selection.all cannot be combined with selection.request or selection.tag",
			),
		}
	case out.Request != "" && out.Tag != "":
		return runner.Select{}, UsageError{
			err: fmt.Errorf("selection.request cannot be combined with selection.tag"),
		}
	default:
		return out, nil
	}
}

func absPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

func workspacePath(path, work string) (string, error) {
	work = strings.TrimSpace(work)
	switch {
	case work != "":
		return absPath(work)
	case path != "":
		return filepath.Dir(path), nil
	default:
		return "", nil
	}
}

func environmentOptions(
	opt Options,
	path, work string,
) (vars.EnvironmentSet, string, string, error) {
	envs := environmentSet(opt.Environment.Set)
	envFile := strings.TrimSpace(opt.Environment.FilePath)
	switch {
	case len(envs) > 0:
	case envFile != "":
		set, err := vars.LoadEnvironmentFile(envFile)
		if err != nil {
			return nil, "", "", UsageError{
				err: fmt.Errorf("load environment.filePath: %w", err),
			}
		}
		envs = set
	default:
		set, file, err := vars.ResolveEnvironment(envPaths(path, work))
		if err != nil {
			return nil, "", "", UsageError{
				err: fmt.Errorf("resolve environment file: %w", err),
			}
		}
		envs = set
		envFile = file
	}
	envName := strings.TrimSpace(opt.Environment.Name)
	if err := cli.ValidateReservedEnvironment(envName, "environment.name"); err != nil {
		return nil, "", "", UsageError{err: err}
	}
	if envName == "" && len(envs) > 0 {
		envName, _ = cli.SelectDefaultEnvironment(envs)
	}
	return envs, envFile, envName, nil
}

func envPaths(path, work string) []string {
	out := make([]string, 0, 2)
	if path != "" {
		out = append(out, filepath.Dir(path))
	}
	if work != "" && (len(out) == 0 || out[len(out)-1] != work) {
		out = append(out, work)
	}
	return out
}

func environmentSet(src EnvironmentSet) vars.EnvironmentSet {
	if len(src) == 0 {
		return nil
	}
	out := make(vars.EnvironmentSet, len(src))
	for env, vals := range src {
		out[env] = maps.Clone(vals)
	}
	return out
}

func compareTargets(src []string) ([]string, error) {
	if len(src) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(src))
	out := make([]string, 0, len(src))
	for _, item := range src {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		if err := cli.ValidateReservedEnvironment(name, "compare.targets"); err != nil {
			return nil, UsageError{err: err}
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil, nil
	}
	if len(out) < 2 {
		return nil, UsageError{err: ErrTooFewTargets}
	}
	return out, nil
}

func httpOptions(opt HTTPOptions) httpclient.Options {
	return httpclient.Options{
		Timeout:            timeoutOf(opt.Timeout),
		FollowRedirects:    boolVal(opt.FollowRedirects, true),
		InsecureSkipVerify: opt.InsecureSkipVerify,
		ProxyURL:           strings.TrimSpace(opt.ProxyURL),
	}
}

func grpcOptions(opt GRPCOptions) grpcclient.Options {
	return grpcclient.Options{
		DefaultPlaintext:    boolVal(opt.Plaintext, true),
		DefaultPlaintextSet: true,
	}
}

func timeoutOf(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return DefaultHTTPTimeout
}

func boolVal(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}
