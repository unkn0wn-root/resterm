package grpcclient

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

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
