package grpcclient

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
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

func baseStreamReq(target, method string) *restfile.GRPCRequest {
	return &restfile.GRPCRequest{
		Target:        target,
		Package:       "grpc.testing",
		Service:       "TestService",
		Method:        method,
		FullMethod:    "/grpc.testing.TestService/" + method,
		UseReflection: true,
		Plaintext:     true,
		PlaintextSet:  true,
	}
}

func TestStreamServerOutput(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	req := &restfile.Request{Settings: map[string]string{}}
	grpcReq := baseStreamReq(addr, "StreamingOutputCall")
	client := NewClient()
	opts := Options{DefaultPlaintext: true, DefaultPlaintextSet: true, DialTimeout: time.Second}

	resp, err := client.Execute(context.Background(), req, grpcReq, opts, nil)
	if err != nil {
		t.Fatalf("execute streaming output: %v", err)
	}

	var out []map[string]interface{}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(out))
	}
}

func TestStreamClientInput(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	req := &restfile.Request{Settings: map[string]string{}}
	grpcReq := baseStreamReq(addr, "StreamingInputCall")
	grpcReq.Message = `[{}, {}, {}]`
	client := NewClient()
	opts := Options{DefaultPlaintext: true, DefaultPlaintextSet: true, DialTimeout: time.Second}

	resp, err := client.Execute(context.Background(), req, grpcReq, opts, nil)
	if err != nil {
		t.Fatalf("execute streaming input: %v", err)
	}

	var out []map[string]interface{}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 response, got %d", len(out))
	}
	if out[0]["aggregatedPayloadSize"] != float64(3) {
		t.Fatalf("expected aggregated payload size 3, got %#v", out[0]["aggregatedPayloadSize"])
	}
}

func TestStreamBidi(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	req := &restfile.Request{Settings: map[string]string{}}
	grpcReq := baseStreamReq(addr, "FullDuplexCall")
	grpcReq.Message = `[{}, {}]`
	client := NewClient()
	opts := Options{DefaultPlaintext: true, DefaultPlaintextSet: true, DialTimeout: time.Second}

	resp, err := client.Execute(context.Background(), req, grpcReq, opts, nil)
	if err != nil {
		t.Fatalf("execute bidi stream: %v", err)
	}

	var out []map[string]interface{}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(out))
	}
}
