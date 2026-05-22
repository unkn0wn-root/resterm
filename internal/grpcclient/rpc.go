package grpcclient

import (
	"context"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func (c *Client) executeUnary(
	ctx context.Context,
	conn *grpc.ClientConn,
	req *restfile.Request,
	gr *restfile.GRPCRequest,
	md protoreflect.MethodDescriptor,
	body string,
	cd codec,
) (*Response, error) {
	input := dynamicpb.NewMessage(md.Input())
	if strings.TrimSpace(body) != "" {
		if err := cd.unmarshalInto([]byte(body), input); err != nil {
			return nil, diag.WrapAs(diag.ClassProtocol, err, "decode grpc request body")
		}
	}

	callCtx, err := outgoingContext(ctx, gr, req)
	if err != nil {
		return nil, err
	}

	headerMD := metadata.MD{}
	trailerMD := metadata.MD{}
	output := dynamicpb.NewMessage(md.Output())
	start := time.Now()
	callErr := conn.Invoke(
		callCtx,
		gr.FullMethod,
		input,
		output,
		grpc.Header(&headerMD),
		grpc.Trailer(&trailerMD),
	)
	resp := newResponse(headerMD, trailerMD, time.Since(start))

	if callErr != nil {
		setResponseStatus(resp, callErr)
		ensureContentType(resp)
		return resp, diag.WrapAs(diag.ClassProtocol, callErr, "invoke grpc method")
	}

	data, err := cd.marshal(output)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "encode grpc response")
	}
	resp.Message = string(data)
	resp.Body = data
	if wire, err := proto.Marshal(output); err == nil {
		resp.Wire = wire
	}
	ensureContentType(resp)
	return resp, nil
}

func outgoingContext(
	ctx context.Context,
	gr *restfile.GRPCRequest,
	req *restfile.Request,
) (context.Context, error) {
	pairs, err := collectMetadata(gr, req)
	if err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return ctx, nil
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...), nil
}
