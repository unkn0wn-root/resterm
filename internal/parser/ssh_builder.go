package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/duration"
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
		b.ensureRequest(line)
		if b.request.k8s != nil {
			b.addError(line, "@ssh cannot be combined with @k8s on the same request")
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
		prof.Persist = restfile.Opt[bool]{}
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
	return parseDirectiveScope(
		token,
		restfile.SSHScopeRequest,
		restfile.SSHScopeFile,
		restfile.SSHScopeGlobal,
	)
}

func applySSHOptions(prof *restfile.SSHProfile, opts map[string]string) {
	if host, ok := firstOpt(opts, "host"); ok {
		prof.Host = host
	}
	if port, ok := firstOpt(opts, "port"); ok {
		prof.PortStr = port
		if n, err := strconv.Atoi(port); err == nil && n > 0 {
			prof.Port = n
		}
	}
	if user, ok := firstOpt(opts, "user"); ok {
		prof.User = user
	}
	if pw, ok := firstOpt(opts, "password", "pass"); ok {
		prof.Pass = pw
	}
	if key, ok := firstOpt(opts, "key"); ok {
		prof.Key = key
	}
	if kp, ok := firstOpt(opts, "passphrase"); ok {
		prof.KeyPass = kp
	}
	setOptBool(&prof.Agent, opts, "agent")
	if kh, ok := firstOpt(opts, "known_hosts", "known-hosts"); ok {
		prof.KnownHosts = kh
	}
	setOptBool(&prof.Strict, opts, "strict_hostkey", "strict-hostkey", "strict_host_key")
	setOptBool(&prof.Persist, opts, "persist")

	if raw, ok := firstOpt(opts, "timeout"); ok {
		prof.TimeoutStr = raw
		prof.Timeout.Set = true
		if dur, ok := duration.Parse(raw); ok && dur >= 0 {
			prof.Timeout.Val = dur
		}
	}
	if raw, ok := firstOpt(opts, "keepalive"); ok {
		prof.KeepAliveStr = raw
		prof.KeepAlive.Set = true
		if dur, ok := duration.Parse(raw); ok && dur >= 0 {
			prof.KeepAlive.Val = dur
		}
	}
	if raw, ok := firstOpt(opts, "retries"); ok {
		prof.RetriesStr = raw
		prof.Retries.Set = true
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			prof.Retries.Val = n
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
	return directiveScopeLabel(
		scope,
		restfile.SSHScopeRequest,
		restfile.SSHScopeFile,
		restfile.SSHScopeGlobal,
	)
}
