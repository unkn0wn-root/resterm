package k8s

import (
	"strings"
	"time"
)

type sessionKey struct {
	label        string
	name         string
	namespace    string
	target       string
	port         string
	context      string
	kubeconfig   string
	container    string
	address      string
	localPort    int
	persist      bool
	podWait      time.Duration
	retries      int
	policy       ExecPolicy
	stdinUnavail bool
	stdinMsg     string
	allowlistKey string
}

func sessionKeyFor(cfg execConfig, load loadSettings) sessionKey {
	return sessionKey{
		label:        cfg.Label,
		name:         cfg.Name,
		namespace:    cfg.Namespace,
		target:       cfg.targetRef(),
		port:         cfg.portRef(),
		context:      cfg.Context,
		kubeconfig:   cfg.Kubeconfig,
		container:    cfg.Container,
		address:      cfg.Address,
		localPort:    cfg.LocalPort,
		persist:      cfg.Persist,
		podWait:      cfg.PodWait,
		retries:      cfg.Retries,
		policy:       load.policy,
		stdinUnavail: load.stdinUnavail,
		stdinMsg:     load.stdinMsg,
		allowlistKey: allowlistCacheKey(load.allowlist),
	}
}

func allowlistCacheKey(allowlist []string) string {
	return strings.Join(allowlist, "\x00")
}
