package headless

import (
	"bytes"
	"fmt"
	"maps"
	"path/filepath"
	"time"

	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/runx/check"
	str "github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// DefaultHTTPTimeout is the timeout applied when Options.HTTP.Timeout is zero.
const DefaultHTTPTimeout = 30 * time.Second

// Plan stores a prepared run configuration.
// A Plan can be reused across multiple RunPlan calls, including concurrently.
// The zero value is invalid.
type Plan struct {
	pl *runner.Plan
}

// Build prepares o for execution and returns a reusable plan.
// Use Build with RunPlan when you want to validate once and reuse the same
// source and selection across multiple runs.
func Build(o Options) (Plan, error) {
	ro, err := buildOptions(o)
	if err != nil {
		return Plan{}, err
	}

	pl, err := runner.Build(ro)
	if err != nil {
		if runner.IsUsageError(err) {
			return Plan{}, UsageError{err: err}
		}
		return Plan{}, err
	}
	return Plan{pl: pl}, nil
}

type builder struct {
	opt Options
	out runner.Options
	sel runner.Select
}

func buildOptions(o Options) (runner.Options, error) {
	sel, err := selectionOptions(o.Selection)
	if err != nil {
		return runner.Options{}, err
	}
	b := builder{opt: o, sel: sel}
	if err := b.buildPaths(); err != nil {
		return runner.Options{}, err
	}
	b.buildSelection()
	if err := b.buildEnvironment(); err != nil {
		return runner.Options{}, err
	}
	if err := b.buildCompare(); err != nil {
		return runner.Options{}, err
	}
	b.buildHTTP()
	b.buildGRPC()
	b.finalize()
	return b.out, nil
}

func (b *builder) buildPaths() error {
	path, err := absPath(b.opt.Source.Path)
	if err != nil {
		return UsageError{err: fmt.Errorf("resolve source.path: %w", err)}
	}
	if path == "" {
		return UsageError{err: ErrNoSourcePath}
	}
	work, err := workspacePath(path, b.opt.WorkspaceRoot)
	if err != nil {
		return UsageError{err: fmt.Errorf("resolve workspaceRoot: %w", err)}
	}
	b.out.FilePath = path
	b.out.WorkspaceRoot = work
	return nil
}

func (b *builder) buildSelection() {
	b.out.Select = b.sel
}

func (b *builder) buildEnvironment() error {
	envs, file, name, err := environmentOptions(b.opt, b.out.FilePath, b.out.WorkspaceRoot)
	if err != nil {
		return err
	}
	b.out.EnvSet = envs
	b.out.EnvironmentFile = file
	b.out.EnvName = name
	return nil
}

func (b *builder) buildCompare() error {
	targets, err := compareTargets(b.opt.Compare.Targets)
	if err != nil {
		return err
	}
	base := str.Trim(b.opt.Compare.Base)
	if err := runcheck.ValidateConcreteEnvironment(base, "compare.base"); err != nil {
		return UsageError{err: err}
	}
	ns := runcheck.Names{
		Profile:  "profile.enabled",
		Compare:  "compare.targets",
		Workflow: "selection.workflow",
	}
	if err := runcheck.ValidateProfileCompare(
		b.opt.Profile.Enabled,
		len(targets) > 0,
		ns,
	); err != nil {
		return UsageError{err: err}
	}
	if err := runcheck.ValidateWorkflowMode(
		b.sel.Workflow != "",
		b.opt.Profile.Enabled,
		len(targets) > 0,
		ns,
	); err != nil {
		return UsageError{err: err}
	}
	b.out.CompareTargets = targets
	b.out.CompareBase = base
	return nil
}

func (b *builder) buildHTTP() {
	b.out.HTTPOptions = httpOptions(b.opt.HTTP)
}

func (b *builder) buildGRPC() {
	b.out.GRPCOptions = grpcOptions(b.opt.GRPC)
}

func (b *builder) finalize() {
	b.out.Version = str.Trim(b.opt.Version)
	b.out.FileContent = bytes.Clone(b.opt.Source.Content)
	b.out.Recursive = b.opt.Recursive
	b.out.ArtifactDir = str.Trim(b.opt.State.ArtifactDir)
	b.out.StateDir = str.Trim(b.opt.State.StateDir)
	b.out.PersistGlobals = b.opt.State.PersistGlobals
	b.out.PersistAuth = b.opt.State.PersistAuth
	b.out.History = b.opt.State.History
	b.out.FailFast = b.opt.FailFast
	b.out.Profile = b.opt.Profile.Enabled
}

func httpOptions(opt HTTPOptions) httpclient.Options {
	return httpclient.Options{
		Timeout:            timeoutOf(opt.Timeout),
		FollowRedirects:    boolOr(opt.FollowRedirects, true),
		InsecureSkipVerify: opt.InsecureSkipVerify,
		ProxyURL:           str.Trim(opt.ProxyURL),
	}
}

func grpcOptions(opt GRPCOptions) grpcclient.Options {
	return grpcclient.Options{
		DefaultPlaintext:    boolOr(opt.Plaintext, true),
		DefaultPlaintextSet: true,
	}
}

func selectionOptions(sel Selection) (runner.Select, error) {
	out := runner.Select{
		Request:  str.Trim(sel.Request),
		Workflow: str.Trim(sel.Workflow),
		Tag:      str.Trim(sel.Tag),
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
	path = str.Trim(path)
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
	work = str.Trim(work)
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
	envFile := str.Trim(opt.Environment.FilePath)

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

	envName := str.Trim(opt.Environment.Name)
	if err := runcheck.ValidateConcreteEnvironment(envName, "environment.name"); err != nil {
		return nil, "", "", UsageError{err: err}
	}
	if envName == "" && len(envs) > 0 {
		envName = vars.DefaultEnvironment(envs)
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
		name := str.Trim(item)
		if name == "" {
			continue
		}
		if err := runcheck.ValidateConcreteEnvironment(name, "compare.targets"); err != nil {
			return nil, UsageError{err: err}
		}
		key := str.LowerTrim(name)
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

func timeoutOf(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return DefaultHTTPTimeout
}

func boolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}
