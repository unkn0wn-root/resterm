package grpcclient

import (
	"io"
	"net"
	"testing"

	"google.golang.org/grpc"
	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/reflection"
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

func startTestServer(t *testing.T) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	testgrpc.RegisterTestServiceServer(srv, &testSvc{})
	reflection.Register(srv)

	go func() {
		_ = srv.Serve(lis)
	}()

	stop := func() {
		srv.Stop()
		_ = lis.Close()
	}
	return lis.Addr().String(), stop
}
