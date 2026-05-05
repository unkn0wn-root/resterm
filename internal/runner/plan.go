package runner

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/engine"
	engheadless "github.com/unkn0wn-root/resterm/internal/engine/headless"
	"github.com/unkn0wn-root/resterm/internal/grpcclient"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runx/check"
	"github.com/unkn0wn-root/resterm/internal/runx/report"
	str "github.com/unkn0wn-root/resterm/internal/util"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

// Plan stores prepared runner inputs that can be executed multiple times.
// It is safe for concurrent reuse; runs that share persisted state may be
// internally serialized.
type Plan struct {
	mu     sync.Mutex
	serial bool
	opt    Options
	state  statePaths
	doc    *restfile.Document
	sel    selectedTarget
}

func Build(opts Options) (*Plan, error) {
	path := str.Trim(opts.FilePath)
	if path == "" {
		return nil, usageError("--file is required")
	}

	data := bytes.Clone(opts.FileContent)
	if data == nil {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		data = raw
	}

	doc := parser.Parse(path, data)
	if err := parser.Check(doc); err != nil {
		return nil, err
	}

	work := str.Trim(opts.WorkspaceRoot)
	if work == "" {
		work = filepath.Dir(path)
	}

	art, err := absCleanPath(opts.ArtifactDir)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact dir: %w", err)
	}
	ns := runcheck.Names{
		Profile:  "--profile",
		Compare:  "--compare",
		Workflow: "--workflow",
	}

	err = runcheck.ValidateProfileCompare(opts.Profile, len(opts.CompareTargets) > 0, ns)
	if err != nil {
		return nil, UsageError{err: err}
	}

	st, err := resolveStatePaths(opts)
	if err != nil {
		return nil, fmt.Errorf("resolve runner state: %w", err)
	}

	sel, err := selectTarget(doc, newSelectSpec(opts.Select))
	if err != nil {
		return nil, err
	}
	if err := runcheck.ValidateWorkflowMode(
		sel.hasWorkflow(),
		opts.Profile,
		len(opts.CompareTargets) > 0,
		ns,
	); err != nil {
		return nil, UsageError{err: err}
	}

	return &Plan{
		serial: usesStateDir(opts),
		opt:    clonePlanOptions(opts, path, work, art),
		doc:    doc,
		sel:    sel,
		state:  st,
	}, nil
}

func RunPlan(ctx context.Context, pl *Plan) (*Report, error) {
	if ctx == nil {
		return nil, ErrNilContext
	}
	if pl == nil {
		return nil, usageError("runner plan is nil")
	}
	if pl.serial {
		pl.mu.Lock()
		defer pl.mu.Unlock()
	}

	doc := cloneDoc(pl.doc)
	tg, err := pl.sel.resolve(doc)
	if err != nil {
		return nil, err
	}

	opt := pl.opt
	start := time.Now()
	hist := openHistoryStore(pl.state, opt)

	exec := engheadless.New(engine.Config{
		FilePath:        opt.FilePath,
		Client:          opt.Client,
		EnvironmentSet:  cloneEnvSet(opt.EnvSet),
		EnvironmentName: opt.EnvName,
		EnvironmentFile: opt.EnvironmentFile,
		CompareTargets:  slices.Clone(opt.CompareTargets),
		CompareBase:     str.Trim(opt.CompareBase),
		HTTPOptions:     cloneHTTPOptions(opt.HTTPOptions),
		GRPCOptions:     cloneGRPCOptions(opt.GRPCOptions),
		WorkspaceRoot:   opt.WorkspaceRoot,
		Recursive:       opt.Recursive,
		History:         hist,
	})

	defer func() { _ = exec.Close() }()

	if err := loadRunnerState(exec, pl.state, opt); err != nil {
		return nil, fmt.Errorf("load runner state: %w", err)
	}

	rep := &Report{
		Version:       opt.Version,
		SchemaVersion: runfmt.ReportSchemaVersion,
		FilePath:      opt.FilePath,
		EnvName:       opt.EnvName,
		StartedAt:     start,
	}

	if tg.workflow != nil {
		rep.Results = make([]Result, 0, 1)
		out, err := exec.ExecuteWorkflowContext(ctx, doc, tg.workflow, opt.EnvName)
		if err != nil {
			return nil, err
		}
		rep.add(workflowRunResult(*out, opt.EnvName))
		return finishRun(rep, exec, pl.state, opt)
	}

	rep.Results = make([]Result, 0, len(tg.requests))
	for i, req := range tg.requests {
		runReq := cloneReq(req)
		if opt.Profile && runReq.Metadata.Profile == nil {
			runReq.Metadata.Profile = &restfile.ProfileSpec{}
		}
		res, err := exec.ExecuteRequestContext(ctx, doc, runReq, opt.EnvName)
		if err != nil {
			return nil, err
		}
		switch {
		case res.Workflow != nil:
			rep.add(workflowRunResult(*res.Workflow, opt.EnvName))
		case res.Compare != nil:
			rep.add(compareRunResult(runReq, *res.Compare, opt.EnvName))
		case res.Profile != nil:
			rep.add(profileRunResult(runReq, *res.Profile, opt.EnvName))
		default:
			rep.add(requestRunResult(runReq, res, opt.EnvName))
		}
		if opt.FailFast && resultFailed(rep.Results[len(rep.Results)-1]) {
			rep.StopReason = stopReasonFailFast
			for _, skipped := range tg.requests[i+1:] {
				rep.add(skippedRequestResult(skipped, opt.EnvName, "skipped after --fail-fast"))
			}
			break
		}
	}
	return finishRun(rep, exec, pl.state, opt)
}

func finishRun(
	rep *Report,
	exec engine.Executor,
	st statePaths,
	opt Options,
) (*Report, error) {
	if rep == nil {
		return nil, nil
	}
	rep.EndedAt = time.Now()
	rep.Duration = rep.EndedAt.Sub(rep.StartedAt)
	if err := saveRunnerState(exec, st, opt); err != nil {
		return nil, fmt.Errorf("save runner state: %w", err)
	}
	if err := rep.writeArtifacts(opt.ArtifactDir); err != nil {
		return nil, err
	}
	return rep, nil
}

func clonePlanOptions(opts Options, path, work, art string) Options {
	out := opts
	out.Version = str.Trim(opts.Version)
	out.FilePath = path
	out.FileContent = nil
	out.WorkspaceRoot = work
	out.ArtifactDir = art
	out.StateDir = str.Trim(opts.StateDir)
	out.EnvSet = cloneEnvSet(opts.EnvSet)
	out.EnvName = str.Trim(opts.EnvName)
	out.EnvironmentFile = str.Trim(opts.EnvironmentFile)
	out.CompareTargets = slices.Clone(opts.CompareTargets)
	out.CompareBase = str.Trim(opts.CompareBase)
	out.HTTPOptions = cloneHTTPOptions(opts.HTTPOptions)
	out.GRPCOptions = cloneGRPCOptions(opts.GRPCOptions)
	out.Client = opts.Client.Clone()
	return out
}

func cloneEnvSet(src vars.EnvironmentSet) vars.EnvironmentSet {
	if len(src) == 0 {
		return nil
	}
	out := make(vars.EnvironmentSet, len(src))
	for name, vals := range src {
		out[name] = maps.Clone(vals)
	}
	return out
}

func cloneHTTPOptions(src httpclient.Options) httpclient.Options {
	out := src
	out.RootCAs = slices.Clone(src.RootCAs)
	out.FallbackBaseDirs = slices.Clone(src.FallbackBaseDirs)
	if src.TraceBudget != nil {
		b := src.TraceBudget.Clone()
		out.TraceBudget = &b
	}
	return out
}

func cloneGRPCOptions(src grpcclient.Options) grpcclient.Options {
	out := src
	out.DescriptorPaths = slices.Clone(src.DescriptorPaths)
	out.RootCAs = slices.Clone(src.RootCAs)
	return out
}
