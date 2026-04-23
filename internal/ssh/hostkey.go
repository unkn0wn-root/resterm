package ssh

import (
	"errors"

	xssh "golang.org/x/crypto/ssh"
	knownhosts "golang.org/x/crypto/ssh/knownhosts"
)

type hostKeySpec struct {
	strict     bool
	knownHosts string
}

func hostKeySpecFor(cfg Config) hostKeySpec {
	return hostKeySpec{
		strict:     cfg.Strict,
		knownHosts: cfg.KnownHosts,
	}
}

func (sp hostKeySpec) callback() (xssh.HostKeyCallback, error) {
	if !sp.strict {
		return xssh.InsecureIgnoreHostKey(), nil
	}
	if sp.knownHosts == "" {
		return nil, errors.New("strict host key but no known_hosts")
	}
	return knownhosts.New(sp.knownHosts)
}
