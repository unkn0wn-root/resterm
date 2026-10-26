package ssh

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/connprofile"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/duration"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Directive struct {
	Scope          directive.Scope
	Profile        restfile.SSHProfile
	Spec           *restfile.SSHSpec
	PersistIgnored bool
}

func ParseDirective(rest string) (res Directive, err error) {
	head, ok, headErr := directive.ParseProfileHeader(directive.SSH, rest)
	if !ok {
		return res, fmt.Errorf("@ssh requires options")
	}
	defer func() { err = errors.Join(headErr, err) }()
	scope, opts := head.Scope, head.Options
	name := head.Name
	if name == "" {
		name = connprofile.DefaultName
	}

	prof := restfile.SSHProfile{Scope: scope, Name: name}
	optErr := applySSHOptions(&prof, opts)
	use := opts.Pop("use")
	// Everything valid has been popped by now, so Leftover reports only mistakes.
	left := opts.Leftover(directive.SSH)
	if optErr != nil {
		return res, errors.Join(optErr, left)
	}

	if scope == directive.ScopeRequest {
		// Request-scoped persist is ignored to avoid leaking tunnels.
		res.PersistIgnored = prof.Persist.Set
		prof.Persist = restfile.Opt[bool]{}
	}

	if scope != directive.ScopeRequest {
		if strings.TrimSpace(prof.Host) == "" && !opts.Bare("host") {
			return res, errors.Join(fmt.Errorf("@ssh %s scope requires host", scope.String()), left)
		}
		res.Scope = scope
		res.Profile = prof
		return res, left
	}

	inline := buildInlineSSH(prof)
	if use == "" && inline == nil && !opts.Bare("host") && !opts.Bare("use") {
		return res, errors.Join(errors.New("@ssh requires host or use="), left)
	}

	res.Scope = scope
	res.Profile = prof
	res.Spec = &restfile.SSHSpec{Use: use, Inline: inline}
	return res, left
}

// Every option is read even after one fails, so two mistakes are both reported.
func applySSHOptions(prof *restfile.SSHProfile, opts directive.Options) error {
	if host, ok := opts.PopAny("host"); ok {
		prof.Host = host
	}
	if port, ok := opts.PopAny("port"); ok {
		prof.PortStr = port
		if n, err := strconv.Atoi(port); err == nil && n > 0 {
			prof.Port = n
		}
	}
	if user, ok := opts.PopAny("user"); ok {
		prof.User = user
	}
	if pw, ok := opts.PopAny("password", "pass"); ok {
		prof.Pass = pw
	}
	if key, ok := opts.PopAny("key"); ok {
		prof.Key = key
	}
	if kp, ok := opts.PopAny("passphrase"); ok {
		prof.KeyPass = kp
	}
	if kh, ok := opts.PopAny("known_hosts", "known-hosts"); ok {
		prof.KnownHosts = kh
	}

	if raw, ok := opts.PopAny("timeout"); ok {
		prof.TimeoutStr = raw
		prof.Timeout.Set = true
		if dur, ok := duration.Parse(raw); ok && dur >= 0 {
			prof.Timeout.Val = dur
		}
	}
	if raw, ok := opts.PopAny("keepalive"); ok {
		prof.KeepAliveStr = raw
		prof.KeepAlive.Set = true
		if dur, ok := duration.Parse(raw); ok && dur >= 0 {
			prof.KeepAlive.Val = dur
		}
	}
	if raw, ok := opts.PopAny("retries"); ok {
		prof.RetriesStr = raw
		prof.Retries.Set = true
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			prof.Retries.Val = n
		}
	}
	return errors.Join(
		setBoolOption(&prof.Agent, opts, "agent"),
		setBoolOption(&prof.Strict, opts, "strict_hostkey", "strict-hostkey", "strict_host_key"),
		setBoolOption(&prof.Persist, opts, "persist"),
	)
}

func setBoolOption(opt *restfile.Opt[bool], opts directive.Options, keys ...string) error {
	value, ok, bad := opts.PopBool(keys...)
	if !ok {
		return nil
	}
	if bad != "" {
		return fmt.Errorf("invalid @ssh %s: %q", keys[0], bad)
	}
	opt.Set = true
	opt.Val = value
	return nil
}

func buildInlineSSH(prof restfile.SSHProfile) *restfile.SSHProfile {
	if !sshInlineSet(prof) {
		return nil
	}
	copy := prof
	copy.Scope = directive.ScopeRequest
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
