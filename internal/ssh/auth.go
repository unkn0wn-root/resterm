package ssh

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	xssh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

var defaultKeyPaths = func() []string {
	home := userHomeDir()
	return []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
	}
}()

type authSpec struct {
	keyPath string
	keyPass string
	pass    string
	agent   bool
}

func authSpecFor(cfg Config) authSpec {
	return authSpec{
		keyPath: cfg.KeyPath,
		keyPass: cfg.KeyPass,
		pass:    cfg.Pass,
		agent:   cfg.Agent,
	}
}

func (sp authSpec) methods() ([]xssh.AuthMethod, func() error, error) {
	var methods []xssh.AuthMethod
	var closers []func() error

	if sp.keyPath != "" {
		signer, err := sp.explicitKey()
		if err != nil {
			return nil, closeAll(closers), err
		}
		methods = append(methods, xssh.PublicKeys(signer))
	}

	if sp.keyPath == "" && sp.pass == "" {
		if signer := loadDefaultKey(sp.keyPass); signer != nil {
			methods = append(methods, xssh.PublicKeys(signer))
		}
	}

	if sp.agent {
		if method, closeFn, ok := agentAuthMethod(); ok {
			closers = append(closers, closeFn)
			methods = append(methods, method)
		}
	}

	if sp.pass != "" {
		methods = append(methods, xssh.Password(sp.pass))
	}

	if len(methods) == 0 {
		return nil, closeAll(closers), errors.New("no ssh auth methods")
	}
	return methods, closeAll(closers), nil
}

func (sp authSpec) explicitKey() (xssh.Signer, error) {
	data, err := os.ReadFile(sp.keyPath)
	if err != nil {
		return nil, fmt.Errorf("read ssh key: %w", err)
	}
	return parseKey(data, sp.keyPass)
}

func agentAuthMethod() (xssh.AuthMethod, func() error, bool) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, nil, false
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, nil, false
	}
	return xssh.PublicKeysCallback(agent.NewClient(conn).Signers), conn.Close, true
}

func closeAll(fns []func() error) func() error {
	if len(fns) == 0 {
		return nil
	}
	return func() error {
		var errs []error
		for _, fn := range fns {
			if fn == nil {
				continue
			}
			if err := fn(); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
}

func parseKey(data []byte, pass string) (xssh.Signer, error) {
	if pass == "" {
		return xssh.ParsePrivateKey(data)
	}
	return xssh.ParsePrivateKeyWithPassphrase(data, []byte(pass))
}

func loadDefaultKey(pass string) xssh.Signer {
	for _, p := range defaultKeyPaths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		signer, err := parseKey(data, pass)
		if err != nil {
			continue
		}
		return signer
	}
	return nil
}

func userHomeDir() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return dir
}
