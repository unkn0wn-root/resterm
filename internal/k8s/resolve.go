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
) (*Config, error) {
	if spec == nil {
		return nil, nil
	}

	p, err := resolveProfileSpec(spec, fileProfiles, globalProfiles)
	if err != nil {
		return nil, err
	}
	p, err = expandProfile(p, resolver)
	if err != nil {
		return nil, err
	}

	cfg, err := normalizeProfile(p)
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

	base, ok := resolveNamedProfile(fileProfiles, globalProfiles, use)
	if !ok {
		return restfile.K8sProfile{}, fmt.Errorf("k8s: profile %q not found", use)
	}
	base.Name = use

	if spec.Inline == nil {
		return base, nil
	}
	return mergeProfile(base, *spec.Inline), nil
}

func resolveNamedProfile(
	fileProfiles []restfile.K8sProfile,
	globalProfiles []restfile.K8sProfile,
	name string,
) (restfile.K8sProfile, bool) {
	sf := func(p restfile.K8sProfile) restfile.K8sScope { return p.Scope }
	nf := func(p restfile.K8sProfile) string { return p.Name }
	p, ok := restfile.ResolveNamedScoped(
		fileProfiles,
		globalProfiles,
		name,
		restfile.K8sScopeFile,
		restfile.K8sScopeGlobal,
		sf,
		nf,
	)
	if !ok {
		return restfile.K8sProfile{}, false
	}
	return *p, true
}

func mergeProfile(base restfile.K8sProfile, override restfile.K8sProfile) restfile.K8sProfile {
	out := base

	connprofile.SetIf(&out.Name, override.Name)
	connprofile.SetIf(&out.Namespace, override.Namespace)

	target := strings.TrimSpace(override.Target)
	if target != "" {
		out.Target = target
		out.Pod = ""
		k, n, err := k8starget.ParseRef(target)
		if err == nil && k == TargetPod {
			out.Pod = n
		}
	}
	if pod := strings.TrimSpace(override.Pod); pod != "" {
		out.Pod = pod
		if target == "" {
			out.Target = ""
		}
	}

	if port := strings.TrimSpace(override.PortStr); port != "" {
		out.PortStr = port
		out.Port = override.Port
	}

	connprofile.SetIf(&out.Context, override.Context)
	connprofile.SetIf(&out.Kubeconfig, override.Kubeconfig)
	connprofile.SetIf(&out.Container, override.Container)
	connprofile.SetIf(&out.Address, override.Address)

	if port := strings.TrimSpace(override.LocalPortStr); port != "" {
		out.LocalPortStr = port
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
	fs := []*string{
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
	for _, f := range fs {
		val := strings.TrimSpace(*f)
		if val == "" {
			continue
		}
		v, err := connprofile.ExpandValue(val, resolver)
		if err != nil {
			return restfile.K8sProfile{}, err
		}
		*f = v
	}
	return p, nil
}
