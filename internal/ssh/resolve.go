package ssh

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/connprofile"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func Resolve(
	spec *restfile.SSHSpec,
	fileProfiles []restfile.SSHProfile,
	globalProfiles []restfile.SSHProfile,
	resolver *vars.Resolver,
	envLabel string,
) (*Config, error) {
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
	spec *restfile.SSHSpec,
	fileProfiles []restfile.SSHProfile,
	globalProfiles []restfile.SSHProfile,
) (restfile.SSHProfile, error) {
	use := strings.TrimSpace(spec.Use)
	if use == "" {
		if spec.Inline == nil {
			return restfile.SSHProfile{}, nil
		}
		return *spec.Inline, nil
	}

	base, ok := resolveNamedProfile(fileProfiles, globalProfiles, use)
	if !ok {
		return restfile.SSHProfile{}, fmt.Errorf("ssh profile %q not found", use)
	}
	base.Name = use

	if spec.Inline == nil {
		return base, nil
	}
	return mergeProfile(base, *spec.Inline), nil
}

func resolveNamedProfile(
	fileProfiles []restfile.SSHProfile,
	globalProfiles []restfile.SSHProfile,
	name string,
) (restfile.SSHProfile, bool) {
	sf := func(p restfile.SSHProfile) directive.Scope { return p.Scope }
	nf := func(p restfile.SSHProfile) string { return p.Name }
	p, ok := restfile.ResolveNamedScoped(
		fileProfiles,
		globalProfiles,
		name,
		directive.ScopeFile,
		directive.ScopeGlobal,
		sf,
		nf,
	)
	if !ok {
		return restfile.SSHProfile{}, false
	}
	return *p, true
}

func mergeProfile(base restfile.SSHProfile, override restfile.SSHProfile) restfile.SSHProfile {
	out := base
	connprofile.SetIf(&out.Name, override.Name)
	connprofile.SetIf(&out.Host, override.Host)
	if port := strings.TrimSpace(override.PortStr); port != "" {
		out.PortStr = port
		out.Port = override.Port
	}

	connprofile.SetIf(&out.User, override.User)
	connprofile.SetIf(&out.Pass, override.Pass)
	connprofile.SetIf(&out.Key, override.Key)
	connprofile.SetIf(&out.KeyPass, override.KeyPass)
	connprofile.SetIf(&out.KnownHosts, override.KnownHosts)
	if override.Agent.Set {
		out.Agent = override.Agent
	}
	if override.Strict.Set {
		out.Strict = override.Strict
	}
	if override.Persist.Set {
		out.Persist = override.Persist
	}
	if connprofile.OptSet(override.Timeout, override.TimeoutStr) {
		out.Timeout = override.Timeout
		out.TimeoutStr = override.TimeoutStr
	}
	if connprofile.OptSet(override.KeepAlive, override.KeepAliveStr) {
		out.KeepAlive = override.KeepAlive
		out.KeepAliveStr = override.KeepAliveStr
	}
	if connprofile.OptSet(override.Retries, override.RetriesStr) {
		out.Retries = override.Retries
		out.RetriesStr = override.RetriesStr
	}
	return out
}

func expandProfile(p restfile.SSHProfile, resolver *vars.Resolver) (restfile.SSHProfile, error) {
	fields := []*string{
		&p.Name,
		&p.Host,
		&p.User,
		&p.Pass,
		&p.Key,
		&p.KeyPass,
		&p.KnownHosts,
		&p.PortStr,
		&p.TimeoutStr,
		&p.KeepAliveStr,
		&p.RetriesStr,
	}

	for _, field := range fields {
		val := strings.TrimSpace(*field)
		if val == "" {
			continue
		}
		expanded, err := connprofile.ExpandValue(val, resolver)
		if err != nil {
			return restfile.SSHProfile{}, err
		}
		*field = expanded
	}

	return p, nil
}
