package ssh

import (
	"fmt"
	"os"
	"strings"

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
	var found bool

	if name := strings.TrimSpace(spec.Use); name != "" {
		if prof, ok := lookupProfile(fileProfiles, name, restfile.SSHScopeFile); ok {
			merged = *prof
			found = true
		} else if prof, ok := lookupProfile(globalProfiles, name, restfile.SSHScopeGlobal); ok {
			merged = *prof
			found = true
		}
		merged.Name = name
	}

	if spec.Inline != nil {
		merged = mergeProfile(merged, *spec.Inline)
		found = true
	}

	if strings.TrimSpace(spec.Use) != "" && !found {
		return nil, fmt.Errorf("ssh profile %q not found", spec.Use)
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
	for i := range profiles {
		p := profiles[i]
		if p.Scope != scope {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(p.Name), strings.TrimSpace(name)) {
			return &p, true
		}
	}
	return nil, false
}

func mergeProfile(base restfile.SSHProfile, override restfile.SSHProfile) restfile.SSHProfile {
	out := base
	setIf(&out.Name, override.Name)
	setIf(&out.Host, override.Host)
	setIf(&out.PortStr, override.PortStr)

	if override.PortStr != "" {
		out.Port = override.Port
	}

	setIf(&out.User, override.User)
	setIf(&out.Pass, override.Pass)
	setIf(&out.Key, override.Key)
	setIf(&out.KeyPass, override.KeyPass)
	setIf(&out.KnownHosts, override.KnownHosts)
	if override.Agent.Set {
		out.Agent = override.Agent
	}
	if override.Strict.Set {
		out.Strict = override.Strict
	}
	if override.Persist.Set {
		out.Persist = override.Persist
	}
	if optSet(override.Timeout, override.TimeoutStr) {
		out.Timeout = override.Timeout
		out.TimeoutStr = override.TimeoutStr
	}
	if optSet(override.KeepAlive, override.KeepAliveStr) {
		out.KeepAlive = override.KeepAlive
		out.KeepAliveStr = override.KeepAliveStr
	}
	if optSet(override.Retries, override.RetriesStr) {
		out.Retries = override.Retries
		out.RetriesStr = override.RetriesStr
	}
	return out
}

func setIf(dst *string, val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	*dst = val
}

func optSet[T any](opt restfile.SSHOpt[T], raw string) bool {
	return opt.Set || strings.TrimSpace(raw) != ""
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
		expanded, err := expandValue(val, resolver)
		if err != nil {
			return restfile.SSHProfile{}, err
		}
		*field = expanded
	}

	return p, nil
}

func expandValue(raw string, resolver *vars.Resolver) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(trimmed), "env:") {
		key := strings.TrimSpace(trimmed[4:])
		if key == "" {
			return "", fmt.Errorf("empty env: token")
		}
		if v, ok := os.LookupEnv(key); ok {
			return v, nil
		}
		if resolver != nil {
			if v, ok := resolver.Resolve(key); ok {
				return v, nil
			}
		}
		return "", nil
	}
	if resolver == nil {
		return trimmed, nil
	}
	return resolver.ExpandTemplates(trimmed)
}
