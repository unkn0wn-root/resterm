package headless

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const defTimeout = 30 * time.Second

func Run(ctx context.Context, opt Opt) (*Report, error) {
	ro, err := runnerOpt(opt)
	if err != nil {
		return nil, err
	}
	rep, err := runner.RunContext(ctx, ro)
	if err != nil {
		if runner.IsUsageError(err) {
			return nil, ErrUsage{err: err}
		}
		return nil, err
	}
	return reportFromRunner(rep), nil
}

func runnerOpt(opt Opt) (runner.Options, error) {
	path, err := absPath(opt.FilePath)
	if err != nil {
		return runner.Options{}, err
	}
	work, err := workspacePath(path, opt.Workspace)
	if err != nil {
		return runner.Options{}, err
	}
	envs, envFile, envName, err := envOpt(opt, path, work)
	if err != nil {
		return runner.Options{}, err
	}
	targets, err := compareTargets(opt.CompareTargets)
	if err != nil {
		return runner.Options{}, err
	}
	base := strings.TrimSpace(opt.CompareBase)
	if err := cli.ValidateReservedEnvironment(base, "compareBase"); err != nil {
		return runner.Options{}, ErrUsage{err: err}
	}
	return runner.Options{
		Version:         strings.TrimSpace(opt.Version),
		FilePath:        path,
		FileContent:     append([]byte(nil), opt.FileContent...),
		WorkspaceRoot:   work,
		Recursive:       opt.Recursive,
		ArtifactDir:     strings.TrimSpace(opt.ArtifactDir),
		StateDir:        strings.TrimSpace(opt.StateDir),
		PersistGlobals:  opt.PersistGlobals,
		PersistAuth:     opt.PersistAuth,
		History:         opt.History,
		EnvSet:          envs,
		EnvName:         envName,
		EnvironmentFile: envFile,
		CompareTargets:  targets,
		CompareBase:     base,
		Profile:         opt.Profile,
		HTTPOptions:     httpOpt(opt.HTTP),
		GRPCOptions:     grpcOpt(opt.GRPC),
		Select: runner.Select{
			Request:  strings.TrimSpace(opt.Select.Request),
			Workflow: strings.TrimSpace(opt.Select.Workflow),
			Tag:      strings.TrimSpace(opt.Select.Tag),
			All:      opt.Select.All,
		},
	}, nil
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

func envOpt(opt Opt, path, work string) (vars.EnvironmentSet, string, string, error) {
	envs := envSet(opt.Envs)
	envFile := strings.TrimSpace(opt.EnvFile)
	switch {
	case len(envs) > 0:
	case envFile != "":
		set, err := vars.LoadEnvironmentFile(envFile)
		if err != nil {
			return nil, "", "", fmt.Errorf("load env file: %w", err)
		}
		envs = set
	default:
		set, file, err := vars.ResolveEnvironment(envPaths(path, work))
		if err != nil {
			return nil, "", "", fmt.Errorf("resolve env file: %w", err)
		}
		envs = set
		envFile = file
	}
	envName := strings.TrimSpace(opt.EnvName)
	if err := cli.ValidateReservedEnvironment(envName, "env"); err != nil {
		return nil, "", "", ErrUsage{err: err}
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

func envSet(src EnvSet) vars.EnvironmentSet {
	if len(src) == 0 {
		return nil
	}
	out := make(vars.EnvironmentSet, len(src))
	for env, vals := range src {
		if vals == nil {
			out[env] = nil
			continue
		}
		cp := make(map[string]string, len(vals))
		for key, val := range vals {
			cp[key] = val
		}
		out[env] = cp
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
		if err := cli.ValidateReservedEnvironment(name, "compareTargets"); err != nil {
			return nil, ErrUsage{err: err}
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
		return nil, ErrUsage{err: fmt.Errorf("compareTargets requires at least two environments")}
	}
	return out, nil
}

func httpOpt(opt HTTPOpt) httpclient.Options {
	return httpclient.Options{
		Timeout:            timeoutOf(opt.Timeout),
		FollowRedirects:    boolVal(opt.Follow, true),
		InsecureSkipVerify: opt.Insecure,
		ProxyURL:           strings.TrimSpace(opt.Proxy),
	}
}

func grpcOpt(opt GRPCOpt) grpcclient.Options {
	return grpcclient.Options{
		DefaultPlaintext:    boolVal(opt.Plaintext, true),
		DefaultPlaintextSet: true,
	}
}

func timeoutOf(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return defTimeout
}

func boolVal(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}
