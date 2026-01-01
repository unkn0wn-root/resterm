package parser

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type sshDirective struct {
	scope   restfile.SSHScope
	profile restfile.SSHProfile
	spec    *restfile.SSHSpec
}

func (b *documentBuilder) handleSSH(line int, rest string) {
	res, err := parseSSHDirective(rest)
	if err != nil {
		b.addError(line, err.Error())
		return
	}

	if res.scope == restfile.SSHScopeRequest {
		if !b.ensureRequest(line) {
			return
		}
		if b.request.ssh != nil {
			b.addError(line, "@ssh already defined for this request")
			return
		}
		b.request.ssh = res.spec
		return
	}

	if res.scope == restfile.SSHScopeGlobal || res.scope == restfile.SSHScopeFile {
		res.profile.Scope = res.scope
		b.sshDefs = append(b.sshDefs, res.profile)
	}
}

func parseSSHDirective(rest string) (sshDirective, error) {
	res := sshDirective{}
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return res, fmt.Errorf("@ssh requires options")
	}

	fields := tokenizeOptionTokens(trimmed)
	if len(fields) == 0 {
		return res, fmt.Errorf("@ssh requires options")
	}

	scope := restfile.SSHScopeRequest
	idx := 0
	if sc, ok := parseSSHScope(fields[idx]); ok {
		scope = sc
		idx++
	}

	name := "default"
	if idx < len(fields) && !strings.Contains(fields[idx], "=") {
		name = strings.TrimSpace(fields[idx])
		idx++
	}
	if name == "" {
		name = "default"
	}

	opts := parseOptionTokens(strings.Join(fields[idx:], " "))
	prof := restfile.SSHProfile{Scope: scope, Name: name}
	applySSHOptions(&prof, opts)
	if scope == restfile.SSHScopeRequest {
		// Request-scoped persist is ignored to avoid leaking tunnels.
		prof.Persist = restfile.SSHOpt[bool]{}
	}

	if scope != restfile.SSHScopeRequest {
		if strings.TrimSpace(prof.Host) == "" {
			return res, fmt.Errorf("@ssh %s scope requires host", sshScopeLabel(scope))
		}
		res.scope = scope
		res.profile = prof
		return res, nil
	}

	use := strings.TrimSpace(opts["use"])
	inline := buildInlineSSH(prof)
	if use == "" && inline == nil {
		return res, fmt.Errorf("@ssh requires host or use=")
	}

	res.scope = scope
	res.profile = prof
	res.spec = &restfile.SSHSpec{Use: use, Inline: inline}
	return res, nil
}

func parseSSHScope(token string) (restfile.SSHScope, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "global":
		return restfile.SSHScopeGlobal, true
	case "file":
		return restfile.SSHScopeFile, true
	case "request":
		return restfile.SSHScopeRequest, true
	default:
		return 0, false
	}
}

func applySSHOptions(prof *restfile.SSHProfile, opts map[string]string) {
	if host := strings.TrimSpace(opts["host"]); host != "" {
		prof.Host = host
	}
	if port := strings.TrimSpace(opts["port"]); port != "" {
		prof.PortStr = port
		if n, err := strconv.Atoi(port); err == nil && n > 0 {
			prof.Port = n
		}
	}
	if user := strings.TrimSpace(opts["user"]); user != "" {
		prof.User = user
	}
	if pw := strings.TrimSpace(opts["password"]); pw != "" {
		prof.Pass = pw
	} else if pw := strings.TrimSpace(opts["pass"]); pw != "" {
		prof.Pass = pw
	}
	if key := strings.TrimSpace(opts["key"]); key != "" {
		prof.Key = key
	}
	if kp := strings.TrimSpace(opts["passphrase"]); kp != "" {
		prof.KeyPass = kp
	}
	setSSHBool(&prof.Agent, opts, "agent")
	if kh := strings.TrimSpace(opts["known_hosts"]); kh != "" {
		prof.KnownHosts = kh
	} else if kh := strings.TrimSpace(opts["known-hosts"]); kh != "" {
		prof.KnownHosts = kh
	}
	setSSHBool(&prof.Strict, opts, "strict_hostkey", "strict-hostkey", "strict_host_key")
	setSSHBool(&prof.Persist, opts, "persist")

	if raw := strings.TrimSpace(opts["timeout"]); raw != "" {
		prof.TimeoutStr = raw
		prof.Timeout.Set = true
		if dur, err := time.ParseDuration(raw); err == nil && dur >= 0 {
			prof.Timeout.Val = dur
		}
	}
	if raw := strings.TrimSpace(opts["keepalive"]); raw != "" {
		prof.KeepAliveStr = raw
		prof.KeepAlive.Set = true
		if dur, err := time.ParseDuration(raw); err == nil && dur >= 0 {
			prof.KeepAlive.Val = dur
		}
	}
	if raw := strings.TrimSpace(opts["retries"]); raw != "" {
		prof.RetriesStr = raw
		prof.Retries.Set = true
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			prof.Retries.Val = n
		}
	}
}

func setSSHBool(opt *restfile.SSHOpt[bool], opts map[string]string, keys ...string) {
	for _, key := range keys {
		if raw, ok := opts[key]; ok {
			opt.Set = true
			val := true
			if raw != "" {
				if parsed, ok := parseBool(raw); ok {
					val = parsed
				}
			}
			opt.Val = val
			return
		}
	}
}

func buildInlineSSH(prof restfile.SSHProfile) *restfile.SSHProfile {
	if !sshInlineSet(prof) {
		return nil
	}
	copy := prof
	copy.Scope = restfile.SSHScopeRequest
	return &copy
}

func sshInlineSet(prof restfile.SSHProfile) bool {
	return prof.Host != "" ||
		prof.PortStr != "" ||
		prof.User != "" ||
		prof.Pass != "" ||
		prof.Key != "" ||
		prof.KeyPass != "" ||
		prof.KnownHosts != "" ||
		prof.Agent.Set ||
		prof.Strict.Set ||
		prof.Persist.Set ||
		prof.Timeout.Set ||
		prof.KeepAlive.Set ||
		prof.Retries.Set
}

func sshScopeLabel(scope restfile.SSHScope) string {
	switch scope {
	case restfile.SSHScopeGlobal:
		return "global"
	case restfile.SSHScopeFile:
		return "file"
	default:
		return "request"
	}
}
