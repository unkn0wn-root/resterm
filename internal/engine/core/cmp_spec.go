package core

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// BuildCompareSpec turns a run-wide compare override into a spec, or returns
// nil when the override does not name at least two targets.
func BuildCompareSpec(cfg engine.CompareConfig) *restfile.CompareSpec {
	envs := normalizeCompareTargets(cfg.Targets)
	if len(envs) < 2 {
		return nil
	}

	base := normalizeCompareBaseline(envs, cfg.Base)
	if base == "" {
		base = envs[0]
	}

	return &restfile.CompareSpec{
		Environments: envs,
		Baseline:     base,
		Group:        strings.TrimSpace(cfg.Group),
	}
}

// NormalizeCompareSpec keeps an explicitly set but unknown baseline as typed
// so the run can reject it before any request is sent.
func NormalizeCompareSpec(spec *restfile.CompareSpec) *restfile.CompareSpec {
	if spec == nil {
		return nil
	}

	envs := normalizeCompareTargets(spec.Environments)
	base := normalizeCompareBaseline(envs, spec.Baseline)
	if len(envs) > 0 && base == "" {
		base = envs[0]
	}

	return &restfile.CompareSpec{
		Environments: envs,
		Baseline:     base,
		Group:        strings.TrimSpace(spec.Group),
	}
}

func normalizeCompareTargets(targets []string) []string {
	if len(targets) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(targets))
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		val := strings.TrimSpace(target)
		if val == "" {
			continue
		}
		key := strings.ToLower(val)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, val)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeCompareBaseline(envs []string, baseline string) string {
	base := strings.TrimSpace(baseline)
	if base == "" {
		return ""
	}
	for _, env := range envs {
		if strings.EqualFold(env, base) {
			return env
		}
	}
	return base
}
