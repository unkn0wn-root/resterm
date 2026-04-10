package headless

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func (e *Engine) compareSpec(req *restfile.Request) *restfile.CompareSpec {
	if req == nil {
		return nil
	}
	if spec := buildConfigCompareSpec(e.cfg.CompareTargets, e.cfg.CompareBase); spec != nil {
		return spec
	}
	return cloneCompareSpec(req.Metadata.Compare)
}

func buildConfigCompareSpec(targets []string, base string) *restfile.CompareSpec {
	clean := normalizeCompareTargets(targets)
	if len(clean) < 2 {
		return nil
	}
	base = strings.TrimSpace(base)
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
	cp := *spec
	if len(spec.Environments) > 0 {
		cp.Environments = append([]string(nil), spec.Environments...)
	}
	return &cp
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
