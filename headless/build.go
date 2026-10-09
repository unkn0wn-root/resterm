package headless

import (
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
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
		return Plan{}, wrapUsage(err)
	}
	return Plan{pl: pl}, nil
}

func buildOptions(o Options) (runner.Options, error) {
	sel, err := selectionOptions(o.Selection)
	if err != nil {
		return runner.Options{}, err
	}

	path, err := absPath(o.Source.Path)
	if err != nil {
		return runner.Options{}, usageError("resolve source.path: %w", err)
	}
	if path == "" {
		return runner.Options{}, UsageError{err: ErrNoSourcePath}
	}
	work := filepath.Dir(path)
	if root := strings.TrimSpace(o.WorkspaceRoot); root != "" {
		work, err = absPath(root)
		if err != nil {
			return runner.Options{}, usageError("resolve workspaceRoot: %w", err)
		}
	}

	env, err := environmentOptions(o.Environment, path, work)
	if err != nil {
		return runner.Options{}, err
	}
	cmp, err := compareOptions(o, sel.Workflow != "", env)
	if err != nil {
		return runner.Options{}, err
	}
	httpOpts, err := httpOptions(o.HTTP)
	if err != nil {
		return runner.Options{}, err
	}

	return runner.Options{
		Version:         o.Version,
		FilePath:        path,
		FileContent:     o.Source.Content,
		WorkspaceRoot:   work,
		Recursive:       o.Recursive,
		ArtifactDir:     o.State.ArtifactDir,
		StateDir:        o.State.StateDir,
		PersistGlobals:  o.State.PersistGlobals,
		PersistAuth:     o.State.PersistAuth,
		History:         o.State.History,
		FailFast:        o.FailFast,
		Catalog:         env.cat,
		Selection:       env.sel,
		EnvironmentFile: env.file,
		Compare:         cmp,
		Profile:         o.Profile.Enabled,
		HTTPOptions:     httpOpts,
		GRPCOptions:     grpcx.Options{DefaultPlaintext: restfile.OptOf(boolOr(o.GRPC.Plaintext, true))},
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
		return runner.Select{}, usageError(
			"selection.workflow cannot be combined with selection.request, selection.tag, or selection.all",
		)
	case out.All && (out.Request != "" || out.Tag != ""):
		return runner.Select{}, usageError("selection.all cannot be combined with selection.request or selection.tag")
	case out.Request != "" && out.Tag != "":
		return runner.Select{}, usageError("selection.request cannot be combined with selection.tag")
	default:
		return out, nil
	}
}

type environment struct {
	cat  vars.Catalog
	file string
	sel  vars.Selection
}

func environmentOptions(opt EnvironmentOptions, path, work string) (environment, error) {
	if opt.Set != nil && opt.Grouped != nil {
		return environment{}, usageError("environment.set cannot be combined with environment.grouped")
	}
	if strings.TrimSpace(opt.Name) != "" && opt.Selection != nil {
		return environment{}, usageError("environment.name cannot be combined with environment.selection")
	}

	var env environment
	var err error
	file := strings.TrimSpace(opt.FilePath)
	switch {
	case opt.Set != nil:
		env.cat, err = vars.NewCatalog(environmentSet(opt.Set))
	case opt.Grouped != nil:
		env.cat, err = groupedCatalog(opt.Grouped)
	case file != "":
		env.file = file
		env.cat, err = vars.LoadEnvironmentFile(file)
	default:
		env.cat, env.file, err = vars.Discover(filepath.Dir(path), work)
	}
	if err != nil {
		return environment{}, usageError("load environments: %w", err)
	}

	name := strings.TrimSpace(opt.Name)
	if err := runcheck.ValidateConcreteEnvironment(name, "environment.name"); err != nil {
		return environment{}, UsageError{err: err}
	}
	env.sel, err = env.cat.Select(name, map[string]string(opt.Selection))
	if err != nil {
		return environment{}, UsageError{err: err}
	}
	return env, nil
}

func groupedCatalog(src *GroupedEnvironmentSet) (vars.Catalog, error) {
	groups := make([]vars.Group, 0, len(src.Groups))
	for name, group := range src.Groups {
		groups = append(groups, vars.Group{
			Name:     name,
			Default:  group.Default,
			Profiles: environmentSet(group.Profiles),
		})
	}
	return vars.NewGroupedCatalog(src.Shared, groups)
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

func compareOptions(o Options, workflow bool, env environment) (engine.CompareConfig, error) {
	targets, err := compareTargets(o.Compare.Targets)
	if err != nil {
		return engine.CompareConfig{}, err
	}
	base := strings.TrimSpace(o.Compare.Base)
	if err := runcheck.ValidateConcreteEnvironment(base, "compare.base"); err != nil {
		return engine.CompareConfig{}, UsageError{err: err}
	}
	ns := runcheck.Names{
		Profile:  "profile.enabled",
		Compare:  "compare.targets",
		Workflow: "selection.workflow",
	}
	compare := len(targets) > 0
	if err := runcheck.ValidateProfileCompare(o.Profile.Enabled, compare, ns); err != nil {
		return engine.CompareConfig{}, UsageError{err: err}
	}
	if err := runcheck.ValidateWorkflowMode(workflow, o.Profile.Enabled, compare, ns); err != nil {
		return engine.CompareConfig{}, UsageError{err: err}
	}
	group := strings.TrimSpace(o.Compare.Group)
	if group != "" && !compare {
		return engine.CompareConfig{}, usageError("compare.group requires compare.targets")
	}
	if compare {
		if _, err := env.cat.CompareTargets(env.sel, group, base, targets); err != nil {
			return engine.CompareConfig{}, UsageError{err: err}
		}
	}
	return engine.CompareConfig{Targets: targets, Base: base, Group: group}, nil
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

func httpOptions(opt HTTPOptions) (httpx.Options, error) {
	timeout := DefaultHTTPTimeout
	if opt.Timeout > 0 {
		timeout = opt.Timeout
	}
	out := httpx.Options{
		Timeout:            timeout,
		FollowRedirects:    boolOr(opt.FollowRedirects, true),
		InsecureSkipVerify: opt.InsecureSkipVerify,
		ProxyURL:           strings.TrimSpace(opt.ProxyURL),
	}
	if opt.MaxRedirects != nil {
		if err := nonNegative("http.maxRedirects", int64(*opt.MaxRedirects)); err != nil {
			return httpx.Options{}, err
		}
		out.MaxRedirects = restfile.OptOf(*opt.MaxRedirects)
	}
	if opt.MaxResponseBytes != nil {
		if err := nonNegative("http.maxResponseBytes", *opt.MaxResponseBytes); err != nil {
			return httpx.Options{}, err
		}
		// Zero means no limit, the same as max-response-size none.
		out.MaxResponseBytes = bytesize.Of(*opt.MaxResponseBytes)
	}
	return out, nil
}

func nonNegative(name string, value int64) error {
	if value < 0 {
		return usageError("%s: %d must be non-negative", name, value)
	}
	return nil
}

func absPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	return filepath.Abs(path)
}

func boolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}
