package core

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func BuildCompareSpec(targets []string, baseline string) *restfile.CompareSpec {
	envs := normalizeCompareTargets(targets)
	if len(envs) < 2 {
		return nil
	}

	base, found := normalizeCompareBaseline(envs, baseline)
	switch {
	case base == "":
		base = envs[0]
	case !found:
		envs = append(envs, base)
	}

	return &restfile.CompareSpec{
		Environments: envs,
		Baseline:     base,
	}
}

func CloneCompareSpec(spec *restfile.CompareSpec) *restfile.CompareSpec {
	if spec == nil {
		return nil
	}

	cp := *spec
	if len(spec.Environments) > 0 {
		cp.Environments = append([]string(nil), spec.Environments...)
	}
	return &cp
}

func prepareCompareSpec(spec *restfile.CompareSpec) *restfile.CompareSpec {
	if spec == nil {
		return nil
	}

	envs := normalizeCompareTargets(spec.Environments)
	base, found := normalizeCompareBaseline(envs, spec.Baseline)
	switch {
	case len(envs) == 0:
	case base == "":
		base = envs[0]
	case found:
	default:
		// Preserve an explicit missing baseline so callers can keep the requested
		// label while still falling back to the first row for effective diffing.
	}

	return &restfile.CompareSpec{
		Environments: envs,
		Baseline:     base,
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

func normalizeCompareBaseline(envs []string, baseline string) (string, bool) {
	base := strings.TrimSpace(baseline)
	if base == "" {
		return "", false
	}
	for _, env := range envs {
		if strings.EqualFold(env, base) {
			return env, true
		}
	}
	return base, false
}
