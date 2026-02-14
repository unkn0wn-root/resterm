package k8s

import (
	"fmt"
	"strings"

	k8starget "github.com/unkn0wn-root/resterm/internal/k8s/target"
	"github.com/unkn0wn-root/resterm/internal/profileutil"
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

	var merged restfile.K8sProfile
	var useFound bool

	use := strings.TrimSpace(spec.Use)
	if use != "" {
		if prof, ok := lookupProfile(fileProfiles, use, restfile.K8sScopeFile); ok {
			merged = *prof
			useFound = true
		} else if prof, ok := lookupProfile(globalProfiles, use, restfile.K8sScopeGlobal); ok {
			merged = *prof
			useFound = true
		}
		merged.Name = use
	}

	if use != "" && !useFound {
		return nil, fmt.Errorf("k8s: profile %q not found", use)
	}

	if spec.Inline != nil {
		merged = mergeProfile(merged, *spec.Inline)
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

	profileutil.SetIf(&out.Name, override.Name)
	profileutil.SetIf(&out.Namespace, override.Namespace)

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

	profileutil.SetIf(&out.Context, override.Context)
	profileutil.SetIf(&out.Kubeconfig, override.Kubeconfig)
	profileutil.SetIf(&out.Container, override.Container)
	profileutil.SetIf(&out.Address, override.Address)

	if v := strings.TrimSpace(override.LocalPortStr); v != "" {
		out.LocalPortStr = v
		out.LocalPort = override.LocalPort
	}

	if override.Persist.Set {
		out.Persist = override.Persist
	}
	if profileutil.OptSet(override.PodWait, override.PodWaitStr) {
		out.PodWait = override.PodWait
		out.PodWaitStr = override.PodWaitStr
	}
	if profileutil.OptSet(override.Retries, override.RetriesStr) {
		out.Retries = override.Retries
		out.RetriesStr = override.RetriesStr
	}

	return out
}

func expandProfile(p restfile.K8sProfile, resolver *vars.Resolver) (restfile.K8sProfile, error) {
	fields := []*string{
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
	}

	for _, field := range fields {
		val := strings.TrimSpace(*field)
		if val == "" {
			continue
		}
		expanded, err := profileutil.ExpandValue(val, resolver)
		if err != nil {
			return restfile.K8sProfile{}, err
		}
		*field = expanded
	}

	return p, nil
}
