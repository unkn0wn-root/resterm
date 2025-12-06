package tlsconfig

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"

	"github.com/unkn0wn-root/resterm/internal/errdef"
)

type Files struct {
	RootCAs    []string
	ClientCert string
	ClientKey  string
	Insecure   bool
	RootMode   RootMode
}

type RootMode string

const (
	RootModeReplace RootMode = "replace"
	RootModeAppend  RootMode = "append"
)

// Build constructs a tls.Config using custom CAs with optional system roots plus any client certs.
// Paths are resolved relative to baseDir when not absolute.
func Build(cfg Files, baseDir string) (*tls.Config, error) {
	mode := cfg.RootMode
	if mode == "" {
		mode = RootModeReplace
	}

	tc := &tls.Config{InsecureSkipVerify: cfg.Insecure} // nolint:gosec

	if len(cfg.RootCAs) > 0 {
		pool, err := loadRootCAs(cfg.RootCAs, baseDir, mode == RootModeAppend)
		if err != nil {
			return nil, err
		}
		tc.RootCAs = pool
	} else if sys, err := x509.SystemCertPool(); err == nil && sys != nil {
		tc.RootCAs = sys
	}

	if cfg.ClientCert != "" || cfg.ClientKey != "" {
		if cfg.ClientCert == "" || cfg.ClientKey == "" {
			return nil, errdef.New(errdef.CodeHTTP, "client certificate and key are both required")
		}
		cert, err := loadClientCert(cfg.ClientCert, cfg.ClientKey, baseDir)
		if err != nil {
			return nil, err
		}
		tc.Certificates = []tls.Certificate{cert}
	}

	return tc, nil
}

func loadRootCAs(paths []string, baseDir string, mergeSystem bool) (*x509.CertPool, error) {
	var pool *x509.CertPool
	if mergeSystem {
		pool, _ = x509.SystemCertPool()
	}
	if pool == nil {
		pool = x509.NewCertPool()
	}

	for _, p := range paths {
		resolved := resolvePath(p, baseDir)
		data, readErr := os.ReadFile(resolved)
		if readErr != nil {
			return nil, errdef.Wrap(errdef.CodeFilesystem, readErr, "read root ca %s", p)
		}
		if ok := pool.AppendCertsFromPEM(data); !ok {
			return nil, errdef.New(errdef.CodeHTTP, "append cert from %s", p)
		}
	}
	return pool, nil
}

func loadClientCert(certPath, keyPath, baseDir string) (tls.Certificate, error) {
	certFile := resolvePath(certPath, baseDir)
	keyFile := resolvePath(keyPath, baseDir)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return tls.Certificate{}, errdef.Wrap(errdef.CodeHTTP, err, "load client certificate")
	}
	return cert, nil
}

func resolvePath(path, baseDir string) string {
	if filepath.IsAbs(path) || baseDir == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}
