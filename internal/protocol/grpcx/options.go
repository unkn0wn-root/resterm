package grpcx

import (
	"time"

	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
)

type Options struct {
	BaseDir          string
	FallbackBaseDirs []string
	NoFallback       bool
	DefaultPlaintext restfile.Opt[bool]
	// DialTimeout bounds connection setup and descriptor resolution. The socket
	// connection itself is lazy.
	DialTimeout time.Duration
	// Timeout applies to unary calls. Streams run until completion or cancellation.
	Timeout     time.Duration
	MaxRecvSize int
	MaxSendSize int
	Compression string
	RootCAs     []string
	ClientCert  string
	ClientKey   string
	Insecure    bool
	RootMode    tlsconfig.RootMode
	SSH         *ssh.Plan
	K8s         *k8s.Plan
}
