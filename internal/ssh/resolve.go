package ssh

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/profileutil"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func Resolve(
	spec *restfile.SSHSpec,
	fileProfiles []restfile.SSHProfile,
	globalProfiles []restfile.SSHProfile,
	resolver *vars.Resolver,
	envLabel string,
) (*Cfg, error) {
	if spec == nil {
		return nil, nil
	}

	var merged restfile.SSHProfile
	var useFound bool

	use := strings.TrimSpace(spec.Use)
	if use != "" {
		if prof, ok := lookupProfile(fileProfiles, use, restfile.SSHScopeFile); ok {
			merged = *prof
			useFound = true
		} else if prof, ok := lookupProfile(globalProfiles, use, restfile.SSHScopeGlobal); ok {
			merged = *prof
			useFound = true
		}
		merged.Name = use
	}

	if use != "" && !useFound {
		return nil, fmt.Errorf("ssh profile %q not found", use)
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
	profiles []restfile.SSHProfile,
	name string,
	scope restfile.SSHScope,
) (*restfile.SSHProfile, bool) {
	sf := func(p restfile.SSHProfile) restfile.SSHScope { return p.Scope }
	nf := func(p restfile.SSHProfile) string { return p.Name }
	return restfile.LookupNamedScoped(profiles, name, scope, sf, nf)
}

func mergeProfile(base restfile.SSHProfile, override restfile.SSHProfile) restfile.SSHProfile {
	out := base
	profileutil.SetIf(&out.Name, override.Name)
	profileutil.SetIf(&out.Host, override.Host)
	profileutil.SetIf(&out.PortStr, override.PortStr)

	if override.PortStr != "" {
		out.Port = override.Port
	}

	profileutil.SetIf(&out.User, override.User)
	profileutil.SetIf(&out.Pass, override.Pass)
	profileutil.SetIf(&out.Key, override.Key)
	profileutil.SetIf(&out.KeyPass, override.KeyPass)
	profileutil.SetIf(&out.KnownHosts, override.KnownHosts)
	if override.Agent.Set {
		out.Agent = override.Agent
	}
	if override.Strict.Set {
		out.Strict = override.Strict
	}
	if override.Persist.Set {
		out.Persist = override.Persist
	}
	if profileutil.OptSet(override.Timeout, override.TimeoutStr) {
		out.Timeout = override.Timeout
		out.TimeoutStr = override.TimeoutStr
	}
	if profileutil.OptSet(override.KeepAlive, override.KeepAliveStr) {
		out.KeepAlive = override.KeepAlive
		out.KeepAliveStr = override.KeepAliveStr
	}
	if profileutil.OptSet(override.Retries, override.RetriesStr) {
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
		expanded, err := profileutil.ExpandValue(val, resolver)
		if err != nil {
			return restfile.SSHProfile{}, err
		}
		*field = expanded
	}

	return p, nil
}
