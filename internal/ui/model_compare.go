package ui

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (m *Model) compareSpecForRequest(req *restfile.Request) *restfile.CompareSpec {
	if req == nil || req.Metadata.Compare == nil {
		return nil
	}
	if spec := buildConfigCompareSpec(m.cfg.CompareTargets, m.cfg.CompareBase); spec != nil {
		return spec
	}
	return cloneCompareSpec(req.Metadata.Compare)
}

func buildConfigCompareSpec(targets []string, baseline string) *restfile.CompareSpec {
	clean := normalizeCompareTargets(targets)
	if len(clean) < 2 {
		return nil
	}

	base := strings.TrimSpace(baseline)
	if base == "" {
		base = clean[0]
	} else {
		match := ""
		for _, env := range clean {
			if strings.EqualFold(env, base) {
				match = env
				break
			}
		}
		if match == "" {
			clean = append(clean, base)
			match = base
		}
		base = match
	}

	return &restfile.CompareSpec{
		Environments: clean,
		Baseline:     base,
	}
}

func cloneCompareSpec(spec *restfile.CompareSpec) *restfile.CompareSpec {
	if spec == nil {
		return nil
	}

	clone := *spec
	if len(spec.Environments) > 0 {
		clone.Environments = append([]string(nil), spec.Environments...)
	}
	return &clone
}

func normalizeCompareTargets(targets []string) []string {
	if len(targets) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(targets))
	result := make([]string, 0, len(targets))
	for _, target := range targets {
		value := strings.TrimSpace(target)
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		result = append(result, value)
	}

	if len(result) == 0 {
		return nil
	}
	return result
}
