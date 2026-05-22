package grpcclient

import (
	"context"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/k8s"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/ssh"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
	"google.golang.org/grpc/codes"
)

type Options struct {
	BaseDir             string
	DefaultPlaintext    bool
	DefaultPlaintextSet bool
	DialTimeout         time.Duration
	RootCAs             []string
	ClientCert          string
	ClientKey           string
	Insecure            bool
	RootMode            tlsconfig.RootMode
	SSH                 *ssh.Plan
	K8s                 *k8s.Plan
}

type Response struct {
	Message         string
	Body            []byte
	ContentType     string
	Wire            []byte
	WireContentType string
	Headers         map[string][]string
	Trailers        map[string][]string
	StatusCode      codes.Code
	StatusMessage   string
	Duration        time.Duration
}

type StreamHook func(*stream.Session)

// gRPC stream event metadata keys.
const (
	MetaMethod   = "grpc.method"
	MetaMsgType  = "grpc.msg.type"
	MetaMsgIndex = "grpc.msg.index"
	MetaStatus   = "grpc.status"
	MetaReason   = "grpc.reason"
)

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) Execute(
	parent context.Context,
	req *restfile.Request,
	gr *restfile.GRPCRequest,
	opt Options,
	hook StreamHook,
) (resp *Response, err error) {
	if gr == nil {
		return nil, diag.New(diag.ClassProtocol, "missing grpc metadata")
	}

	if strings.TrimSpace(gr.Target) == "" {
		return nil, diag.New(diag.ClassProtocol, "grpc target not specified")
	}
	if strings.TrimSpace(gr.FullMethod) == "" {
		return nil, diag.New(diag.ClassProtocol, "grpc method not specified")
	}

	ctx, cancel := contextWithTimeout(parent, req, opt)
	defer cancel()

	target, dialOpts, err := buildDial(gr, opt)
	if err != nil {
		return nil, err
	}

	conn, err := dialGRPC(target, dialOpts)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && err == nil {
			err = diag.WrapAs(diag.ClassProtocol, closeErr, "close grpc connection")
		}
	}()

	files, md, err := c.resolveMethod(ctx, conn, gr, opt)
	if err != nil {
		return nil, err
	}

	body, err := c.resolveMessage(gr, opt.BaseDir)
	if err != nil {
		return nil, err
	}

	cd := newCodec(files)
	if isStreaming(md) {
		return c.executeStream(ctx, conn, req, gr, md, body, cd, hook)
	}
	return c.executeUnary(ctx, conn, req, gr, md, body, cd)
}

func contextWithTimeout(
	parent context.Context,
	req *restfile.Request,
	opt Options,
) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}

	var timeout string
	if req != nil {
		timeout = req.Settings["timeout"]
	}
	if timeout != "" {
		if dur, err := time.ParseDuration(timeout); err == nil && dur > 0 {
			return context.WithTimeout(parent, dur)
		}
	}
	if opt.DialTimeout > 0 {
		return context.WithTimeout(parent, opt.DialTimeout)
	}
	return parent, func() {}
}
