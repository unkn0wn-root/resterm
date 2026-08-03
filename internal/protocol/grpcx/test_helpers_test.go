package grpcx

import (
	"context"
	"io"
	"net"
	"slices"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

type testSvc struct {
	testgrpc.UnimplementedTestServiceServer
}

func (s *testSvc) StreamingOutputCall(
	_ *testgrpc.StreamingOutputCallRequest,
	stream testgrpc.TestService_StreamingOutputCallServer,
) error {
	if err := stream.Send(&testgrpc.StreamingOutputCallResponse{
		Payload: &testgrpc.Payload{Body: []byte("one")},
	}); err != nil {
		return err
	}
	return stream.Send(&testgrpc.StreamingOutputCallResponse{
		Payload: &testgrpc.Payload{Body: []byte("two")},
	})
}

func (s *testSvc) StreamingInputCall(
	stream testgrpc.TestService_StreamingInputCallServer,
) error {
	var count int32
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&testgrpc.StreamingInputCallResponse{
				AggregatedPayloadSize: count,
			})
		}
		if err != nil {
			return err
		}
		count++
	}
}

func (s *testSvc) FullDuplexCall(
	stream testgrpc.TestService_FullDuplexCallServer,
) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&testgrpc.StreamingOutputCallResponse{
			Payload: &testgrpc.Payload{Body: []byte("ok")},
		}); err != nil {
			return err
		}
	}
}

func startTestServer(t *testing.T) string {
	t.Helper()
	return startTestServerWith(t, func(srv *grpc.Server) {
		reflection.RegisterV1(srv)
	})
}

func startTestServerWith(t *testing.T, register func(*grpc.Server), opts ...grpc.ServerOption) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer(opts...)
	testgrpc.RegisterTestServiceServer(srv, &testSvc{})
	if register != nil {
		register(srv)
	}

	go func() {
		_ = srv.Serve(lis)
	}()

	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})
	return lis.Addr().String()
}

const testAuthValue = "Bearer reflect-ok"

func requireTestAuth(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	if !slices.Contains(md.Get("authorization"), testAuthValue) {
		return status.Error(codes.Unauthenticated, "missing auth metadata")
	}
	return nil
}

// startAuthedTestServer rejects any call without the test auth metadata,
// including reflection.
func startAuthedTestServer(t *testing.T, register func(*grpc.Server)) string {
	t.Helper()

	unary := func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if err := requireTestAuth(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
	strm := func(
		srv any,
		ss grpc.ServerStream,
		_ *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if err := requireTestAuth(ss.Context()); err != nil {
			return err
		}
		return handler(srv, ss)
	}
	return startTestServerWith(t, register, grpc.UnaryInterceptor(unary), grpc.StreamInterceptor(strm))
}
