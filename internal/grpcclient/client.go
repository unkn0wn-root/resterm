package grpcclient

import (
	"context"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/filelookup"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type StreamHook func(*stream.Session)

const (
	MetaMethod   = "grpc.method"
	MetaMsgType  = "grpc.msg.type"
	MetaMsgIndex = "grpc.msg.index"
	MetaStatus   = "grpc.status"
	MetaReason   = "grpc.reason"
)

var grpcComponent = diag.WithComponent(diag.ComponentGRPC)

// Conn is the connection interface used by Client.
type Conn interface {
	grpc.ClientConnInterface
	Close() error
}

// DialFunc creates a lazy gRPC connection. A context is not needed until the
// first RPC starts.
type DialFunc func(target string, opts []grpc.DialOption) (Conn, error)

type ClientOption func(*Client)

type Client struct {
	dial DialFunc
	fs   filelookup.FileSystem
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	if c.dial == nil {
		c.dial = dialGRPC
	}
	if c.fs == nil {
		c.fs = filelookup.OSFileSystem{}
	}
	return c
}

func WithDialer(dial DialFunc) ClientOption {
	return func(c *Client) {
		if dial != nil {
			c.dial = dial
		}
	}
}

func WithFileSystem(fs filelookup.FileSystem) ClientOption {
	return func(c *Client) {
		if fs != nil {
			c.fs = fs
		}
	}
}

// Clone returns a snapshot of the client configuration.
func (c *Client) Clone() *Client {
	if c == nil {
		return nil
	}
	return &Client{dial: c.dial, fs: c.fs}
}

func (c *Client) Execute(
	parent context.Context,
	req *restfile.Request,
	opt Options,
	hook StreamHook,
) (resp *Response, err error) {
	gr := req.GRPC
	if gr == nil {
		return nil, diag.New(diag.ClassProtocol, "missing grpc metadata", grpcComponent)
	}
	if gr.Target == "" {
		return nil, diag.New(diag.ClassProtocol, "grpc target not specified", grpcComponent)
	}

	id, err := parseFullMethod(gr.FullMethod)
	if err != nil {
		return nil, err
	}

	if parent == nil {
		parent = context.Background()
	}

	target, dialOpts, err := buildDial(gr, opt)
	if err != nil {
		return nil, err
	}

	conn, err := c.dial(target, dialOpts)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && err == nil {
			err = diag.WrapAs(diag.ClassProtocol, closeErr, "close grpc connection", grpcComponent)
		}
	}()

	// Setup and unary calls use separate timeouts. Streams keep the parent
	// context so they can run until the session is cancelled.
	setupCtx, cancelSetup := withTimeout(parent, opt.DialTimeout)
	in, err := c.prepare(setupCtx, conn, req, id, opt)
	cancelSetup()
	if err != nil {
		return nil, err
	}

	if in.kind != callUnary {
		return in.stream(parent, hook)
	}

	callCtx, cancel := withTimeout(parent, opt.Timeout)
	defer cancel()
	return in.unary(callCtx)
}

func withTimeout(
	parent context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}

type invocation struct {
	conn   Conn
	method protoreflect.MethodDescriptor
	kind   streamKind
	name   string
	codec  codec
	body   string
	md     mdPairs
	calls  []grpc.CallOption
}

func (c *Client) prepare(
	ctx context.Context,
	conn Conn,
	req *restfile.Request,
	id methodID,
	opt Options,
) (invocation, error) {
	gr := req.GRPC
	rd := newReader(c.fs, opt)
	target, err := resolveMethod(ctx, conn, gr, id, rd)
	if err != nil {
		return invocation{}, err
	}

	body, err := resolveMessage(gr, rd)
	if err != nil {
		return invocation{}, err
	}

	md, err := collectMetadata(gr, req)
	if err != nil {
		return invocation{}, err
	}

	return invocation{
		conn:   conn,
		method: target.desc,
		kind:   streamKindOf(target.desc),
		name:   id.full,
		codec:  target.codec,
		body:   body,
		md:     md,
		calls:  callOptions(opt),
	}, nil
}

func callOptions(opt Options) []grpc.CallOption {
	var out []grpc.CallOption
	if opt.MaxRecvSize > 0 {
		out = append(out, grpc.MaxCallRecvMsgSize(opt.MaxRecvSize))
	}
	if opt.MaxSendSize > 0 {
		out = append(out, grpc.MaxCallSendMsgSize(opt.MaxSendSize))
	}
	if opt.Compression != "" {
		out = append(out, grpc.UseCompressor(opt.Compression))
	}
	return out
}
