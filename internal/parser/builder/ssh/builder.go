package ssh

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/lex"
	"github.com/unkn0wn-root/resterm/internal/parser/directive/options"
	dscope "github.com/unkn0wn-root/resterm/internal/parser/directive/scope"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Directive struct {
	Scope          restfile.SSHScope
	Profile        restfile.SSHProfile
	Spec           *restfile.SSHSpec
	PersistIgnored bool
}

func ParseDirective(rest string) (Directive, error) {
	res := Directive{}
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return res, fmt.Errorf("@ssh requires options")
	}

	fields := lex.TokenizeFieldsEscaped(trimmed)
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

	opts := options.Parse(strings.Join(fields[idx:], " "))
	prof := restfile.SSHProfile{Scope: scope, Name: name}
	applySSHOptions(&prof, opts)
	if scope == restfile.SSHScopeRequest {
		// Request-scoped persist is ignored to avoid leaking tunnels.
		res.PersistIgnored = prof.Persist.Set
		prof.Persist = restfile.Opt[bool]{}
	}

	if scope != restfile.SSHScopeRequest {
		if strings.TrimSpace(prof.Host) == "" {
			return res, fmt.Errorf("@ssh %s scope requires host", sshScopeLabel(scope))
		}
		res.Scope = scope
		res.Profile = prof
		return res, nil
	}

	use := strings.TrimSpace(opts["use"])
	inline := buildInlineSSH(prof)
	if use == "" && inline == nil {
		return res, fmt.Errorf("@ssh requires host or use=")
	}

	res.Scope = scope
	res.Profile = prof
	res.Spec = &restfile.SSHSpec{Use: use, Inline: inline}
	return res, nil
}

func parseSSHScope(token string) (restfile.SSHScope, bool) {
	return dscope.Parse(
		token,
		restfile.SSHScopeRequest,
		restfile.SSHScopeFile,
		restfile.SSHScopeGlobal,
	)
}

func applySSHOptions(prof *restfile.SSHProfile, opts map[string]string) {
	if host, ok := options.First(opts, "host"); ok {
		prof.Host = host
	}
	if port, ok := options.First(opts, "port"); ok {
		prof.PortStr = port
		if n, err := strconv.Atoi(port); err == nil && n > 0 {
			prof.Port = n
		}
	}
	if user, ok := options.First(opts, "user"); ok {
		prof.User = user
	}
	if pw, ok := options.First(opts, "password", "pass"); ok {
		prof.Pass = pw
	}
	if key, ok := options.First(opts, "key"); ok {
		prof.Key = key
	}
	if kp, ok := options.First(opts, "passphrase"); ok {
		prof.KeyPass = kp
	}
	setBoolOption(&prof.Agent, opts, "agent")
	if kh, ok := options.First(opts, "known_hosts", "known-hosts"); ok {
		prof.KnownHosts = kh
	}
	setBoolOption(&prof.Strict, opts, "strict_hostkey", "strict-hostkey", "strict_host_key")
	setBoolOption(&prof.Persist, opts, "persist")

	if raw, ok := options.First(opts, "timeout"); ok {
		prof.TimeoutStr = raw
		prof.Timeout.Set = true
		if dur, ok := duration.Parse(raw); ok && dur >= 0 {
			prof.Timeout.Val = dur
		}
	}
	if raw, ok := options.First(opts, "keepalive"); ok {
		prof.KeepAliveStr = raw
		prof.KeepAlive.Set = true
		if dur, ok := duration.Parse(raw); ok && dur >= 0 {
			prof.KeepAlive.Val = dur
		}
	}
	if raw, ok := options.First(opts, "retries"); ok {
		prof.RetriesStr = raw
		prof.Retries.Set = true
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			prof.Retries.Val = n
		}
	}
}

func setBoolOption(opt *restfile.Opt[bool], opts map[string]string, keys ...string) {
	if value, ok := options.Bool(opts, keys...); ok {
		opt.Set = true
		opt.Val = value
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
	return dscope.Label(
		scope,
		restfile.SSHScopeRequest,
		restfile.SSHScopeFile,
		restfile.SSHScopeGlobal,
	)
}
