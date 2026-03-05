package k8s

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/connprofile"
	k8starget "github.com/unkn0wn-root/resterm/internal/k8s/target"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func Resolve(
	spec *restfile.K8sSpec,
	fileProfiles []restfile.K8sProfile,
	globalProfiles []restfile.K8sProfile,
	resolver *vars.Resolver,
	envLabel string,
) (*Cfg, error) {
	if spec == nil {
		return nil, nil
	}

	merged, err := resolveProfileSpec(spec, fileProfiles, globalProfiles)
	if err != nil {
		return nil, err
	}

	expanded, err := expandProfile(merged, resolver)
	if err != nil {
		return nil, err
	}

	cfg, err := NormalizeProfile(expanded)
	if err != nil {
		return nil, err
	}
	cfg.Label = strings.TrimSpace(envLabel)
	return &cfg, nil
}

func resolveProfileSpec(
	spec *restfile.K8sSpec,
	fileProfiles []restfile.K8sProfile,
	globalProfiles []restfile.K8sProfile,
) (restfile.K8sProfile, error) {
	use := strings.TrimSpace(spec.Use)
	if use == "" {
		if spec.Inline == nil {
			return restfile.K8sProfile{}, nil
		}
		return *spec.Inline, nil
	}

	base, ok := lookupNamedProfile(fileProfiles, globalProfiles, use)
	if !ok {
		return restfile.K8sProfile{}, fmt.Errorf("k8s: profile %q not found", use)
	}
	base.Name = use

	if spec.Inline == nil {
		return base, nil
	}
	return mergeProfile(base, *spec.Inline), nil
}

func lookupNamedProfile(
	fileProfiles []restfile.K8sProfile,
	globalProfiles []restfile.K8sProfile,
	name string,
) (restfile.K8sProfile, bool) {
	if prof, ok := lookupProfile(fileProfiles, name, restfile.K8sScopeFile); ok {
		return *prof, true
	}
	if prof, ok := lookupProfile(globalProfiles, name, restfile.K8sScopeGlobal); ok {
		return *prof, true
	}
	return restfile.K8sProfile{}, false
}

func lookupProfile(
	profiles []restfile.K8sProfile,
	name string,
	scope restfile.K8sScope,
) (*restfile.K8sProfile, bool) {
	sf := func(p restfile.K8sProfile) restfile.K8sScope { return p.Scope }
	nf := func(p restfile.K8sProfile) string { return p.Name }
	return restfile.LookupNamedScoped(profiles, name, scope, sf, nf)
}

func mergeProfile(base restfile.K8sProfile, override restfile.K8sProfile) restfile.K8sProfile {
	out := base

	connprofile.SetIf(&out.Name, override.Name)
	connprofile.SetIf(&out.Namespace, override.Namespace)

	// Target and Pod override precedence:
	// 1) Target overrides clear Pod by default.
	// 2) A literal pod target (pod:<name> or <name>) mirrors Pod.
	// 3) Explicit Pod override wins when both Target and Pod are set.
	//
	// We intentionally do not fail merge-time parsing for templated targets.
	// NormalizeProfile validates the expanded final value later.
	target := strings.TrimSpace(override.Target)
	if target != "" {
		out.Target = target
		out.Pod = ""
		k, n, err := k8starget.ParseRef(target)
		if err == nil && k == targetKindPod {
			out.Pod = n
		}
	}
	if v := strings.TrimSpace(override.Pod); v != "" {
		out.Pod = v
		if target == "" {
			out.Target = ""
		}
	}

	if v := strings.TrimSpace(override.PortStr); v != "" {
		out.PortStr = v
		out.Port = override.Port
	}

	connprofile.SetIf(&out.Context, override.Context)
	connprofile.SetIf(&out.Kubeconfig, override.Kubeconfig)
	connprofile.SetIf(&out.Container, override.Container)
	connprofile.SetIf(&out.Address, override.Address)

	if v := strings.TrimSpace(override.LocalPortStr); v != "" {
		out.LocalPortStr = v
		out.LocalPort = override.LocalPort
	}

	if override.Persist.Set {
		out.Persist = override.Persist
	}
	if connprofile.OptSet(override.PodWait, override.PodWaitStr) {
		out.PodWait = override.PodWait
		out.PodWaitStr = override.PodWaitStr
	}
	if connprofile.OptSet(override.Retries, override.RetriesStr) {
		out.Retries = override.Retries
		out.RetriesStr = override.RetriesStr
	}

	return out
}

func expandProfile(p restfile.K8sProfile, resolver *vars.Resolver) (restfile.K8sProfile, error) {
	if err := expandProfileFields(
		resolver,
		&p.Name,
		&p.Namespace,
		&p.Target,
		&p.Pod,
		&p.PortStr,
		&p.Context,
		&p.Kubeconfig,
		&p.Container,
		&p.Address,
		&p.LocalPortStr,
		&p.PodWaitStr,
		&p.RetriesStr,
	); err != nil {
		return restfile.K8sProfile{}, err
	}
	return p, nil
}

func expandProfileFields(resolver *vars.Resolver, fields ...*string) error {
	for _, field := range fields {
		val := strings.TrimSpace(*field)
		if val == "" {
			continue
		}

		expanded, err := connprofile.ExpandValue(val, resolver)
		if err != nil {
			return err
		}
		*field = expanded
	}
	return nil
}
