package grpcx

import (
	"context"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// stubConn and stubStream exercise calls without a server. The stream fills
// caller-owned messages because dynamic messages from separate registries cannot merge.
type stubConn struct {
	invoke  func(ctx context.Context, method string, args, reply any) error
	stream  *stubStream
	header  metadata.MD
	trailer metadata.MD
	closed  bool
	calls   []grpc.CallOption
	lastCtx context.Context
}

func (c *stubConn) Invoke(
	ctx context.Context,
	method string,
	args, reply any,
	opts ...grpc.CallOption,
) error {
	c.lastCtx = ctx
	c.record(opts)
	if c.invoke == nil {
		return nil
	}
	return c.invoke(ctx, method, args, reply)
}

func (c *stubConn) NewStream(
	ctx context.Context,
	_ *grpc.StreamDesc,
	_ string,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	c.lastCtx = ctx
	c.record(opts)
	if c.stream == nil {
		return &stubStream{}, nil
	}
	c.stream.ctx = ctx
	return c.stream, nil
}

func (c *stubConn) Close() error {
	c.closed = true
	return nil
}

func (c *stubConn) record(opts []grpc.CallOption) {
	c.calls = append(c.calls, opts...)
	for _, opt := range opts {
		switch o := opt.(type) {
		case grpc.HeaderCallOption:
			*o.HeaderAddr = c.header
		case grpc.TrailerCallOption:
			*o.TrailerAddr = c.trailer
		}
	}
}

type stubStream struct {
	ctx       context.Context
	sent      []proto.Message
	recvCount int
	fill      func(idx int, msg proto.Message)
	recvErr   error
	closeErr  error
	blockCtx  bool
	closed    bool
	pos       int
}

func (s *stubStream) Header() (metadata.MD, error) { return nil, nil }
func (s *stubStream) Trailer() metadata.MD         { return nil }
func (s *stubStream) Context() context.Context     { return s.ctx }

func (s *stubStream) CloseSend() error {
	s.closed = true
	return s.closeErr
}

func (s *stubStream) SendMsg(m any) error {
	msg, ok := m.(proto.Message)
	if !ok {
		return nil
	}
	s.sent = append(s.sent, proto.Clone(msg))
	return nil
}

func (s *stubStream) RecvMsg(m any) error {
	if s.pos >= s.recvCount {
		if s.blockCtx {
			<-s.ctx.Done()
			return s.ctx.Err()
		}
		if s.recvErr != nil {
			return s.recvErr
		}
		return io.EOF
	}

	idx := s.pos
	s.pos++
	if dst, ok := m.(proto.Message); ok && s.fill != nil {
		s.fill(idx, dst)
	}
	return nil
}
