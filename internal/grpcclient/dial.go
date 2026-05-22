package grpcclient

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
	"github.com/unkn0wn-root/resterm/internal/tunnel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func buildDial(gr *restfile.GRPCRequest, opt Options) (string, []grpc.DialOption, error) {
	sshOn := opt.SSH != nil && opt.SSH.Active()
	k8sOn := opt.K8s != nil && opt.K8s.Active()
	if tunnel.HasConflict(sshOn, k8sOn) {
		return "", nil, diag.New(diag.ClassRoute, "ssh and k8s transports cannot be combined")
	}

	var dialOpts []grpc.DialOption
	if shouldUsePlaintext(gr, opt) {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		creds, err := buildTransportCredentials(opt)
		if err != nil {
			return "", nil, err
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	}

	if sshOn {
		plan := opt.SSH
		cfg := *plan.Config
		dialOpts = append(dialOpts, tunnel.GRPCDialOption(tunnel.DialerFor(plan.Manager, cfg)))
	}
	if k8sOn {
		plan := opt.K8s
		cfg := *plan.Config
		dialOpts = append(dialOpts, tunnel.GRPCDialOption(tunnel.DialerFor(plan.Manager, cfg)))
	}
	if auth := strings.TrimSpace(gr.Authority); auth != "" {
		dialOpts = append(dialOpts, grpc.WithAuthority(auth))
	}

	return dialTarget(gr.Target, sshOn || k8sOn), dialOpts, nil
}

func dialGRPC(target string, opts []grpc.DialOption) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "dial grpc target")
	}
	return conn, nil
}

func dialTarget(target string, routed bool) string {
	target = strings.TrimSpace(target)
	if !routed {
		return target
	}
	if target == "" || hasTargetScheme(target) {
		return target
	}
	return "passthrough:///" + target
}

func hasTargetScheme(target string) bool {
	idx := strings.Index(target, "://")
	if idx <= 0 {
		return false
	}
	for _, r := range target[:idx] {
		if !isSchemeChar(r) {
			return false
		}
	}
	return true
}

func isSchemeChar(r rune) bool {
	return r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		r == '+' ||
		r == '-' ||
		r == '.'
}

func buildTransportCredentials(opt Options) (credentials.TransportCredentials, error) {
	cfg, err := tlsconfig.Build(tlsconfig.Files{
		RootCAs:    opt.RootCAs,
		ClientCert: opt.ClientCert,
		ClientKey:  opt.ClientKey,
		Insecure:   opt.Insecure,
		RootMode:   opt.RootMode,
	}, opt.BaseDir)
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(cfg), nil
}

func shouldUsePlaintext(gr *restfile.GRPCRequest, opt Options) bool {
	if gr.PlaintextSet {
		return gr.Plaintext
	}
	if opt.DefaultPlaintextSet {
		return opt.DefaultPlaintext
	}
	if hasTLS(opt) {
		return false
	}
	return true
}

func hasTLS(opt Options) bool {
	return len(opt.RootCAs) > 0 ||
		opt.ClientCert != "" ||
		opt.ClientKey != "" ||
		opt.Insecure
}
