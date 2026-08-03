package grpcx

import (
	"context"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"
)

func (in invocation) unary(ctx context.Context) (*Response, error) {
	msgs, err := parseInput(in.body, in.method.Input(), false, in.codec)
	if err != nil {
		return nil, err
	}
	input := msgs[0]

	var md mdCapture
	output := dynamicpb.NewMessage(in.method.Output())
	start := time.Now()
	callErr := in.conn.Invoke(
		in.md.attach(ctx),
		in.name,
		input,
		output,
		in.callOpts(md.opts()...)...,
	)
	resp := md.response(start)

	if callErr != nil {
		setResponseStatus(resp, callErr, in.codec)
		return resp, diag.WrapAs(diag.ClassProtocol, callErr, "invoke grpc method", grpcComponent)
	}

	data, err := in.codec.marshal(output)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "encode grpc response", grpcComponent)
	}
	resp.Message = string(data)
	resp.Body = data
	if wire, err := proto.Marshal(output); err == nil {
		resp.Wire = wire
	}
	return resp, nil
}

func (in invocation) callOpts(extra ...grpc.CallOption) []grpc.CallOption {
	return append(extra, in.calls...)
}
